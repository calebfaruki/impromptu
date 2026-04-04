package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// CookieSigner signs and verifies session cookie values using HMAC-SHA256.
type CookieSigner struct {
	key []byte
}

// NewCookieSigner creates a cookie signer with the given secret key.
func NewCookieSigner(key []byte) *CookieSigner {
	return &CookieSigner{key: key}
}

// Sign returns "token|hmac" where hmac is the hex-encoded HMAC-SHA256 of the token.
func (c *CookieSigner) Sign(token string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(token))
	sig := hex.EncodeToString(mac.Sum(nil))
	return token + "|" + sig
}

// Verify splits "token|hmac", recomputes the HMAC, and returns the token if valid.
func (c *CookieSigner) Verify(cookieValue string) (string, error) {
	idx := strings.LastIndex(cookieValue, "|")
	if idx <= 0 {
		return "", fmt.Errorf("malformed cookie: %w", ErrInvalidSignature)
	}
	token := cookieValue[:idx]
	sig := cookieValue[idx+1:]

	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return "", fmt.Errorf("decoding signature: %w", ErrInvalidSignature)
	}

	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(token))
	expected := mac.Sum(nil)

	if !hmac.Equal(sigBytes, expected) {
		return "", ErrInvalidSignature
	}
	return token, nil
}
