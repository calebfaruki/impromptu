package index

import (
	"context"
	"fmt"
)

// InsertAuthor creates an author or returns the existing ID if the username already exists.
func (d *DB) InsertAuthor(ctx context.Context, username, displayName, avatarURL, profileURL string) (int64, error) {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO authors (username, display_name, avatar_url, profile_url)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(username) DO NOTHING`,
		username, displayName, avatarURL, profileURL)
	if err != nil {
		return 0, fmt.Errorf("inserting author %q: %w", username, err)
	}

	var id int64
	err = d.db.QueryRowContext(ctx,
		"SELECT id FROM authors WHERE username = ?", username).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("finding author %q after insert: %w", username, err)
	}
	return id, nil
}

// FindAuthor retrieves an author by username.
// Returns sql.ErrNoRows (wrapped) if not found.
func (d *DB) FindAuthor(ctx context.Context, username string) (Author, error) {
	var a Author
	var createdAt string
	err := d.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, avatar_url, profile_url, created_at
		 FROM authors WHERE username = ?`, username).Scan(
		&a.ID, &a.Username, &a.DisplayName, &a.AvatarURL, &a.ProfileURL, &createdAt)
	if err != nil {
		return Author{}, fmt.Errorf("finding author %q: %w", username, err)
	}
	a.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Author{}, fmt.Errorf("finding author %q: %w", username, err)
	}
	return a, nil
}
