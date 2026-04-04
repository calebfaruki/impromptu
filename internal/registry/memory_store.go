package registry

import (
	"context"
	"fmt"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// MemoryStore is an in-memory BlobStore for testing.
type MemoryStore struct {
	blobs map[string][]byte
}

// NewMemoryStore creates an empty in-memory blob store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{blobs: make(map[string][]byte)}
}

func (m *MemoryStore) Put(_ context.Context, digest oci.Digest, data []byte) error {
	if err := digest.Validate(); err != nil {
		return fmt.Errorf("invalid digest: %w", err)
	}
	key := digest.String()
	if _, ok := m.blobs[key]; ok {
		return nil
	}
	m.blobs[key] = append([]byte(nil), data...)
	return nil
}

func (m *MemoryStore) Get(_ context.Context, digest oci.Digest) ([]byte, error) {
	if err := digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid digest: %w", err)
	}
	data, ok := m.blobs[digest.String()]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (m *MemoryStore) Exists(_ context.Context, digest oci.Digest) (bool, error) {
	if err := digest.Validate(); err != nil {
		return false, fmt.Errorf("invalid digest: %w", err)
	}
	_, ok := m.blobs[digest.String()]
	return ok, nil
}

// Len returns the number of stored blobs.
func (m *MemoryStore) Len() int {
	return len(m.blobs)
}
