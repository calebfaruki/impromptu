package resolver

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

var commitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// GitResult holds the result of resolving a git dependency.
type GitResult struct {
	Entry      lockfile.LockfileEntry
	Dir        string
	CleanupDir string
	Warnings   []string
}

// GitResolver clones git repos and checks out specific refs.
type GitResolver struct {
	Progress io.Writer
}

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
		URL:      src.Git,
		Depth:    1,
		Progress: g.Progress,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("cloning %s: %w", src.Git, err)
	}

	result := &GitResult{
		Entry: lockfile.LockfileEntry{
			Source: promptfile.SourceGit,
			Git:    src.Git,
			Ref:    src.Ref,
			Path:   src.Path,
		},
	}

	commitSHA, refType, err := resolveRef(repo, src.Ref, result, force)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	result.Entry.Commit = commitSHA
	result.Entry.RefType = refType

	checkDir := tmpDir
	if src.Path != "" {
		checkDir = filepath.Join(tmpDir, src.Path)
		if _, err := os.Stat(checkDir); err != nil {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("subdirectory %q not found in repo: %w", src.Path, err)
		}
	}

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

// resolveRef auto-detects whether ref is a tag, branch, or commit SHA, then checks it out.
// Returns (commitSHA, refType, error).
func resolveRef(repo *git.Repository, ref string, result *GitResult, force bool) (string, string, error) {
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return "", "", fmt.Errorf("getting HEAD: %w", err)
		}
		return head.Hash().String(), "", nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", "", fmt.Errorf("getting worktree: %w", err)
	}

	// Try tag first
	tagRef, err := repo.Tag(ref)
	if err == nil {
		if err := wt.Checkout(&git.CheckoutOptions{Hash: tagRef.Hash()}); err != nil {
			return "", "", fmt.Errorf("checking out tag %q: %w", ref, err)
		}
		return tagRef.Hash().String(), "tag", nil
	}

	// Try branch
	branchRefName := plumbing.NewBranchReferenceName(ref)
	branchRef, err := repo.Reference(branchRefName, true)
	if err == nil {
		if err := wt.Checkout(&git.CheckoutOptions{Hash: branchRef.Hash()}); err != nil {
			return "", "", fmt.Errorf("checking out branch %q: %w", ref, err)
		}
		if !force {
			return "", "", fmt.Errorf("branch %q is a mutable ref, use --force to bypass", ref)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("branch %q is mutable", ref))
		return branchRef.Hash().String(), "branch", nil
	}

	// Try commit SHA
	if commitSHAPattern.MatchString(ref) {
		hash := plumbing.NewHash(ref)
		if err := wt.Checkout(&git.CheckoutOptions{Hash: hash}); err != nil {
			return "", "", fmt.Errorf("checking out commit %q: %w", ref, err)
		}
		return ref, "commit", nil
	}

	return "", "", fmt.Errorf("ref %q not found as tag, branch, or commit", ref)
}
