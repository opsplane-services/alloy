package tokenexchange

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenCache_EmptyCache(t *testing.T) {
	cache := newTokenCache(time.Minute)

	_, err := cache.Token()
	require.Error(t, err)
	assert.True(t, cache.NeedsRefresh())
}

func TestTokenCache_SetAndGet(t *testing.T) {
	cache := newTokenCache(time.Minute)
	expiresAt := time.Now().Add(time.Hour)

	cache.Set("test-token", expiresAt)

	token, err := cache.Token()
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
	assert.Equal(t, expiresAt, cache.ExpiresAt())
}

func TestTokenCache_ExpiredToken(t *testing.T) {
	cache := newTokenCache(time.Minute)
	cache.Set("expired-token", time.Now().Add(-time.Second))

	_, err := cache.Token()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestTokenCache_NeedsRefreshWithinBuffer(t *testing.T) {
	cache := newTokenCache(5 * time.Minute)
	// Token expires in 3 minutes, buffer is 5 minutes — needs refresh.
	cache.Set("token", time.Now().Add(3*time.Minute))

	assert.True(t, cache.NeedsRefresh())
}

func TestTokenCache_NoRefreshNeeded(t *testing.T) {
	cache := newTokenCache(time.Minute)
	// Token expires in 1 hour, buffer is 1 minute — no refresh needed.
	cache.Set("token", time.Now().Add(time.Hour))

	assert.False(t, cache.NeedsRefresh())
}

func TestTokenCache_ZeroExpiry(t *testing.T) {
	cache := newTokenCache(time.Minute)
	cache.Set("token", time.Time{})

	token, err := cache.Token()
	require.NoError(t, err)
	assert.Equal(t, "token", token)
	assert.False(t, cache.NeedsRefresh())
}

func TestTokenCache_ConcurrentAccess(t *testing.T) {
	cache := newTokenCache(time.Minute)
	var wg sync.WaitGroup

	// Concurrent writes.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Set("token", time.Now().Add(time.Hour))
		}()
	}

	// Concurrent reads.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.Token()
			_ = cache.NeedsRefresh()
			_ = cache.ExpiresAt()
		}()
	}

	wg.Wait()
}
