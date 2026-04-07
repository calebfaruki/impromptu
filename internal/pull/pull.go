package pull

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	internaloci "github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/resolver"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

// Config holds the dependencies and flags for a pull operation.
type Config struct {
	Dir         string
	Force       bool
	Yes         bool
	Confirm     func(summary string) bool
	RegistryURL string
	IndexURL    string
	Verifier    sigstore.Verifier
	Searcher    sigstore.Searcher
	Progress    io.Writer
}

// Result reports what happened during the pull.
type Result struct {
	Added     []string
	Removed   []string
	Unchanged []string
	Warnings  []string
}

// Pull reads the Promptfile, diffs against the lockfile, resolves dependencies,
// and writes the updated lockfile.
func Pull(ctx context.Context, cfg Config) (*Result, error) {
	pfPath := filepath.Join(cfg.Dir, "Promptfile")
	lfPath := filepath.Join(cfg.Dir, "Promptfile.lock")

	pfData, err := os.ReadFile(pfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no Promptfile found: run impromptu init first")
		}
		return nil, fmt.Errorf("reading Promptfile: %w", err)
	}

	pf, err := promptfile.Parse(pfData)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

	lf := &lockfile.Lockfile{Version: 1, Entries: map[string]lockfile.LockfileEntry{}}
	if lfData, err := os.ReadFile(lfPath); err == nil {
		parsed, err := lockfile.ParseLockfile(lfData)
		if err != nil {
			return nil, fmt.Errorf("parsing lockfile: %w", err)
		}
		lf = parsed
	}

	diff := lockfile.Diff(pf, lf)
	result := &Result{}

	// Check unchanged deps for on-disk digest mismatch
	var actuallyUnchanged []string
	for _, name := range diff.Unchanged {
		entry := lf.Entries[name]
		dir := filepath.Join(cfg.Dir, name)
		if err := lockfile.VerifyDigest(dir, entry.Digest); err != nil {
			diff.Added = append(diff.Added, name)
		} else {
			actuallyUnchanged = append(actuallyUnchanged, name)
		}
	}
	result.Unchanged = actuallyUnchanged

	// Resolve added deps
	newEntries := make(map[string]lockfile.LockfileEntry)
	newBlobs := make(map[string][]byte)
	for _, name := range diff.Added {
		src := pf.Prompts[name]
		logProgress(cfg, "Resolving %s (%s)...\n", name, sourceDesc(src))
		entry, blob, warnings, err := resolveSource(ctx, name, src, cfg)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", name, err)
		}
		entry.Name = name
		newEntries[name] = entry
		newBlobs[name] = blob
		result.Warnings = append(result.Warnings, warnings...)

		// Auto-index: discover Rekor signature and submit to index if signed + public
		sourceURL := src.Git
		if src.Kind == promptfile.SourceOCI {
			sourceURL = src.OCI
		}
		if cfg.Searcher != nil {
			lookupHash := entry.Digest
			if src.Kind == promptfile.SourceGit && entry.Commit != "" {
				lookupHash = "sha1:" + entry.Commit
			}
			indexWarnings := MaybeIndex(ctx, cfg.IndexURL, sourceURL, lookupHash, cfg.Searcher)
			result.Warnings = append(result.Warnings, indexWarnings...)
		}
	}
	result.Added = diff.Added
	result.Removed = diff.Removed

	// Build summary
	if len(diff.Added) == 0 && len(diff.Removed) == 0 {
		result.Unchanged = actuallyUnchanged
		return result, nil
	}

	summary := buildSummary(diff.Added, diff.Removed, newEntries)
	if !cfg.Yes && cfg.Confirm != nil && !cfg.Confirm(summary) {
		return nil, fmt.Errorf("pull cancelled")
	}

	// Write resolved files
	for name, blob := range newBlobs {
		src := pf.Prompts[name]
		entry := newEntries[name]

		// Check for inline/non-inline conflict with existing lockfile entry
		if existingEntry, ok := lf.Entries[name]; ok && existingEntry.Inline && !src.Inline {
			return nil, fmt.Errorf("%s is already pulled as inline; specify --inline or remove it first", name)
		}

		if src.Inline {
			// Inline: extract to temp, verify single file, place in cwd
			tmpDir, err := os.MkdirTemp("", "impromptu-inline-*")
			if err != nil {
				return nil, fmt.Errorf("creating temp dir: %w", err)
			}
			if err := internaloci.Unpackage(bytes.NewReader(blob), tmpDir); err != nil {
				os.RemoveAll(tmpDir)
				return nil, fmt.Errorf("extracting %s: %w", name, err)
			}
			files, _ := os.ReadDir(tmpDir)
			if len(files) != 1 {
				os.RemoveAll(tmpDir)
				return nil, fmt.Errorf("%s: inline only works with single-file prompts (found %d files)", name, len(files))
			}
			filename := files[0].Name()
			targetPath := filepath.Join(cfg.Dir, filename)

			// Collision detection
			if _, err := os.Stat(targetPath); err == nil {
				if !cfg.Yes {
					if cfg.Confirm == nil || !cfg.Confirm(fmt.Sprintf("%s already exists. Replace? [y/N] ", filename)) {
						os.RemoveAll(tmpDir)
						return nil, fmt.Errorf("pull cancelled: %s already exists", filename)
					}
				}
			}

			data, _ := os.ReadFile(filepath.Join(tmpDir, filename))
			os.WriteFile(targetPath, data, 0644)
			os.RemoveAll(tmpDir)

			entry.Inline = true
			entry.Filename = filename
			newEntries[name] = entry
		} else {
			dir := filepath.Join(cfg.Dir, name)
			os.RemoveAll(dir)
			os.MkdirAll(dir, 0755)
			if err := internaloci.Unpackage(bytes.NewReader(blob), dir); err != nil {
				return nil, fmt.Errorf("extracting %s: %w", name, err)
			}
		}
	}

	// Delete removed
	for _, name := range diff.Removed {
		os.RemoveAll(filepath.Join(cfg.Dir, name))
		delete(lf.Entries, name)
	}

	// Update lockfile
	for name, entry := range newEntries {
		lf.Entries[name] = entry
	}

	lfBytes, err := lf.Bytes()
	if err != nil {
		return nil, fmt.Errorf("writing lockfile: %w", err)
	}
	if err := atomicWrite(lfPath, lfBytes); err != nil {
		return nil, fmt.Errorf("writing lockfile: %w", err)
	}

	return result, nil
}

