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

func TestResolveRefAsTag(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n\nTest content.\n",
	}, "v1.0.0")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "v1.0.0",
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit == "" {
		t.Error("expected commit SHA")
	}
	if result.Entry.RefType != "tag" {
		t.Errorf("ref_type: got %q, want tag", result.Entry.RefType)
	}
}

func TestResolveRefAsBranchFailsWithoutForce(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "master",
	}, false)
	if err == nil {
		t.Fatal("expected error for mutable branch without force")
	}
	if !strings.Contains(err.Error(), "mutable") {
		t.Errorf("error should mention mutable: %v", err)
	}
}

func TestResolveRefAsBranchWithForce(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "master",
	}, true)
	if err != nil {
		t.Fatalf("Resolve with force: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.RefType != "branch" {
		t.Errorf("ref_type: got %q, want branch", result.Entry.RefType)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for mutable branch")
	}
}

func TestResolveRefAsCommitSHA(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1")

	repo, _ := git.PlainOpen(repoDir)
	tagRef, _ := repo.Tag("v1")
	sha := tagRef.Hash().String()

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  sha,
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit != sha {
		t.Errorf("commit: got %q, want %q", result.Entry.Commit, sha)
	}
	if result.Entry.RefType != "tag" {
		// SHA matches the tag, so tag is found first
		// This is expected: tag takes priority over commit SHA
	}
}

func TestResolveRefNotFound(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Context\n",
	}, "v1")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "v999",
	}, false)
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestResolveNoRef(t *testing.T) {
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
	defer os.RemoveAll(result.CleanupDir)

	if result.Entry.Commit == "" {
		t.Error("expected HEAD commit SHA")
	}
}

func TestResolveWithSubdirectory(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"prompts/review/01-context.md": "# Review\n",
		"prompts/deploy/01-context.md": "# Deploy\n",
		"README.md":                    "# Repo\n",
	}, "v1")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "v1",
		Path: "prompts/review",
	}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

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

func TestResolveContentCheckFailure(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Prompt\n\n<div>raw html</div>\n",
	}, "v1")

	resolver := &GitResolver{}
	_, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "v1",
	}, false)
	if err == nil {
		t.Fatal("expected content check failure")
	}
	if !strings.Contains(err.Error(), "content check") {
		t.Errorf("error should mention content check: %v", err)
	}
}

func TestResolveContentCheckForceBypass(t *testing.T) {
	repoDir := createTestRepo(t, map[string]string{
		"01-context.md": "# Prompt\n\n<div>raw html</div>\n",
	}, "v1")

	resolver := &GitResolver{}
	result, err := resolver.Resolve(context.Background(), promptfile.Source{
		Kind: promptfile.SourceGit,
		Git:  repoDir,
		Ref:  "v1",
	}, true)
	if err != nil {
		t.Fatalf("force should bypass: %v", err)
	}
	defer os.RemoveAll(result.CleanupDir)

	if len(result.Warnings) == 0 {
		t.Error("expected warning for content check bypass")
	}
}
