package index

import (
	"context"
	"testing"
)

// seedSearchData populates the database with test data for search tests.
// Returns author IDs and prompt IDs for verification.
func seedSearchData(t *testing.T, d *DB) {
	t.Helper()
	ctx := context.Background()

	alice, err := d.InsertAuthor(ctx, "alice", "Alice Smith", "", "")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := d.InsertAuthor(ctx, "bob", "Bob Jones", "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = d.InsertPrompt(ctx, alice, "code-review", "A prompt for reviewing pull requests")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertPrompt(ctx, alice, "deploy", "Helps deploy applications to production")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InsertPrompt(ctx, bob, "helper", "A deploy automation tool for CI pipelines")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchByName(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	results, err := d.SearchPrompts(ctx, "code-review", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].Name != "code-review" {
		t.Errorf("got name %q, want %q", results[0].Name, "code-review")
	}
	if results[0].Author != "alice" {
		t.Errorf("got author %q, want %q", results[0].Author, "alice")
	}
}

func TestSearchByDescription(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	results, err := d.SearchPrompts(ctx, "pull requests", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	found := false
	for _, r := range results {
		if r.Name == "code-review" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected code-review in results")
	}
}

func TestSearchByAuthor(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	results, err := d.SearchPrompts(ctx, "alice", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	for _, r := range results {
		if r.Author != "alice" {
			t.Errorf("got author %q in results, expected only alice", r.Author)
		}
	}
}

func TestSearchNoResults(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	results, err := d.SearchPrompts(ctx, "zzzznonexistent", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchRanking(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	// "deploy" appears in alice's prompt name AND bob's prompt description.
	// The name match (alice/deploy) should rank higher than the description match (bob/helper).
	results, err := d.SearchPrompts(ctx, "deploy", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Name != "deploy" {
		t.Errorf("first result: got name %q, want %q (name match should rank higher)", results[0].Name, "deploy")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	results, err := d.SearchPrompts(ctx, "", 20, 0)
	if err != nil {
		t.Fatalf("SearchPrompts: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestSearchSpecialCharacters(t *testing.T) {
	d := testDB(t)
	seedSearchData(t, d)
	ctx := context.Background()

	// FTS5 syntax characters should not cause errors
	queries := []string{
		`"code*`,
		`code AND review`,
		`code OR review`,
		`(code`,
		`code"`,
	}
	for _, q := range queries {
		_, err := d.SearchPrompts(ctx, q, 20, 0)
		if err != nil {
			t.Errorf("SearchPrompts(%q) errored: %v", q, err)
		}
	}
}
