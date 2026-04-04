package contentcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFile(t *testing.T) {
	tmp := t.TempDir()

	// Valid UTF-8 markdown
	validPath := filepath.Join(tmp, "valid.md")
	if err := os.WriteFile(validPath, []byte("# Hello\n\nClean content.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Binary file (non-UTF-8)
	binaryPath := filepath.Join(tmp, "binary.md")
	binaryData := make([]byte, 256)
	for i := range binaryData {
		binaryData[i] = byte(i)
	}
	if err := os.WriteFile(binaryPath, binaryData, 0644); err != nil {
		t.Fatal(err)
	}

	// File with unicode violation
	unicodePath := filepath.Join(tmp, "unicode.md")
	if err := os.WriteFile(unicodePath, []byte("hello\u200Bworld\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// File with HTML violation
	htmlPath := filepath.Join(tmp, "html.md")
	if err := os.WriteFile(htmlPath, []byte("<div>bad</div>\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		path     string
		relPath  string
		wantErr  bool
		wantKind Kind
		wantN    int
	}{
		{"valid file", validPath, "valid.md", false, "", 0},
		{"binary file", binaryPath, "binary.md", true, KindBinary, 1},
		{"unicode violation", unicodePath, "unicode.md", true, KindUnicode, 1},
		{"html violation", htmlPath, "html.md", true, KindHTML, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := CheckFile(tt.path, tt.relPath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantN == 0 && len(violations) > 0 {
				t.Errorf("unexpected violations: %v", violations)
			}
			if tt.wantN > 0 && len(violations) != tt.wantN {
				t.Errorf("got %d violations, want %d", len(violations), tt.wantN)
			}
			if tt.wantErr && len(violations) > 0 && violations[0].Kind != tt.wantKind {
				t.Errorf("got kind %s, want %s", violations[0].Kind, tt.wantKind)
			}
		})
	}
}

func TestCheckFileNotFound(t *testing.T) {
	_, err := CheckFile("/nonexistent/path.md", "path.md")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}
