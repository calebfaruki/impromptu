package sigstore

import (
	"context"
	"errors"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	signer := &FakeSigner{}
	verifier := &FakeVerifier{}
	ctx := context.Background()

	bundle, err := signer.Sign(ctx, "sha256:abc123", "github.com/alice")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(bundle.BundleJSON) == 0 {
		t.Fatal("empty bundle")
	}
	if bundle.SignerIdentity != "github.com/alice" {
		t.Errorf("identity: got %q, want %q", bundle.SignerIdentity, "github.com/alice")
	}

	err = verifier.Verify(ctx, bundle.BundleJSON, "sha256:abc123", "github.com/alice")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyWrongDigest(t *testing.T) {
	signer := &FakeSigner{}
	verifier := &FakeVerifier{}
	ctx := context.Background()

	bundle, _ := signer.Sign(ctx, "sha256:abc123", "github.com/alice")

	err := verifier.Verify(ctx, bundle.BundleJSON, "sha256:wrong", "github.com/alice")
	if err == nil {
		t.Fatal("expected error for wrong digest, got nil")
	}
}

func TestVerifyWrongIdentity(t *testing.T) {
	signer := &FakeSigner{}
	verifier := &FakeVerifier{}
	ctx := context.Background()

	bundle, _ := signer.Sign(ctx, "sha256:abc123", "github.com/alice")

	err := verifier.Verify(ctx, bundle.BundleJSON, "sha256:abc123", "github.com/bob")
	if err == nil {
		t.Fatal("expected error for wrong identity, got nil")
	}
}

func TestVerifyTamperedBundle(t *testing.T) {
	signer := &FakeSigner{}
	verifier := &FakeVerifier{}
	ctx := context.Background()

	bundle, _ := signer.Sign(ctx, "sha256:abc123", "github.com/alice")

	tampered := make([]byte, len(bundle.BundleJSON))
	copy(tampered, bundle.BundleJSON)
	// Flip a byte in the middle to corrupt the JSON values
	tampered[len(tampered)/2] ^= 0xFF

	err := verifier.Verify(ctx, tampered, "sha256:abc123", "github.com/alice")
	if err == nil {
		t.Fatal("expected error for tampered bundle, got nil")
	}
}

func TestVerifyEmptyBundle(t *testing.T) {
	verifier := &FakeVerifier{}
	ctx := context.Background()

	err := verifier.Verify(ctx, []byte{}, "sha256:abc123", "github.com/alice")
	if err == nil {
		t.Fatal("expected error for empty bundle, got nil")
	}
}

func TestVerifyMalformedJSON(t *testing.T) {
	verifier := &FakeVerifier{}
	ctx := context.Background()

	err := verifier.Verify(ctx, []byte("not json"), "sha256:abc123", "github.com/alice")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSignerError(t *testing.T) {
	signer := &FakeSigner{Err: errors.New("signing failed")}
	ctx := context.Background()

	_, err := signer.Sign(ctx, "sha256:abc123", "github.com/alice")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestVerifierError(t *testing.T) {
	signer := &FakeSigner{}
	verifier := &FakeVerifier{Err: errors.New("verify failed")}
	ctx := context.Background()

	bundle, _ := signer.Sign(ctx, "sha256:abc123", "github.com/alice")

	err := verifier.Verify(ctx, bundle.BundleJSON, "sha256:abc123", "github.com/alice")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
