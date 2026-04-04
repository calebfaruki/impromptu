package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	migrations := os.DirFS(filepath.Join("..", "..", "."))
	if err := Migrate(context.Background(), d, migrations); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOpen(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer d.Close()

	// Verify WAL mode is enabled
	var mode string
	err = d.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if mode != "wal" && mode != "memory" {
		// in-memory databases report "memory" instead of "wal"
		t.Errorf("got journal_mode %q, want wal or memory", mode)
	}

	// Verify foreign keys are enabled
	var fk int
	err = d.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("querying foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("got foreign_keys %d, want 1", fk)
	}
}

func TestMigrate(t *testing.T) {
	d := testDB(t)

	// Verify schema_migrations recorded version 1
	var version int
	err := d.db.QueryRow("SELECT version FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if version != 1 {
		t.Errorf("got version %d, want 1", version)
	}

	// Verify tables exist by running SELECT on each
	tables := []string{"authors", "prompts", "versions"}
	for _, table := range tables {
		_, err := d.db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	migrations := os.DirFS(filepath.Join("..", "..", "."))

	if err := Migrate(context.Background(), d, migrations); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(context.Background(), d, migrations); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("got %d migration records, want 1", count)
	}
}
