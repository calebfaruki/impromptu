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

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

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
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"https://github.com/test/repo\"\nref = \"v1\"\n"), 0644)

	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceGit, Git: "https://github.com/test/repo", Ref: "v1"},
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

	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if strings.Contains(string(pfData), "coder") {
		t.Error("Promptfile should not contain coder")
	}

	if _, err := os.Stat(coderDir); !os.IsNotExist(err) {
		t.Error("coder directory should be deleted")
	}

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
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"https://github.com/test/repo\"\nref = \"v1\"\n"), 0644)

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
		{SourceURL: "https://github.com/alice/coder", SignerIdentity: "alice@github.com", Digest: "sha256:abc"},
	})
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "coder")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SourceURL != "https://github.com/alice/coder" {
		t.Errorf("source_url: got %q", results[0].SourceURL)
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

func TestUpdateTagLatest(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# test",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.internal]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, "internal")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected 'latest tag' warning")
	}
	if len(result.Added) > 0 {
		t.Error("should not update when already at latest tag")
	}
}

func TestUpdateNonexistentAlias(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n"), 0644)

	_, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, "nonexistent")
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
			{SourceURL: "https://github.com/alice/coder", SignerIdentity: "alice@github.com"},
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

func TestUpdateAllDepsLatest(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# test",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n\n[prompts.reviewer]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := Update(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Update all: %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings (both at latest), got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestRemoveInlineDep(t *testing.T) {
	dir := t.TempDir()

	// Set up Promptfile with an inline entry
	pf := "version = 1\n\n[prompts]\n[prompts.claude]\ngit = \"https://github.com/alice/claude-md\"\nref = \"v1\"\ninline = true\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	// Set up lockfile with inline + filename
	lf := &lockfile.Lockfile{
		Version: 1,
		Entries: map[string]lockfile.LockfileEntry{
			"claude": {
				Name:     "claude",
				Source:   promptfile.SourceGit,
				Git:      "https://github.com/alice/claude-md",
				Ref:      "v1",
				Inline:   true,
				Filename: "CLAUDE.md",
			},
		},
	}
	lfData, _ := lf.Bytes()
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), lfData, 0644)

	// Create the inline file in cwd
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Claude\n"), 0644)

	// Remove
	if err := Remove(dir, "claude"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// File should be deleted from cwd
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("inline file should be deleted from cwd")
	}

	// Promptfile entry gone
	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	if strings.Contains(string(pfData), "claude") {
		t.Error("Promptfile should not contain claude")
	}

	// Lockfile entry gone
	lfData2, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	if strings.Contains(string(lfData2), "claude") {
		t.Error("lockfile should not contain claude")
	}
}
