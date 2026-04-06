package pull

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	if _, err := os.Stat(filepath.Join(dir, "Promptfile.lock")); err != nil {
		t.Error("lockfile not created")
	}

	if _, err := os.Stat(filepath.Join(dir, "coder", "01-context.md")); err != nil {
		t.Error("pulled files not written")
	}
}

func TestPullMatchingLockfileNoop(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	// First pull to get the correct digest and commit
	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Fatalf("first pull expected 1 added, got %d", len(result.Added))
	}

	// Second pull should be a no-op
	result, err = Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n\n[prompts.reviewer]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	// First pull to establish lockfile with only coder
	firstPF := "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"" + repoDir + "\"\ntag = \"v1\"\n"
	writePromptfile(t, dir, firstPF)
	_, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}

	// Now add reviewer to the Promptfile
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n\n[prompts.reviewer]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	// First pull with coder
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")
	_, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}

	coderDir := filepath.Join(dir, "coder")
	if _, err := os.Stat(coderDir); err != nil {
		t.Fatal("coder dir should exist after first pull")
	}

	// Now remove coder from Promptfile
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	// First pull
	_, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}

	// Modify file on disk to cause digest mismatch
	os.WriteFile(filepath.Join(dir, "coder", "01-context.md"), []byte("# MODIFIED\n"), 0644)

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	_, err := Pull(context.Background(), Config{
		Dir: dir, Force: true,
		Verifier: &sigstore.FakeVerifier{},
		Confirm:  neverConfirm,
	})
	if err == nil {
		t.Fatal("expected error when confirmation declined")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancelled: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "coder")); !os.IsNotExist(err) {
		t.Error("files should not be written when confirmation declined")
	}
}

func TestPullYesSkipsConfirmation(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	result, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
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
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Tag: "v1"}
	result, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "")
	if err != nil {
		t.Fatalf("InlinePull: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(result.Added))
	}

	pfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile"))
	// AliasFromSource for git extracts base of URL
	alias := filepath.Base(repoDir)
	if !strings.Contains(string(pfData), alias) {
		t.Errorf("Promptfile should contain new entry with alias %q", alias)
	}
}

func TestInlinePullCollision(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Tag: "v2"}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "coder")
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "--as") {
		t.Errorf("error should mention --as: %v", err)
	}
}

func TestInlinePullWithAs(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Test\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\ntag = \"v1\"\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Tag: "v1"}
	result, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "alice-coder")
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

// --- Git source through pull ---

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

	if _, err := os.Stat(filepath.Join(dir, "internal", "01-context.md")); err != nil {
		t.Error("git files not written")
	}

	lfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	if !strings.Contains(string(lfData), "commit") {
		t.Error("lockfile should contain commit SHA for git source")
	}
}
