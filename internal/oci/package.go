package oci

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Package reads all .md files from dir and writes a deterministic tar archive to w.
// Files are sorted lexicographically. Headers are normalized for reproducibility.
func Package(dir string, w io.Writer) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	tw := tar.NewWriter(w)
	found := false

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("reading file %s: %w", name, err)
		}

		hdr := &tar.Header{
			Name:     name,
			Size:     int64(len(data)),
			Mode:     0644,
			Typeflag: tar.TypeReg,
			ModTime:  time.Time{},
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("writing tar header for %s: %w", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("writing tar data for %s: %w", name, err)
		}
		found = true
	}

	if !found {
		return fmt.Errorf("directory %s contains no .md files", dir)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}
	return nil
}

// PackageBytes is a convenience wrapper that returns the tar as a byte slice.
func PackageBytes(dir string) ([]byte, error) {
	var buf bytes.Buffer
	if err := Package(dir, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unpackage extracts a tar archive from r into dir.
// Only regular files are accepted; anything else is rejected.
func Unpackage(r io.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("unexpected entry type %d for %s: only regular files allowed", hdr.Typeflag, hdr.Name)
		}
		if strings.Contains(filepath.Clean(hdr.Name), "..") {
			return fmt.Errorf("path traversal in tar entry: %s", hdr.Name)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("reading tar data for %s: %w", hdr.Name, err)
		}

		path := filepath.Join(dir, filepath.Base(hdr.Name))
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("writing file %s: %w", hdr.Name, err)
		}
	}
	return nil
}

// UnpackageToMap extracts a tar archive into an in-memory map of filename to content.
func UnpackageToMap(r io.Reader) (map[string]string, error) {
	tr := tar.NewReader(r)
	files := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("unexpected entry type for %s", hdr.Name)
		}
		if strings.Contains(filepath.Clean(hdr.Name), "..") {
			return nil, fmt.Errorf("path traversal in tar entry: %s", hdr.Name)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", hdr.Name, err)
		}
		files[hdr.Name] = string(data)
	}
	return files, nil
}
