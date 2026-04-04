package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Digest represents a content-addressable identifier in the form "sha256:<hex>".
type Digest string

// ComputeDigest computes the SHA256 digest of the given bytes.
func ComputeDigest(data []byte) Digest {
	h := sha256.Sum256(data)
	return Digest("sha256:" + hex.EncodeToString(h[:]))
}

// Validate checks that a Digest has the correct format: "sha256:" followed by 64 lowercase hex chars.
func (d Digest) Validate() error {
	s := string(d)
	if !strings.HasPrefix(s, "sha256:") {
		return fmt.Errorf("digest must start with sha256: prefix, got %q", s)
	}
	hexPart := s[7:]
	if len(hexPart) != 64 {
		return fmt.Errorf("digest hex must be 64 characters, got %d", len(hexPart))
	}
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("digest contains invalid hex character %q", c)
		}
	}
	return nil
}

// Hex returns the hex portion of the digest without the "sha256:" prefix.
func (d Digest) Hex() string {
	return string(d)[7:]
}

// String returns the full digest string.
func (d Digest) String() string {
	return string(d)
}
