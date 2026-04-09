package pull

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/calebfaruki/impromptu/internal/authprobe"
)

var allowlistedHosts = map[string]bool{
	"github.com":   true,
	"codeberg.org": true,
}

// SubmitToIndex submits verified release metadata to the index server.
// Only called for release mode deps with a verified signer.
// Returns warnings (never errors). Pull always succeeds regardless of indexing outcome.
func SubmitToIndex(ctx context.Context, indexURL, sourceURL, digest, signer string, logIndex int64) []string {
	if indexURL == "" || signer == "" {
		return nil
	}

	host, _, _ := authprobe.ParseSourceURL(sourceURL)
	if !allowlistedHosts[host] {
		return nil
	}

	body, _ := json.Marshal(map[string]any{
		"source_url":      sourceURL,
		"digest":          digest,
		"rekor_log_index": logIndex,
		"signer_identity": signer,
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
