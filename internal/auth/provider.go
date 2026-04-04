package auth

import "context"

// OAuthProvider abstracts the GitHub OAuth flow for testability.
type OAuthProvider interface {
	// AuthCodeURL returns the URL to redirect the user to for authorization.
	AuthCodeURL(state string) string

	// Exchange trades an authorization code for a GitHub user profile.
	Exchange(ctx context.Context, code string) (GitHubUser, error)
}
