package promptfile

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Parse reads a Promptfile from TOML bytes.
func Parse(data []byte) (*Promptfile, error) {
	var raw struct {
		Version int            `toml:"version"`
		Prompts map[string]any `toml:"prompts"`
	}

	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

	// Check for duplicate keys via undecoded keys (TOML lib handles this)
	_ = md

	if raw.Version != 1 {
		return nil, fmt.Errorf("unsupported Promptfile version %d (expected 1)", raw.Version)
	}

	pf := &Promptfile{
		Version: raw.Version,
		Prompts: make(map[string]Source),
	}

	for name, entry := range raw.Prompts {
		src, err := parseEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("prompt %q: %w", name, err)
		}
		pf.Prompts[name] = src
	}

	return pf, nil
}

func parseEntry(entry any) (Source, error) {
	switch v := entry.(type) {
	case string:
		if _, _, _, err := parseRef(v); err != nil {
			return Source{}, err
		}
		return Source{Kind: SourceRegistry, Ref: v}, nil
	case map[string]any:
		return parseSource(v)
	default:
		return Source{}, fmt.Errorf("unexpected type %T", entry)
	}
}

// Bytes serializes the Promptfile to TOML.
func (pf *Promptfile) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "version = %d\n\n[prompts]\n", pf.Version)

	names := make([]string, 0, len(pf.Prompts))
	for name := range pf.Prompts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		src := pf.Prompts[name]
		line, err := formatSource(src)
		if err != nil {
			return nil, fmt.Errorf("formatting %q: %w", name, err)
		}
		fmt.Fprintf(&buf, "%s = %s\n", name, line)
	}

	return buf.Bytes(), nil
}

func formatSource(src Source) (string, error) {
	switch src.Kind {
	case SourceRegistry:
		return fmt.Sprintf("%q", src.Ref), nil
	case SourceGit:
		return formatGitSource(src), nil
	case SourceOCI:
		return formatOCISource(src), nil
	case SourcePrivate:
		return fmt.Sprintf("{registry = %q, ref = %q}", src.Registry, src.Ref), nil
	default:
		return "", fmt.Errorf("unknown source kind %q", src.Kind)
	}
}

func formatGitSource(src Source) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("git = %q", src.Git))
	if src.Tag != "" {
		parts = append(parts, fmt.Sprintf("tag = %q", src.Tag))
	}
	if src.Branch != "" {
		parts = append(parts, fmt.Sprintf("branch = %q", src.Branch))
	}
	if src.Commit != "" {
		parts = append(parts, fmt.Sprintf("commit = %q", src.Commit))
	}
	if src.Path != "" {
		parts = append(parts, fmt.Sprintf("path = %q", src.Path))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatOCISource(src Source) string {
	if src.OCITag != "" {
		return fmt.Sprintf("{oci = %q, tag = %q}", src.OCI, src.OCITag)
	}
	return fmt.Sprintf("{oci = %q, digest = %q}", src.OCI, src.Digest)
}

// AddEntry adds a prompt entry parsed from a short-form ref string.
func (pf *Promptfile) AddEntry(name, ref string) error {
	if _, exists := pf.Prompts[name]; exists {
		return fmt.Errorf("prompt %q already exists", name)
	}
	if _, _, _, err := parseRef(ref); err != nil {
		return err
	}
	pf.Prompts[name] = Source{Kind: SourceRegistry, Ref: ref}
	return nil
}

// RemoveEntry removes a prompt entry by name.
func (pf *Promptfile) RemoveEntry(name string) error {
	if _, exists := pf.Prompts[name]; !exists {
		return fmt.Errorf("prompt %q not found", name)
	}
	delete(pf.Prompts, name)
	return nil
}
