package contentcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxFileCount = 100
	maxTotalSize = 10 * 1024 * 1024 // 10 MB
)

// CheckDirectory validates a directory of markdown files.
// Returns violations for content problems and an error for infrastructure failures.
func CheckDirectory(dir string) ([]Violation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var violations []Violation
	foundMD := false
	mdCount := 0
	var totalSize int64

	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		if strings.HasPrefix(name, ".") {
			continue
		}

		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			violations = append(violations, Violation{
				File:   name,
				Kind:   KindSymlink,
				Reason: "symlinks are not allowed",
			})
			continue
		}

		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			violations = append(violations, Violation{
				File:   name,
				Kind:   KindFiletype,
				Reason: fmt.Sprintf("non-markdown file %q is not allowed", name),
			})
			continue
		}

		mdCount++
		totalSize += info.Size()
		foundMD = true
		fileViolations, err := CheckFile(fullPath, name)
		if err != nil {
			return nil, err
		}
		violations = append(violations, fileViolations...)
	}

	if mdCount > maxFileCount {
		violations = append(violations, Violation{
			File:   dir,
			Kind:   KindLimit,
			Reason: fmt.Sprintf("too many .md files: %d (max %d)", mdCount, maxFileCount),
		})
	}

	if totalSize > maxTotalSize {
		violations = append(violations, Violation{
			File:   dir,
			Kind:   KindLimit,
			Reason: fmt.Sprintf("total .md size %d bytes exceeds limit of %d bytes", totalSize, maxTotalSize),
		})
	}

	if !foundMD && len(violations) == 0 {
		violations = append(violations, Violation{
			File:   dir,
			Kind:   KindEmpty,
			Reason: "directory contains no markdown files",
		})
	}

	return violations, nil
}
