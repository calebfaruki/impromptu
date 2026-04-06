package lockfile

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// LockfileEntry represents a resolved dependency.
type LockfileEntry struct {
	Name     string
	Source   promptfile.SourceKind
	Git      string
	Tag      string
	Branch   string
	Commit   string
	Path     string
	OCI      string
	Digest   string
	Signer   string
	Inline   bool
	Filename string
}

// Lockfile represents a parsed Promptfile.lock.
type Lockfile struct {
	Version int
	Entries map[string]LockfileEntry
}

// rawLockfile mirrors the TOML structure for parsing.
type rawLockfile struct {
	Version int            `toml:"version"`
	Prompt  []rawLockEntry `toml:"prompt"`
}

type rawLockEntry struct {
	Name     string `toml:"name"`
	Source   string `toml:"source"`
	Git      string `toml:"git,omitempty"`
	Tag      string `toml:"tag,omitempty"`
	Branch   string `toml:"branch,omitempty"`
	Commit   string `toml:"commit,omitempty"`
	Path     string `toml:"path,omitempty"`
	OCI      string `toml:"oci,omitempty"`
	Digest   string `toml:"digest,omitempty"`
	Signer   string `toml:"signer,omitempty"`
	Inline   bool   `toml:"inline,omitempty"`
	Filename string `toml:"filename,omitempty"`
}

// ParseLockfile reads a Promptfile.lock from TOML bytes.
func ParseLockfile(data []byte) (*Lockfile, error) {
	var raw rawLockfile
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("parsing lockfile: %w", err)
	}
	if raw.Version != 1 {
		return nil, fmt.Errorf("unsupported lockfile version %d (expected 1)", raw.Version)
	}

	lf := &Lockfile{
		Version: raw.Version,
		Entries: make(map[string]LockfileEntry, len(raw.Prompt)),
	}
	for _, r := range raw.Prompt {
		lf.Entries[r.Name] = LockfileEntry{
			Name:     r.Name,
			Source:   promptfile.SourceKind(r.Source),
			Git:      r.Git,
			Tag:      r.Tag,
			Branch:   r.Branch,
			Commit:   r.Commit,
			Path:     r.Path,
			OCI:      r.OCI,
			Digest:   r.Digest,
			Signer:   r.Signer,
			Inline:   r.Inline,
			Filename: r.Filename,
		}
	}
	return lf, nil
}

// Bytes serializes the Lockfile to TOML.
func (lf *Lockfile) Bytes() ([]byte, error) {
	names := make([]string, 0, len(lf.Entries))
	for name := range lf.Entries {
		names = append(names, name)
	}
	sort.Strings(names)

	var entries []rawLockEntry
	for _, name := range names {
		e := lf.Entries[name]
		entries = append(entries, rawLockEntry{
			Name:     e.Name,
			Source:   string(e.Source),
			Git:      e.Git,
			Tag:      e.Tag,
			Branch:   e.Branch,
			Commit:   e.Commit,
			Path:     e.Path,
			OCI:      e.OCI,
			Digest:   e.Digest,
			Signer:   e.Signer,
			Inline:   e.Inline,
			Filename: e.Filename,
		})
	}

	raw := rawLockfile{Version: lf.Version, Prompt: entries}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(raw); err != nil {
		return nil, fmt.Errorf("encoding lockfile: %w", err)
	}
	return buf.Bytes(), nil
}
