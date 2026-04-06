package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Handlers provides HTTP handlers for the OAuth flow.
type Handlers struct {
	Provider     OAuthProvider
	Sessions     *SessionStore
	Signer       *CookieSigner
	EnsureAuthor func(ctx context.Context, user GitHubUser) (int64, error)
	CookieName   string
	StateCookie  string
	Secure       bool
}

// HandleLogin initiates the GitHub OAuth flow.
// If cli_redirect query param is present, the callback will redirect to that
// URL with a session token instead of setting a cookie (for CLI login).
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Encode cli_redirect into state cookie if present
	stateValue := state
	if cliRedirect := r.URL.Query().Get("cli_redirect"); cliRedirect != "" {
		stateValue = state + "|" + cliRedirect
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.StateCookie,
		Value:    stateValue,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.Secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.Provider.AuthCodeURL(state), http.StatusFound)
}

// HandleCallback completes the OAuth flow after GitHub redirects back.
func (h *Handlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(h.StateCookie)
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusForbidden)
		return
	}

	// Extract state and optional cli_redirect from cookie
	cookieValue := stateCookie.Value
	state := cookieValue
	var cliRedirect string
	if idx := strings.Index(cookieValue, "|"); idx >= 0 {
		state = cookieValue[:idx]
		cliRedirect = cookieValue[idx+1:]
	}

	if r.URL.Query().Get("state") != state {
		http.Error(w, "state mismatch", http.StatusForbidden)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   h.StateCookie,
		Path:   "/",
		MaxAge: -1,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ghUser, err := h.Provider.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("oauth exchange failed: %v", err), http.StatusForbidden)
		return
	}

	authorID, err := h.EnsureAuthor(r.Context(), ghUser)
	if err != nil {
		http.Error(w, "failed to create author", http.StatusInternalServerError)
		return
	}

	session, err := h.Sessions.Create(r.Context(), authorID)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// CLI login: redirect to localhost with raw token
	if cliRedirect != "" {
		sep := "?"
		if strings.Contains(cliRedirect, "?") {
			sep = "&"
		}
		http.Redirect(w, r, cliRedirect+sep+"token="+session.Token, http.StatusFound)
		return
	}

	// Web login: set cookie and redirect to dashboard
	http.SetCookie(w, &http.Cookie{
		Name:     h.CookieName,
		Value:    h.Signer.Sign(session.Token),
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   h.Secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/dashboard/prompts", http.StatusFound)
}

// HandleLogout clears the session and redirects to the home page.
func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.CookieName)
	if err == nil {
		token, err := h.Signer.Verify(cookie.Value)
		if err == nil {
			h.Sessions.Delete(r.Context(), token)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:   h.CookieName,
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}
