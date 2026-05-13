package tokenexchange

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"
)

// tokenExchangeExtension implements extension.Extension and extensionauth.HTTPClient,
// providing a RoundTripper that injects the exchanged access token into outgoing HTTP requests.
type tokenExchangeExtension struct {
	cfg      *Config
	cache    *tokenCache
	provider TokenExchanger
	logger   *zap.Logger
	cancel   context.CancelFunc
}

var (
	_ extension.Extension    = (*tokenExchangeExtension)(nil)
	_ extensionauth.HTTPClient = (*tokenExchangeExtension)(nil)
)

func newTokenExchangeExtension(_ context.Context, settings extension.Settings, cfg *Config) (*tokenExchangeExtension, error) {
	ext := &tokenExchangeExtension{
		cfg:    cfg,
		cache:  newTokenCache(cfg.ExpiryBuffer),
		logger: settings.Logger,
	}

	return ext, nil
}

// Start initializes the provider and performs the initial token exchange.
// It starts a background goroutine for proactive token refresh.
func (e *tokenExchangeExtension) Start(ctx context.Context, _ component.Host) error {
	var err error

	switch e.cfg.Provider {
	case "gcp":
		if e.cfg.GCPManagedIdentity {
			e.provider = newGCPMetadataProvider(e.cfg)
		} else {
			e.provider = newGCPProvider(e.cfg)
		}
	case "azure":
		e.provider, err = newAzureProvider(e.cfg)
		if err != nil {
			return fmt.Errorf("initializing Azure provider: %w", err)
		}
	default:
		return fmt.Errorf("unsupported provider: %s", e.cfg.Provider)
	}

	// Perform initial token exchange to fail fast on misconfiguration.
	if err := e.refreshToken(ctx); err != nil {
		return fmt.Errorf("initial token exchange: %w", err)
	}

	// Start background refresh goroutine.
	refreshCtx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	go e.refreshLoop(refreshCtx)

	return nil
}

// Shutdown stops the background refresh goroutine.
func (e *tokenExchangeExtension) Shutdown(_ context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}
	return nil
}

// RoundTripper returns a RoundTripper that injects the cached access token
// as an Authorization: Bearer header into outgoing requests.
func (e *tokenExchangeExtension) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &tokenRoundTripper{
		base:   base,
		cache:  e.cache,
		header: e.cfg.Header,
		scheme: e.cfg.Scheme,
	}, nil
}

func (e *tokenExchangeExtension) refreshToken(ctx context.Context) error {
	var subjectToken string
	// For managed identity, there's no token file to read.
	if e.cfg.TokenFile != "" {
		var err error
		subjectToken, err = e.readTokenFile()
		if err != nil {
			return err
		}
	}

	accessToken, expiresAt, err := e.provider.Exchange(ctx, subjectToken)
	if err != nil {
		return err
	}

	e.cache.Set(accessToken, expiresAt)
	e.logger.Debug("token exchanged successfully", zap.Time("expires_at", expiresAt))
	return nil
}

func (e *tokenExchangeExtension) readTokenFile() (string, error) {
	data, err := os.ReadFile(e.cfg.TokenFile)
	if err != nil {
		return "", fmt.Errorf("reading token file %s: %w", e.cfg.TokenFile, err)
	}
	return strings.TrimSpace(string(data)), nil
}

const minRefreshInterval = 30 * time.Second

func (e *tokenExchangeExtension) refreshLoop(ctx context.Context) {
	for {
		expiresAt := e.cache.ExpiresAt()
		var sleepDuration time.Duration

		if expiresAt.IsZero() {
			sleepDuration = e.cfg.TokenFilePollInterval
		} else {
			sleepDuration = time.Until(expiresAt.Add(-e.cfg.ExpiryBuffer))
		}

		// Enforce a minimum sleep interval to prevent tight loops when:
		// - the token expiry is in the past or within the buffer
		// - the provider returns a cached token with the same expiry (e.g., Azure SDK internal cache)
		if sleepDuration < minRefreshInterval {
			sleepDuration = minRefreshInterval
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			if err := e.refreshToken(ctx); err != nil {
				e.logger.Error("token refresh failed", zap.Error(err))
				// Retry after a short backoff on failure.
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
				}
			}
		}
	}
}

// tokenRoundTripper wraps an http.RoundTripper and injects the cached token.
type tokenRoundTripper struct {
	base   http.RoundTripper
	cache  *tokenCache
	header string
	scheme string
}

func (t *tokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.cache.Token()
	if err != nil {
		return nil, fmt.Errorf("getting cached token for request: %w", err)
	}

	headerVal := token
	if t.scheme != "" {
		headerVal = t.scheme + " " + token
	}

	reqClone := req.Clone(req.Context())
	reqClone.Header.Set(t.header, headerVal)
	return t.base.RoundTrip(reqClone)
}
