package web

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HandleAuthor renders an author's profile page.
func (s *Server) HandleAuthor(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "author")

	author, err := s.db.FindAuthor(r.Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.HandleNotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	prompts, err := s.db.ListPromptsByAuthor(r.Context(), author.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "author.html", http.StatusOK, map[string]any{
		"Author":  author,
		"Prompts": prompts,
	})
}
