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
	if src.Git != entry.Git {
		return false
	}
	switch src.Kind {
	case promptfile.SourceGit:
		if src.Ref != entry.Ref || src.Path != entry.Path {
			return false
		}
		return true
	case promptfile.SourceRelease:
		if src.Release != entry.Release || src.Asset != entry.Asset {
			return false
		}
		return true
	}
	return false
}
