package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/resolver"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

// Update checks for newer versions of registry deps and re-pulls them.
// If names is empty, all registry deps are checked.
// Git/OCI deps are skipped (pinned deliberately).
func Update(ctx context.Context, cfg pull.Config, verifier sigstore.Verifier, names ...string) (*pull.Result, error) {
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
	lf := &lockfile.Lockfile{Version: 1, Entries: map[string]lockfile.LockfileEntry{}}
	if lfData, err := os.ReadFile(lfPath); err == nil {
		if parsed, err := lockfile.ParseLockfile(lfData); err == nil {
			lf = parsed
		}
	}

	// Determine which deps to check
	targets := names
	if len(targets) == 0 {
		for name := range pf.Prompts {
			targets = append(targets, name)
		}
	}

	var skipped []string
	var updated []string
	result := &pull.Result{}

	for _, name := range targets {
		src, ok := pf.Prompts[name]
		if !ok {
			return nil, fmt.Errorf("alias %q not found in Promptfile", name)
		}

		// Skip non-registry deps
		if src.Kind != promptfile.SourceRegistry {
			skipped = append(skipped, name)
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: skipped (not a registry dep)", name))
			continue
		}

		// Check for newer version
		client := resolver.NewRegistryClient(cfg.RegistryURL, verifier)
		author, _, _, err := promptfile.ParseRef(src.Ref)
		if err != nil {
			return nil, fmt.Errorf("parsing ref for %s: %w", name, err)
		}
		_ = author

		// Resolve @latest to see if there's a newer version
		latestRef := src.Ref[:len(src.Ref)-len(versionFromRef(src.Ref))] + "latest"
		latestResult, err := client.Resolve(ctx, latestRef, cfg.Force)
		if err != nil {
			return nil, fmt.Errorf("checking %s for updates: %w", name, err)
		}

		// Compare against locked digest
		entry, hasLock := lf.Entries[name]
		if hasLock && entry.Digest == latestResult.Entry.Digest {
			result.Unchanged = append(result.Unchanged, name)
			continue
		}

		updated = append(updated, name)
	}

	if len(updated) == 0 && len(skipped) == len(targets) {
		return result, nil
	}
	if len(updated) == 0 {
		return result, nil
	}

	// Re-pull with updated refs
	for _, name := range updated {
		src := pf.Prompts[name]
		newRef := src.Ref[:len(src.Ref)-len(versionFromRef(src.Ref))] + "latest"
		pf.Prompts[name] = promptfile.Source{Kind: promptfile.SourceRegistry, Ref: newRef}
	}

	pfBytes, err := pf.Bytes()
	if err != nil {
		return nil, fmt.Errorf("writing Promptfile: %w", err)
	}
	os.WriteFile(pfPath, pfBytes, 0644)

	return pull.Pull(ctx, cfg)
}

func versionFromRef(ref string) string {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == '@' {
			return ref[i+1:]
		}
	}
	return ""
}
