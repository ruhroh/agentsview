package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

func parseOptionalTimestampPtr(value *string) any {
	if value == nil {
		return nil
	}
	return parseOptionalTimestamp(*value)
}

// UpsertSession inserts or updates a hosted-viewer session.
func (s *Store) UpsertSession(session db.Session) error {
	createdAt, ok := ParseSQLiteTimestamp(session.CreatedAt)
	if !ok {
		createdAt = time.Now().UTC()
	}

	_, err := s.pg.Exec(`
		INSERT INTO sessions (
			id, project, machine, agent, first_message, display_name,
			started_at, ended_at, message_count, user_message_count,
			parent_session_id, relationship_type,
			total_output_tokens, peak_context_tokens,
			deleted_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12,
			$13, $14,
			NULL, $15, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			project = EXCLUDED.project,
			machine = EXCLUDED.machine,
			agent = EXCLUDED.agent,
			first_message = EXCLUDED.first_message,
			display_name = EXCLUDED.display_name,
			started_at = EXCLUDED.started_at,
			ended_at = EXCLUDED.ended_at,
			message_count = EXCLUDED.message_count,
			user_message_count = EXCLUDED.user_message_count,
			parent_session_id = EXCLUDED.parent_session_id,
			relationship_type = EXCLUDED.relationship_type,
			total_output_tokens = EXCLUDED.total_output_tokens,
			peak_context_tokens = EXCLUDED.peak_context_tokens,
			deleted_at = NULL,
			updated_at = NOW()`,
		session.ID, session.Project, session.Machine, session.Agent,
		session.FirstMessage, session.DisplayName,
		parseOptionalTimestampPtr(session.StartedAt),
		parseOptionalTimestampPtr(session.EndedAt),
		session.MessageCount, session.UserMessageCount,
		session.ParentSessionID, session.RelationshipType,
		session.TotalOutputTokens, session.PeakContextTokens,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("upserting session %s: %w", session.ID, err)
	}
	return nil
}

// SoftDeleteSession marks a session as deleted.
func (s *Store) SoftDeleteSession(id string) error {
	_, err := s.pg.Exec(`
		UPDATE sessions
		SET deleted_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("soft deleting session %s: %w", id, err)
	}
	return nil
}

// StarSession marks a session as starred.
func (s *Store) StarSession(sessionID string) (bool, error) {
	res, err := s.pg.Exec(`
		INSERT INTO starred_sessions (session_id)
		SELECT $1
		WHERE EXISTS (SELECT 1 FROM sessions WHERE id = $1)
		ON CONFLICT (session_id) DO NOTHING`,
		sessionID,
	)
	if err != nil {
		return false, fmt.Errorf("starring session %s: %w", sessionID, err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected > 0 {
		return true, nil
	}

	var exists int
	err = s.pg.QueryRow("SELECT 1 FROM sessions WHERE id = $1", sessionID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking session %s: %w", sessionID, err)
	}
	return true, nil
}

// UnstarSession removes a session star.
func (s *Store) UnstarSession(sessionID string) error {
	if _, err := s.pg.Exec(`DELETE FROM starred_sessions WHERE session_id = $1`, sessionID); err != nil {
		return fmt.Errorf("unstarring session %s: %w", sessionID, err)
	}
	return nil
}

// ListStarredSessionIDs returns all starred session IDs.
func (s *Store) ListStarredSessionIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pg.QueryContext(ctx, `SELECT session_id FROM starred_sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing starred sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning starred session: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// BulkStarSessions stars multiple sessions, skipping stale IDs.
func (s *Store) BulkStarSessions(sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	tx, err := s.pg.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO starred_sessions (session_id)
		SELECT $1
		WHERE EXISTS (SELECT 1 FROM sessions WHERE id = $1)
		ON CONFLICT (session_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, id := range sessionIDs {
		if _, err := stmt.Exec(id); err != nil {
			return fmt.Errorf("starring session %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RecordShare records or updates a shared session entry.
func (s *Store) RecordShare(sessionID, shareID, serverURL string) error {
	_, err := s.pg.Exec(`
		INSERT INTO shared_sessions (session_id, share_id, server_url, shared_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (session_id) DO UPDATE SET
			share_id = EXCLUDED.share_id,
			server_url = EXCLUDED.server_url,
			updated_at = NOW()`,
		sessionID, shareID, serverURL,
	)
	if err != nil {
		return fmt.Errorf("recording share for %s: %w", sessionID, err)
	}
	return nil
}

// RemoveShare deletes a shared session record.
func (s *Store) RemoveShare(sessionID string) error {
	if _, err := s.pg.Exec(`DELETE FROM shared_sessions WHERE session_id = $1`, sessionID); err != nil {
		return fmt.Errorf("removing share for %s: %w", sessionID, err)
	}
	return nil
}

// GetShare returns the share record for a session, or nil if not shared.
func (s *Store) GetShare(ctx context.Context, sessionID string) (*db.SharedSession, error) {
	var share db.SharedSession
	var sharedAt time.Time
	var updatedAt time.Time
	err := s.pg.QueryRowContext(ctx, `
		SELECT session_id, share_id, server_url, shared_at, updated_at
		FROM shared_sessions
		WHERE session_id = $1`,
		sessionID,
	).Scan(&share.SessionID, &share.ShareID, &share.ServerURL, &sharedAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting share for %s: %w", sessionID, err)
	}
	share.SharedAt = FormatISO8601(sharedAt)
	share.UpdatedAt = FormatISO8601(updatedAt)
	return &share, nil
}

// ListSharedSessionIDs returns all shared session IDs.
func (s *Store) ListSharedSessionIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pg.QueryContext(ctx, `SELECT session_id FROM shared_sessions ORDER BY shared_at DESC`)
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
