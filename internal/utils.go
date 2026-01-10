// Internal utilities for the x402 middleware
package internal

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateToken generates a random token for testing purposes
func GenerateToken(prefix string) string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a fixed string if random fails (should never happen)
		return prefix + "fallback_token"
	}
	return prefix + hex.EncodeToString(bytes)
}
