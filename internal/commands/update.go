package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	goconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/calebfaruki/impromptu/internal/authprobe"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
)

// Update checks deps for newer versions and re-resolves changed ones.
func Update(ctx context.Context, cfg pull.Config, names ...string) (*pull.Result, error) {
	pfPath := filepath.Join(cfg.Dir, "Promptfile")
	pfData, err := os.ReadFile(pfPath)
	if err != nil {
		return nil, fmt.Errorf("reading Promptfile: %w", err)
	}

	pf, err := promptfile.Parse(pfData)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

	lfPath := filepath.Join(cfg.Dir, "Promptfile.lock")
	lfData, _ := os.ReadFile(lfPath)
	var lf *lockfile.Lockfile
	if lfData != nil {
		lf, _ = lockfile.ParseLockfile(lfData)
	}
	if lf == nil {
		lf = &lockfile.Lockfile{Version: 1, Entries: map[string]lockfile.LockfileEntry{}}
	}

	targets := names
	if len(targets) == 0 {
		for name := range pf.Prompts {
			targets = append(targets, name)
		}
		sort.Strings(targets)
	}

	result := &pull.Result{}
	changed := false

	for _, name := range targets {
		src, ok := pf.Prompts[name]
		if !ok {
			return nil, fmt.Errorf("alias %q not found in Promptfile", name)
		}

		entry, hasLock := lf.Entries[name]

		switch src.Kind {
		case promptfile.SourceGit:
			updated, warn, err := checkCloneUpdate(ctx, name, src, entry, hasLock)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
				continue
			}
			if updated != "" {
				pf.Prompts[name] = promptfile.Source{
					Kind:   src.Kind,
					Git:    src.Git,
					Ref:    updated,
					Path:   src.Path,
					Inline: src.Inline,
				}
				changed = true
				result.Added = append(result.Added, name)
			}

		case promptfile.SourceRelease:
			updated, warn, err := checkReleaseUpdate(ctx, name, src)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
				continue
			}
			if updated != "" {
				pf.Prompts[name] = promptfile.Source{
					Kind:    src.Kind,
					Git:     src.Git,
					Release: updated,
					Asset:   src.Asset,
					Inline:  src.Inline,
				}
				changed = true
				result.Added = append(result.Added, name)
			}
		}
	}

	if !changed {
		return result, nil
	}

	pfBytes, err := pf.Bytes()
	if err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}
	if err := os.WriteFile(pfPath, pfBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}

	pullResult, err := pull.Pull(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("re-resolving after update: %w", err)
	}
	pullResult.Warnings = append(result.Warnings, pullResult.Warnings...)
	return pullResult, nil
}

// checkCloneUpdate checks if a clone-mode dep has a newer version.
// Returns (newRef, warning, error). newRef is empty if no update available.
func checkCloneUpdate(ctx context.Context, name string, src promptfile.Source, entry lockfile.LockfileEntry, hasLock bool) (string, string, error) {
	if src.Ref == "" {
		return "", fmt.Sprintf("%s: no ref specified, skipping", name), nil
	}

	// Commit SHA: pinned, cannot update
	if commitSHAPattern.MatchString(src.Ref) {
		return "", fmt.Sprintf("%s: pinned to commit %s", name, src.Ref[:8]), nil
	}

	refs, err := lsRemote(ctx, src.Git)
	if err != nil {
		return "", "", fmt.Errorf("checking remote: %w", err)
	}

	// Branch: check if HEAD moved
	branchRef := "refs/heads/" + src.Ref
	if sha, ok := refs[branchRef]; ok {
		if hasLock && entry.Commit == sha {
			return "", fmt.Sprintf("%s: %s@%s (up to date)", name, src.Ref, sha[:8]), nil
		}
		return src.Ref, "", nil
	}

	// Tag: check if there's a newer tag
	tagRef := "refs/tags/" + src.Ref
	if _, ok := refs[tagRef]; ok {
		newer := findNewerTag(src.Ref, refs)
		if newer == "" {
			return "", fmt.Sprintf("%s: %s (latest tag)", name, src.Ref), nil
		}
		return newer, "", nil
	}

	return "", fmt.Sprintf("%s: ref %q not found on remote", name, src.Ref), nil
}

// checkReleaseUpdate checks if there's a newer release on GitHub/Codeberg.
func checkReleaseUpdate(ctx context.Context, name string, src promptfile.Source) (string, string, error) {
	host, owner, repo := authprobe.ParseSourceURL(src.Git)

	var apiURL string
	switch host {
	case "github.com":
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	case "codeberg.org":
		apiURL = fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s/releases", owner, repo)
	default:
		return "", fmt.Sprintf("%s: unsupported host for release updates", name), nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("checking releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("releases API returned HTTP %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", "", fmt.Errorf("parsing releases: %w", err)
	}

	for _, r := range releases {
		if r.TagName != src.Release && isNewer(r.TagName, src.Release) {
			return r.TagName, "", nil
		}
	}

	return "", fmt.Sprintf("%s: %s (latest release)", name, src.Release), nil
}

func lsRemote(ctx context.Context, gitURL string) (map[string]string, error) {
	remote := gogit.NewRemote(memory.NewStorage(), &goconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{gitURL},
	})

	refList, err := remote.ListContext(ctx, &gogit.ListOptions{})
	if err != nil {
		return nil, err
	}

	refs := make(map[string]string, len(refList))
	for _, ref := range refList {
		refs[ref.Name().String()] = ref.Hash().String()
	}
	return refs, nil
}

// findNewerTag finds a tag newer than current using simple string comparison.
// Handles v-prefixed semver (v1.0.0 < v2.0.0) via lexicographic ordering.
func findNewerTag(current string, refs map[string]string) string {
	var tags []string
	for ref := range refs {
		if strings.HasPrefix(ref, "refs/tags/") {
			tag := strings.TrimPrefix(ref, "refs/tags/")
			if tag > current {
				tags = append(tags, tag)
			}
		}
	}
	if len(tags) == 0 {
		return ""
	}
	sort.Strings(tags)
	return tags[0]
}

// isNewer returns true if candidate is newer than current.
func isNewer(candidate, current string) bool {
	return candidate > current
}

var commitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
