package resolver

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"

	internaloci "github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// pushTestImage creates an OCI image with a tar layer and pushes it to the
// in-memory registry. Returns the registry host and image reference.
func pushTestImage(t *testing.T, regHost string, repoTag string, files map[string]string) {
	t.Helper()

	// Create a tar from the files
	tmpDir := t.TempDir()
	for fname, content := range files {
		os.WriteFile(tmpDir+"/"+fname, []byte(content), 0644)
	}
	tarData, err := internaloci.PackageBytes(tmpDir)
	if err != nil {
		t.Fatalf("packaging: %v", err)
	}

	// Create OCI image with the tar as a layer
	layer := stream.NewLayer(io.NopCloser(bytes.NewReader(tarData)))
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatalf("creating image: %v", err)
	}

	ref, err := name.ParseReference(regHost + "/" + repoTag)
	if err != nil {
		t.Fatalf("parsing ref: %v", err)
	}

	if err := remote.Write(ref, img); err != nil {
		t.Fatalf("pushing image: %v", err)
	}
}

func startRegistry(t *testing.T) string {
	t.Helper()
	reg := registry.New()
	srv := httptest.NewServer(reg)
	t.Cleanup(srv.Close)
	// Return host without scheme for OCI references
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestOCIPullFromRegistry(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Context\n\nTest content.\n",
	})

	resolver := &OCIResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		OCITag: "v1",
	}, true) // force=true because OCI is always unsigned
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Digest == "" {
		t.Error("expected non-empty digest")
	}

	// Check files exist
	entries, _ := os.ReadDir(result.Dir)
	found := false
	for _, e := range entries {
		if e.Name() == "01-context.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 01-context.md in extracted dir")
	}
}

func TestOCIDigestMatches(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Context\n",
	})

	resolver := &OCIResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		OCITag: "v1",
	}, true)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if !strings.HasPrefix(result.Entry.Digest, "sha256:") {
		t.Errorf("digest should start with sha256:, got %q", result.Entry.Digest)
	}
}

func TestOCIDigestPinMismatch(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Context\n",
	})

	// Pull by digest that doesn't exist -- the registry rejects it
	resolver := &OCIResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}, true)
	if err == nil {
		t.Fatal("expected error for invalid digest pin")
	}
}

func TestOCIContentCheckFailure(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Prompt\n\n<div>raw html</div>\n",
	})

	resolver := &OCIResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		OCITag: "v1",
	}, false)
	if err == nil {
		t.Fatal("expected content check or unsigned failure")
	}
}

func TestOCIUnsignedRejectsWithoutForce(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Context\n",
	})

	resolver := &OCIResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		OCITag: "v1",
	}, false)
	if err == nil {
		t.Fatal("expected error for unsigned artifact without force")
	}
	if !strings.Contains(err.Error(), "unsigned") {
		t.Errorf("error should mention unsigned: %v", err)
	}
}

func TestOCIUnsignedWithForce(t *testing.T) {
	host := startRegistry(t)
	pushTestImage(t, host, "org/prompt:v1", map[string]string{
		"01-context.md": "# Context\n",
	})

	resolver := &OCIResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/org/prompt",
		OCITag: "v1",
	}, true)
	if err != nil {
		t.Fatalf("force should bypass unsigned: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if len(result.Warnings) == 0 {
		t.Error("expected warning for unsigned")
	}
}

func TestOCIPullNotFound(t *testing.T) {
	host := startRegistry(t)

	resolver := &OCIResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceOCI,
		OCI:    host + "/nonexistent/image",
		OCITag: "v1",
	}, true)
	if err == nil {
		t.Fatal("expected error for nonexistent image")
	}
}
