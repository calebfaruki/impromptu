package web

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// HandlePromptIndex renders the prompt index detail page for a given source URL.
func (s *Server) HandlePromptIndex(w http.ResponseWriter, r *http.Request) {
	rawURL, err := url.PathUnescape(chi.URLParam(r, "url"))
	if err != nil {
		s.HandleNotFound(w, r)
		return
	}

	entries, err := s.idx.FindBySourceURL(r.Context(), rawURL)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(entries) == 0 {
		s.HandleNotFound(w, r)
		return
	}

	s.render(w, r, "prompt_index.html", http.StatusOK, map[string]any{
		"SourceURL": rawURL,
		"Entries":   entries,
		"Latest":    entries[0],
	})
}
