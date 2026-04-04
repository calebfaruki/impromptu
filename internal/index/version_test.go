package index

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func insertTestPrompt(t *testing.T, d *DB) (authorID, promptID int64) {
	t.Helper()
	ctx := context.Background()
	authorID = insertTestAuthor(t, d)
	promptID, err := d.InsertPrompt(ctx, authorID, "test-prompt", "A test prompt")
	if err != nil {
		t.Fatal(err)
	}
	return authorID, promptID
}

func TestInsertVersion(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	id, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:abc123")
	if err != nil {
		t.Fatalf("InsertVersion: %v", err)
	}
	if id <= 0 {
		t.Errorf("got id %d, want > 0", id)
	}
}

func TestInsertVersionDuplicate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}

	_, err = d.InsertVersion(ctx, promptID, "1.0.0", "sha256:def")
	if err == nil {
		t.Fatal("expected error for duplicate version, got nil")
	}
}

func TestFindVersion(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:abc123")
	if err != nil {
		t.Fatal(err)
	}

	v, err := d.FindVersion(ctx, promptID, "1.0.0")
	if err != nil {
		t.Fatalf("FindVersion: %v", err)
	}
	if v.Version != "1.0.0" {
		t.Errorf("got version %q, want %q", v.Version, "1.0.0")
	}
	if v.Digest != "sha256:abc123" {
		t.Errorf("got digest %q, want %q", v.Digest, "sha256:abc123")
	}
	if v.PromptID != promptID {
		t.Errorf("got prompt_id %d, want %d", v.PromptID, promptID)
	}
	if v.CreatedAt.IsZero() {
		t.Error("created_at is zero")
	}
}

func TestFindVersionNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.FindVersion(ctx, promptID, "9.9.9")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestListVersions(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:first")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertVersion(ctx, promptID, "2.0.0", "sha256:second")
	if err != nil {
		t.Fatal(err)
	}

	versions, err := d.ListVersions(ctx, promptID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	// Newest first
	if versions[0].Version != "2.0.0" {
		t.Errorf("first version: got %q, want %q", versions[0].Version, "2.0.0")
	}
	if versions[1].Version != "1.0.0" {
		t.Errorf("second version: got %q, want %q", versions[1].Version, "1.0.0")
	}
}

func TestLatestVersion(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:first")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertVersion(ctx, promptID, "2.0.0", "sha256:second")
	if err != nil {
		t.Fatal(err)
	}

	v, err := d.LatestVersion(ctx, promptID)
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if v.Version != "2.0.0" {
		t.Errorf("got version %q, want %q", v.Version, "2.0.0")
	}
}

func TestLatestVersionNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.LatestVersion(ctx, promptID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestFindVersionUnsigned(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	_, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}

	v, err := d.FindVersion(ctx, promptID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.SignatureBundle != "" {
		t.Errorf("expected empty signature_bundle, got %q", v.SignatureBundle)
	}
	if v.RekorLogIndex != 0 {
		t.Errorf("expected rekor_log_index 0, got %d", v.RekorLogIndex)
	}
}

func TestSetVersionSignature(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	_, promptID := insertTestPrompt(t, d)

	versionID, err := d.InsertVersion(ctx, promptID, "1.0.0", "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}

	bundle := `{"digest":"sha256:abc","identity":"github.com/testuser"}`
	err = d.SetVersionSignature(ctx, versionID, bundle, 42)
	if err != nil {
		t.Fatalf("SetVersionSignature: %v", err)
	}

	v, err := d.FindVersion(ctx, promptID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.SignatureBundle != bundle {
		t.Errorf("got bundle %q, want %q", v.SignatureBundle, bundle)
	}
	if v.RekorLogIndex != 42 {
		t.Errorf("got rekor_log_index %d, want 42", v.RekorLogIndex)
	}
}

func TestSetVersionSignatureNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	err := d.SetVersionSignature(ctx, 9999, "bundle", 1)
	if err == nil {
		t.Fatal("expected error for nonexistent version, got nil")
	}
}
