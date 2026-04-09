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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n\n[prompts.reviewer]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

	// First pull to establish lockfile with only coder
	firstPF := "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n"
	writePromptfile(t, dir, firstPF)
	_, err := Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}

	// Now add reviewer to the Promptfile
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n\n[prompts.reviewer]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")
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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

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

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1"}
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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v2"}
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
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n[prompts.coder]\ngit = \""+repoDir+"\"\nref = \"v1\"\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1"}
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
	pf := "version = 1\n\n[prompts]\n[prompts.internal]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n"
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

// --- Inline pull tests ---

func TestInlinePullSingleFile(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"CLAUDE.md": "# Claude Config\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1", Inline: true}
	result, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "claude")
	if err != nil {
		t.Fatalf("InlinePull: %v", err)
	}
	if len(result.Added) == 0 {
		t.Error("expected dep added")
	}

	// File should be in cwd, not in a subdirectory
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Error("inline file should be in cwd")
	}
	if _, err := os.Stat(filepath.Join(dir, "claude")); !os.IsNotExist(err) {
		t.Error("inline should NOT create a subdirectory")
	}

	// Lockfile should have inline + filename
	lfData, _ := os.ReadFile(filepath.Join(dir, "Promptfile.lock"))
	if !strings.Contains(string(lfData), "inline") {
		t.Error("lockfile should contain inline flag")
	}
	if !strings.Contains(string(lfData), "CLAUDE.md") {
		t.Error("lockfile should contain filename")
	}
}

func TestInlinePullMultiFileError(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md":      "# Context\n",
		"02-instructions.md": "# Instructions\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1", Inline: true}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "multi")
	if err == nil {
		t.Fatal("expected error for multi-file inline")
	}
	if !strings.Contains(err.Error(), "single-file") {
		t.Errorf("error should mention single-file: %v", err)
	}
}

func TestInlinePullCollisionDenied(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"CLAUDE.md": "# New\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Existing\n"), 0644)

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1", Inline: true}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Force: true,
		Verifier: &sigstore.FakeVerifier{},
		Confirm:  func(s string) bool { return false },
	}, src, "claude")
	if err == nil {
		t.Fatal("expected error when collision denied")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancelled: %v", err)
	}

	// Original file should be unchanged
	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(data), "Existing") {
		t.Error("original file should be preserved")
	}
}

func TestInlinePullCollisionYes(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"CLAUDE.md": "# New Content\n",
	}, "v1")

	dir := t.TempDir()
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Old\n"), 0644)

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1", Inline: true}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "claude")
	if err != nil {
		t.Fatalf("InlinePull with --yes: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(data), "New Content") {
		t.Error("file should be overwritten with new content")
	}
}

func TestPullWithoutInlineAfterInline(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"CLAUDE.md": "# Claude\n",
	}, "v1")

	dir := t.TempDir()

	// First: inline pull
	writePromptfile(t, dir, "version = 1\n\n[prompts]\n")
	src := promptfile.Source{Kind: promptfile.SourceGit, Git: repoDir, Ref: "v1", Inline: true}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "claude")
	if err != nil {
		t.Fatalf("first inline pull: %v", err)
	}

	// Now modify Promptfile to remove inline flag (simulate pulling without --inline)
	pf := "version = 1\n\n[prompts]\n[prompts.claude]\ngit = \"" + repoDir + "\"\nref = \"v1\"\n"
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte(pf), 0644)

	// Pull again without inline -- should error
	_, err = Pull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	})
	if err == nil {
		t.Fatal("expected error when pulling without --inline after inline")
	}
	if !strings.Contains(err.Error(), "inline") {
		t.Errorf("error should mention inline: %v", err)
	}
}

func TestInlinePullRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	pfPath := filepath.Join(dir, "Promptfile")
	original := []byte("version = 1\n\n[prompts]\n")
	os.WriteFile(pfPath, original, 0644)

	src := promptfile.Source{Kind: promptfile.SourceGit, Git: "/nonexistent/repo", Ref: "v1"}
	_, err := InlinePull(context.Background(), Config{
		Dir: dir, Yes: true, Force: true,
		Verifier: &sigstore.FakeVerifier{},
	}, src, "broken")
	if err == nil {
		t.Fatal("expected error for bad git URL")
	}

	// Promptfile should be restored to original — no "broken" entry
	data, _ := os.ReadFile(pfPath)
	if strings.Contains(string(data), "broken") {
		t.Error("Promptfile should not contain failed alias after rollback")
	}
	if string(data) != string(original) {
		t.Errorf("Promptfile should be restored to original.\ngot:  %q\nwant: %q", string(data), string(original))
	}
}
