package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Token prefixes make each kind of secret visually distinct from the others
// (and from e.g. inventory system credentials) in logs and admin tooling.
const (
	APIKeyPrefix       = "sk_store_" // storefront API key
	SessionTokenPrefix = "vs_"       // vendor portal session token
)

// GenerateToken returns prefix followed by 32 random bytes, hex-encoded. The
// plaintext is handed to the caller exactly once - only its hash (HashAPIKey)
// is ever persisted.
func GenerateToken(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// GenerateAPIKey returns a new random storefront API key.
func GenerateAPIKey() (string, error) {
	return GenerateToken(APIKeyPrefix)
}

// HashAPIKey returns the SHA-256 digest of a plaintext API key, used both to
// store it and to look it up on incoming requests. Unlike a password, a
// high-entropy 32-byte token doesn't need a slow/salted hash - a fast digest
// plus an indexed exact-match lookup is the standard approach for API
// tokens (this is what GitHub PATs and Stripe keys do).
func HashAPIKey(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}

// KeyPrefix returns a short, safe-to-display slice of a plaintext key, so an
// admin UI can show which key is active without ever re-exposing the secret.
func KeyPrefix(plaintext string) string {
	const n = 16
	if len(plaintext) <= n {
		return plaintext
	}
	return plaintext[:n]
}
