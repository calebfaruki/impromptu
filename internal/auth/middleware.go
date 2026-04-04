package auth

import "net/http"

// RequireAuth returns middleware that redirects to /login if no valid session exists.
func RequireAuth(sessions *SessionStore, signer *CookieSigner, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := authenticate(r, sessions, signer, cookieName)
			if user == nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			ctx := WithAuthor(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that sets the user in context if a valid session exists.
// Does not redirect on failure.
func OptionalAuth(sessions *SessionStore, signer *CookieSigner, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := authenticate(r, sessions, signer, cookieName)
			if user != nil {
				ctx := WithAuthor(r.Context(), user)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authenticate(r *http.Request, sessions *SessionStore, signer *CookieSigner, cookieName string) *AuthenticatedUser {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	token, err := signer.Verify(cookie.Value)
	if err != nil {
		return nil
	}
	info, err := sessions.Find(r.Context(), token)
	if err != nil {
		return nil
	}
	return &AuthenticatedUser{
		AuthorID: info.Session.AuthorID,
		Username: info.Username,
	}
}
