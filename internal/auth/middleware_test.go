package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testCookieName = "session"

func setupMiddlewareTest(t *testing.T) (*SessionStore, *CookieSigner, int64) {
	t.Helper()
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	signer := NewCookieSigner([]byte("test-key-32-bytes-long-for-hmac!"))
	return store, signer, authorID
}

func TestRequireAuthNoSession(t *testing.T) {
	store, signer, _ := setupMiddlewareTest(t)
	middleware := RequireAuth(store, signer, testCookieName)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/dashboard/prompts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("handler should not have been called")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("got redirect %q, want /login", loc)
	}
}

func TestRequireAuthValidSession(t *testing.T) {
	store, signer, authorID := setupMiddlewareTest(t)
	middleware := RequireAuth(store, signer, testCookieName)

	session, err := store.Create(context.Background(), authorID)
	if err != nil {
		t.Fatal(err)
	}

	var gotUser *AuthenticatedUser
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = AuthorFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/dashboard/prompts", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: signer.Sign(session.Token)})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotUser == nil {
		t.Fatal("user not set in context")
	}
	if gotUser.AuthorID != authorID {
		t.Errorf("got author_id %d, want %d", gotUser.AuthorID, authorID)
	}
	if gotUser.Username != "testuser" {
		t.Errorf("got username %q, want %q", gotUser.Username, "testuser")
	}
}

func TestRequireAuthTamperedCookie(t *testing.T) {
	store, signer, authorID := setupMiddlewareTest(t)
	middleware := RequireAuth(store, signer, testCookieName)

	session, _ := store.Create(context.Background(), authorID)
	signed := signer.Sign(session.Token)
	tampered := signed[:len(signed)-1] + "X"

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: tampered})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("handler should not have been called with tampered cookie")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusFound)
	}
}

func TestRequireAuthExpiredSession(t *testing.T) {
	store, signer, authorID := setupMiddlewareTest(t)
	middleware := RequireAuth(store, signer, testCookieName)

	// Insert expired session directly
	db := store.db
	expired := "2020-01-01T00:00:00Z"
	db.ExecContext(context.Background(),
		"INSERT INTO sessions (token, author_id, expires_at) VALUES (?, ?, ?)",
		"expired-token", authorID, expired)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: signer.Sign("expired-token")})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("handler should not have been called with expired session")
	}
}

func TestOptionalAuthNoSession(t *testing.T) {
	store, signer, _ := setupMiddlewareTest(t)
	middleware := OptionalAuth(store, signer, testCookieName)

	var gotUser *AuthenticatedUser
	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotUser = AuthorFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called")
	}
	if gotUser != nil {
		t.Error("expected nil user for no session")
	}
}

func TestOptionalAuthValidSession(t *testing.T) {
	store, signer, authorID := setupMiddlewareTest(t)
	middleware := OptionalAuth(store, signer, testCookieName)

	session, _ := store.Create(context.Background(), authorID)

	var gotUser *AuthenticatedUser
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = AuthorFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: signer.Sign(session.Token)})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.Username != "testuser" {
		t.Errorf("got username %q, want %q", gotUser.Username, "testuser")
	}
}
