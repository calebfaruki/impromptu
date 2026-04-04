package web

import (
	"encoding/json"
	"net/http"
)

// HandleHealthz returns the health status of the service.
func (s *Server) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dbErr := s.db.Ping(ctx)

	status := "healthy"
	httpStatus := http.StatusOK
	checks := map[string]string{
		"database": "ok",
	}

	if dbErr != nil {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
		checks["database"] = dbErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"checks": checks,
	})
}
