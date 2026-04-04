package index

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection for the prompt index.
type DB struct {
	db *sql.DB
}

// Author represents a registered prompt author.
type Author struct {
	ID          int64
	Username    string
	DisplayName string
	AvatarURL   string
	ProfileURL  string
	CreatedAt   time.Time
}

// Prompt represents a named prompt owned by an author.
type Prompt struct {
	ID          int64
	AuthorID    int64
	Name        string
	Description string
	CreatedAt   time.Time
}

// Version represents a published version of a prompt.
type Version struct {
	ID              int64
	PromptID        int64
	Version         string
	Digest          string
	SignatureBundle string
	RekorLogIndex   int64
	CreatedAt       time.Time
}

// SearchResult holds a prompt with its author info from a search query.
type SearchResult struct {
	PromptID    int64   `json:"prompt_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Author      string  `json:"author"`
	DisplayName string  `json:"display_name"`
	Rank        float64 `json:"rank"`
}

// Open creates a new database connection with WAL mode and foreign keys enabled.
func Open(dsn string) (*DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}
	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// RawDB returns the underlying *sql.DB for use by other packages
// that need to share the same database connection (e.g. auth sessions).
func (d *DB) RawDB() *sql.DB {
	return d.db
}

// Ping verifies the database connection is alive.
func (d *DB) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// Migrate applies SQL migration files from the given filesystem.
// Files must be in a "migrations" subdirectory, named with a numeric prefix (e.g. 001_create_tables.sql).
func Migrate(ctx context.Context, d *DB, migrations fs.FS) error {
	_, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("parsing migration filename %q: %w", entry.Name(), err)
		}

		var exists int
		err = d.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", version, err)
		}
		if exists > 0 {
			continue
		}

		content, err := fs.ReadFile(migrations, "migrations/"+entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", version, err)
		}
	}

	return nil
}

func parseVersion(filename string) (int, error) {
	idx := strings.Index(filename, "_")
	if idx < 0 {
		return 0, fmt.Errorf("no underscore in filename")
	}
	return strconv.Atoi(filename[:idx])
}

const timeLayout = "2006-01-02T15:04:05Z"

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", s, err)
	}
	return t, nil
}
