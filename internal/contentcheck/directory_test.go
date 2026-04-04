package contentcheck

import (
	"os"
	"path/filepath"
	"testing"
)

// setupProgrammaticFixtures creates test fixtures that cannot be stored in git:
// zero-width characters, RTL overrides, symlinks, binary files.
func setupProgrammaticFixtures(t *testing.T, root string) {
	t.Helper()

	// zero-width: file with U+200B
	zwDir := filepath.Join(root, "zero-width")
	if err := os.MkdirAll(zwDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(zwDir, "01-context.md"),
		[]byte("# Innocent Looking Prompt\n\nThis line contains a zero-width space between these two\u200B words.\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	// rtl-override: file with U+202E
	rtlDir := filepath.Join(root, "rtl-override")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(rtlDir, "01-context.md"),
		[]byte("# Another Innocent Prompt\n\nThis line contains an RTL override\u202E that could hide text direction.\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	// symlink: directory with a symlink
	symDir := filepath.Join(root, "symlink")
	if err := os.MkdirAll(symDir, 0755); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(symDir, "01-context.md")
	if err := os.WriteFile(realFile, []byte("# Real File\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, filepath.Join(symDir, "link.md")); err != nil {
		t.Fatal(err)
	}

	// binary: file with non-UTF-8 bytes
	binDir := filepath.Join(root, "binary")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	binaryData := make([]byte, 256)
	for i := range binaryData {
		binaryData[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(binDir, "01-context.md"), binaryData, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDirectory(t *testing.T) {
	// Create programmatic fixtures in temp dir
	tmpDir := t.TempDir()
	setupProgrammaticFixtures(t, tmpDir)

	// Static fixtures live at repo root testdata/
	repoTestdata := filepath.Join("..", "..", "testdata")

	tests := []struct {
		name     string
		dir      string
		wantErr  bool
		wantKind Kind
	}{
		{
			name: "valid simple",
			dir:  filepath.Join(repoTestdata, "valid", "simple"),
		},
		{
			name: "valid frontmatter",
			dir:  filepath.Join(repoTestdata, "valid", "with-frontmatter"),
		},
		{
			name: "valid code blocks",
			dir:  filepath.Join(repoTestdata, "valid", "with-code-blocks"),
		},
		{
			name:     "reject zero-width",
			dir:      filepath.Join(tmpDir, "zero-width"),
			wantErr:  true,
			wantKind: KindUnicode,
		},
		{
			name:     "reject rtl override",
			dir:      filepath.Join(tmpDir, "rtl-override"),
			wantErr:  true,
			wantKind: KindUnicode,
		},
		{
			name:     "reject raw html",
			dir:      filepath.Join(repoTestdata, "invalid", "raw-html"),
			wantErr:  true,
			wantKind: KindHTML,
		},
		{
			name:     "reject mixed files",
			dir:      filepath.Join(repoTestdata, "invalid", "mixed-files"),
			wantErr:  true,
			wantKind: KindFiletype,
		},
		{
			name:     "reject symlink",
			dir:      filepath.Join(tmpDir, "symlink"),
			wantErr:  true,
			wantKind: KindSymlink,
		},
		{
			name:     "reject empty",
			dir:      filepath.Join(repoTestdata, "invalid", "empty"),
			wantErr:  true,
			wantKind: KindEmpty,
		},
		{
			name:     "reject binary",
			dir:      filepath.Join(tmpDir, "binary"),
			wantErr:  true,
			wantKind: KindBinary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := CheckDirectory(tt.dir)
			if err != nil {
				t.Fatalf("unexpected infrastructure error: %v", err)
			}
			if tt.wantErr && len(violations) == 0 {
				t.Errorf("expected violations of kind %s, got none", tt.wantKind)
			}
			if !tt.wantErr && len(violations) > 0 {
				t.Errorf("unexpected violations:")
				for _, v := range violations {
					t.Logf("  %s", v.Error())
				}
			}
			if tt.wantErr && len(violations) > 0 {
				found := false
				for _, v := range violations {
					if v.Kind == tt.wantKind {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation kind %s, got:", tt.wantKind)
					for _, v := range violations {
						t.Logf("  %s", v.Error())
					}
				}
			}
		})
	}
}

func TestCheckDirectoryNotFound(t *testing.T) {
	_, err := CheckDirectory("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}
