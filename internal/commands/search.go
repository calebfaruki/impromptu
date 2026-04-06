package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SearchResult holds a search result from the index API.
type SearchResult struct {
	SourceURL      string `json:"source_url"`
	SignerIdentity string `json:"signer_identity"`
	Digest         string `json:"digest"`
}

// Search queries the index API for prompts matching the given query.
func Search(ctx context.Context, indexURL, query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/api/search?q=%s", indexURL, url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index returned %d", resp.StatusCode)
	}

	var data struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}

	return data.Results, nil
}
