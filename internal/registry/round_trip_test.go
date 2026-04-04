package registry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebfaruki/impromptu/internal/oci"
)

var testdataDir = filepath.Join("..", "..", "testdata")

func roundTripTest(t *testing.T, store BlobStore, srcDir string) {
	t.Helper()
	ctx := context.Background()

	// Package
	tarData, err := oci.PackageBytes(srcDir)
	if err != nil {
		t.Fatalf("PackageBytes: %v", err)
	}

	// Compute digest
	digest := oci.ComputeDigest(tarData)
	if err := digest.Validate(); err != nil {
		t.Fatalf("computed digest is invalid: %v", err)
	}

	// Store
	if err := store.Put(ctx, digest, tarData); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Retrieve
	retrieved, err := store.Get(ctx, digest)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(tarData, retrieved) {
		t.Fatal("retrieved bytes differ from stored bytes")
	}

	// Unpackage
	dstDir := t.TempDir()
	if err := oci.Unpackage(bytes.NewReader(retrieved), dstDir); err != nil {
		t.Fatalf("Unpackage: %v", err)
	}

	// Compare files
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	dstEntries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}

	var srcFiles []string
	for _, e := range srcEntries {
		if !e.IsDir() && e.Name()[0] != '.' {
			srcFiles = append(srcFiles, e.Name())
		}
	}
	var dstFiles []string
	for _, e := range dstEntries {
		if !e.IsDir() {
			dstFiles = append(dstFiles, e.Name())
		}
	}

	if len(srcFiles) != len(dstFiles) {
		t.Fatalf("file count mismatch: src=%d dst=%d", len(srcFiles), len(dstFiles))
	}
	for i := range srcFiles {
		if srcFiles[i] != dstFiles[i] {
			t.Errorf("filename mismatch at %d: %q vs %q", i, srcFiles[i], dstFiles[i])
			continue
		}
		srcData, _ := os.ReadFile(filepath.Join(srcDir, srcFiles[i]))
		dstData, _ := os.ReadFile(filepath.Join(dstDir, dstFiles[i]))
		if !bytes.Equal(srcData, dstData) {
			t.Errorf("content mismatch for %s", srcFiles[i])
		}
	}
}

func TestFullRoundTripMemory(t *testing.T) {
	store := NewMemoryStore()
	dirs := []string{
		filepath.Join(testdataDir, "valid", "simple"),
		filepath.Join(testdataDir, "valid", "with-frontmatter"),
		filepath.Join(testdataDir, "valid", "with-code-blocks"),
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			roundTripTest(t, store, dir)
		})
	}
}

func TestFullRoundTripFilesystem(t *testing.T) {
	store, err := NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{
		filepath.Join(testdataDir, "valid", "simple"),
		filepath.Join(testdataDir, "valid", "with-frontmatter"),
		filepath.Join(testdataDir, "valid", "with-code-blocks"),
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			roundTripTest(t, store, dir)
		})
	}
}
