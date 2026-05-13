package tokenexchange

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const idTokenType = "urn:ietf:params:oauth:token-type:id_token"

// gcpMetadataProvider fetches tokens from the GCE/GKE metadata server
// for managed identity (VM or node service account).
// It supports both opaque access tokens (/token endpoint) and
// JWT ID tokens (/identity endpoint) for federated auth.
type gcpMetadataProvider struct {
	metadataEndpoint string
	serviceAccount   string
	scopes           []string
	requestedTokenType string
	audience           string
	httpClient         *http.Client
}

type gcpMetadataTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func newGCPMetadataProvider(cfg *Config) *gcpMetadataProvider {
	return &gcpMetadataProvider{
		metadataEndpoint:   cfg.GCPMetadataEndpoint,
		serviceAccount:     cfg.GCPMetadataServiceAccount,
		scopes:             cfg.GCPScopes,
		requestedTokenType: cfg.GCPRequestedTokenType,
		audience:           cfg.GCPAudience,
		httpClient:         cfg.HTTPClient,
	}
}

// Exchange fetches a token from the GCE metadata server.
// When requested_token_type is id_token, it fetches a JWT ID token
// from the /identity endpoint. Otherwise it fetches an opaque access token.
func (p *gcpMetadataProvider) Exchange(ctx context.Context, _ string) (string, time.Time, error) {
	if p.requestedTokenType == idTokenType {
		return p.fetchIDToken(ctx)
	}
	return p.fetchAccessToken(ctx)
}

func (p *gcpMetadataProvider) fetchAccessToken(ctx context.Context) (string, time.Time, error) {
	tokenURL := fmt.Sprintf("%s/computeMetadata/v1/instance/service-accounts/%s/token", p.metadataEndpoint, p.serviceAccount)
	if len(p.scopes) > 0 {
		tokenURL += "?scopes=" + joinScopes(p.scopes)
	}

	body, err := p.metadataGet(ctx, tokenURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetching access token from metadata server: %w", err)
	}

	var tokenResp gcpMetadataTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing metadata token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("metadata server response missing access_token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return tokenResp.AccessToken, expiresAt, nil
}

func (p *gcpMetadataProvider) fetchIDToken(ctx context.Context) (string, time.Time, error) {
	if p.audience == "" {
		return "", time.Time{}, fmt.Errorf("audience is required for GCP metadata ID token")
	}

	identityURL := fmt.Sprintf("%s/computeMetadata/v1/instance/service-accounts/%s/identity?audience=%s&format=full",
		p.metadataEndpoint, p.serviceAccount, p.audience)

	body, err := p.metadataGet(ctx, identityURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetching ID token from metadata server: %w", err)
	}

	// The /identity endpoint returns the JWT directly as the response body.
	idToken := strings.TrimSpace(string(body))
	if idToken == "" {
		return "", time.Time{}, fmt.Errorf("metadata server returned empty ID token")
	}

	expiresAt := parseJWTExpiry(idToken)
	return idToken, expiresAt, nil
}

// parseJWTExpiry extracts the exp claim from a JWT without verifying the signature.
// Falls back to 1 hour from now if parsing fails.
func parseJWTExpiry(token string) time.Time {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return time.Now().Add(time.Hour)
	}

	// Decode the payload (second part), handling missing padding.
	payload := parts[1]
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return time.Now().Add(time.Hour)
	}

	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Now().Add(time.Hour)
	}

	expUnix, err := claims.Exp.Int64()
	if err != nil {
		return time.Now().Add(time.Hour)
	}

	return time.Unix(expUnix, 0)
}

func (p *gcpMetadataProvider) metadataGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating metadata request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending metadata request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading metadata response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata server returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
