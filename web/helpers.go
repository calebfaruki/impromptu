package web

import (
	"net/http"

	"github.com/calebfaruki/impromptu/internal/auth"
)

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, status int, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["User"] = auth.AuthorFromContext(r.Context())

	tmpl, ok := s.pages[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// HandleNotFound renders the 404 page.
func (s *Server) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "404.html", http.StatusNotFound, nil)
}
