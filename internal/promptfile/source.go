package promptfile

import (
	"fmt"
	"strings"
)

// parseRef parses "author/name@version" into its parts and validates the version.
func parseRef(ref string) (author, name, version string, err error) {
	atIdx := strings.LastIndex(ref, "@")
	if atIdx < 0 {
		return "", "", "", fmt.Errorf("ref %q missing @version", ref)
	}
	authorName := ref[:atIdx]
	version = ref[atIdx+1:]

	slashIdx := strings.Index(authorName, "/")
	if slashIdx < 0 {
		return "", "", "", fmt.Errorf("ref %q missing author/name", ref)
	}
	author = authorName[:slashIdx]
	name = authorName[slashIdx+1:]

	if author == "" || name == "" {
		return "", "", "", fmt.Errorf("ref %q has empty author or name", ref)
	}

	if err := ValidateVersion(version); err != nil {
		return "", "", "", fmt.Errorf("ref %q: %w", ref, err)
	}
	return author, name, version, nil
}

// parseSource detects the source type from a TOML table entry.
func parseSource(raw map[string]any) (Source, error) {
	// Check registry before ref -- private registry tables have both keys
	if regURL, ok := raw["registry"].(string); ok {
		return parsePrivateSource(regURL, raw)
	}

	if ref, ok := raw["ref"].(string); ok {
		if _, _, _, err := parseRef(ref); err != nil {
			return Source{}, err
		}
		return Source{Kind: SourceRegistry, Ref: ref}, nil
	}

	if gitURL, ok := raw["git"].(string); ok {
		return parseGitSource(gitURL, raw)
	}

	if ociRef, ok := raw["oci"].(string); ok {
		return parseOCISource(ociRef, raw)
	}

	return Source{}, fmt.Errorf("unknown source type: must have ref, git, oci, or registry key")
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

	if path, ok := raw["path"].(string); ok && path != "" {
		if err := ValidatePath(path); err != nil {
			return Source{}, fmt.Errorf("git path: %w", err)
		}
		s.Path = path
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

	if tag != "" {
		s.OCITag = tag
	}
	if digest != "" {
		s.Digest = digest
	}

	return s, nil
}

func parsePrivateSource(regURL string, raw map[string]any) (Source, error) {
	ref, ok := raw["ref"].(string)
	if !ok || ref == "" {
		return Source{}, fmt.Errorf("private registry source must have ref")
	}
	if _, _, _, err := parseRef(ref); err != nil {
		return Source{}, err
	}
	return Source{Kind: SourcePrivate, Registry: regURL, Ref: ref}, nil
}
