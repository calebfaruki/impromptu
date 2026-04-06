package promptfile

// SourceKind identifies how a prompt dependency is sourced.
type SourceKind string

const (
	SourceGit SourceKind = "git"
	SourceOCI SourceKind = "oci"
)

// Source describes where a prompt dependency comes from.
type Source struct {
	Kind SourceKind

	// Git
	Git    string
	Tag    string
	Branch string
	Commit string
	Path   string

	// OCI
	OCI    string
	OCITag string
	Digest string

	// Inline: single-file prompt placed directly in cwd
	Inline bool
}

// Promptfile represents a parsed Promptfile.
type Promptfile struct {
	Version int
	Prompts map[string]Source
}
