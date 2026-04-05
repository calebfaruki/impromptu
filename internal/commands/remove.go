package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// Remove deletes a prompt dependency: Promptfile entry, lockfile entry, and on-disk directory.
func Remove(dir string, alias string) error {
	pfPath := filepath.Join(dir, "Promptfile")
	pfData, err := os.ReadFile(pfPath)
	if err != nil {
		return fmt.Errorf("reading Promptfile: %w", err)
	}

	pf, err := promptfile.Parse(pfData)
	if err != nil {
		return fmt.Errorf("parsing Promptfile: %w", err)
	}

	if err := pf.RemoveEntry(alias); err != nil {
		return err
	}

	pfBytes, err := pf.Bytes()
	if err != nil {
		return fmt.Errorf("writing Promptfile: %w", err)
	}
	if err := os.WriteFile(pfPath, pfBytes, 0644); err != nil {
		return fmt.Errorf("writing Promptfile: %w", err)
	}

	// Delete on-disk directory
	os.RemoveAll(filepath.Join(dir, alias))

	// Update lockfile
	lfPath := filepath.Join(dir, "Promptfile.lock")
	lfData, err := os.ReadFile(lfPath)
	if err == nil {
		lf, err := lockfile.ParseLockfile(lfData)
		if err == nil {
			delete(lf.Entries, alias)
			lfBytes, err := lf.Bytes()
			if err == nil {
				os.WriteFile(lfPath, lfBytes, 0644)
			}
		}
	}

	return nil
}
