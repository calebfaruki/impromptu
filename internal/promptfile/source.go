package promptfile

import (
	"fmt"
	"path"
	"strings"
)

// parseSource detects the source type from a TOML table entry.
func parseSource(raw map[string]any) (Source, error) {
	_, hasGit := raw["git"].(string)
	_, hasOCI := raw["oci"].(string)

	if hasGit && hasOCI {
		return Source{}, fmt.Errorf("entry cannot have both git and oci")
	}

	if hasGit {
		return parseGitSource(raw["git"].(string), raw)
	}
	if hasOCI {
		return parseOCISource(raw["oci"].(string), raw)
	}

	return Source{}, fmt.Errorf("entry must have git or oci key")
}

func parseGitSource(gitURL string, raw map[string]any) (Source, error) {
	s := Source{Kind: SourceGit, Git: gitURL}

	tag, _ := raw["tag"].(string)
	branch, _ := raw["branch"].(string)
	commit, _ := raw["commit"].(string)

	refCount := 0
	if tag != "" {
		refCount++
		s.Tag = tag
	}
	if branch != "" {
		refCount++
		s.Branch = branch
	}
	if commit != "" {
		refCount++
		s.Commit = commit
	}
	if refCount != 1 {
		return Source{}, fmt.Errorf("git source must have exactly one of tag, branch, or commit")
	}

	if p, ok := raw["path"].(string); ok && p != "" {
		if err := ValidatePath(p); err != nil {
			return Source{}, fmt.Errorf("git path: %w", err)
		}
		s.Path = p
	}

	if inline, ok := raw["inline"].(bool); ok {
		s.Inline = inline
	}

	return s, nil
}

func parseOCISource(ociRef string, raw map[string]any) (Source, error) {
	s := Source{Kind: SourceOCI, OCI: ociRef}

	tag, _ := raw["tag"].(string)
	digest, _ := raw["digest"].(string)

	if tag != "" && digest != "" {
		return Source{}, fmt.Errorf("OCI source must have tag or digest, not both")
	}
	if tag == "" && digest == "" {
		return Source{}, fmt.Errorf("OCI source must have tag or digest")
	}

	if _, hasPath := raw["path"].(string); hasPath {
		return Source{}, fmt.Errorf("path is not valid for OCI sources")
	}

	if tag != "" {
		s.OCITag = tag
	}
	if digest != "" {
		s.Digest = digest
	}

	if inline, ok := raw["inline"].(bool); ok {
		s.Inline = inline
	}

	return s, nil
}

// SourceFromFlags builds a Source from CLI flags and validates mutual exclusivity.
func SourceFromFlags(git, oci, tag, branch, commit, digest, p string, inline bool) (Source, error) {
	if git != "" && oci != "" {
		return Source{}, fmt.Errorf("cannot specify both --git and --oci")
	}
	if git == "" && oci == "" {
		return Source{}, fmt.Errorf("must specify --git or --oci")
	}

	if git != "" {
		raw := map[string]any{"git": git}
		if tag != "" {
			raw["tag"] = tag
		}
		if branch != "" {
			raw["branch"] = branch
		}
		if commit != "" {
			raw["commit"] = commit
		}
		if p != "" {
			raw["path"] = p
		}
		if inline {
			raw["inline"] = true
		}
		return parseGitSource(git, raw)
	}

	// OCI
	raw := map[string]any{"oci": oci}
	if tag != "" {
		raw["tag"] = tag
	}
	if digest != "" {
		raw["digest"] = digest
	}
	if inline {
		raw["inline"] = true
	}
	return parseOCISource(oci, raw)
}

// AliasFromSource derives a default alias from the source URL.
func AliasFromSource(src Source) string {
	switch src.Kind {
	case SourceGit:
		base := path.Base(src.Git)
		return strings.TrimSuffix(base, ".git")
	case SourceOCI:
		parts := strings.Split(src.OCI, "/")
		return parts[len(parts)-1]
	}
	return ""
}
