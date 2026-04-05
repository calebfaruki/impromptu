package pull

import (
	"bytes"
	"context"
	"fmt"
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
	Verifier    sigstore.Verifier
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
		entry, blob, warnings, err := resolveSource(ctx, name, src, cfg)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", name, err)
		}
		entry.Name = name
		newEntries[name] = entry
		newBlobs[name] = blob
		result.Warnings = append(result.Warnings, warnings...)
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
		dir := filepath.Join(cfg.Dir, name)
		os.RemoveAll(dir)
		os.MkdirAll(dir, 0755)
		if err := internaloci.Unpackage(bytes.NewReader(blob), dir); err != nil {
			return nil, fmt.Errorf("extracting %s: %w", name, err)
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
	if err := os.WriteFile(lfPath, lfBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing lockfile: %w", err)
	}

	return result, nil
}

// InlinePull parses an inline ref, adds it to the Promptfile, and pulls.
func InlinePull(ctx context.Context, cfg Config, ref string, alias string) (*Result, error) {
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

	// Determine alias
	if alias == "" {
		parts := strings.SplitN(ref, "@", 2)
		nameParts := strings.SplitN(parts[0], "/", 2)
		if len(nameParts) < 2 {
			return nil, fmt.Errorf("invalid ref %q: expected author/name@version", ref)
		}
		alias = nameParts[1]
	}

	// Check collision
	if _, exists := pf.Prompts[alias]; exists {
		return nil, fmt.Errorf("alias %q already exists in Promptfile, use --as to choose a different name", alias)
	}

	// Add to Promptfile
	if err := pf.AddEntry(alias, ref); err != nil {
		return nil, fmt.Errorf("adding %s: %w", ref, err)
	}

	pfBytes, err := pf.Bytes()
	if err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}
	if err := os.WriteFile(pfPath, pfBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}

	return Pull(ctx, cfg)
}

func resolveSource(ctx context.Context, name string, src promptfile.Source, cfg Config) (lockfile.LockfileEntry, []byte, []string, error) {
	switch src.Kind {
	case promptfile.SourceRegistry:
		client := resolver.NewRegistryClient(cfg.RegistryURL, cfg.Verifier)
		result, err := client.Resolve(ctx, src.Ref, cfg.Force)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, err
		}
		return result.Entry, result.Blob, result.Warnings, nil

	case promptfile.SourceGit:
		gr := &resolver.GitResolver{}
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
		or := &resolver.OCIResolver{}
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

	case promptfile.SourcePrivate:
		client := resolver.NewRegistryClient(src.Registry, cfg.Verifier)
		result, err := client.Resolve(ctx, src.Ref, cfg.Force)
		if err != nil {
			return lockfile.LockfileEntry{}, nil, nil, err
		}
		return result.Entry, result.Blob, result.Warnings, nil

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
