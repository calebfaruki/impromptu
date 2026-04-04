package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// FilesystemStore stores blobs as files on disk.
// Used for local development with the --dev flag.
type FilesystemStore struct {
	root string
}

// NewFilesystemStore creates a filesystem-backed blob store at the given root directory.
func NewFilesystemStore(root string) (*FilesystemStore, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("creating blob store root %s: %w", root, err)
	}
	return &FilesystemStore{root: root}, nil
}

func (f *FilesystemStore) Put(_ context.Context, digest oci.Digest, data []byte) error {
	if err := digest.Validate(); err != nil {
		return fmt.Errorf("invalid digest: %w", err)
	}

	path := f.blobPath(digest)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating blob directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing blob data: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming blob file: %w", err)
	}
	return nil
}

func (f *FilesystemStore) Get(_ context.Context, digest oci.Digest) ([]byte, error) {
	if err := digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid digest: %w", err)
	}

	data, err := os.ReadFile(f.blobPath(digest))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("reading blob %s: %w", digest, err)
	}
	return data, nil
}

func (f *FilesystemStore) Exists(_ context.Context, digest oci.Digest) (bool, error) {
	if err := digest.Validate(); err != nil {
		return false, fmt.Errorf("invalid digest: %w", err)
	}

	_, err := os.Stat(f.blobPath(digest))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("checking blob %s: %w", digest, err)
	}
	return true, nil
}

// blobPath returns the filesystem path for a given digest.
// Uses a two-character prefix subdirectory to avoid flat directories with many files.
func (f *FilesystemStore) blobPath(digest oci.Digest) string {
	hex := digest.Hex()
	return filepath.Join(f.root, "sha256", hex[:2], hex)
}
