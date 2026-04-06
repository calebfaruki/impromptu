package sigstore

import (
	"context"
	"errors"
	"testing"
)

func TestVerifyValidEntry(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 42, Digest: "sha256:abc123", SignerIdentity: "alice@github.com"})

	entry, err := v.Verify(context.Background(), 42, "sha256:abc123")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if entry.SignerIdentity != "alice@github.com" {
		t.Errorf("identity: got %q", entry.SignerIdentity)
	}
	if entry.Digest != "sha256:abc123" {
		t.Errorf("digest: got %q", entry.Digest)
	}
}

func TestVerifyDigestMismatch(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 42, Digest: "sha256:correct", SignerIdentity: "alice@github.com"})

	_, err := v.Verify(context.Background(), 42, "sha256:wrong")
	if err == nil {
		t.Fatal("expected error for digest mismatch")
	}
}

func TestVerifyInvalidLogIndex(t *testing.T) {
	v := NewFakeVerifier()

	_, err := v.Verify(context.Background(), 999, "sha256:abc")
	if err == nil {
		t.Fatal("expected error for invalid log index")
	}
}

func TestVerifyReturnsIdentity(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 1, Digest: "sha256:abc", SignerIdentity: "alice@github.com"})

	entry, err := v.Verify(context.Background(), 1, "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}
	if entry.SignerIdentity != "alice@github.com" {
		t.Errorf("got %q", entry.SignerIdentity)
	}
}

func TestVerifyGitHubOIDCIdentity(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 10, Digest: "sha256:abc", SignerIdentity: "alice@users.noreply.github.com"})

	entry, _ := v.Verify(context.Background(), 10, "sha256:abc")
	if entry.SignerIdentity != "alice@users.noreply.github.com" {
		t.Errorf("got %q", entry.SignerIdentity)
	}
}

func TestVerifyCodebergOIDCIdentity(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 20, Digest: "sha256:abc", SignerIdentity: "alice@codeberg.org"})

	entry, _ := v.Verify(context.Background(), 20, "sha256:abc")
	if entry.SignerIdentity != "alice@codeberg.org" {
		t.Errorf("got %q", entry.SignerIdentity)
	}
}

func TestVerifyConfiguredError(t *testing.T) {
	v := NewFakeVerifier()
	v.Err = errors.New("rekor unavailable")

	_, err := v.Verify(context.Background(), 1, "sha256:abc")
	if err == nil {
		t.Fatal("expected configured error")
	}
}
