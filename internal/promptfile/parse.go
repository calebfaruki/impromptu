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

	_, err := toml.Decode(string(data), &raw)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

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
		return Source{}, fmt.Errorf("string format %q is no longer supported; use { git = \"...\", ref = \"...\" } instead", v)
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
	case SourceGit:
		return formatCloneSource(src), nil
	case SourceRelease:
		return formatReleaseSource(src), nil
	default:
		return "", fmt.Errorf("unknown source kind %q", src.Kind)
	}
}

func formatCloneSource(src Source) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("git = %q", src.Git))
	parts = append(parts, fmt.Sprintf("ref = %q", src.Ref))
	if src.Path != "" {
		parts = append(parts, fmt.Sprintf("path = %q", src.Path))
	}
	if src.Inline {
		parts = append(parts, "inline = true")
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatReleaseSource(src Source) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("git = %q", src.Git))
	parts = append(parts, fmt.Sprintf("release = %q", src.Release))
	if src.Asset != "" {
		parts = append(parts, fmt.Sprintf("asset = %q", src.Asset))
	}
	if src.Inline {
		parts = append(parts, "inline = true")
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// AddSource adds a prompt entry with a structured Source. Returns error if name exists.
func (pf *Promptfile) AddSource(name string, src Source) error {
	if _, exists := pf.Prompts[name]; exists {
		return fmt.Errorf("prompt %q already exists", name)
	}
	pf.Prompts[name] = src
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
