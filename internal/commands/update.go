package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
)

// Update checks deps for newer versions. In v3, git/OCI deps are skipped
// (pinned deliberately). Full update support will be added in a later phase.
func Update(ctx context.Context, cfg pull.Config, names ...string) (*pull.Result, error) {
	pfPath := filepath.Join(cfg.Dir, "Promptfile")
	pfData, err := os.ReadFile(pfPath)
	if err != nil {
		return nil, fmt.Errorf("reading Promptfile: %w", err)
	}

	pf, err := promptfile.Parse(pfData)
	if err != nil {
		return nil, fmt.Errorf("parsing Promptfile: %w", err)
	}

	targets := names
	if len(targets) == 0 {
		for name := range pf.Prompts {
			targets = append(targets, name)
		}
	}

	result := &pull.Result{}

	for _, name := range targets {
		_, ok := pf.Prompts[name]
		if !ok {
			return nil, fmt.Errorf("alias %q not found in Promptfile", name)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: skipped (pinned dep, update not yet supported for git/oci sources)", name))
	}

	return result, nil
}
