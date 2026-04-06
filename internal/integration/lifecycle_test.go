package integration

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

	"github.com/calebfaruki/impromptu/internal/commands"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/resolver"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

func testBlob(t *testing.T) ([]byte, string, string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Test Prompt\n\nContent here.\n"), 0644)
	blob, _ := oci.PackageBytes(dir)
	digest := oci.ComputeDigest(blob).String()
	s := &sigstore.FakeSigner{}
	b, _ := s.Sign(context.Background(), digest, "github.com/alice")
	return blob, digest, string(b.BundleJSON)
}

func mockRegistry(t *testing.T, blob []byte, digest, bundle string) *httptest.Server {
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

func createGitRepo(t *testing.T, files map[string]string, tag string) string {
	t.Helper()
	dir := t.TempDir()
	repo, _ := gogit.PlainInit(dir, false)
	wt, _ := repo.Worktree()
	for name, content := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755)
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
		wt.Add(name)
	}
	commit, _ := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	if tag != "" {
		repo.CreateTag(tag, commit, nil)
	}
	return dir
}

// TestFullLifecycle chains the entire dependency management flow.
func TestFullLifecycle(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistry(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	ctx := context.Background()
	cfg := pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}

	// 1. Init
	if err := commands.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Promptfile")); err != nil {
		t.Fatal("Promptfile not created")
	}

	// 2. Inline pull
	result, err := pull.InlinePull(ctx, cfg, "alice/coder@1.0.0", "")
	if err != nil {
		t.Fatalf("InlinePull: %v", err)
	}
	if len(result.Added) == 0 {
		t.Error("expected dep added")
	}

	// 3. Verify files on disk
	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("pulled files missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); err != nil {
		t.Error("lockfile missing")
	}

	// 4. Modify file on disk
	os.WriteFile(filepath.Join(dir, "coder", "01-context.md"), []byte("# MODIFIED\n"), 0644)

	// 5. Re-pull -- should detect mismatch and re-pull
	result, err = pull.Pull(ctx, cfg)
	if err != nil {
		t.Fatalf("Re-pull: %v", err)
	}
	if len(result.Added) == 0 {
		t.Error("expected re-pull on digest mismatch")
	}

	// 6. Verify restored content
	data, _ := os.ReadFile(filepath.Join(dir, "coder", "01-context.md"))
	if strings.Contains(string(data), "MODIFIED") {
		t.Error("file should be restored to original")
	}

	// 7. Remove
	if err := commands.Remove(dir, "coder"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("coder directory should be deleted")
	}

	// Promptfile should have empty prompts
	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if strings.Contains(string(pfData), "coder") {
		t.Error("Promptfile should not contain coder")
	}
}

// TestMixedSources uses registry + git deps in one Promptfile.
func TestMixedSources(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistry(t, blob, digest, bundle)
	defer srv.Close()

	gitRepo := createGitRepo(t, map[string]string{
		"01-context.md": "# Git Prompt\n",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n\n[prompts.internal]\ngit = \"" + gitRepo + "\"\ntag = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull mixed: %v", err)
	}
	if len(result.Added) != 2 {
		t.Errorf("expected 2 added, got %d: %v", len(result.Added), result.Added)
	}

	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("registry dep files missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "01-context.md")); err != nil {
		t.Error("git dep files missing")
	}
}

// TestPullFailureCleanState proves resolver errors leave no partial state.
func TestPullFailureCleanState(t *testing.T) {
	// Server returns 404 for everything
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n"), 0644)

	_, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// No lockfile should exist
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); !os.IsNotExist(err) {
		t.Error("lockfile should not exist after failed pull")
	}
	// No dep directory
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("dep directory should not exist after failed pull")
	}
}

// TestLongPromptName proves long aliases don't cause panics.
func TestLongPromptName(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistry(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	longName := strings.Repeat("a", 200)
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n"+longName+" = \"alice/coder@1.0.0\"\n"), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull with long name: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	// Directory with long name should exist
	if _, err := os.Stat(filepath.Join(dir, longName)); err != nil {
		t.Error("long-named directory should exist")
	}
}

// TestLockfileDigestVerification proves on-disk digest check works through the full pull flow.
func TestLockfileDigestVerification(t *testing.T) {
	blob, digest, bundle := testBlob(t)
	srv := mockRegistry(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n"), 0644)

	// Create lockfile with matching digest
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: "registry", Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	// Write correct files on disk
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01-context.md"), []byte("# Test Prompt\n\nContent here.\n"), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged (digest match), got unchanged=%v added=%v", result.Unchanged, result.Added)
	}
}
