package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebfaruki/impromptu/internal/auth"
	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/registry"
)

func testServer(t *testing.T) (*Server, *index.DB, *registry.MemoryStore) {
	t.Helper()
	db, err := index.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	migrations := os.DirFS(filepath.Join("..", "."))
	if err := index.Migrate(context.Background(), db, migrations); err != nil {
		t.Fatal(err)
	}

	blobs := registry.NewMemoryStore()
	sessions := auth.NewSessionStore(db.RawDB())
	signer := auth.NewCookieSigner([]byte("test-key-32-bytes-long-for-hmac!"))

	ah := &auth.Handlers{
		Provider: &auth.FakeProvider{
			User: auth.GitHubUser{Username: "testuser", Name: "Test User"},
			URL:  "https://github.com/login/oauth/authorize",
		},
		Sessions:    sessions,
		Signer:      signer,
		CookieName:  "session",
		StateCookie: "oauth_state",
		Secure:      false,
		EnsureAuthor: func(ctx context.Context, user auth.GitHubUser) (int64, error) {
			return db.InsertAuthor(ctx, user.Username, user.Name, user.AvatarURL, user.ProfileURL)
		},
	}

	srv := NewServer(db, blobs, ah, sessions, signer, "session")
	return srv, db, blobs
}

func seedData(t *testing.T, db *index.DB, blobs *registry.MemoryStore) {
	t.Helper()
	ctx := context.Background()

	aliceID, err := db.InsertAuthor(ctx, "alice", "Alice Smith", "", "https://github.com/alice")
	if err != nil {
		t.Fatal(err)
	}
	promptID, err := db.InsertPrompt(ctx, aliceID, "code-review", "A prompt for reviewing pull requests")
	if err != nil {
		t.Fatal(err)
	}

	tarData, err := oci.PackageBytes(filepath.Join("..", "testdata", "valid", "simple"))
	if err != nil {
		t.Fatal(err)
	}
	digest := oci.ComputeDigest(tarData)
	if err := blobs.Put(ctx, digest, tarData); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertVersion(ctx, promptID, "1.0.0", digest.String()); err != nil {
		t.Fatal(err)
	}
}

func authenticatedRequest(t *testing.T, srv *Server, db *index.DB, method, path string) *http.Request {
	t.Helper()
	ctx := context.Background()

	authorID, err := db.InsertAuthor(ctx, "authuser", "Auth User", "", "")
	if err != nil {
		// Author may already exist
		a, _ := db.FindAuthor(ctx, "authuser")
		authorID = a.ID
	}

	session, err := srv.sessions.Create(ctx, authorID)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: srv.signer.Sign(session.Token),
	})
	return req
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// --- Route tests ---

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
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/search?q=code-review")
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
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/api/v1/search?q=code-review")
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

func TestAuthorPageReturns200(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestAuthorPageRendersPrompts(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice")
	if !strings.Contains(rec.Body.String(), "code-review") {
		t.Error("author page should list prompts")
	}
}

func TestAuthorNotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/nonexistent-author")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestPromptPageReturns200(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/code-review")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestPromptPageShowsVersion(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/code-review")
	if !strings.Contains(rec.Body.String(), "1.0.0") {
		t.Error("prompt page should show version 1.0.0")
	}
}

func TestPromptPageShowsContent(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/code-review")
	body := rec.Body.String()
	if !strings.Contains(body, "code review") {
		t.Error("prompt page should show markdown content preview")
	}
}

func TestPromptNotFound(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestPromptVersionsPage(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/code-review/versions")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1.0.0") {
		t.Error("versions page should list 1.0.0")
	}
}

func TestPromptSpecificVersion(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/alice/code-review/v/1.0.0")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestDashboardWithoutAuthRedirects(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/dashboard/prompts")
	if rec.Code != http.StatusFound {
		t.Errorf("got %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("got redirect %q, want /login", loc)
	}
}

func TestDashboardWithAuthReturns200(t *testing.T) {
	srv, db, _ := testServer(t)
	req := authenticatedRequest(t, srv, db, "GET", "/dashboard/prompts")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestPublishFormWithoutAuthRedirects(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/publish")
	if rec.Code != http.StatusFound {
		t.Errorf("got %d, want 302", rec.Code)
	}
}

func TestPublishFormWithAuthReturns200(t *testing.T) {
	srv, db, _ := testServer(t)
	req := authenticatedRequest(t, srv, db, "GET", "/publish")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestBlobDownloadReturns200(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)

	// Get the digest from the version
	ctx := context.Background()
	author, _ := db.FindAuthor(ctx, "alice")
	prompt, _ := db.FindPromptByAuthorName(ctx, author.ID, "code-review")
	version, _ := db.LatestVersion(ctx, prompt.ID)

	rec := get(t, srv.Routes(), "/api/v1/blobs/"+version.Digest)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("blob download returned empty body")
	}
}

func TestBlobDownloadNotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/api/v1/blobs/sha256:0000000000000000000000000000000000000000000000000000000000000000")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- HTML quality ---

func TestAllPagesHaveOneH1(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)

	pages := []string{
		"/",
		"/search?q=test",
		"/alice",
		"/alice/code-review",
		"/alice/code-review/versions",
		"/alice/code-review/v/1.0.0",
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
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)

	pages := []string{
		"/",
		"/search?q=test",
		"/alice",
		"/alice/code-review",
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
