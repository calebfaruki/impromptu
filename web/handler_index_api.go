package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/calebfaruki/impromptu/internal/authprobe"
)

type indexRequest struct {
	SourceURL      string `json:"source_url"`
	Digest         string `json:"digest"`
	RekorLogIndex  int64  `json:"rekor_log_index"`
	SignerIdentity string `json:"signer_identity"`
}

// HandleIndexAPI accepts POST /api/index with JSON body and indexes a prompt.
// If signer_identity is provided (release mode), uses it directly.
// Otherwise falls back to Rekor verification by log index (server-side).
func (s *Server) HandleIndexAPI(w http.ResponseWriter, r *http.Request) {
	var req indexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.SourceURL == "" || req.Digest == "" {
		writeJSONError(w, http.StatusBadRequest, "source_url and digest are required")
		return
	}

	ctx := r.Context()

	var vis authprobe.Visibility
	var err error
	if s.probeClient != nil {
		vis, err = authprobe.ProbeWithClient(ctx, req.SourceURL, s.probeClient)
	} else {
		vis, err = authprobe.Probe(ctx, req.SourceURL)
	}
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "auth probe failed: "+err.Error())
		return
	}
	if vis == authprobe.Private {
		writeJSONError(w, http.StatusForbidden, "source is private or inaccessible")
		return
	}

	signer := req.SignerIdentity
	if signer == "" && req.RekorLogIndex > 0 {
		entry, err := s.verifier.Verify(ctx, req.RekorLogIndex, req.Digest)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("rekor verification failed: %v", err))
			return
		}
		signer = entry.SignerIdentity
	}

	if signer == "" {
		writeJSONError(w, http.StatusBadRequest, "signer_identity required (or provide rekor_log_index for server-side verification)")
		return
	}

	if err := s.idx.InsertIndexEntry(ctx, req.SourceURL, req.Digest, signer, req.RekorLogIndex); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "indexing failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"source_url":      req.SourceURL,
		"digest":          req.Digest,
		"signer_identity": signer,
		"rekor_log_index": req.RekorLogIndex,
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
