package pull

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/resolver"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

func mockRegistryServer(t *testing.T, blob []byte, digest string, bundle string) *httptest.Server {
	t.Helper()
	createdAt := time.Now().UTC().Add(-100 * time.Hour).Format("2006-01-02T15:04:05Z")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"versions": []resolver.VersionInfo{
					{Version: "1.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: createdAt},
				},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "coder", "author": "alice"})
	})
	mux.HandleFunc("/api/v1/blobs/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(blob)
	})
	return httptest.NewServer(mux)
}

func testBlob(t *testing.T) ([]byte, string, string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Test\n"), 0644)
	blob, err := oci.PackageBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	digest := oci.ComputeDigest(blob).String()
	s := &sigstore.FakeSigner{}
	b, _ := s.Sign(context.Background(), digest, "github.com/alice")
	return blob, digest, string(b.BundleJSON)
}

func writePromptfile(t *testing.T, dir string, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(content), 0644)
}

func writeLockfile(t *testing.T, dir string, lf *lockfile.Lockfile) {
	t.Helper()
	data, err := lf.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), data, 0644)
}

func alwaysConfirm(_ string) bool { return true }
func neverConfirm(_ string) bool  { return false }

// --- Core pull tests ---

func TestPullNoLockfile(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	// Lockfile should exist
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); err != nil {
		t.Error("lockfile not created")
	}

	// Files should exist
	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("pulled files not written")
	}
}

func TestPullMatchingLockfileNoop(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	// Create matching lockfile and on-disk files
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	writeLockfile(t, dir, lf)

	// Write matching files on disk
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01-context.md"), []byte("# Test\n"), 0644)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 0 {
		t.Errorf("expected 0 added (no-op), got %d", len(result.Added))
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(result.Unchanged))
	}
}

func TestPullNewDepAdded(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\nreviewer = \"alice/coder@1.0.0\"\n")

	// Lockfile has only coder
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	writeLockfile(t, dir, lf)
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01-context.md"), []byte("# Test\n"), 0644)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "reviewer" {
		t.Errorf("expected reviewer added, got %v", result.Added)
	}
}

func TestPullDepRemoved(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	// Lockfile has coder
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	writeLockfile(t, dir, lf)
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01-context.md"), []byte("# Test\n"), 0644)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(result.Removed))
	}
	if _, err := os.Stat(coderDir); !os.IsNotExist(err) {
		t.Error("removed dep directory should be deleted")
	}
}

func TestPullDigestMismatchRepulls(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	writeLockfile(t, dir, lf)
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01-context.md"), []byte("# MODIFIED\n"), 0644)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("digest mismatch should trigger re-pull, got added=%v", result.Added)
	}
}

func TestPullNoPromptfile(t *testing.T) {
	dir := t.TempDir()
	_, err := Pull(context.Background(), Config{Dir: dir, Yes: true})
	if err == nil {
		t.Fatal("expected error for missing Promptfile")
	}
	if !strings.Contains(err.Error(), "init") {
		t.Errorf("error should mention init: %v", err)
	}
}

func TestPullConfirmationDeclined(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	_, err := Pull(context.Background(), Config{
		Dir: dir, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
		Confirm:  neverConfirm,
	})
	if err == nil {
		t.Fatal("expected error when confirmation declined")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancelled: %v", err)
	}

	// Files should NOT exist
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("files should not be written when confirmation declined")
	}
}

func TestPullYesSkipsConfirmation(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull with --yes: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}
}

// --- Inline pull tests ---

func TestInlinePull(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	result, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, "alice/coder@1.0.0", "")
	if err != nil {
		t.Fatalf("InlinePull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	// Promptfile should be updated
	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if !strings.Contains(string(pfData), "coder") {
		t.Error("Promptfile should contain new entry")
	}
}

func TestInlinePullCollision(t *testing.T) {
	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"bob/coder@1\"\n")

	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, "alice/coder@2", "")
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "--as") {
		t.Errorf("error should mention --as: %v", err)
	}
}

func TestInlinePullWithAs(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistryServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"bob/coder@1\"\n")

	// Use --as to avoid collision. Force=true because existing dep "bob/coder"
	// has mismatched author identity in the mock server.
	result, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, "alice/coder@1.0.0", "alice-coder")
	if err != nil {
		t.Fatalf("InlinePull with --as: %v", err)
	}
	// 2 added: existing "coder" (no lockfile) + new "alice-coder"
	if len(result.Added) != 2 {
		t.Errorf("expected 2 added, got %d", len(result.Added))
	}

	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if !strings.Contains(string(pfData), "alice-coder") {
		t.Error("Promptfile should contain alias 'alice-coder'")
	}
}

// --- Security failure tests ---

func mockUnsignedServer(t *testing.T, blob []byte, digest string) *httptest.Server {
	t.Helper()
	createdAt := time.Now().UTC().Add(-100 * time.Hour).Format("2006-01-02T15:04:05Z")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"versions": []resolver.VersionInfo{
					{Version: "1.0.0", Digest: digest, SignatureBundle: "", CreatedAt: createdAt},
				},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "coder", "author": "alice"})
	})
	mux.HandleFunc("/api/v1/blobs/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(blob)
	})
	return httptest.NewServer(mux)
}

func TestPullSecurityFailureNoForce(t *testing.T) {
	blob, digest, _ := testBlob(t)
	srv := mockUnsignedServer(t, blob, digest)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	_, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: false, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err == nil {
		t.Fatal("expected error for unsigned artifact without force")
	}

	// Nothing should be written
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); !os.IsNotExist(err) {
		t.Error("lockfile should not be written on security failure")
	}
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("files should not be written on security failure")
	}
}

func TestPullSecurityFailureForce(t *testing.T) {
	blob, digest, _ := testBlob(t)
	srv := mockUnsignedServer(t, blob, digest)
	defer srv.Close()

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("force should bypass: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for unsigned artifact")
	}
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); err != nil {
		t.Error("lockfile should be written with force")
	}
	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("files should be written with force")
	}
}

// --- Git source through pull ---

func createTestRepo(t *testing.T, files map[string]string, tag string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755)
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
		wt.Add(name)
	}
	commit, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tag != "" {
		repo.CreateTag(tag, commit, nil)
	}
	return dir
}

func TestPullGitSource(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Git Test\n",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.internal]\ngit = \"" + repoDir + "\"\ntag = \"v1\"\n"
	writePromptfile(t, dir, pf)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull git source: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	// Files should exist
	if _, err := os.Stat(filepath.Join(dir, "internal", "01-context.md")); err != nil {
		t.Error("git files not written")
	}

	// Lockfile should have commit SHA
	lfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	if !strings.Contains(string(lfData), "commit") {
		t.Error("lockfile should contain commit SHA for git source")
	}
}
