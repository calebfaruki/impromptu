package index

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func insertTestAuthor(t *testing.T, d *DB) int64 {
	t.Helper()
	id, err := d.InsertAuthor(context.Background(), "testuser", "Test User", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInsertPrompt(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	authorID := insertTestAuthor(t, d)

	id, err := d.InsertPrompt(ctx, authorID, "code-review", "A prompt for code review")
	if err != nil {
		t.Fatalf("InsertPrompt: %v", err)
	}
	if id <= 0 {
		t.Errorf("got id %d, want > 0", id)
	}
}

func TestFindPromptByAuthorName(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	authorID := insertTestAuthor(t, d)

	_, err := d.InsertPrompt(ctx, authorID, "code-review", "A prompt for code review")
	if err != nil {
		t.Fatal(err)
	}

	p, err := d.FindPromptByAuthorName(ctx, authorID, "code-review")
	if err != nil {
		t.Fatalf("FindPromptByAuthorName: %v", err)
	}
	if p.Name != "code-review" {
		t.Errorf("got name %q, want %q", p.Name, "code-review")
	}
	if p.Description != "A prompt for code review" {
		t.Errorf("got description %q, want %q", p.Description, "A prompt for code review")
	}
	if p.AuthorID != authorID {
		t.Errorf("got author_id %d, want %d", p.AuthorID, authorID)
	}
	if p.CreatedAt.IsZero() {
		t.Error("created_at is zero")
	}
}

func TestInsertPromptDuplicate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	authorID := insertTestAuthor(t, d)

	_, err := d.InsertPrompt(ctx, authorID, "code-review", "first")
	if err != nil {
		t.Fatal(err)
	}

	_, err = d.InsertPrompt(ctx, authorID, "code-review", "second")
	if err == nil {
		t.Fatal("expected error for duplicate prompt, got nil")
	}
}

func TestListPromptsByAuthor(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	authorID := insertTestAuthor(t, d)

	_, err := d.InsertPrompt(ctx, authorID, "alpha", "first prompt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertPrompt(ctx, authorID, "beta", "second prompt")
	if err != nil {
		t.Fatal(err)
	}

	prompts, err := d.ListPromptsByAuthor(ctx, authorID)
	if err != nil {
		t.Fatalf("ListPromptsByAuthor: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(prompts))
	}
}

func TestFindPromptNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	authorID := insertTestAuthor(t, d)

	_, err := d.FindPromptByAuthorName(ctx, authorID, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}
