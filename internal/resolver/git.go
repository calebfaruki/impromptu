package resolver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// GitResult holds the result of resolving a git dependency.
type GitResult struct {
	Entry      lockfile.LockfileEntry
	Dir        string
	CleanupDir string
	Warnings   []string
}

// GitResolver clones git repos and checks out specific refs.
type GitResolver struct{}

// Resolve clones a git repo, checks out the specified ref, runs content checks,
// and returns the resolved lockfile entry.
func (g *GitResolver) Resolve(ctx context.Context, src promptfile.Source, force bool) (*GitResult, error) {
	if src.Kind != promptfile.SourceGit {
		return nil, fmt.Errorf("git resolver: expected git source, got %s", src.Kind)
	}

	tmpDir, err := os.MkdirTemp("", "impromptu-git-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	repo, err := git.PlainCloneContext(ctx, tmpDir, false, &git.CloneOptions{
		URL: src.Git,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("cloning %s: %w", src.Git, err)
	}

	result := &GitResult{
		Entry: lockfile.LockfileEntry{
			Source: promptfile.SourceGit,
			Git:    src.Git,
			Tag:    src.Tag,
			Branch: src.Branch,
			Path:   src.Path,
		},
	}

	commitSHA, err := checkoutRef(repo, src, result, force)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	result.Entry.Commit = commitSHA

	// Determine the directory to check
	checkDir := tmpDir
	if src.Path != "" {
		checkDir = filepath.Join(tmpDir, src.Path)
		if _, err := os.Stat(checkDir); err != nil {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("subdirectory %q not found in repo: %w", src.Path, err)
		}
	}

	// Content checks
	violations, err := contentcheck.CheckDirectory(checkDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("content check: %w", err)
	}
	if len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Error())
		}
		if !force {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("content check failed:\n%s", strings.Join(msgs, "\n"))
		}
		result.Warnings = append(result.Warnings, "content check violations bypassed with --force")
	}

	result.Dir = checkDir
	result.CleanupDir = tmpDir
	return result, nil
}

func checkoutRef(repo *git.Repository, src promptfile.Source, result *GitResult, force bool) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("getting worktree: %w", err)
	}

	if src.Tag != "" {
		ref, err := repo.Tag(src.Tag)
		if err != nil {
			return "", fmt.Errorf("tag %q not found: %w", src.Tag, err)
		}
		if err := wt.Checkout(&git.CheckoutOptions{Hash: ref.Hash()}); err != nil {
			return "", fmt.Errorf("checking out tag %q: %w", src.Tag, err)
		}
		return ref.Hash().String(), nil
	}

	if src.Branch != "" {
		refName := plumbing.NewBranchReferenceName(src.Branch)
		ref, err := repo.Reference(refName, true)
		if err != nil {
			return "", fmt.Errorf("branch %q not found: %w", src.Branch, err)
		}
		if err := wt.Checkout(&git.CheckoutOptions{Hash: ref.Hash()}); err != nil {
			return "", fmt.Errorf("checking out branch %q: %w", src.Branch, err)
		}
		if !force {
			return "", fmt.Errorf("branch %q is a mutable ref, use --force to bypass", src.Branch)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("branch %q is mutable", src.Branch))
		return ref.Hash().String(), nil
	}

	if src.Commit != "" {
		hash := plumbing.NewHash(src.Commit)
		if err := wt.Checkout(&git.CheckoutOptions{Hash: hash}); err != nil {
			return "", fmt.Errorf("checking out commit %q: %w", src.Commit, err)
		}
		return src.Commit, nil
	}

	return "", fmt.Errorf("git source must have tag, branch, or commit")
}
