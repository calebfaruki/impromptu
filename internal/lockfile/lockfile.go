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
	Ref      string // clone mode: original ref string
	RefType  string // "tag", "branch", or "commit"
	Release  string // release mode: release tag name
	Asset    string // release mode: non-standard asset filename
	Commit   string // resolved commit SHA (clone mode)
	Path     string
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

// rawLockEntry supports both old (tag/branch/commit) and new (ref/ref_type/release/asset) fields for migration.
type rawLockEntry struct {
	Name     string `toml:"name"`
	Source   string `toml:"source"`
	Git      string `toml:"git,omitempty"`
	Ref      string `toml:"ref,omitempty"`
	RefType  string `toml:"ref_type,omitempty"`
	Release  string `toml:"release,omitempty"`
	Asset    string `toml:"asset,omitempty"`
	Commit   string `toml:"commit,omitempty"`
	Path     string `toml:"path,omitempty"`
	Digest   string `toml:"digest,omitempty"`
	Signer   string `toml:"signer,omitempty"`
	Inline   bool   `toml:"inline,omitempty"`
	Filename string `toml:"filename,omitempty"`
	// Old fields for migration (read-only, never written)
	Tag    string `toml:"tag,omitempty"`
	Branch string `toml:"branch,omitempty"`
}

// ParseLockfile reads a Promptfile.lock from TOML bytes.
// Handles migration from old format (tag/branch/commit) to new format (ref/ref_type).
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
		if r.Source == "oci" {
			continue
		}
		entry := migrateEntry(r)
		lf.Entries[entry.Name] = entry
	}
	return lf, nil
}

// migrateEntry converts a raw TOML entry to a LockfileEntry, handling old→new field migration.
func migrateEntry(r rawLockEntry) LockfileEntry {
	e := LockfileEntry{
		Name:     r.Name,
		Source:   promptfile.SourceKind(r.Source),
		Git:      r.Git,
		Ref:      r.Ref,
		RefType:  r.RefType,
		Release:  r.Release,
		Asset:    r.Asset,
		Commit:   r.Commit,
		Path:     r.Path,
		Digest:   r.Digest,
		Signer:   r.Signer,
		Inline:   r.Inline,
		Filename: r.Filename,
	}

	// Migrate old tag/branch/commit fields to ref/ref_type
	if e.Ref == "" && e.Release == "" {
		switch {
		case r.Tag != "":
			e.Ref = r.Tag
			e.RefType = "tag"
		case r.Branch != "":
			e.Ref = r.Branch
			e.RefType = "branch"
		case r.Commit != "" && e.Source == promptfile.SourceGit:
			e.Ref = r.Commit
			e.RefType = "commit"
		}
	}

	return e
}

// Bytes serializes the Lockfile to TOML (always writes new format).
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
			Ref:      e.Ref,
			RefType:  e.RefType,
			Release:  e.Release,
			Asset:    e.Asset,
			Commit:   e.Commit,
			Path:     e.Path,
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
