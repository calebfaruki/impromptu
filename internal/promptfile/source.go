package promptfile

import (
	"fmt"
	"path"
	"strings"

	"github.com/calebfaruki/impromptu/internal/authprobe"
)

var allowedHosts = map[string]bool{
	"github.com":   true,
	"codeberg.org": true,
}

func parseSource(raw map[string]any) (Source, error) {
	if _, hasOCI := raw["oci"].(string); hasOCI {
		return Source{}, fmt.Errorf("OCI sources are not supported; use git clone or release")
	}

	gitURL, hasGit := raw["git"].(string)
	if !hasGit {
		return Source{}, fmt.Errorf("entry must have git key")
	}

	return parseGitSource(gitURL, raw)
}

func parseGitSource(gitURL string, raw map[string]any) (Source, error) {
	ref, _ := raw["ref"].(string)
	release, _ := raw["release"].(string)
	p, _ := raw["path"].(string)
	asset, _ := raw["asset"].(string)
	inline, _ := raw["inline"].(bool)

	if ref != "" && release != "" {
		return Source{}, fmt.Errorf("ref and release are mutually exclusive")
	}
	if ref == "" && release == "" {
		return Source{}, fmt.Errorf("entry must have ref or release")
	}
	if release != "" && p != "" {
		return Source{}, fmt.Errorf("path is not valid on release entries")
	}
	if ref != "" && asset != "" {
		return Source{}, fmt.Errorf("asset is not valid on clone entries")
	}

	if p != "" {
		if err := ValidatePath(p); err != nil {
			return Source{}, fmt.Errorf("git path: %w", err)
		}
	}

	if release != "" {
		return Source{
			Kind:    SourceRelease,
			Git:     gitURL,
			Release: release,
			Asset:   asset,
			Inline:  inline,
		}, nil
	}

	return Source{
		Kind:   SourceGit,
		Git:    gitURL,
		Ref:    ref,
		Path:   p,
		Inline: inline,
	}, nil
}

func validateGitHost(gitURL string) error {
	host, _, _ := authprobe.ParseSourceURL(gitURL)
	if !allowedHosts[host] {
		return fmt.Errorf("unsupported git host %q (supported: github.com, codeberg.org)", host)
	}
	return nil
}

// SourceFromFlags builds a Source from CLI flags.
// Validates URL scheme and host allowlist for release mode.
func SourceFromFlags(git, ref, release, p, asset string, inline bool) (Source, error) {
	if git == "" {
		return Source{}, fmt.Errorf("must specify --git")
	}

	if !strings.HasPrefix(git, "https://") && !strings.HasPrefix(git, "git@") && !strings.HasPrefix(git, "/") {
		return Source{}, fmt.Errorf("git URL must start with https:// or git@")
	}

	if release != "" {
		if err := validateGitHost(git); err != nil {
			return Source{}, fmt.Errorf("release mode requires a supported host: %w", err)
		}
	}

	raw := map[string]any{"git": git}
	if ref != "" {
		raw["ref"] = ref
	}
	if release != "" {
		raw["release"] = release
	}
	if p != "" {
		raw["path"] = p
	}
	if asset != "" {
		raw["asset"] = asset
	}
	if inline {
		raw["inline"] = true
	}
	return parseGitSource(git, raw)
}

// AliasFromSource derives a default alias from the source URL.
// If a path is set, uses the last segment of the path instead.
func AliasFromSource(src Source) string {
	if src.Path != "" {
		return path.Base(src.Path)
	}
	base := path.Base(src.Git)
	return strings.TrimSuffix(base, ".git")
}
