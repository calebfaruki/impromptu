package commands

import (
	"fmt"
	"os"
	"path/filepath"
)

// Init creates a new Promptfile in the given directory.
func Init(dir string) error {
	path := filepath.Join(dir, "Promptfile")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("Promptfile already exists in %s", dir)
	}
	content := "version = 1\n\n[prompts]\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("creating Promptfile: %w", err)
	}
	return nil
}
