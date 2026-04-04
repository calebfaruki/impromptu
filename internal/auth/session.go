package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// SessionStore manages session persistence in SQLite.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates a session store using the given database connection.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// SessionInfo is returned by Find and includes the author's username.
type SessionInfo struct {
	Session  Session
	Username string
}

// Create generates a new session for the given author.
func (s *SessionStore) Create(ctx context.Context, authorID int64) (Session, error) {
	token, err := generateToken()
	if err != nil {
		return Session{}, fmt.Errorf("generating session token: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(sessionDuration)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, author_id, expires_at) VALUES (?, ?, ?)`,
		token, authorID, expiresAt.Format(timeLayout))
	if err != nil {
		return Session{}, fmt.Errorf("inserting session: %w", err)
	}

	return Session{
		Token:     token,
		AuthorID:  authorID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

// Find looks up a session by token, joining with authors for the username.
// Returns ErrSessionNotFound if the token does not exist.
// Returns ErrSessionExpired if the session is past its expiry time.
func (s *SessionStore) Find(ctx context.Context, token string) (SessionInfo, error) {
	var info SessionInfo
	var createdAt, expiresAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.token, s.author_id, s.created_at, s.expires_at, a.username
		FROM sessions s
		JOIN authors a ON a.id = s.author_id
		WHERE s.token = ?`, token).Scan(
		&info.Session.ID, &info.Session.Token, &info.Session.AuthorID,
		&createdAt, &expiresAt, &info.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			return SessionInfo{}, ErrSessionNotFound
		}
		return SessionInfo{}, fmt.Errorf("finding session: %w", err)
	}

	info.Session.CreatedAt, _ = time.Parse(timeLayout, createdAt)
	info.Session.ExpiresAt, _ = time.Parse(timeLayout, expiresAt)

	if time.Now().UTC().After(info.Session.ExpiresAt) {
		return SessionInfo{}, ErrSessionExpired
	}

	return info, nil
}

// Delete removes a session by token.
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteExpired removes all expired sessions. Returns the number deleted.
func (s *SessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(timeLayout)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE expires_at < ?", now)
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return result.RowsAffected()
}

const timeLayout = "2006-01-02T15:04:05Z"

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
