package oci

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

var testdataDir = filepath.Join("..", "..", "testdata")

func TestPackageValidTar(t *testing.T) {
	var buf bytes.Buffer
	err := Package(filepath.Join(testdataDir, "valid", "simple"), &buf)
	if err != nil {
		t.Fatalf("Package: %v", err)
	}

	tr := tar.NewReader(&buf)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(names), names)
	}
	if names[0] != "01-context.md" || names[1] != "02-instructions.md" {
		t.Errorf("got entries %v, want [01-context.md 02-instructions.md]", names)
	}
}

func TestDeterministic(t *testing.T) {
	dir := filepath.Join(testdataDir, "valid", "simple")

	var buf1, buf2 bytes.Buffer
	if err := Package(dir, &buf1); err != nil {
		t.Fatal(err)
	}
	if err := Package(dir, &buf2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Error("packaging same directory twice produced different bytes")
	}
}

func TestDeterministicDigest(t *testing.T) {
	dir := filepath.Join(testdataDir, "valid", "simple")

	b1, err := PackageBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := PackageBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	d1 := ComputeDigest(b1)
	d2 := ComputeDigest(b2)
	if d1 != d2 {
		t.Errorf("digests differ: %s vs %s", d1, d2)
	}
}

func TestFileOrdering(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"c.md", "a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := Package(dir, &buf); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&buf)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}
	want := []string{"a.md", "b.md", "c.md"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

func TestHeadersNormalized(t *testing.T) {
	var buf bytes.Buffer
	if err := Package(filepath.Join(testdataDir, "valid", "simple"), &buf); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.ModTime.Unix() != 0 {
			t.Errorf("%s: ModTime is %v, want Unix epoch", hdr.Name, hdr.ModTime)
		}
		if hdr.Uid != 0 {
			t.Errorf("%s: Uid is %d, want 0", hdr.Name, hdr.Uid)
		}
		if hdr.Gid != 0 {
			t.Errorf("%s: Gid is %d, want 0", hdr.Name, hdr.Gid)
		}
		if hdr.Uname != "" {
			t.Errorf("%s: Uname is %q, want empty", hdr.Name, hdr.Uname)
		}
		if hdr.Gname != "" {
			t.Errorf("%s: Gname is %q, want empty", hdr.Name, hdr.Gname)
		}
		if hdr.Mode != 0644 {
			t.Errorf("%s: Mode is %o, want 0644", hdr.Name, hdr.Mode)
		}
	}
}

func TestHiddenFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden.md"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.md"), []byte("visible"), 0644)

	var buf bytes.Buffer
	if err := Package(dir, &buf); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&buf)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 1 || names[0] != "visible.md" {
		t.Errorf("got %v, want [visible.md]", names)
	}
}

func TestEmptyDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0644)

	var buf bytes.Buffer
	err := Package(dir, &buf)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestPackageNonexistentDirectory(t *testing.T) {
	var buf bytes.Buffer
	err := Package("/nonexistent/path", &buf)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestPackageBytesError(t *testing.T) {
	data, err := PackageBytes("/nonexistent/path")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if data != nil {
		t.Errorf("expected nil data on error, got %d bytes", len(data))
	}
}

func TestPackageBytesSuccess(t *testing.T) {
	data, err := PackageBytes(filepath.Join(testdataDir, "valid", "simple"))
	if err != nil {
		t.Fatalf("PackageBytes: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestRoundTrip(t *testing.T) {
	srcDir := filepath.Join(testdataDir, "valid", "simple")

	var buf bytes.Buffer
	if err := Package(srcDir, &buf); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	if err := Unpackage(&buf, dstDir); err != nil {
		t.Fatal(err)
	}

	// Compare files
	srcEntries, _ := os.ReadDir(srcDir)
	dstEntries, _ := os.ReadDir(dstDir)

	var srcFiles, dstFiles []string
	for _, e := range srcEntries {
		if !e.IsDir() {
			srcFiles = append(srcFiles, e.Name())
		}
	}
	for _, e := range dstEntries {
		if !e.IsDir() {
			dstFiles = append(dstFiles, e.Name())
		}
	}

	if len(srcFiles) != len(dstFiles) {
		t.Fatalf("file count: src=%d, dst=%d", len(srcFiles), len(dstFiles))
	}
	for i := range srcFiles {
		if srcFiles[i] != dstFiles[i] {
			t.Errorf("filename mismatch: src=%q, dst=%q", srcFiles[i], dstFiles[i])
		}
		srcData, _ := os.ReadFile(filepath.Join(srcDir, srcFiles[i]))
		dstData, _ := os.ReadFile(filepath.Join(dstDir, dstFiles[i]))
		if !bytes.Equal(srcData, dstData) {
			t.Errorf("content mismatch for %s", srcFiles[i])
		}
	}
}

func TestUnpackageRejectsNonRegular(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})
	tw.Close()

	err := Unpackage(&buf, t.TempDir())
	if err == nil {
		t.Error("expected error for directory entry in tar, got nil")
	}
}
