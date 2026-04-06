package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
