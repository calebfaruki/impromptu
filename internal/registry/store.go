package registry

import (
	"context"
	"errors"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// ErrNotFound is returned by Get when a digest does not exist in the store.
var ErrNotFound = errors.New("blob not found")

// BlobStore stores and retrieves content-addressable blobs.
type BlobStore interface {
	// Put stores a blob. If the digest already exists, the operation is
	// idempotent and returns no error.
	Put(ctx context.Context, digest oci.Digest, data []byte) error

	// Get retrieves a blob by digest.
	// Returns ErrNotFound if the digest does not exist.
	Get(ctx context.Context, digest oci.Digest) ([]byte, error)

	// Exists checks whether a blob with the given digest is stored.
	Exists(ctx context.Context, digest oci.Digest) (bool, error)
}
