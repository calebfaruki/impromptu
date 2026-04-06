package web

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
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
	"github.com/calebfaruki/impromptu/internal/sigstore"
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

	artSigner := &sigstore.FakeSigner{}
	srv := NewServer(db, blobs, artSigner, ah, sessions, signer, "session")
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
		Value: srv.cookieSigner.Sign(session.Token),
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

func TestPromptAPIReturnsJSON(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/api/v1/prompts/alice/code-review")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("got content-type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "code-review") {
		t.Error("response should contain prompt name")
	}
	if !strings.Contains(body, "alice") {
		t.Error("response should contain author")
	}
}

func TestPromptAPINotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/api/v1/prompts/nobody/nothing")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestVersionsAPIReturnsJSON(t *testing.T) {
	srv, db, blobs := testServer(t)
	seedData(t, db, blobs)
	rec := get(t, srv.Routes(), "/api/v1/prompts/alice/code-review/versions")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("got content-type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "1.0.0") {
		t.Error("response should contain version 1.0.0")
	}
	if !strings.Contains(body, "sha256:") {
		t.Error("response should contain digest")
	}
}

func TestVersionsAPINotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	rec := get(t, srv.Routes(), "/api/v1/prompts/nobody/nothing/versions")
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
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

// --- Publish flow tests ---

func createTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func publishRequest(t *testing.T, srv *Server, db *index.DB, name, version string, zipData []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("name", name)
	mw.WriteField("description", "test prompt")
	mw.WriteField("version", version)

	fw, err := mw.CreateFormFile("archive", "prompt.zip")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(zipData)
	mw.Close()

	req := authenticatedRequest(t, srv, db, "POST", "/publish")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Body = io.NopCloser(&body)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func TestPublishValidZip(t *testing.T) {
	srv, db, blobs := testServer(t)
	zipData := createTestZip(t, map[string]string{
		"01-context.md":      "# Context\n\nSome instructions.\n",
		"02-instructions.md": "# Instructions\n\nMore text.\n",
	})

	rec := publishRequest(t, srv, db, "my-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("got %d, want 303; body: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	author, err := db.FindAuthor(ctx, "authuser")
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := db.FindPromptByAuthorName(ctx, author.ID, "my-prompt")
	if err != nil {
		t.Fatalf("prompt not in index: %v", err)
	}

	v, err := db.LatestVersion(ctx, prompt.ID)
	if err != nil {
		t.Fatalf("version not in index: %v", err)
	}
	if v.Version != "1.0.0" {
		t.Errorf("got version %q, want 1.0.0", v.Version)
	}

	exists, err := blobs.Exists(ctx, oci.Digest(v.Digest))
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("blob not in store")
	}
	if v.SignatureBundle == "" {
		t.Error("expected signature bundle to be set")
	}
}

func TestPublishZeroWidthUnicode(t *testing.T) {
	srv, db, _ := testServer(t)
	zipData := createTestZip(t, map[string]string{
		"01-context.md": "# Prompt\n\nHidden\u200Bcharacter.\n",
	})

	rec := publishRequest(t, srv, db, "bad-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unicode") {
		t.Error("error should mention unicode violation")
	}
}

func TestPublishRawHTML(t *testing.T) {
	srv, db, _ := testServer(t)
	zipData := createTestZip(t, map[string]string{
		"01-context.md": "# Prompt\n\n<div>hidden</div>\n",
	})

	rec := publishRequest(t, srv, db, "html-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestPublishNonMdFile(t *testing.T) {
	srv, db, _ := testServer(t)
	zipData := createTestZip(t, map[string]string{
		"01-context.md": "# Valid\n",
		"helper.py":     "print('hello')\n",
	})

	rec := publishRequest(t, srv, db, "mixed-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestPublishEmptyZip(t *testing.T) {
	srv, db, _ := testServer(t)
	zipData := createTestZip(t, map[string]string{})

	rec := publishRequest(t, srv, db, "empty-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestPublishDuplicateVersion(t *testing.T) {
	srv, db, _ := testServer(t)
	zipData := createTestZip(t, map[string]string{
		"01-context.md": "# Context\n\nFirst version.\n",
	})

	rec := publishRequest(t, srv, db, "dup-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("first publish: got %d, want 303", rec.Code)
	}

	rec = publishRequest(t, srv, db, "dup-prompt", "1.0.0", zipData)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate: got %d, want 409", rec.Code)
	}
}

func TestPublishRoundTrip(t *testing.T) {
	srv, db, blobs := testServer(t)

	original := map[string]string{
		"01-context.md": "# Round Trip Test\n\nOriginal content.\n",
	}
	zipData := createTestZip(t, original)

	rec := publishRequest(t, srv, db, "roundtrip", "1.0.0", zipData)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish: got %d, want 303", rec.Code)
	}

	ctx := context.Background()
	author, _ := db.FindAuthor(ctx, "authuser")
	prompt, _ := db.FindPromptByAuthorName(ctx, author.ID, "roundtrip")
	v, _ := db.LatestVersion(ctx, prompt.ID)

	blobData, err := blobs.Get(ctx, oci.Digest(v.Digest))
	if err != nil {
		t.Fatal(err)
	}

	files, err := oci.UnpackageToMap(bytes.NewReader(blobData))
	if err != nil {
		t.Fatal(err)
	}

	for name, want := range original {
		got, ok := files[name]
		if !ok {
			t.Errorf("missing file %s in downloaded blob", name)
			continue
		}
		if got != want {
			t.Errorf("file %s: content mismatch", name)
		}
	}
}

// --- API Publish tests ---

func apiPublishRequest(t *testing.T, srv *Server, db *index.DB, tarData []byte, name, version string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()

	authorID, err := db.InsertAuthor(ctx, "apiuser", "API User", "", "")
	if err != nil {
		a, _ := db.FindAuthor(ctx, "apiuser")
		authorID = a.ID
	}

	session, err := srv.sessions.Create(ctx, authorID)
	if err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("name", name)
	mw.WriteField("description", "test")
	mw.WriteField("version", version)

	fw, _ := mw.CreateFormFile("archive", "prompt.tar")
	fw.Write(tarData)
	mw.Close()

	req := httptest.NewRequest("POST", "/api/v1/publish", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+session.Token)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func TestAPIPublishValid(t *testing.T) {
	srv, db, _ := testServer(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Test\n"), 0644)
	tarData, _ := oci.PackageBytes(dir)

	rec := apiPublishRequest(t, srv, db, tarData, "api-prompt", "1.0.0")
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sha256:") {
		t.Error("response should contain digest")
	}
	if !strings.Contains(body, "api-prompt") {
		t.Error("response should contain name")
	}
}

func TestAPIPublishNoAuth(t *testing.T) {
	srv, _, _ := testServer(t)

	req := httptest.NewRequest("POST", "/api/v1/publish", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

func TestAPIPublishReturnsJSON(t *testing.T) {
	srv, db, _ := testServer(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Test\n"), 0644)
	tarData, _ := oci.PackageBytes(dir)

	rec := apiPublishRequest(t, srv, db, tarData, "json-test", "1.0.0")
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("got content-type %q, want application/json", ct)
	}
}
