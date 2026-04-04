package web

import "net/http"

// HandleHome renders the landing page with search bar.
func (s *Server) HandleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home.html", http.StatusOK, nil)
}
