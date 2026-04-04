package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/calebfaruki/impromptu/internal/auth"
	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/oci"
)

// HandlePublishForm renders the upload form.
func (s *Server) HandlePublishForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "publish.html", http.StatusOK, nil)
}

// HandlePublish processes a publish upload through the full pipeline:
// zip extract -> content check -> package -> digest -> store -> sign -> index -> redirect.
func (s *Server) HandlePublish(w http.ResponseWriter, r *http.Request) {
	user := auth.AuthorFromContext(r.Context())
	ctx := r.Context()

	name := r.FormValue("name")
	description := r.FormValue("description")
	version := r.FormValue("version")
	if name == "" || version == "" {
		s.publishError(w, r, http.StatusBadRequest, "name and version are required")
		return
	}

	file, _, err := r.FormFile("archive")
	if err != nil {
		s.publishError(w, r, http.StatusBadRequest, "missing archive file")
		return
	}
	defer file.Close()

	zipData, err := io.ReadAll(file)
	if err != nil {
		s.publishError(w, r, http.StatusBadRequest, "reading archive: "+err.Error())
		return
	}

	// Extract zip to temp dir
	tempDir, err := os.MkdirTemp("", "impromptu-publish-*")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	if err := extractZip(zipData, tempDir); err != nil {
		s.publishError(w, r, http.StatusBadRequest, "extracting archive: "+err.Error())
		return
	}

	// Content check
	violations, err := contentcheck.CheckDirectory(tempDir)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Error())
		}
		s.publishError(w, r, http.StatusBadRequest, strings.Join(msgs, "\n"))
		return
	}

	// Package as tar
	tarData, err := oci.PackageBytes(tempDir)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	digest := oci.ComputeDigest(tarData)

	// Find or create prompt
	author, err := s.db.FindAuthor(ctx, user.Username)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	promptID, err := s.db.InsertPrompt(ctx, author.ID, name, description)
	if err != nil {
		// Prompt may already exist -- look it up
		existing, findErr := s.db.FindPromptByAuthorName(ctx, author.ID, name)
		if findErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		promptID = existing.ID
	}

	// Insert version (fails fast if duplicate)
	versionID, err := s.db.InsertVersion(ctx, promptID, version, digest.String())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			s.publishError(w, r, http.StatusConflict, fmt.Sprintf("version %s already exists", version))
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Upload blob
	if err := s.blobs.Put(ctx, digest, tarData); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Sign
	identity := "github.com/" + user.Username
	bundle, err := s.artSigner.Sign(ctx, digest.String(), identity)
	if err == nil {
		s.db.SetVersionSignature(ctx, versionID, string(bundle.BundleJSON), bundle.RekorLogIndex)
	}

	http.Redirect(w, r, "/"+user.Username+"/"+name, http.StatusSeeOther)
}

func (s *Server) publishError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.render(w, r, "publish.html", status, map[string]any{
		"Error": msg,
	})
}

func extractZip(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Prevent directory traversal
		name := filepath.Base(f.Name)
		if name == "." || name == ".." {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Name, err)
		}

		if err := os.WriteFile(filepath.Join(destDir, name), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}
