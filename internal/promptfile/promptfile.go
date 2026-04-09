package promptfile

// SourceKind identifies how a prompt dependency is sourced.
type SourceKind string

const (
	SourceGit     SourceKind = "git"
	SourceRelease SourceKind = "release"
)

// Source describes where a prompt dependency comes from.
type Source struct {
	Kind    SourceKind
	Git     string // required: HTTPS or SSH URL
	Ref     string // clone mode: tag, branch, or commit SHA
	Release string // release mode: release tag name
	Path    string // clone only: subdirectory within repo
	Asset   string // release only: non-standard asset filename
	Inline  bool
}

// Promptfile represents a parsed Promptfile.
type Promptfile struct {
	Version int
	Prompts map[string]Source
}
