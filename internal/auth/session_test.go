package auth

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testSessionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.Exec("PRAGMA foreign_keys=ON")

	rootFS := os.DirFS(filepath.Join("..", "..", "."))
	entries, err := fs.ReadDir(rootFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		content, err := fs.ReadFile(rootFS, "migrations/"+e.Name())
		if err != nil {
			t.Fatalf("reading migration %s: %v", e.Name(), err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", e.Name(), err)
		}
	}
	return db
}

func insertTestAuthorDirect(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	result, err := db.ExecContext(context.Background(),
		"INSERT INTO authors (username, display_name) VALUES (?, ?)",
		"testuser", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestSessionCreate(t *testing.T) {
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	ctx := context.Background()

	session, err := store.Create(ctx, authorID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(session.Token) != 64 {
		t.Errorf("token length: got %d, want 64", len(session.Token))
	}
	if session.AuthorID != authorID {
		t.Errorf("author_id: got %d, want %d", session.AuthorID, authorID)
	}
	if session.ExpiresAt.Before(time.Now().Add(29 * 24 * time.Hour)) {
		t.Error("expiry too soon")
	}
}

func TestSessionFind(t *testing.T) {
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	ctx := context.Background()

	session, err := store.Create(ctx, authorID)
	if err != nil {
		t.Fatal(err)
	}

	info, err := store.Find(ctx, session.Token)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if info.Session.Token != session.Token {
		t.Errorf("token mismatch")
	}
	if info.Username != "testuser" {
		t.Errorf("username: got %q, want %q", info.Username, "testuser")
	}
	if info.Session.AuthorID != authorID {
		t.Errorf("author_id: got %d, want %d", info.Session.AuthorID, authorID)
	}
}

func TestSessionFindNotFound(t *testing.T) {
	db := testSessionDB(t)
	store := NewSessionStore(db)
	ctx := context.Background()

	_, err := store.Find(ctx, "nonexistent-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestSessionFindExpired(t *testing.T) {
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	ctx := context.Background()

	// Insert an already-expired session directly
	expired := time.Now().UTC().Add(-1 * time.Hour).Format(timeLayout)
	_, err := db.ExecContext(ctx,
		"INSERT INTO sessions (token, author_id, expires_at) VALUES (?, ?, ?)",
		"expired-token", authorID, expired)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Find(ctx, "expired-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("expected ErrSessionExpired, got: %v", err)
	}
}

func TestSessionDelete(t *testing.T) {
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	ctx := context.Background()

	session, err := store.Create(ctx, authorID)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(ctx, session.Token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Find(ctx, session.Token)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound after delete, got: %v", err)
	}
}

func TestSessionDeleteExpired(t *testing.T) {
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	ctx := context.Background()

	// Create a valid session
	valid, err := store.Create(ctx, authorID)
	if err != nil {
		t.Fatal(err)
	}

	// Insert an expired session directly
	expired := time.Now().UTC().Add(-1 * time.Hour).Format(timeLayout)
	_, err = db.ExecContext(ctx,
		"INSERT INTO sessions (token, author_id, expires_at) VALUES (?, ?, ?)",
		"old-token", authorID, expired)
	if err != nil {
		t.Fatal(err)
	}

	count, err := store.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 1 {
		t.Errorf("deleted %d, want 1", count)
	}

	// Valid session should still exist
	_, err = store.Find(ctx, valid.Token)
	if err != nil {
		t.Errorf("valid session missing after cleanup: %v", err)
	}
}
