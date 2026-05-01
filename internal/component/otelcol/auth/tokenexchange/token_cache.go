package tokenexchange

import (
	"fmt"
	"sync"
	"time"
)

// tokenCache provides a thread-safe cache for a single access token with expiry tracking.
type tokenCache struct {
	mu           sync.RWMutex
	accessToken  string
	expiresAt    time.Time
	expiryBuffer time.Duration
}

func newTokenCache(expiryBuffer time.Duration) *tokenCache {
	return &tokenCache{
		expiryBuffer: expiryBuffer,
	}
}

// Token returns the cached access token. It returns an error if no token is cached
// or if the cached token has expired.
func (c *tokenCache) Token() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.accessToken == "" {
		return "", fmt.Errorf("no token cached")
	}
	if !c.expiresAt.IsZero() && time.Now().After(c.expiresAt) {
		return "", fmt.Errorf("cached token has expired")
	}
	return c.accessToken, nil
}

// Set stores a new token and its expiry time in the cache.
func (c *tokenCache) Set(token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.accessToken = token
	c.expiresAt = expiresAt
}

// NeedsRefresh returns true if the token is within the expiry buffer window
// or if no token is cached.
func (c *tokenCache) NeedsRefresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.accessToken == "" {
		return true
	}
	if c.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.expiresAt.Add(-c.expiryBuffer))
}

// ExpiresAt returns the expiry time of the cached token.
func (c *tokenCache) ExpiresAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.expiresAt
}
