package authprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseGitHubHTTPS(t *testing.T) {
	host, owner, repo := ParseSourceURL("https://github.com/alice/coder")
	if host != "github.com" || owner != "alice" || repo != "coder" {
		t.Errorf("got %s/%s/%s", host, owner, repo)
	}
}

func TestParseGitHubSSH(t *testing.T) {
	host, owner, repo := ParseSourceURL("git@github.com:alice/coder.git")
	if host != "github.com" || owner != "alice" || repo != "coder" {
		t.Errorf("got %s/%s/%s", host, owner, repo)
	}
}

func TestParseCodeberg(t *testing.T) {
	host, owner, repo := ParseSourceURL("https://codeberg.org/alice/prompts")
	if host != "codeberg.org" || owner != "alice" || repo != "prompts" {
		t.Errorf("got %s/%s/%s", host, owner, repo)
	}
}

func TestProbeGitHubPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Override: use the mock server URL parsed as github.com
	vis, err := probeHTTP(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if vis != Public {
		t.Errorf("got %q, want public", vis)
	}
}

func TestProbeGitHubPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	vis, _ := probeHTTP(context.Background(), srv.Client(), srv.URL)
	if vis != Private {
		t.Errorf("got %q, want private", vis)
	}
}

func TestProbeUnknownHost(t *testing.T) {
	vis, _ := ProbeWithClient(context.Background(), "https://unknown.example.com/alice/thing", nil)
	if vis != Private {
		t.Errorf("unknown host should be private, got %q", vis)
	}
}

func TestProbeNonexistentRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	vis, _ := probeHTTP(context.Background(), srv.Client(), srv.URL+"/nonexistent")
	if vis != Private {
		t.Errorf("nonexistent should be private, got %q", vis)
	}
}

func TestProbeHTTPFailureModes(t *testing.T) {
	codes := []struct {
		name string
		code int
	}{
		{"rate_limited", http.StatusTooManyRequests},
		{"server_error", http.StatusInternalServerError},
		{"service_unavailable", http.StatusServiceUnavailable},
	}
	for _, tc := range codes {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			vis, err := probeHTTP(context.Background(), srv.Client(), srv.URL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if vis != Private {
				t.Errorf("HTTP %d should be private, got %q", tc.code, vis)
			}
		})
	}
}

func TestProbeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	vis, err := probeHTTP(context.Background(), client, srv.URL)
	if err != nil {
		t.Fatalf("timeout should not return error, got: %v", err)
	}
	if vis != Private {
		t.Errorf("timeout should be private, got %q", vis)
	}
}
