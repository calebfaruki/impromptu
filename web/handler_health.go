package web

import (
	"encoding/json"
	"net/http"
)

// HandleHealthz returns the health status of the service.
func (s *Server) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
