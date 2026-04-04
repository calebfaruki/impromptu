package index

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestInsertAuthor(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.InsertAuthor(ctx, "testuser", "Test User", "https://avatar.url", "https://profile.url")
	if err != nil {
		t.Fatalf("InsertAuthor: %v", err)
	}
	if id <= 0 {
		t.Errorf("got id %d, want > 0", id)
	}
}

func TestFindAuthor(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	_, err := d.InsertAuthor(ctx, "testuser", "Test User", "https://avatar.url", "https://profile.url")
	if err != nil {
		t.Fatal(err)
	}

	a, err := d.FindAuthor(ctx, "testuser")
	if err != nil {
		t.Fatalf("FindAuthor: %v", err)
	}
	if a.Username != "testuser" {
		t.Errorf("got username %q, want %q", a.Username, "testuser")
	}
	if a.DisplayName != "Test User" {
		t.Errorf("got display_name %q, want %q", a.DisplayName, "Test User")
	}
	if a.AvatarURL != "https://avatar.url" {
		t.Errorf("got avatar_url %q, want %q", a.AvatarURL, "https://avatar.url")
	}
	if a.ProfileURL != "https://profile.url" {
		t.Errorf("got profile_url %q, want %q", a.ProfileURL, "https://profile.url")
	}
	if a.CreatedAt.IsZero() {
		t.Error("created_at is zero")
	}
}

func TestInsertAuthorIdempotent(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id1, err := d.InsertAuthor(ctx, "testuser", "Test User", "", "")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := d.InsertAuthor(ctx, "testuser", "Different Name", "", "")
	if err != nil {
		t.Fatalf("second insert should not error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("got different ids: %d vs %d", id1, id2)
	}
}

func TestFindAuthorNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	_, err := d.FindAuthor(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}
