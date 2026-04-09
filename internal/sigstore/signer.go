package sigstore

import "context"

// RekorEntry holds data extracted from a Rekor transparency log entry.
type RekorEntry struct {
	LogIndex       int64
	Digest         string // sha256:hex of the signed artifact
	SignerIdentity string // email from the OIDC certificate
}

// Verifier verifies Sigstore signatures via the Rekor transparency log.
type Verifier interface {
	// Verify queries Rekor by log index and verifies the entry matches
	// the expected digest. Returns the entry with signer identity on success.
	Verify(ctx context.Context, logIndex int64, expectedDigest string) (*RekorEntry, error)
}

