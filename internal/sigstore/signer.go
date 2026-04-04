package sigstore

import "context"

// SignatureBundle holds the result of a signing operation.
type SignatureBundle struct {
	BundleJSON     []byte
	RekorLogIndex  int64
	SignerIdentity string
}

// Signer signs artifact digests using Sigstore keyless signing.
type Signer interface {
	Sign(ctx context.Context, digest string, identity string) (SignatureBundle, error)
}

// Verifier verifies Sigstore signature bundles.
type Verifier interface {
	Verify(ctx context.Context, bundleJSON []byte, expectedDigest string, expectedIdentity string) error
}
