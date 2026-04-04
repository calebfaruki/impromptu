package index

import (
	"context"
	"fmt"
)

// InsertVersion creates a new version for a prompt.
// Returns an error if the version already exists for this prompt.
func (d *DB) InsertVersion(ctx context.Context, promptID int64, version, digest string) (int64, error) {
	result, err := d.db.ExecContext(ctx,
		"INSERT INTO versions (prompt_id, version, digest) VALUES (?, ?, ?)",
		promptID, version, digest)
	if err != nil {
		return 0, fmt.Errorf("inserting version %q for prompt %d: %w", version, promptID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting version id: %w", err)
	}
	return id, nil
}

// FindVersion retrieves a specific version by prompt ID and version string.
// Returns sql.ErrNoRows (wrapped) if not found.
func (d *DB) FindVersion(ctx context.Context, promptID int64, version string) (Version, error) {
	var v Version
	var createdAt string
	err := d.db.QueryRowContext(ctx,
		`SELECT id, prompt_id, version, digest, created_at
		 FROM versions WHERE prompt_id = ? AND version = ?`,
		promptID, version).Scan(&v.ID, &v.PromptID, &v.Version, &v.Digest, &createdAt)
	if err != nil {
		return Version{}, fmt.Errorf("finding version %q for prompt %d: %w", version, promptID, err)
	}
	v.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Version{}, fmt.Errorf("finding version %q for prompt %d: %w", version, promptID, err)
	}
	return v, nil
}

// ListVersions returns all versions for a prompt, newest first.
func (d *DB) ListVersions(ctx context.Context, promptID int64) ([]Version, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, prompt_id, version, digest, created_at
		 FROM versions WHERE prompt_id = ? ORDER BY id DESC`,
		promptID)
	if err != nil {
		return nil, fmt.Errorf("listing versions for prompt %d: %w", promptID, err)
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		var v Version
		var createdAt string
		if err := rows.Scan(&v.ID, &v.PromptID, &v.Version, &v.Digest, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning version: %w", err)
		}
		var parseErr error
		v.CreatedAt, parseErr = parseTime(createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("scanning version: %w", parseErr)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// LatestVersion returns the most recent version for a prompt.
// Returns sql.ErrNoRows (wrapped) if the prompt has no versions.
func (d *DB) LatestVersion(ctx context.Context, promptID int64) (Version, error) {
	var v Version
	var createdAt string
	err := d.db.QueryRowContext(ctx,
		`SELECT id, prompt_id, version, digest, created_at
		 FROM versions WHERE prompt_id = ? ORDER BY id DESC LIMIT 1`,
		promptID).Scan(&v.ID, &v.PromptID, &v.Version, &v.Digest, &createdAt)
	if err != nil {
		return Version{}, fmt.Errorf("finding latest version for prompt %d: %w", promptID, err)
	}
	v.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Version{}, fmt.Errorf("finding latest version for prompt %d: %w", promptID, err)
	}
	return v, nil
}
