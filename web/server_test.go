package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/calebfaruki/impromptu/internal/indexdb"
	"github.com/calebfaruki/impromptu/internal/sigstore"
	_ "modernc.org/sqlite"
)

func testServer(t *testing.T) (*Server, *indexdb.DB, *sigstore.FakeVerifier) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.Exec("PRAGMA foreign_keys=ON")

	rootFS := os.DirFS("..")
	entries, err := fs.ReadDir(rootFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		content, err := fs.ReadFile(rootFS, "migrations/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", e.Name(), err)
		}
	}

	idx := indexdb.New(db)
	verifier := sigstore.NewFakeVerifier()
	srv := NewServer(idx, verifier)
	return srv, idx, verifier
}

// publicProbeClient returns an HTTP client that always responds 200 OK,
// making authprobe treat any github.com URL as public.
func publicProbeClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusOK)
			rec.Write([]byte(`{}`))
			return rec.Result(), nil
		}),
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func seedIndex(t *testing.T, idx *indexdb.DB) {
	t.Helper()
	ctx := context.Background()
	if err := idx.InsertIndexEntry(ctx,
		"https://github.com/alice/code-review",
		"sha256:abc123",
		"alice@github.com",
		42,
	); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// --- Route tests ---

func TestHealthCheckReturns200(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/healthz")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "healthy") {
		t.Error("expected healthy status in response")
	}
}

func TestNotFoundReturns404(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/nonexistent/path/here")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestStaticCSS(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/static/style.css")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "css") {
		t.Errorf("got content-type %q, want css", rec.Header().Get("Content-Type"))
	}
}

func TestHomePageReturns200(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestHomePageContainsSearchForm(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/")
	body := rec.Body.String()
	if !strings.Contains(body, "<form") {
		t.Error("home page missing <form")
	}
	if !strings.Contains(body, `name="q"`) {
		t.Error("home page missing search input")
	}
}

func TestHomePageHasOneH1(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/")
	body := rec.Body.String()
	count := strings.Count(body, "<h1")
	if count != 1 {
		t.Errorf("got %d <h1> tags, want 1", count)
	}
}

func TestSearchReturns200(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/search?q=test")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestSearchRendersResults(t *testing.T) {
	srv, idx, _ := testServer(t)
	seedIndex(t, idx)
	rec := get(t, srv.Routes(), "/search?q=alice")
	body := rec.Body.String()
	if !strings.Contains(body, "code-review") {
		t.Error("search results should contain 'code-review'")
	}
}

func TestSearchEmptyState(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/search?q=zzzznonexistent")
	body := rec.Body.String()
	if !strings.Contains(body, "No results") {
		t.Error("expected empty state message")
	}
}

func TestSearchAPIReturnsJSON(t *testing.T) {
	srv, idx, _ := testServer(t)
	seedIndex(t, idx)
	rec := get(t, srv.Routes(), "/api/search?q=alice")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("got content-type %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), "code-review") {
		t.Error("JSON response should contain 'code-review'")
	}
}

// --- Index API tests ---

func TestIndexAPIValid(t *testing.T) {
	srv, _, verifier := testServer(t)
	srv.probeClient = publicProbeClient(t)

	verifier.AddEntry(sigstore.RekorEntry{
		LogIndex:       100,
		Digest:         "sha256:validdigest",
		SignerIdentity: "user@github.com",
	})

	rec := postJSON(t, srv.Routes(), "/api/index", map[string]any{
		"source_url":      "https://github.com/alice/prompts",
		"digest":          "sha256:validdigest",
		"rekor_log_index": 100,
	})

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "user@github.com") {
		t.Error("response should contain signer identity")
	}
}

func TestIndexAPIPrivateRejected(t *testing.T) {
	srv, _, verifier := testServer(t)

	verifier.AddEntry(sigstore.RekorEntry{
		LogIndex:       100,
		Digest:         "sha256:validdigest",
		SignerIdentity: "user@github.com",
	})

	// authprobe returns Private for unknown hosts
	rec := postJSON(t, srv.Routes(), "/api/index", map[string]any{
		"source_url":      "https://private.example.com/owner/repo",
		"digest":          "sha256:validdigest",
		"rekor_log_index": 100,
	})

	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestIndexAPIInvalidRekor(t *testing.T) {
	srv, _, _ := testServer(t)
	srv.probeClient = publicProbeClient(t)

	// No entries in the fake verifier, so verification will fail
	rec := postJSON(t, srv.Routes(), "/api/index", map[string]any{
		"source_url":      "https://github.com/alice/prompts",
		"digest":          "sha256:baddigest",
		"rekor_log_index": 999,
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestIndexAPIDuplicateIdempotent(t *testing.T) {
	srv, _, verifier := testServer(t)
	srv.probeClient = publicProbeClient(t)

	verifier.AddEntry(sigstore.RekorEntry{
		LogIndex:       100,
		Digest:         "sha256:validdigest",
		SignerIdentity: "user@github.com",
	})

	body := map[string]any{
		"source_url":      "https://github.com/alice/prompts",
		"digest":          "sha256:validdigest",
		"rekor_log_index": 100,
	}

	rec1 := postJSON(t, srv.Routes(), "/api/index", body)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first index: got %d, want 200", rec1.Code)
	}

	rec2 := postJSON(t, srv.Routes(), "/api/index", body)
	if rec2.Code != http.StatusOK {
		t.Errorf("duplicate index: got %d, want 200; body: %s", rec2.Code, rec2.Body.String())
	}
}

// --- Old routes return 404 ---

func TestOldPublishReturns404(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/publish")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestOldLoginReturns404(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/login")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- HTML quality ---

func TestAllPagesHaveOneH1(t *testing.T) {
	srv, _, _ := testServer(t)

	pages := []string{
		"/",
		"/search?q=test",
	}

	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			rec := get(t, srv.Routes(), path)
			if rec.Code != http.StatusOK {
				t.Skipf("page returned %d", rec.Code)
			}
			body := rec.Body.String()
			count := strings.Count(body, "<h1")
			if count != 1 {
				t.Errorf("got %d <h1> tags, want 1", count)
			}
		})
	}
}

func TestAllPagesHaveSemanticMarkup(t *testing.T) {
	srv, _, _ := testServer(t)

	pages := []string{
		"/",
		"/search?q=test",
	}

	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			rec := get(t, srv.Routes(), path)
			if rec.Code != http.StatusOK {
				t.Skipf("page returned %d", rec.Code)
			}
			body := rec.Body.String()
			for _, tag := range []string{"<nav", "<main", "<footer"} {
				if !strings.Contains(body, tag) {
					t.Errorf("missing semantic tag %s", tag)
				}
			}
		})
	}
}
