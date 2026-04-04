package contentcheck

import "fmt"

// Kind classifies a content violation.
type Kind string

const (
	KindUnicode  Kind = "unicode"
	KindHTML     Kind = "html"
	KindFiletype Kind = "filetype"
	KindSymlink  Kind = "symlink"
	KindBinary   Kind = "binary"
	KindEmpty    Kind = "empty"
)

// Violation describes a single content check failure.
// Line and Column are 1-based. Column is a byte offset within the line.
// Line and Column are 0 when not applicable (e.g. symlink, filetype checks).
type Violation struct {
	File   string
	Line   int
	Column int
	Reason string
	Kind   Kind
}

func (v Violation) Error() string {
	if v.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: [%s] %s", v.File, v.Line, v.Column, v.Kind, v.Reason)
	}
	return fmt.Sprintf("%s: [%s] %s", v.File, v.Kind, v.Reason)
}
