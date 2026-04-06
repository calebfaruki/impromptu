package authprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestParseGHCR(t *testing.T) {
	host, owner, repo := ParseSourceURL("ghcr.io/alice/reviewer")
	if host != "ghcr.io" || owner != "alice" || repo != "reviewer" {
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

func TestProbeOCIPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	vis, _ := probeOCI(context.Background(), srv.Client(), srv.URL)
	if vis != Public {
		t.Errorf("got %q, want public", vis)
	}
}

func TestProbeOCIPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	vis, _ := probeOCI(context.Background(), srv.Client(), srv.URL)
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
