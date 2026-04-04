package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupHandlerTest(t *testing.T) *Handlers {
	t.Helper()
	db := testSessionDB(t)
	authorID := insertTestAuthorDirect(t, db)
	store := NewSessionStore(db)
	signer := NewCookieSigner([]byte("test-key-32-bytes-long-for-hmac!"))

	return &Handlers{
		Provider: &FakeProvider{
			User: GitHubUser{
				Username:   "testuser",
				Name:       "Test User",
				AvatarURL:  "https://avatar.test",
				ProfileURL: "https://github.com/testuser",
			},
			URL: "https://github.com/login/oauth/authorize",
		},
		Sessions:    store,
		Signer:      signer,
		CookieName:  "session",
		StateCookie: "oauth_state",
		Secure:      false,
		EnsureAuthor: func(_ context.Context, user GitHubUser) (int64, error) {
			return authorID, nil
		},
	}
}

func TestHandleLoginRedirects(t *testing.T) {
	h := setupHandlerTest(t)

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	h.HandleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("no Location header")
	}
	// Should redirect to GitHub
	if len(loc) < 10 {
		t.Errorf("redirect URL too short: %s", loc)
	}

	// Should set state cookie
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "oauth_state" {
			found = true
			if len(c.Value) != 64 {
				t.Errorf("state cookie length: got %d, want 64", len(c.Value))
			}
		}
	}
	if !found {
		t.Error("state cookie not set")
	}
}

func TestHandleCallbackValid(t *testing.T) {
	h := setupHandlerTest(t)

	state := "test-state-value-0123456789abcdef0123456789abcdef"
	req := httptest.NewRequest("GET", "/callback?state="+state+"&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	h.HandleCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard/prompts" {
		t.Errorf("got redirect %q, want /dashboard/prompts", loc)
	}

	// Should set session cookie
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			found = true
			if len(c.Value) == 0 {
				t.Error("session cookie is empty")
			}
		}
	}
	if !found {
		t.Error("session cookie not set")
	}
}

func TestHandleCallbackInvalidState(t *testing.T) {
	h := setupHandlerTest(t)

	req := httptest.NewRequest("GET", "/callback?state=wrong&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct"})
	rec := httptest.NewRecorder()
	h.HandleCallback(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleCallbackMissingState(t *testing.T) {
	h := setupHandlerTest(t)

	req := httptest.NewRequest("GET", "/callback?code=test-code", nil)
	rec := httptest.NewRecorder()
	h.HandleCallback(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleCallbackExpiredCode(t *testing.T) {
	h := setupHandlerTest(t)
	h.Provider = &FakeProvider{Err: errors.New("invalid code")}

	state := "test-state"
	req := httptest.NewRequest("GET", "/callback?state="+state+"&code=expired", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	h.HandleCallback(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleCallbackMissingCode(t *testing.T) {
	h := setupHandlerTest(t)

	state := "test-state"
	req := httptest.NewRequest("GET", "/callback?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	h.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleLogout(t *testing.T) {
	h := setupHandlerTest(t)

	// Create a session first
	session, err := h.Sessions.Create(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: h.Signer.Sign(session.Token)})
	rec := httptest.NewRecorder()
	h.HandleLogout(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("got redirect %q, want /", loc)
	}

	// Session should be deleted
	_, err = h.Sessions.Find(context.Background(), session.Token)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected session deleted, got: %v", err)
	}
}

func TestHandleCallbackCreatesAndReusesAuthor(t *testing.T) {
	h := setupHandlerTest(t)

	var callCount int
	h.EnsureAuthor = func(_ context.Context, user GitHubUser) (int64, error) {
		callCount++
		return 1, nil
	}

	state := "test-state"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/callback?state="+state+"&code=code", nil)
		req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
		rec := httptest.NewRecorder()
		h.HandleCallback(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("iteration %d: got status %d", i, rec.Code)
		}
	}

	if callCount != 2 {
		t.Errorf("EnsureAuthor called %d times, want 2", callCount)
	}
}
