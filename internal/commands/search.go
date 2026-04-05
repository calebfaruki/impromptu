package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SearchResult holds a search result from the registry API.
type SearchResult struct {
	Name        string `json:"name"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

// Search queries the registry API for prompts matching the given query.
func Search(ctx context.Context, registryURL, query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/api/v1/search?q=%s", registryURL, url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}

	var data struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}

	return data.Results, nil
}
