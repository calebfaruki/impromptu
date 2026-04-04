package registry

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// validDigest is a precomputed digest for test data.
var testData = []byte("test blob content")
var testDigest = oci.ComputeDigest(testData)

func storeTests(t *testing.T, newStore func(t *testing.T) BlobStore) {
	t.Run("put and get returns identical bytes", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if err := s.Put(ctx, testDigest, testData); err != nil {
			t.Fatalf("Put: %v", err)
		}
		got, err := s.Get(ctx, testDigest)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !bytes.Equal(got, testData) {
			t.Error("retrieved data does not match stored data")
		}
	})

	t.Run("get nonexistent returns ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		missing := oci.ComputeDigest([]byte("does not exist"))
		_, err := s.Get(ctx, missing)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("put duplicate is idempotent", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if err := s.Put(ctx, testDigest, testData); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(ctx, testDigest, testData); err != nil {
			t.Fatalf("second put should not error: %v", err)
		}
		got, err := s.Get(ctx, testDigest)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, testData) {
			t.Error("data corrupted after duplicate put")
		}
	})

	t.Run("exists returns true for stored blob", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		s.Put(ctx, testDigest, testData)
		exists, err := s.Exists(ctx, testDigest)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Error("expected true, got false")
		}
	})

	t.Run("exists returns false for missing blob", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		missing := oci.ComputeDigest([]byte("missing"))
		exists, err := s.Exists(ctx, missing)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Error("expected false, got true")
		}
	})

	t.Run("put validates digest format", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		err := s.Put(ctx, oci.Digest("badformat"), testData)
		if err == nil {
			t.Error("expected error for invalid digest, got nil")
		}
	})

	t.Run("get validates digest format", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		_, err := s.Get(ctx, oci.Digest(""))
		if err == nil {
			t.Error("expected error for empty digest, got nil")
		}
	})
}

func TestMemoryStore(t *testing.T) {
	storeTests(t, func(t *testing.T) BlobStore {
		return NewMemoryStore()
	})
}

func TestFilesystemStore(t *testing.T) {
	storeTests(t, func(t *testing.T) BlobStore {
		s, err := NewFilesystemStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}
