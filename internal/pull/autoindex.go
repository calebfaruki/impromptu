package pull

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/calebfaruki/impromptu/internal/authprobe"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

var allowlistedHosts = map[string]bool{
	"github.com":   true,
	"codeberg.org": true,
	"ghcr.io":      true,
}

// MaybeIndex submits metadata to the index if the source is signed and public.
// Uses Searcher to discover whether a Rekor entry exists for the digest.
// Returns warnings (never errors). Pull always succeeds regardless of indexing outcome.
func MaybeIndex(ctx context.Context, indexURL string, sourceURL string, digest string, searcher sigstore.Searcher) []string {
	if indexURL == "" {
		return nil
	}

	host, _, _ := authprobe.ParseSourceURL(sourceURL)
	if !allowlistedHosts[host] {
		return nil
	}

	entry, err := searcher.Search(ctx, digest)
	if err != nil {
		return []string{fmt.Sprintf("%s: unsigned, not indexed", sourceURL)}
	}

	body, _ := json.Marshal(map[string]any{
		"source_url":      sourceURL,
		"digest":          digest,
		"rekor_log_index": entry.LogIndex,
		"signer_identity": entry.SignerIdentity,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", indexURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		return []string{fmt.Sprintf("index submission failed: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return []string{fmt.Sprintf("index submission failed (server unreachable): %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return []string{fmt.Sprintf("index submission rejected: HTTP %d", resp.StatusCode)}
	}

	return nil
}
