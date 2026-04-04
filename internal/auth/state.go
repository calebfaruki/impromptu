package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateState produces a cryptographically random state parameter (32 bytes, hex-encoded).
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
