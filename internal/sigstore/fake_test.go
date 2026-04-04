package sigstore

import (
	"context"
	"testing"
)

func TestFakeSignerIncrementsIndex(t *testing.T) {
	signer := &FakeSigner{}
	ctx := context.Background()

	b1, err := signer.Sign(ctx, "sha256:aaa", "github.com/alice")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := signer.Sign(ctx, "sha256:bbb", "github.com/alice")
	if err != nil {
		t.Fatal(err)
	}

	if b1.RekorLogIndex != 0 {
		t.Errorf("first index: got %d, want 0", b1.RekorLogIndex)
	}
	if b2.RekorLogIndex != 1 {
		t.Errorf("second index: got %d, want 1", b2.RekorLogIndex)
	}
}
