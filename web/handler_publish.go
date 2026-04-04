package web

import "net/http"

// HandlePublishForm renders the upload form.
func (s *Server) HandlePublishForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "publish.html", http.StatusOK, nil)
}

// HandlePublish processes a publish upload. Stub for Phase 7.
func (s *Server) HandlePublish(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
