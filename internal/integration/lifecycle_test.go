package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/calebfaruki/impromptu/internal/commands"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

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
	repoDir := createGitRepo(t, map[string]string{
		"01-context.md": "# Test Prompt\n\nContent here.\n",
	}, "v1")

	dir := t.TempDir()
	ctx := context.Background()
	cfg := pull.Config{
		Dir: dir, Yes: true, Force: true,
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
	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Tag: "v1"}
	result, err := pull.InlinePull(ctx, cfg, src, "coder")
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

// TestMixedSources uses two git deps in one Promptfile.
func TestMixedSources(t *testing.T) {
	gitRepo1 := createGitRepo(t, map[string]string{
		"01-context.md": "# Git Prompt 1\n",
	}, "v1")

	gitRepo2 := createGitRepo(t, map[string]string{
		"01-context.md": "# Git Prompt 2\n",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"" + gitRepo1 + "\"\ntag = \"v1\"\n\n[prompts.internal]\ngit = \"" + gitRepo2 + "\"\ntag = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull mixed: %v", err)
	}
	if len(result.Added) != 2 {
		t.Errorf("expected 2 added, got %d: %v", len(result.Added), result.Added)
	}

	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("first dep files missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "01-context.md")); err != nil {
		t.Error("second dep files missing")
	}
}

// TestPullFailureCleanState proves resolver errors leave no partial state.
func TestPullFailureCleanState(t *testing.T) {
	dir := t.TempDir()
	// Use a nonexistent git URL that will fail to resolve
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"/nonexistent/repo/path\"\ntag = \"v1\"\n"), 0644)

	_, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); !os.IsNotExist(err) {
		t.Error("lockfile should not exist after failed pull")
	}
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("dep directory should not exist after failed pull")
	}
}

// TestLongPromptName proves long aliases don't cause panics.
func TestLongPromptName(t *testing.T) {
	repoDir := createGitRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	longName := strings.Repeat("a", 200)
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n[prompts."+longName+"]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n"), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull with long name: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	if _, err := os.Stat(filepath.Join(dir, longName)); err != nil {
		t.Error("long-named directory should exist")
	}
}

// TestLockfileDigestVerification proves on-disk digest check works through the full pull flow.
func TestLockfileDigestVerification(t *testing.T) {
	repoDir := createGitRepo(t, map[string]string{
		"01-context.md": "# Test Prompt\n\nContent here.\n",
	}, "v1")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n"), 0644)

	// First pull to get correct digest
	_, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}

	// Second pull should show unchanged (digest matches)
	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged (digest match), got unchanged=%v added=%v", result.Unchanged, result.Added)
	}
}

// TestInlineAndDirectoryMixed verifies a Promptfile with one inline dep and one directory dep.
func TestInlineAndDirectoryMixed(t *testing.T) {
	inlineRepo := createGitRepo(t, map[string]string{
		"CLAUDE.md": "# Claude Config\n",
	}, "v1")

	dirRepo := createGitRepo(t, map[string]string{
		"01-context.md": "# Context\n",
		"02-rules.md":   "# Rules\n",
	}, "v1")

	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n" +
		"[prompts.claude]\ngit = \"" + inlineRepo + "\"\ntag = \"v1\"\ninline = true\n\n" +
		"[prompts.coder]\ngit = \"" + dirRepo + "\"\ntag = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	result, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 2 {
		t.Errorf("expected 2 added, got %d", len(result.Added))
	}

	// Inline file should be in cwd
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Error("inline file should exist in cwd")
	}

	// Directory dep should be in subdirectory
	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("directory dep first file missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "coder", "02-rules.md")); err != nil {
		t.Error("directory dep second file missing")
	}

	// Inline dep should NOT have a subdirectory
	if _, err := os.Stat(filepath.Join(dir, "claude")); !os.IsNotExist(err) {
		t.Error("inline dep should not create a subdirectory")
	}

	// Lockfile should track both
	lfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	lfStr := string(lfData)
	if !strings.Contains(lfStr, "claude") {
		t.Error("lockfile should contain claude")
	}
	if !strings.Contains(lfStr, "coder") {
		t.Error("lockfile should contain coder")
	}
}

