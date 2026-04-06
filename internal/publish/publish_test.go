package publish

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// --- CollectFiles tests ---

func TestCollectFilesValid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "02-instructions.md"), []byte("# Instructions\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestCollectFilesExcludesPromptfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatalf("CollectFiles: %v", err)
	}
	for _, f := range files {
		if filepath.Base(f) == "Promptfile" {
			t.Error("Promptfile should be excluded")
		}
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestCollectFilesExcludesLockfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), []byte("version = 1\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Base(f) == "Promptfile.lock" {
			t.Error("Promptfile.lock should be excluded")
		}
	}
}

func TestCollectFilesExcludesSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)

	subDir := filepath.Join(dir, "pulled-dep")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "nested.md"), []byte("# Nested\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (subdirs excluded), got %d", len(files))
	}
}

func TestCollectFilesExcludesNonMd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "helper.py"), []byte("print('hi')\n"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 .md file, got %d", len(files))
	}
}

func TestCollectFilesExcludesHidden(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden.md"), []byte("# Hidden\n"), 0644)

	files, err := CollectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (hidden excluded), got %d", len(files))
	}
}

func TestCollectFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := CollectFiles(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

// --- Publish tests ---

func mockPublishServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func TestPublishValid(t *testing.T) {
	srv := mockPublishServer(t)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n\nTest content.\n"), 0644)

	result, err := Publish(context.Background(), PublishConfig{
		Dir:         dir,
		Name:        "test-prompt",
		Description: "A test",
		Version:     "1.0.0",
		RegistryURL: srv.URL,
		Identity:    "github.com/alice",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Digest == "" {
		t.Error("expected non-empty digest")
	}
	if result.Name != "test-prompt" {
		t.Errorf("name: got %q", result.Name)
	}
}

func TestPublishContentCheckFailure(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Prompt\n\n<div>bad html</div>\n"), 0644)

	_, err := Publish(context.Background(), PublishConfig{
		Dir:      dir,
		Name:     "bad",
		Version:  "1.0.0",
		Identity: "github.com/alice",
	})
	if err == nil {
		t.Fatal("expected content check failure")
	}
	if !strings.Contains(err.Error(), "content check") {
		t.Errorf("error should mention content check: %v", err)
	}
}

func TestPublishUnicodeRejection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Prompt\n\nHidden\u200Bchar.\n"), 0644)

	_, err := Publish(context.Background(), PublishConfig{
		Dir:      dir,
		Name:     "bad",
		Version:  "1.0.0",
		Identity: "github.com/alice",
	})
	if err == nil {
		t.Fatal("expected unicode rejection")
	}
}

func TestPublishOnlyPackagesMdFiles(t *testing.T) {
	srv := mockPublishServer(t)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "helper.py"), []byte("print('hi')\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n"), 0644)

	result, err := Publish(context.Background(), PublishConfig{
		Dir:         dir,
		Name:        "test",
		Version:     "1.0.0",
		RegistryURL: srv.URL,
		Identity:    "github.com/alice",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Digest == "" {
		t.Error("expected digest")
	}
}

func TestPublishExcludesSubdirectories(t *testing.T) {
	srv := mockPublishServer(t)
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	subDir := filepath.Join(dir, "pulled-dep")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "nested.md"), []byte("# Should not be included\n"), 0644)

	result, err := Publish(context.Background(), PublishConfig{
		Dir:         dir,
		Name:        "test",
		Version:     "1.0.0",
		RegistryURL: srv.URL,
		Identity:    "github.com/alice",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Digest == "" {
		t.Error("expected digest")
	}
}

func TestPublishEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := Publish(context.Background(), PublishConfig{
		Dir:      dir,
		Name:     "empty",
		Version:  "1.0.0",
		Identity: "github.com/alice",
	})
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestPublishTarballContentsCorrect(t *testing.T) {
	var capturedArchive []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		file, _, err := r.FormFile("archive")
		if err != nil {
			http.Error(w, "no archive", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		capturedArchive = data
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)
	os.WriteFile(filepath.Join(dir, "02-instructions.md"), []byte("# Instructions\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Promptfile"), []byte("version = 1\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Promptfile.lock"), []byte("version = 1\n"), 0644)
	os.WriteFile(filepath.Join(dir, "helper.py"), []byte("print('hi')\n"), 0644)
	subDir := filepath.Join(dir, "pulled-dep")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "nested.md"), []byte("# Nested\n"), 0644)

	_, err := Publish(context.Background(), PublishConfig{
		Dir:         dir,
		Name:        "test",
		Version:     "1.0.0",
		RegistryURL: srv.URL,
		Identity:    "github.com/alice",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(capturedArchive) == 0 {
		t.Fatal("no archive captured by mock server")
	}

	// Unpack and verify only root .md files present
	files, err := oci.UnpackageToMap(bytes.NewReader(capturedArchive))
	if err != nil {
		t.Fatalf("unpacking tarball: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files in tarball, got %d: %v", len(files), fileNames(files))
	}
	if _, ok := files["01-context.md"]; !ok {
		t.Error("missing 01-context.md")
	}
	if _, ok := files["02-instructions.md"]; !ok {
		t.Error("missing 02-instructions.md")
	}
	for name := range files {
		if name == "Promptfile" || name == "Promptfile.lock" || name == "helper.py" || name == "nested.md" {
			t.Errorf("tarball should not contain %s", name)
		}
	}
}

func fileNames(m map[string]string) []string {
	var names []string
	for k := range m {
		names = append(names, k)
	}
	return names
}

func TestPublishRegistryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Context\n"), 0644)

	_, err := Publish(context.Background(), PublishConfig{
		Dir:         dir,
		Name:        "test",
		Version:     "1.0.0",
		RegistryURL: srv.URL,
		Identity:    "github.com/alice",
	})
	if err == nil {
		t.Fatal("expected error for registry failure")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code: %v", err)
	}
}
