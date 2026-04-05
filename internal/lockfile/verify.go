package lockfile

import (
	"fmt"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// ComputeDirectoryDigest computes a deterministic digest of all .md files in dir.
func ComputeDirectoryDigest(dir string) (oci.Digest, error) {
	data, err := oci.PackageBytes(dir)
	if err != nil {
		return "", fmt.Errorf("computing directory digest for %s: %w", dir, err)
	}
	return oci.ComputeDigest(data), nil
}

// VerifyDigest compares the on-disk digest of dir against the expected digest.
// Returns nil if they match or if expected is empty (git entries skip verification).
func VerifyDigest(dir string, expected string) error {
	if expected == "" {
		return nil
	}
	actual, err := ComputeDirectoryDigest(dir)
	if err != nil {
		return err
	}
	if actual.String() != expected {
		return fmt.Errorf("digest mismatch for %s: on-disk %s, expected %s", dir, actual, expected)
	}
	return nil
}
