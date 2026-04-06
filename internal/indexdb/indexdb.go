package indexdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IndexEntry represents a prompt indexed in the registry.
type IndexEntry struct {
	ID             int64
	SourceURL      string
	Digest         string
	SignerIdentity string
	RekorLogIndex  int64
	IndexedAt      time.Time
}

// DB wraps a SQLite connection for index operations.
type DB struct {
	db *sql.DB
}

// New creates an index DB from an existing sql.DB connection.
func New(db *sql.DB) *DB {
	return &DB{db: db}
}

// InsertIndexEntry adds a prompt to the index. Idempotent on (source_url, digest).
func (d *DB) InsertIndexEntry(ctx context.Context, sourceURL, digest, signerIdentity string, rekorLogIndex int64) error {
	result, err := d.db.ExecContext(ctx,
		`INSERT INTO indexed_prompts (source_url, digest, signer_identity, rekor_log_index)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(source_url, digest) DO NOTHING`,
		sourceURL, digest, signerIdentity, rekorLogIndex)
	if err != nil {
		return fmt.Errorf("inserting index entry: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		// Sync FTS
		var id int64
		d.db.QueryRowContext(ctx,
			"SELECT id FROM indexed_prompts WHERE source_url = ? AND digest = ?",
			sourceURL, digest).Scan(&id)

		d.db.ExecContext(ctx,
			"INSERT INTO indexed_prompts_fts (rowid, source_url, signer_identity) VALUES (?, ?, ?)",
			id, sourceURL, signerIdentity)
	}

	return nil
}

// SearchIndex queries the FTS index for prompts matching the query.
func (d *DB) SearchIndex(ctx context.Context, query string, limit int) ([]IndexEntry, error) {
	if query == "" {
		return []IndexEntry{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	// Wrap each word in quotes for safe FTS5 querying
	sanitized := sanitizeQuery(query)
	if sanitized == "" {
		return []IndexEntry{}, nil
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT p.id, p.source_url, p.digest, p.signer_identity, p.rekor_log_index, p.indexed_at
		FROM indexed_prompts_fts
		JOIN indexed_prompts p ON p.id = indexed_prompts_fts.rowid
		WHERE indexed_prompts_fts MATCH ?
		LIMIT ?`,
		sanitized, limit)
	if err != nil {
		return nil, fmt.Errorf("searching index: %w", err)
	}
	defer rows.Close()

	var results []IndexEntry
	for rows.Next() {
		var e IndexEntry
		var indexedAt string
		if err := rows.Scan(&e.ID, &e.SourceURL, &e.Digest, &e.SignerIdentity, &e.RekorLogIndex, &indexedAt); err != nil {
			return nil, fmt.Errorf("scanning index entry: %w", err)
		}
		e.IndexedAt, _ = time.Parse("2006-01-02T15:04:05Z", indexedAt)
		results = append(results, e)
	}
	if results == nil {
		results = []IndexEntry{}
	}
	return results, rows.Err()
}

// FindBySourceURL returns all entries matching the given source URL.
func (d *DB) FindBySourceURL(ctx context.Context, sourceURL string) ([]IndexEntry, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, source_url, digest, signer_identity, rekor_log_index, indexed_at
		 FROM indexed_prompts
		 WHERE source_url = ?
		 ORDER BY indexed_at DESC`,
		sourceURL)
	if err != nil {
		return nil, fmt.Errorf("finding by source url: %w", err)
	}
	defer rows.Close()

	var results []IndexEntry
	for rows.Next() {
		var e IndexEntry
		var indexedAt string
		if err := rows.Scan(&e.ID, &e.SourceURL, &e.Digest, &e.SignerIdentity, &e.RekorLogIndex, &indexedAt); err != nil {
			return nil, fmt.Errorf("scanning index entry: %w", err)
		}
		e.IndexedAt, _ = time.Parse("2006-01-02T15:04:05Z", indexedAt)
		results = append(results, e)
	}
	return results, rows.Err()
}

func sanitizeQuery(q string) string {
	words := splitWords(q)
	var out []string
	for _, w := range words {
		w = stripQuotes(w)
		if w != "" {
			out = append(out, "\""+w+"\"")
		}
	}
	if len(out) == 0 {
		return ""
	}
	result := out[0]
	for i := 1; i < len(out); i++ {
		result += " " + out[i]
	}
	return result
}

func splitWords(s string) []string {
	var words []string
	word := ""
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(c)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

func stripQuotes(s string) string {
	result := ""
	for _, c := range s {
		if c != '"' {
			result += string(c)
		}
	}
	return result
}
