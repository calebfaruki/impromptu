package authprobe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Visibility indicates whether a source is publicly accessible.
type Visibility string

const (
	Public  Visibility = "public"
	Private Visibility = "private"
)

// Probe checks whether a source URL points to a public or private resource.
// Returns Private as the safe default for unknown hosts or errors.
func Probe(ctx context.Context, sourceURL string) (Visibility, error) {
	return ProbeWithClient(ctx, sourceURL, &http.Client{Timeout: 10 * time.Second})
}

// ProbeWithClient allows injecting a custom HTTP client (for testing).
func ProbeWithClient(ctx context.Context, sourceURL string, client *http.Client) (Visibility, error) {
	host, owner, repo := ParseSourceURL(sourceURL)

	switch host {
	case "github.com":
		return probeHTTP(ctx, client, fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo))
	case "codeberg.org":
		return probeHTTP(ctx, client, fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s", owner, repo))
	case "ghcr.io":
		return probeOCI(ctx, client, fmt.Sprintf("https://ghcr.io/v2/%s/%s/manifests/latest", owner, repo))
	default:
		return Private, nil
	}
}

func probeHTTP(ctx context.Context, client *http.Client, url string) (Visibility, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Private, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Private, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return Public, nil
	}
	return Private, nil
}

func probeOCI(ctx context.Context, client *http.Client, url string) (Visibility, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return Private, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Private, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return Public, nil
	}
	return Private, nil
}

// ParseSourceURL extracts host, owner, and repo from a source URL.
// Handles: https://github.com/owner/repo, git@github.com:owner/repo.git,
// ghcr.io/owner/image
func ParseSourceURL(sourceURL string) (host, owner, repo string) {
	// Strip scheme
	url := sourceURL
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Handle SSH format: git@host:owner/repo.git
	if strings.HasPrefix(url, "git@") {
		url = strings.TrimPrefix(url, "git@")
		colonIdx := strings.Index(url, ":")
		if colonIdx >= 0 {
			host = url[:colonIdx]
			path := url[colonIdx+1:]
			path = strings.TrimSuffix(path, ".git")
			parts := strings.SplitN(path, "/", 2)
			if len(parts) == 2 {
				return host, parts[0], parts[1]
			}
		}
		return "", "", ""
	}

	// Handle HTTPS format: host/owner/repo
	parts := strings.SplitN(url, "/", 3)
	if len(parts) < 3 {
		return "", "", ""
	}
	host = parts[0]
	owner = parts[1]
	repo = strings.TrimSuffix(parts[2], ".git")
	// Strip tag/version suffixes
	if atIdx := strings.Index(repo, "@"); atIdx >= 0 {
		repo = repo[:atIdx]
	}
	if colonIdx := strings.Index(repo, ":"); colonIdx >= 0 {
		repo = repo[:colonIdx]
	}
	return host, owner, repo
}
