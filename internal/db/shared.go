package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SharedSession represents a session published to a remote server.
type SharedSession struct {
	SessionID string `json:"session_id"`
	ShareID   string `json:"share_id"`
	ServerURL string `json:"server_url"`
	SharedAt  string `json:"shared_at"`
	UpdatedAt string `json:"updated_at"`
}

// RecordShare records or updates a shared session entry.
func (db *DB) RecordShare(sessionID, shareID, serverURL string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.getWriter().Exec(`
		INSERT INTO shared_sessions (session_id, share_id, server_url, shared_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			share_id = excluded.share_id,
			server_url = excluded.server_url,
			updated_at = excluded.updated_at`,
		sessionID, shareID, serverURL, now, now)
	if err != nil {
		return fmt.Errorf("recording share for %s: %w", sessionID, err)
	}
	return nil
}

// RemoveShare deletes a shared session record.
func (db *DB) RemoveShare(sessionID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.getWriter().Exec(
		"DELETE FROM shared_sessions WHERE session_id = ?",
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("removing share for %s: %w", sessionID, err)
	}
	return nil
}

// GetShare returns the share record for a session, or nil if not shared.
func (db *DB) GetShare(ctx context.Context, sessionID string) (*SharedSession, error) {
	var s SharedSession
	err := db.getReader().QueryRowContext(ctx,
		`SELECT session_id, share_id, server_url, shared_at, updated_at
		 FROM shared_sessions WHERE session_id = ?`, sessionID,
	).Scan(&s.SessionID, &s.ShareID, &s.ServerURL, &s.SharedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting share for %s: %w", sessionID, err)
	}
	return &s, nil
}

// ListSharedSessionIDs returns all shared session IDs.
func (db *DB) ListSharedSessionIDs(ctx context.Context) ([]string, error) {
	rows, err := db.getReader().QueryContext(ctx,
		"SELECT session_id FROM shared_sessions ORDER BY shared_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("listing shared sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning shared session: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
