package tokenexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// gcpProvider exchanges a subject token for a GCP access token using the STS endpoint,
// with optional service account impersonation via the IAM Credentials API.
type gcpProvider struct {
	stsEndpoint             string
	iamEndpoint             string
	audience                string
	scopes                  []string
	subjectTokenType        string
	requestedTokenType      string
	serviceAccountEmail     string
	serviceAccountTokenLife time.Duration
	httpClient              *http.Client
}

type gcpSTSResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type gcpIAMAccessTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireTime  string `json:"expireTime"`
}

func newGCPProvider(cfg *Config) *gcpProvider {
	return &gcpProvider{
		stsEndpoint:             cfg.GCPSTSEndpoint,
		iamEndpoint:             cfg.GCPIAMEndpoint,
		audience:                cfg.GCPAudience,
		scopes:                  cfg.GCPScopes,
		subjectTokenType:        cfg.GCPSubjectTokenType,
		requestedTokenType:      cfg.GCPRequestedTokenType,
		serviceAccountEmail:     cfg.GCPServiceAccountEmail,
		serviceAccountTokenLife: cfg.GCPServiceAccountTokenLife,
		httpClient:              cfg.HTTPClient,
	}
}

// Exchange performs the GCP STS token exchange and optional IAM impersonation.
func (p *gcpProvider) Exchange(ctx context.Context, subjectToken string) (string, time.Time, error) {
	accessToken, expiresIn, err := p.stsExchange(ctx, subjectToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("GCP STS token exchange failed: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	// If service account impersonation is configured, exchange the STS token
	// for an impersonated service account access token.
	if p.serviceAccountEmail != "" {
		accessToken, expiresAt, err = p.impersonate(ctx, accessToken)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("GCP IAM impersonation failed: %w", err)
		}
	}

	return accessToken, expiresAt, nil
}

func (p *gcpProvider) stsExchange(ctx context.Context, subjectToken string) (string, int, error) {
	data := url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"audience":             {p.audience},
		"requested_token_type": {p.requestedTokenType},
		"subject_token":        {subjectToken},
		"subject_token_type":   {p.subjectTokenType},
	}
	if len(p.scopes) > 0 {
		data.Set("scope", joinScopes(p.scopes))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.stsEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("creating STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("sending STS request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("reading STS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("STS returned status %d: %s", resp.StatusCode, string(body))
	}

	var stsResp gcpSTSResponse
	if err := json.Unmarshal(body, &stsResp); err != nil {
		return "", 0, fmt.Errorf("parsing STS response: %w", err)
	}

	if stsResp.AccessToken == "" {
		return "", 0, fmt.Errorf("STS response missing access_token")
	}

	return stsResp.AccessToken, stsResp.ExpiresIn, nil
}

func (p *gcpProvider) impersonate(ctx context.Context, stsToken string) (string, time.Time, error) {
	iamURL := fmt.Sprintf(
		"%s/v1/projects/-/serviceAccounts/%s:generateAccessToken",
		p.iamEndpoint,
		p.serviceAccountEmail,
	)

	lifetime := p.serviceAccountTokenLife
	if lifetime == 0 {
		lifetime = time.Hour
	}

	reqBody := fmt.Sprintf(`{"scope":%s,"lifetime":"%ds"}`,
		mustMarshalJSON(p.scopes),
		int(lifetime.Seconds()),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, iamURL, strings.NewReader(reqBody))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating IAM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+stsToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sending IAM request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reading IAM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("IAM returned status %d: %s", resp.StatusCode, string(body))
	}

	var iamResp gcpIAMAccessTokenResponse
	if err := json.Unmarshal(body, &iamResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing IAM response: %w", err)
	}

	if iamResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("IAM response missing accessToken")
	}

	expiresAt, err := time.Parse(time.RFC3339, iamResp.ExpireTime)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parsing IAM expireTime: %w", err)
	}

	return iamResp.AccessToken, expiresAt, nil
}

func joinScopes(scopes []string) string {
	return strings.Join(scopes, " ")
}

func mustMarshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
