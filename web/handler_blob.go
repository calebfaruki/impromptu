package web

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/registry"
)

// HandleBlobDownload serves a blob by digest.
func (s *Server) HandleBlobDownload(w http.ResponseWriter, r *http.Request) {
	digestStr := chi.URLParam(r, "digest")
	digest := oci.Digest(digestStr)
	if err := digest.Validate(); err != nil {
		http.Error(w, "invalid digest", http.StatusBadRequest)
		return
	}

	data, err := s.blobs.Get(r.Context(), digest)
	if err != nil {
		if errors.Is(err, registry.ErrNotFound) {
			http.Error(w, "blob not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
