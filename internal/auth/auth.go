package auth

import (
	"context"
	"errors"
	"time"
)

// Session represents an authenticated user session.
type Session struct {
	ID        int64
	Token     string
	AuthorID  int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// GitHubUser holds profile data returned by the GitHub API after OAuth.
type GitHubUser struct {
	Username   string
	Name       string
	AvatarURL  string
	ProfileURL string
}

// AuthenticatedUser is placed in request context by auth middleware.
type AuthenticatedUser struct {
	AuthorID int64
	Username string
}

type contextKey string

const authorKey contextKey = "author"

// WithAuthor returns a new context with the authenticated user set.
func WithAuthor(ctx context.Context, user *AuthenticatedUser) context.Context {
	return context.WithValue(ctx, authorKey, user)
}

// AuthorFromContext extracts the authenticated user from context, or nil.
func AuthorFromContext(ctx context.Context) *AuthenticatedUser {
	u, _ := ctx.Value(authorKey).(*AuthenticatedUser)
	return u
}

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionExpired   = errors.New("session expired")
	ErrInvalidSignature = errors.New("invalid cookie signature")
)

const sessionDuration = 30 * 24 * time.Hour
