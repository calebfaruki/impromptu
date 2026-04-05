package promptfile

// SourceKind identifies how a prompt dependency is sourced.
type SourceKind string

const (
	SourceRegistry SourceKind = "registry"
	SourceGit      SourceKind = "git"
	SourceOCI      SourceKind = "oci"
	SourcePrivate  SourceKind = "private"
)

// Source describes where a prompt dependency comes from.
type Source struct {
	Kind SourceKind

	// Registry: "author/name@version"
	Ref string

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

	// Private registry
	Registry string
}

// Promptfile represents a parsed Promptfile.
type Promptfile struct {
	Version int
	Prompts map[string]Source
}
