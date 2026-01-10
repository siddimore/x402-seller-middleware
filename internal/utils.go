// Internal utilities for the x402 middleware
package internal

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateToken generates a random token for testing purposes
func GenerateToken(prefix string) string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return prefix + hex.EncodeToString(bytes)
}
