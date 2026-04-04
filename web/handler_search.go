package web

import (
	"encoding/json"
	"net/http"

	"github.com/calebfaruki/impromptu/internal/index"
)

// HandleSearch renders the HTML search results page.
func (s *Server) HandleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, _ := s.db.SearchPrompts(r.Context(), q, 20, 0)
	s.render(w, r, "search.html", http.StatusOK, map[string]any{
		"Query":   q,
		"Results": results,
	})
}

// HandleSearchAPI returns search results as JSON.
func (s *Server) HandleSearchAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, err := s.db.SearchPrompts(r.Context(), q, 20, 0)
	if err != nil {
		http.Error(w, "search error", http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = make([]index.SearchResult, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}
