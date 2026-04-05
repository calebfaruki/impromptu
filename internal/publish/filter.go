package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CollectFiles returns paths to root-level .md files in dir.
// Excludes: Promptfile, Promptfile.lock, hidden files, subdirectories, non-.md files.
func CollectFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		if name == "Promptfile" || name == "Promptfile.lock" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		files = append(files, filepath.Join(dir, name))
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .md files found in %s", dir)
	}

	return files, nil
}
