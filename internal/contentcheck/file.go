package contentcheck

import (
	"fmt"
	"os"
	"unicode/utf8"
)

// CheckFile reads a file and runs content checks.
// Binary (non-UTF-8) files are rejected immediately.
func CheckFile(path string, relPath string) ([]Violation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", relPath, err)
	}

	if !utf8.Valid(data) {
		return []Violation{{
			File:   relPath,
			Kind:   KindBinary,
			Reason: "file contains non-UTF-8 bytes (binary content)",
		}}, nil
	}

	content := string(data)
	var violations []Violation
	violations = append(violations, CheckUnicode(content, relPath)...)
	violations = append(violations, CheckHTML(content, relPath)...)
	violations = append(violations, CheckFrontmatter(content, relPath)...)
	return violations, nil
}
