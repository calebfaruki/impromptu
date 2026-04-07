package resolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// createTestRepo creates a git repo with files, a commit, and optionally a tag.
// Returns the repo path usable as a clone URL.
func createTestRepo(t *testing.T, files map[string]string, tag string) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}

	commit, err := wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	if tag != "" {
		_, err = repo.CreateTag(tag, commit, nil)
		if err != nil {
			t.Fatalf("tag: %v", err)
		}
	}

	return dir
}

func TestGitCloneWithTag(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n\nTest content.\n",
	}, "v1.0.0")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v1.0.0",
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit == "" {
		t.Error("expected commit SHA to be recorded")
	}
	if result.Entry.Tag != "v1.0.0" {
		t.Errorf("tag: got %q", result.Entry.Tag)
	}
}

func TestGitCloneWithBranchFailsWithoutForce(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceGit,
		Git:    repoDir,
		Branch: "master",
	}, false)
	if err == nil {
		t.Fatal("expected error for mutable branch without force")
	}
	if !strings.Contains(err.Error(), "mutable") {
		t.Errorf("error should mention mutable: %v", err)
	}
}

func TestGitCloneWithBranchForce(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceGit,
		Git:    repoDir,
		Branch: "master",
	}, true)
	if err != nil {
		t.Fatalf("Resolve with force: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if len(result.Warnings) == 0 {
		t.Error("expected warning for mutable branch")
	}
	if result.Entry.Commit == "" {
		t.Error("expected commit SHA")
	}
}

func TestGitCloneWithCommitSHA(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1")

	// Get the commit SHA from the tag
	repo, _ := git.PlainOpen(repoDir)
	tagRef, _ := repo.Tag("v1")
	sha := tagRef.Hash().String()

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind:   promptfile.SourceGit,
		Git:    repoDir,
		Commit: sha,
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit != sha {
		t.Errorf("commit: got %q, want %q", result.Entry.Commit, sha)
	}
}

func TestGitCloneWithSubdirectory(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"prompts/review/01-context.md": "# Review\n",
		"prompts/deploy/01-context.md": "# Deploy\n",
		"README.md":                    "# Repo\n",
	}, "v1")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v1",
		Path: "prompts/review",
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	// Check that only the subdirectory files are in checkDir
	entries, _ := os.ReadDir(result.Dir)
	found := false
	for _, e := range entries {
		if e.Name() == "01-context.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 01-context.md in subdirectory")
	}
}

func TestGitCloneInvalidURL(t *testing.T) {
	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  "/nonexistent/repo/path",
		Tag:  "v1",
	}, false)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestGitCloneNonexistentTag(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v999",
	}, false)
	if err == nil {
		t.Fatal("expected error for nonexistent tag")
	}
}

func TestGitContentCheckFailure(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Prompt\n\n<div>raw html</div>\n",
	}, "v1")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v1",
	}, false)
	if err == nil {
		t.Fatal("expected content check failure")
	}
	if !strings.Contains(err.Error(), "content check") {
		t.Errorf("error should mention content check: %v", err)
	}
}

func TestGitContentCheckForceBypass(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Prompt\n\n<div>raw html</div>\n",
	}, "v1")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v1",
	}, true)
	if err != nil {
		t.Fatalf("force should bypass content check: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if len(result.Warnings) == 0 {
		t.Error("expected warning for content check bypass")
	}
}

func TestGitCommitSHACorrectForTag(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1.0.0")

	repo, _ := git.PlainOpen(repoDir)
	tagRef, _ := repo.Tag("v1.0.0")
	expectedSHA := tagRef.Hash().String()

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Tag:  "v1.0.0",
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit != expectedSHA {
		t.Errorf("commit SHA: got %q, want %q", result.Entry.Commit, expectedSHA)
	}
}

func TestResolveGitNoRef(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
	}, true)
	if err != nil {
		t.Fatalf("Resolve with no ref: %v", err)
	}
	if result.Entry.Commit == "" {
		t.Error("expected HEAD commit SHA to be resolved")
	}
	if _, err := os.Stat(filepath.Join(result.Dir, "01-context.md")); err != nil {
		t.Error("resolved files missing")
	}
	os.RemoveAll(result.CleanupDir)
}