// TestFullV3Lifecycle chains the complete v3 flow: init -> inline pull -> directory pull -> search -> remove all.
func TestFullV3Lifecycle(t *testing.T) {
	singleFileRepo := createGitRepo(t, map[string]string{
		"CLAUDE.md": "# Claude Config\n",
	}, "v1")

	dirRepo := createGitRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1")

	dir := t.TempDir()
	ctx := context.Background()
	cfg := pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}

	// 1. Init
	if err := commands.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// 2. Inline pull (single file placed in cwd)
	src := promptfile.Source{Kind: promptfile.SourceGit, Git: singleFileRepo, Tag: "v1", Inline: true}
	result, err := pull.InlinePull(ctx, cfg, src, "claude")
	if err != nil {
		t.Fatalf("InlinePull: %v", err)
	}
	if len(result.Added) == 0 {
		t.Error("expected inline dep added")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Error("inline file missing from cwd")
	}

	// 3. Pull directory dep
	src2 := promptfile.Source{Kind: promptfile.SourceGit, Git: dirRepo, Tag: "v1"}
	result, err = pull.InlinePull(ctx, cfg, src2, "coder")
	if err != nil {
		t.Fatalf("InlinePull for dir dep: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("directory dep files missing")
	}

	// 4. Search via mock
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{"source_url": "https://github.com/alice/coder", "signer_identity": "alice@github.com"},
			},
		})
	}))
	defer searchSrv.Close()

	results, err := commands.Search(ctx, searchSrv.URL, "coder")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}

	// 5. Remove inline dep
	if err := commands.Remove(dir, "claude"); err != nil {
		t.Fatalf("Remove inline: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("inline file should be deleted from cwd")
	}

	// 6. Remove directory dep
	if err := commands.Remove(dir, "coder"); err != nil {
		t.Fatalf("Remove dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("coder directory should be deleted")
	}

	// Verify clean Promptfile
	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	pf, parseErr := promptfile.Parse(pfData)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if len(pf.Prompts) != 0 {
		t.Errorf("expected empty prompts, got %d", len(pf.Prompts))
	}
}

// TestPullFailureNoPartialInline proves a failed inline pull leaves no files behind.
func TestPullFailureNoPartialInline(t *testing.T) {
	dir := t.TempDir()
	pf := "version = 1\n\n[prompts]\n[prompts.broken]\ngit = \"/nonexistent/repo/path\"\ntag = \"v1\"\ninline = true\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	_, err := pull.Pull(context.Background(), pull.Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err == nil {
		t.Fatal("expected error for bad git URL")
	}

	// No lockfile should be created
	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); !os.IsNotExist(err) {
		t.Error("lockfile should not exist after failed pull")
	}

	// No files in dir except Promptfile
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "Promptfile" {
			t.Errorf("unexpected file after failed inline pull: %s", e.Name())
		}
	}
}

// alwaysSignedSearcher returns a valid RekorEntry for any digest.
type alwaysSignedSearcher struct{}

func (s *alwaysSignedSearcher) Search(_ context.Context, digest string) (*sigstore.RekorEntry, error) {
	return &sigstore.RekorEntry{LogIndex: 42, Digest: digest, SignerIdentity: "test@github.com"}, nil
}

// TestAutoIndexOnPull proves that Pull calls MaybeIndex and the index server receives the submission.
func TestAutoIndexOnPull(t *testing.T) {
	// Allow local paths (ParseSourceURL returns "" host for /tmp/... paths)
	pull.AllowlistedHosts[""] = true
	defer delete(pull.AllowlistedHosts, "")

	// Mock index server that records POSTs
	var postCount atomic.Int32
	var receivedBody map[string]any
	indexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/index" {
			postCount.Add(1)
			json.NewDecoder(r.Body).Decode(&receivedBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer indexSrv.Close()

	repoDir := createGitRepo(t, map[string]string{
		"01-context.md": "# Test Prompt\n",
	}, "v1")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Promptfile"),
		[]byte("version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n"), 0644)

	_, err := pull.Pull(context.Background(), pull.Config{
		Dir:      dir,
		Yes:      true,
		Force:    true,
		IndexURL: indexSrv.URL,
		Verifier: &sigstore.FakeVerifier{},
		Searcher: &alwaysSignedSearcher{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if postCount.Load() != 1 {
		t.Errorf("expected 1 index POST, got %d", postCount.Load())
	}
	if receivedBody == nil {
		t.Fatal("index server received no body")
	}
	if src, _ := receivedBody["source_url"].(string); src != repoDir {
		t.Errorf("source_url: got %q, want %q", src, repoDir)
	}
	if dig, _ := receivedBody["digest"].(string); dig == "" {
		t.Error("digest should not be empty")
	}
}
