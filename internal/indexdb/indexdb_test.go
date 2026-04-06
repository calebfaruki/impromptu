package indexdb

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.Exec("PRAGMA foreign_keys=ON")

	rootFS := os.DirFS(filepath.Join("..", "..", "."))
	entries, _ := fs.ReadDir(rootFS, "migrations")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		content, _ := fs.ReadFile(rootFS, "migrations/"+e.Name())
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", e.Name(), err)
		}
	}

	return New(db)
}

func TestInsertAndRetrieve(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:abc", "alice@github.com", 42)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := db.SearchIndex(ctx, "alice", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SourceURL != "https://github.com/alice/coder" {
		t.Errorf("url: got %q", results[0].SourceURL)
	}
	if results[0].Digest != "sha256:abc" {
		t.Errorf("digest: got %q", results[0].Digest)
	}
}

func TestInsertDuplicateIdempotent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:abc", "alice@github.com", 42)
	if err != nil {
		t.Fatal(err)
	}
	err = db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:abc", "alice@github.com", 42)
	if err != nil {
		t.Fatalf("duplicate should be idempotent: %v", err)
	}
}

func TestInsertSameURLDifferentDigest(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:v1", "alice@github.com", 1)
	db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:v2", "alice@github.com", 2)

	results, _ := db.SearchIndex(ctx, "alice", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 entries for different digests, got %d", len(results))
	}
}

func TestSearchByURLFragment(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:abc", "", 0)
	db.InsertIndexEntry(ctx, "https://github.com/bob/helper", "sha256:def", "", 0)

	results, _ := db.SearchIndex(ctx, "coder", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'coder', got %d", len(results))
	}
}

func TestSearchBySignerIdentity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	db.InsertIndexEntry(ctx, "https://github.com/alice/coder", "sha256:abc", "alice@github.com", 42)
	db.InsertIndexEntry(ctx, "https://github.com/bob/helper", "sha256:def", "bob@github.com", 43)

	results, _ := db.SearchIndex(ctx, "alice@github.com", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for alice identity, got %d", len(results))
	}
}

func TestSearchEmpty(t *testing.T) {
	db := testDB(t)
	results, _ := db.SearchIndex(context.Background(), "", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 for empty query, got %d", len(results))
	}
}