// InlinePull adds a source to the Promptfile and pulls.
func InlinePull(ctx context.Context, cfg Config, src promptfile.Source, alias string) (*Result, error) {
	pfPath := filepath.Join(cfg.Dir, "Promptfile")

	pfData, err := os.ReadFile(pfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no Promptfile found: run impromptu init first")
		}
		return nil, fmt.Errorf("reading Promptfile: %w", err)
	}

	pf, err := promptfile.Parse(pfData)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

	// Derive alias if not provided
	if alias == "" {
		alias = promptfile.AliasFromSource(src)
	}
	if alias == "" {
		return nil, fmt.Errorf("cannot derive alias from source, use --as to specify one")
	}

	// Check collision
	if _, exists := pf.Prompts[alias]; exists {
		return nil, fmt.Errorf("alias %q already exists in Promptfile, use --as to choose a different name", alias)
	}

	// Add to Promptfile
	if err := pf.AddSource(alias, src); err != nil {
		return nil, fmt.Errorf("adding source: %w", err)
	}

	pfBytes, err := pf.Bytes()
	if err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}
	if err := os.WriteFile(pfPath, pfBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}

	result, err := Pull(ctx, cfg)
	if err != nil {
		os.WriteFile(pfPath, pfData, 0644)
		return nil, err
	}
	return result, nil
}

func resolveSource(ctx context.Context, name string, src promptfile.Source, cfg Config) (lockfile.LockfileEntry, []byte, []string, error) {
	switch src.Kind {
	case promptfile.SourceGit:
		gr := &resolver.GitResolver{Progress: cfg.Progress}
		result, err := gr.Resolve(ctx, src, cfg.Force)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, err
		}
		defer os.RemoveAll(result.CleanupDir)
		blob, err := internaloci.PackageBytes(result.Dir)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, fmt.Errorf("packaging git content: %w", err)
		}
		result.Entry.Digest = internaloci.ComputeDigest(blob).String()
		return result.Entry, blob, result.Warnings, nil

	case promptfile.SourceOCI:
		or := &resolver.OCIResolver{Progress: cfg.Progress}
		result, err := or.Resolve(ctx, src, cfg.Force)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, err
		}
		defer os.RemoveAll(result.CleanupDir)
		blob, err := internaloci.PackageBytes(result.Dir)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, fmt.Errorf("packaging oci content: %w", err)
		}
		return result.Entry, blob, result.Warnings, nil

	default:
		return lockfile.LockfileEntry{}, nil, nil, fmt.Errorf("unsupported source kind: %s", src.Kind)
	}
}

func buildSummary(added, removed []string, entries map[string]lockfile.LockfileEntry) string {
	var b strings.Builder
	if len(added) > 0 {
		b.WriteString("Add:\n")
		for _, name := range added {
			e := entries[name]
			b.WriteString(fmt.Sprintf("  %s (%s) %s\n", name, e.Source, e.Digest))
		}
	}
	if len(removed) > 0 {
		b.WriteString("Remove:\n")
		for _, name := range removed {
			b.WriteString(fmt.Sprintf("  %s\n", name))
		}
	}
	return b.String()
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lockfile-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func logProgress(cfg Config, format string, args ...any) {
	if cfg.Progress != nil {
		fmt.Fprintf(cfg.Progress, format, args...)
	}
}

func sourceDesc(src promptfile.Source) string {
	ref := ""
	switch {
	case src.Tag != "":
		ref = "tag: " + src.Tag
	case src.Branch != "":
		ref = "branch: " + src.Branch
	case src.Commit != "":
		ref = "commit: " + src.Commit[:min(8, len(src.Commit))]
	case src.OCITag != "":
		ref = "tag: " + src.OCITag
	case src.Digest != "":
		ref = "digest: " + src.Digest[:min(19, len(src.Digest))]
	}
	url := src.Git
	if src.Kind == promptfile.SourceOCI {
		url = src.OCI
	}
	if ref != "" {
		return url + ", " + ref
	}
	return url
}
