package sigstore

import (
	"context"
	"testing"
)

func TestFakeVerifierMultipleEntries(t *testing.T) {
	v := NewFakeVerifier()
	v.AddEntry(RekorEntry{LogIndex: 1, Digest: "sha256:aaa", SignerIdentity: "alice@github.com"})
	v.AddEntry(RekorEntry{LogIndex: 2, Digest: "sha256:bbb", SignerIdentity: "bob@github.com"})

	e1, err := v.Verify(context.Background(), 1, "sha256:aaa")
	if err != nil {
		t.Fatal(err)
	}
	if e1.SignerIdentity != "alice@github.com" {
		t.Errorf("entry 1: got %q", e1.SignerIdentity)
	}

	e2, err := v.Verify(context.Background(), 2, "sha256:bbb")
	if err != nil {
		t.Fatal(err)
	}
	if e2.SignerIdentity != "bob@github.com" {
		t.Errorf("entry 2: got %q", e2.SignerIdentity)
	}
}
