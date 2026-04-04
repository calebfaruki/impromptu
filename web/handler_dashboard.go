package web

import (
	"net/http"

	"github.com/calebfaruki/impromptu/internal/auth"
)

// HandleDashboard renders the authenticated author's prompt list.
func (s *Server) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.AuthorFromContext(r.Context())

	author, err := s.db.FindAuthor(r.Context(), user.Username)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	prompts, err := s.db.ListPromptsByAuthor(r.Context(), author.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "dashboard.html", http.StatusOK, map[string]any{
		"Author":  author,
		"Prompts": prompts,
	})
}

// HandleDashboardSettings renders the account settings page.
func (s *Server) HandleDashboardSettings(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "settings.html", http.StatusOK, nil)
}
