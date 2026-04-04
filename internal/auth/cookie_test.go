package auth

import (
	"strings"
	"testing"
)

func TestCookieSignerRoundTrip(t *testing.T) {
	signer := NewCookieSigner([]byte("test-secret-key-32-bytes-long!!!"))

	tokens := []string{
		"abc123def456",
		"a-very-long-token-with-many-characters",
		"short",
	}
	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			signed := signer.Sign(token)
			got, err := signer.Verify(signed)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if got != token {
				t.Errorf("got %q, want %q", got, token)
			}
		})
	}
}

func TestSignFormat(t *testing.T) {
	signer := NewCookieSigner([]byte("test-key"))
	signed := signer.Sign("mytoken")

	parts := strings.SplitN(signed, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("expected token|hmac format, got %q", signed)
	}
	if parts[0] != "mytoken" {
		t.Errorf("token part: got %q, want %q", parts[0], "mytoken")
	}
	if len(parts[1]) == 0 {
		t.Error("hmac part is empty")
	}
}

func TestVerifyTampered(t *testing.T) {
	signer := NewCookieSigner([]byte("test-key"))
	signed := signer.Sign("mytoken")

	// Flip a character in the HMAC
	tampered := signed[:len(signed)-1] + "X"
	_, err := signer.Verify(tampered)
	if err == nil {
		t.Error("expected error for tampered cookie, got nil")
	}
}

func TestVerifyWrongKey(t *testing.T) {
	signer1 := NewCookieSigner([]byte("key-one"))
	signer2 := NewCookieSigner([]byte("key-two"))

	signed := signer1.Sign("mytoken")
	_, err := signer2.Verify(signed)
	if err == nil {
		t.Error("expected error for wrong key, got nil")
	}
}

func TestVerifyMalformedNoPipe(t *testing.T) {
	signer := NewCookieSigner([]byte("test-key"))
	_, err := signer.Verify("no-pipe-character")
	if err == nil {
		t.Error("expected error for no pipe, got nil")
	}
}

func TestVerifyEmpty(t *testing.T) {
	signer := NewCookieSigner([]byte("test-key"))
	_, err := signer.Verify("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestVerifyEmptyTokenBeforePipe(t *testing.T) {
	signer := NewCookieSigner([]byte("test-key"))
	_, err := signer.Verify("|somehexvalue")
	if err == nil {
		t.Error("expected error for empty token before pipe, got nil")
	}
}
