package web

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/oci"
)

// HandlePrompt renders the prompt detail page with the latest version.
func (s *Server) HandlePrompt(w http.ResponseWriter, r *http.Request) {
	author, prompt, ok := s.lookupAuthorPrompt(w, r)
	if !ok {
		return
	}

	version, err := s.db.LatestVersion(r.Context(), prompt.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.render(w, r, "prompt.html", http.StatusOK, map[string]any{
				"Author": author, "Prompt": prompt,
			})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "prompt.html", http.StatusOK, map[string]any{
		"Author": author, "Prompt": prompt, "Version": version,
		"Files": s.fetchContent(r, version.Digest),
	})
}

// HandlePromptVersions renders the version history page.
func (s *Server) HandlePromptVersions(w http.ResponseWriter, r *http.Request) {
	author, prompt, ok := s.lookupAuthorPrompt(w, r)
	if !ok {
		return
	}

	versions, err := s.db.ListVersions(r.Context(), prompt.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "versions.html", http.StatusOK, map[string]any{
		"Author": author, "Prompt": prompt, "Versions": versions,
	})
}

// HandlePromptVersion renders a specific version detail page.
func (s *Server) HandlePromptVersion(w http.ResponseWriter, r *http.Request) {
	author, prompt, ok := s.lookupAuthorPrompt(w, r)
	if !ok {
		return
	}

	versionStr := chi.URLParam(r, "version")
	version, err := s.db.FindVersion(r.Context(), prompt.ID, versionStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.HandleNotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "version.html", http.StatusOK, map[string]any{
		"Author": author, "Prompt": prompt, "Version": version,
		"Files": s.fetchContent(r, version.Digest),
	})
}

func (s *Server) lookupAuthorPrompt(w http.ResponseWriter, r *http.Request) (index.Author, index.Prompt, bool) {
	username := chi.URLParam(r, "author")
	name := chi.URLParam(r, "name")

	author, err := s.db.FindAuthor(r.Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.HandleNotFound(w, r)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return index.Author{}, index.Prompt{}, false
	}

	prompt, err := s.db.FindPromptByAuthorName(r.Context(), author.ID, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.HandleNotFound(w, r)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return index.Author{}, index.Prompt{}, false
	}

	return author, prompt, true
}

func (s *Server) fetchContent(r *http.Request, digest string) map[string]string {
	if digest == "" {
		return nil
	}
	data, err := s.blobs.Get(r.Context(), oci.Digest(digest))
	if err != nil {
		return nil
	}
	files, err := oci.UnpackageToMap(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return files
}
