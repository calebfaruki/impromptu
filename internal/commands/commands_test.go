package commands

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

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/resolver"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

// --- Init tests ---

func TestInitEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if err != nil {
		t.Fatal("Promptfile not created")
	}
	if !strings.Contains(string(data), "version = 1") {
		t.Error("Promptfile missing version header")
	}
	if !strings.Contains(string(data), "[prompts]") {
		t.Error("Promptfile missing [prompts] section")
	}
}

func TestInitExistingPromptfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("existing"), 0644)

	err := Init(dir)
	if err == nil {
		t.Fatal("expected error when Promptfile exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists: %v", err)
	}
}

// --- Remove tests ---

func TestRemoveExistingDep(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1\"\n"), 0644)

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)
	os.WriteFile(filepath.Join(coderDir, "01.md"), []byte("# test"), 0644)

	if err := Remove(dir, "coder"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Promptfile entry gone
	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if strings.Contains(string(pfData), "coder") {
		t.Error("Promptfile should not contain coder")
	}

	// Directory gone
	if _, err := os.Stat(coderDir); !os.IsNotExist(err) {
		t.Error("coder directory should be deleted")
	}

	// Lockfile entry gone
	lfData2, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	if strings.Contains(string(lfData2), "coder") {
		t.Error("lockfile should not contain coder")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n"), 0644)

	err := Remove(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent alias")
	}
}

func TestRemoveLastDep(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1\"\n"), 0644)

	if err := Remove(dir, "coder"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	pf, err := promptfile.Parse(pfData)
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Prompts) != 0 {
		t.Errorf("expected empty prompts, got %d", len(pf.Prompts))
	}
}

// --- Search tests ---

func mockSearchServer(t *testing.T, results []SearchResult) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
}

func TestSearchWithResults(t *testing.T) {
	srv := mockSearchServer(t, []SearchResult{
		{Name: "coder", Author: "alice", Description: "Code review prompt"},
	})
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "code")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "coder" {
		t.Errorf("name: got %q", results[0].Name)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := mockSearchServer(t, []SearchResult{})
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "zzz")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Search(context.Background(), srv.URL, "test")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// --- Update tests ---

func mockUpdateServer(t *testing.T, blob []byte, digest string, bundle string) *httptest.Server {
	t.Helper()
	createdAt := time.Now().UTC().Add(-100 * time.Hour).Format("2006-01-02T15:04:05Z")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"versions": []resolver.VersionInfo{
					{Version: "2.0.0", Digest: digest, SignatureBundle: bundle, CreatedAt: createdAt},
					{Version: "1.0.0", Digest: "sha256:old", SignatureBundle: bundle, CreatedAt: createdAt},
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

func testUpdateBlob(t *testing.T) ([]byte, string, string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Updated\n"), 0644)
	blob, _ := oci.PackageBytes(dir)
	digest := oci.ComputeDigest(blob).String()
	s := &sigstore.FakeSigner{}
	b, _ := s.Sign(context.Background(), digest, "github.com/alice")
	return blob, digest, string(b.BundleJSON)
}

func TestUpdateNewerVersion(t *testing.T) {
	blob, digest, bundle := testUpdateBlob(t)
	srv := mockUpdateServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n"), 0644)

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: "sha256:old"},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	// Create on-disk files so unchanged check works
	coderDir := filepath.Join(dir, "coder")
	os.MkdirAll(coderDir, 0755)

	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{}, "coder")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Added) == 0 && len(result.Unchanged) == 0 {
		t.Error("expected update activity")
	}
}

func TestUpdateGitDepSkipped(t *testing.T) {
	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.internal]\ngit = \"https://github.com/org/repo\"\ntag = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{}, "internal")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected skip warning for git dep")
	}
}

func TestUpdateNonexistentAlias(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n"), 0644)

	_, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{}, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent alias")
	}
}

func TestSearchWithSpaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "code review" {
			t.Errorf("query not properly decoded: got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []SearchResult{
			{Name: "coder", Author: "alice", Description: "Code review"},
		}})
	}))
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "code review")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestUpdateAlreadyAtLatest(t *testing.T) {
	blob, digest, bundle := testUpdateBlob(t)
	// Server returns same digest as lockfile -- no update needed
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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n"), 0644)

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: digest},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{}, "coder")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged (already at latest), got unchanged=%v added=%v", result.Unchanged, result.Added)
	}
}

func TestUpdateAllDeps(t *testing.T) {
	blob, digest, bundle := testUpdateBlob(t)
	srv := mockUpdateServer(t, blob, digest, bundle)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\nreviewer = \"alice/coder@1.0.0\"\n"), 0644)

	// Lockfile with old digests for both
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder":    {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: "sha256:old"},
			"reviewer": {Name: "reviewer", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: "sha256:old"},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	// No names arg -> update all
	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{})
	if err != nil {
		t.Fatalf("Update all: %v", err)
	}
	// Both deps should be updated (digests differ from "sha256:old")
	if len(result.Added) < 2 {
		t.Errorf("expected 2 updated deps, got added=%v", result.Added)
	}
}

func TestUpdateSecurityFailure(t *testing.T) {
	blob, digest, _ := testUpdateBlob(t)
	// Server returns unsigned version
	createdAt := time.Now().UTC().Add(-100 * time.Hour).Format("2006-01-02T15:04:05Z")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"versions": []resolver.VersionInfo{
					{Version: "2.0.0", Digest: digest, SignatureBundle: "", CreatedAt: createdAt},
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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1.0.0\"\n"), 0644)

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1.0.0", Digest: "sha256:old"},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	_, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: false, RegistryURL: srv.URL,
		Verifier: &sigstore.FakeVerifier{},
	}, &sigstore.FakeVerifier{}, "coder")
	if err == nil {
		t.Fatal("expected error for unsigned artifact without force")
	}
}
