package tokenexchange

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseJWTExpiry(t *testing.T) {
	// Create a fake JWT with a known exp claim.
	exp := time.Now().Add(2 * time.Hour).Unix()
	payload := fmt.Sprintf(`{"sub":"test","exp":%d}`, exp)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	fakeJWT := "eyJhbGciOiJSUzI1NiJ9." + encodedPayload + ".signature"

	result := parseJWTExpiry(fakeJWT)
	assert.WithinDuration(t, time.Unix(exp, 0), result, time.Second)
}

func TestParseJWTExpiry_InvalidToken(t *testing.T) {
	// Should fall back to ~1 hour from now.
	result := parseJWTExpiry("not-a-jwt")
	assert.WithinDuration(t, time.Now().Add(time.Hour), result, 5*time.Second)
}

func TestParseJWTExpiry_MissingExp(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test"}`))
	fakeJWT := "header." + payload + ".sig"

	// Missing exp → Int64() fails on empty json.Number → falls back to 1 hour.
	result := parseJWTExpiry(fakeJWT)
	assert.WithinDuration(t, time.Now().Add(time.Hour), result, 5*time.Second)
}

func TestParseJWTExpiry_BadBase64(t *testing.T) {
	result := parseJWTExpiry("header.!!!invalid!!!.sig")
	assert.WithinDuration(t, time.Now().Add(time.Hour), result, 5*time.Second)
}
