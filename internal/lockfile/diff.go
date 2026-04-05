package lockfile

import "github.com/calebfaruki/impromptu/internal/promptfile"

// DiffResult categorizes changes between a Promptfile and its lockfile.
type DiffResult struct {
	Added     []string
	Removed   []string
	Unchanged []string
}

// Diff compares a Promptfile against a Lockfile and categorizes each dependency.
func Diff(pf *promptfile.Promptfile, lf *Lockfile) DiffResult {
	var result DiffResult

	for name, src := range pf.Prompts {
		entry, ok := lf.Entries[name]
		if !ok || !sourceMatches(src, entry) {
			result.Added = append(result.Added, name)
		} else {
			result.Unchanged = append(result.Unchanged, name)
		}
	}

	for name := range lf.Entries {
		if _, ok := pf.Prompts[name]; !ok {
			result.Removed = append(result.Removed, name)
		}
	}

	return result
}

func sourceMatches(src promptfile.Source, entry LockfileEntry) bool {
	if src.Kind != entry.Source {
		return false
	}
	switch src.Kind {
	case promptfile.SourceRegistry:
		return src.Ref == entry.Ref
	case promptfile.SourceGit:
		// Compare intent fields only. Commit in the lockfile is the resolved
		// SHA -- only compare if the Promptfile explicitly pins a commit.
		if src.Git != entry.Git || src.Tag != entry.Tag ||
			src.Branch != entry.Branch || src.Path != entry.Path {
			return false
		}
		if src.Commit != "" && src.Commit != entry.Commit {
			return false
		}
		return true
	case promptfile.SourceOCI:
		// Compare OCI registry and the ref the Promptfile declares.
		// If Promptfile uses a tag, don't compare digests (lockfile resolves them).
		if src.OCI != entry.OCI {
			return false
		}
		if src.OCITag != entry.Tag {
			return false
		}
		if src.Digest != "" && src.Digest != entry.Digest {
			return false
		}
		return true
	case promptfile.SourcePrivate:
		return src.Registry == entry.Registry && src.Ref == entry.Ref
	}
	return false
}
