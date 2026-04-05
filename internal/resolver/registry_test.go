package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

// mockRegistry creates an httptest server that serves version and blob data.
func mockRegistry(t *testing.T, versions []VersionInfo, blobs map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/prompts/alice/coder/versions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"versions": versions})
	})

	mux.HandleFunc("/api/v1/prompts/nobody/nothing/versions", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	mux.HandleFunc("/api/v1/blobs/", func(w http.ResponseWriter, r *http.Request) {
		digest := strings.TrimPrefix(r.URL.Path, "/api/v1/blobs/")
		data, ok := blobs[digest]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Write(data)
	})

	return httptest.NewServer(mux)
}

func testBlob(t *testing.T) ([]byte, string) {
	t.Helper()
	data := []byte("test blob content for testing")
	digest := oci.ComputeDigest(data)
	return data, digest.String()
}

func oldTime() string {
	return time.Now().UTC().Add(-100 * time.Hour).Format("2006-01-02T15:04:05Z")
}

func recentTime() string {
	return time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z")
}

func fakeBundle(digest, identity string) string {
	s := &sigstore.FakeSigner{}
	b, _ := s.Sign(context.Background(), digest, identity)
	return string(b.BundleJSON)
}

// --- Version resolution tests ---

func TestResolveLatest(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "2.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Entry.Digest != digest {
		t.Errorf("digest: got %q", result.Entry.Digest)
	}
}

func TestResolveMajor(t *testing.T) {
	blob2 := []byte("version 2 content")
	digest2 := oci.ComputeDigest(blob2).String()
	bundle2 := fakeBundle(digest2, "github.com/alice")

	blob1 := []byte("version 1 content")
	digest1 := oci.ComputeDigest(blob1).String()
	bundle1 := fakeBundle(digest1, "github.com/alice")

	versions := []VersionInfo{
		{Version: "2.1.0", Digest: digest2, SignatureBundle: bundle2, CreatedAt: oldTime()},
		{Version: "1.5.0", Digest: digest1, SignatureBundle: bundle1, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest2: blob2, digest1: blob1})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@2", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Entry.Digest != digest2 {
		t.Errorf("expected version 2.x.x digest %q, got %q", digest2, result.Entry.Digest)
	}
}

func TestResolveExact(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "2.0.0", Digest: "sha256:other", CreatedAt: oldTime()},
		{Version: "1.5.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@1.5.0", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Entry.Digest != digest {
		t.Errorf("digest: got %q, want %q", result.Entry.Digest, digest)
	}
}

func TestResolveDigestPin(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@"+digest, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Entry.Digest != digest {
		t.Errorf("digest: got %q", result.Entry.Digest)
	}
}

func TestResolveNotFoundPrompt(t *testing.T) {
	srv := mockRegistry(t, nil, nil)
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "nobody/nothing@latest", false)
	if err == nil {
		t.Fatal("expected error for nonexistent prompt")
	}
}

func TestResolveNotFoundVersion(t *testing.T) {
	blob, digest := testBlob(t)
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@9.9.9", false)
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

// --- Digest verification ---

func TestDigestMismatch(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	// Serve tampered blob
	tampered := append([]byte{}, blob...)
	tampered[0] ^= 0xFF
	srv := mockRegistry(t, versions, map[string][]byte{digest: tampered})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err == nil {
		t.Fatal("expected error for digest mismatch")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("error should mention digest mismatch: %v", err)
	}
}

// --- Signature verification ---

func TestSignatureVerificationPasses(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err != nil {
		t.Fatalf("should succeed: %v", err)
	}
}

func TestSignatureVerificationFails(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{Err: errors.New("bad sig")})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err == nil {
		t.Fatal("expected error for signature failure")
	}
}

func TestSignatureFailureForceBypass(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{Err: errors.New("bad sig")})
	result, err := client.Resolve(context.Background(), "alice/coder@latest", true)
	if err != nil {
		t.Fatalf("force should bypass: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for forced bypass")
	}
}

// --- Cooldown ---

func TestCooldownRejects(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: recentTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err == nil {
		t.Fatal("expected cooldown error")
	}
	if !strings.Contains(err.Error(), "72-hour") {
		t.Errorf("error should mention cooldown: %v", err)
	}
}

func TestCooldownPasses(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err != nil {
		t.Fatalf("should pass cooldown: %v", err)
	}
}

func TestCooldownForceBypass(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: recentTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@latest", true)
	if err != nil {
		t.Fatalf("force should bypass cooldown: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for cooldown bypass")
	}
}

// --- Major not found ---

func TestResolveMajorNotFound(t *testing.T) {
	blob, digest := testBlob(t)
	bundle := fakeBundle(digest, "github.com/alice")
	versions := []VersionInfo{
		{Version: "2.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@1", false)
	if err == nil {
		t.Fatal("expected error when no version matches major 1")
	}
	if !strings.Contains(err.Error(), "major 1") {
		t.Errorf("error should mention major version: %v", err)
	}
}

// --- Unsigned artifact ---

func TestUnsignedArtifactRejects(t *testing.T) {
	blob, digest := testBlob(t)
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: "", CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	_, err := client.Resolve(context.Background(), "alice/coder@latest", false)
	if err == nil {
		t.Fatal("expected error for unsigned artifact")
	}
	if !strings.Contains(err.Error(), "unsigned") {
		t.Errorf("error should mention unsigned: %v", err)
	}
}

func TestUnsignedArtifactForceBypass(t *testing.T) {
	blob, digest := testBlob(t)
	versions := []VersionInfo{
		{Version: "1.0.0", Digest: digest, SignatureBundle: "", CreatedAt: oldTime()},
	}
	srv := mockRegistry(t, versions, map[string][]byte{digest: blob})
	defer srv.Close()

	client := NewRegistryClient(srv.URL, &sigstore.FakeVerifier{})
	result, err := client.Resolve(context.Background(), "alice/coder@latest", true)
	if err != nil {
		t.Fatalf("force should bypass unsigned: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for unsigned bypass")
	}
}
