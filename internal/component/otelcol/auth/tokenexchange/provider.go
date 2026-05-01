package tokenexchange

import (
	"context"
	"time"
)

// TokenExchanger exchanges a subject token (e.g., a Kubernetes service account token)
// for a cloud provider access token.
type TokenExchanger interface {
	// Exchange takes a subject token and returns an access token, its expiry time, and any error.
	Exchange(ctx context.Context, subjectToken string) (accessToken string, expiresAt time.Time, err error)
}
