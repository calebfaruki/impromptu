package index

import (
	"context"
	"fmt"
)

// InsertPrompt creates a prompt and indexes it for full-text search.
// Returns an error if the author already has a prompt with the same name.
func (d *DB) InsertPrompt(ctx context.Context, authorID int64, name, description string) (int64, error) {
	var username string
	err := d.db.QueryRowContext(ctx,
		"SELECT username FROM authors WHERE id = ?", authorID).Scan(&username)
	if err != nil {
		return 0, fmt.Errorf("finding author %d for prompt insert: %w", authorID, err)
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		"INSERT INTO prompts (author_id, name, description) VALUES (?, ?, ?)",
		authorID, name, description)
	if err != nil {
		return 0, fmt.Errorf("inserting prompt %q: %w", name, err)
	}
	promptID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting prompt id: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO prompts_fts (rowid, name, description, author) VALUES (?, ?, ?, ?)",
		promptID, name, description, username)
	if err != nil {
		return 0, fmt.Errorf("indexing prompt %q for search: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing prompt insert: %w", err)
	}
	return promptID, nil
}

// FindPromptByAuthorName retrieves a prompt by author ID and name.
// Returns sql.ErrNoRows (wrapped) if not found.
func (d *DB) FindPromptByAuthorName(ctx context.Context, authorID int64, name string) (Prompt, error) {
	var p Prompt
	var createdAt string
	err := d.db.QueryRowContext(ctx,
		`SELECT id, author_id, name, description, created_at
		 FROM prompts WHERE author_id = ? AND name = ?`,
		authorID, name).Scan(&p.ID, &p.AuthorID, &p.Name, &p.Description, &createdAt)
	if err != nil {
		return Prompt{}, fmt.Errorf("finding prompt %q for author %d: %w", name, authorID, err)
	}
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Prompt{}, fmt.Errorf("finding prompt %q for author %d: %w", name, authorID, err)
	}
	return p, nil
}

// ListPromptsByAuthor returns all prompts for an author, newest first.
func (d *DB) ListPromptsByAuthor(ctx context.Context, authorID int64) ([]Prompt, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, author_id, name, description, created_at
		 FROM prompts WHERE author_id = ? ORDER BY created_at DESC`,
		authorID)
	if err != nil {
		return nil, fmt.Errorf("listing prompts for author %d: %w", authorID, err)
	}
	defer rows.Close()

	var prompts []Prompt
	for rows.Next() {
		var p Prompt
		var createdAt string
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.Name, &p.Description, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning prompt: %w", err)
		}
		var parseErr error
		p.CreatedAt, parseErr = parseTime(createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("scanning prompt: %w", parseErr)
		}
		prompts = append(prompts, p)
	}
	return prompts, rows.Err()
}
