package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

const pgSelectMessageCols = `id, session_id, ordinal, role, content,
	timestamp, has_thinking, has_tool_use, content_length,
	is_system, model, token_usage, context_tokens, output_tokens`

// GetMessages returns paginated messages for a session.
func (s *Store) GetMessages(ctx context.Context, sessionID string, from, limit int, asc bool) ([]db.Message, error) {
	if limit <= 0 || limit > db.MaxMessageLimit {
		limit = db.DefaultMessageLimit
	}

	dir := "ASC"
	op := ">="
	if !asc {
		dir = "DESC"
		op = "<="
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM messages
		WHERE session_id = $1 AND ordinal %s $2
		ORDER BY ordinal %s
		LIMIT $3`,
		pgSelectMessageCols, op, dir,
	)

	rows, err := s.pg.QueryContext(ctx, query, sessionID, from, limit)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	msgs, err := scanPGMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachToolCalls(ctx, msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// GetAllMessages returns all messages for a session ordered by ordinal.
func (s *Store) GetAllMessages(ctx context.Context, sessionID string) ([]db.Message, error) {
	rows, err := s.pg.QueryContext(ctx, `
		SELECT `+pgSelectMessageCols+`
		FROM messages
		WHERE session_id = $1
		ORDER BY ordinal ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all messages: %w", err)
	}
	defer rows.Close()

	msgs, err := scanPGMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachToolCalls(ctx, msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func scanPGMessages(rows *sql.Rows) ([]db.Message, error) {
	var msgs []db.Message
	for rows.Next() {
		var msg db.Message
		var timestamp *time.Time
		var tokenUsage string
		if err := rows.Scan(
			&msg.ID, &msg.SessionID, &msg.Ordinal, &msg.Role,
			&msg.Content, &timestamp,
			&msg.HasThinking, &msg.HasToolUse, &msg.ContentLength,
			&msg.IsSystem, &msg.Model, &tokenUsage,
			&msg.ContextTokens, &msg.OutputTokens,
		); err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		if timestamp != nil {
			msg.Timestamp = FormatISO8601(*timestamp)
		}
		if tokenUsage != "" {
			msg.TokenUsage = json.RawMessage(tokenUsage)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

func (s *Store) attachToolCalls(ctx context.Context, msgs []db.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	sessionID := msgs[0].SessionID
	ordinals := make([]int, 0, len(msgs))
	ordToIdx := make(map[int]int, len(msgs))
	for i, msg := range msgs {
		ordinals = append(ordinals, msg.Ordinal)
		ordToIdx[msg.Ordinal] = i
	}

	params := &paramBuilder{}
	whereOrdinals := make([]string, len(ordinals))
	params.args = append(params.args, sessionID)
	params.n = 1
	for i, ordinal := range ordinals {
		whereOrdinals[i] = params.add(ordinal)
	}

	query := fmt.Sprintf(`
		SELECT message_ordinal, call_index, tool_name, category,
			tool_use_id, input_json, skill_name,
			result_content_length, result_content, subagent_session_id
		FROM tool_calls
		WHERE session_id = $1
		  AND message_ordinal IN (%s)
		ORDER BY message_ordinal, call_index`,
		strings.Join(whereOrdinals, ","),
	)

	rows, err := s.pg.QueryContext(ctx, query, params.args...)
	if err != nil {
		return fmt.Errorf("querying tool_calls: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ordinal int
		var callIndex int
		var call db.ToolCall
		var inputJSON sql.NullString
		var skillName sql.NullString
		var resultContentLength sql.NullInt64
		var resultContent sql.NullString
		var subagentSessionID sql.NullString
		if err := rows.Scan(
			&ordinal, &callIndex,
			&call.ToolName, &call.Category, &call.ToolUseID,
			&inputJSON, &skillName, &resultContentLength,
			&resultContent, &subagentSessionID,
		); err != nil {
			return fmt.Errorf("scanning tool_call: %w", err)
		}
		if inputJSON.Valid {
			call.InputJSON = inputJSON.String
		}
		if skillName.Valid {
			call.SkillName = skillName.String
		}
		if resultContentLength.Valid {
			call.ResultContentLength = int(resultContentLength.Int64)
		}
		if resultContent.Valid {
			call.ResultContent = resultContent.String
		}
		if subagentSessionID.Valid {
			call.SubagentSessionID = subagentSessionID.String
		}
		idx, ok := ordToIdx[ordinal]
		if !ok {
			continue
		}
		msgs[idx].ToolCalls = append(msgs[idx].ToolCalls, call)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.attachToolResultEvents(ctx, msgs, ordToIdx, ordinals)
}

func (s *Store) attachToolResultEvents(ctx context.Context, msgs []db.Message, ordToIdx map[int]int, ordinals []int) error {
	if len(ordinals) == 0 {
		return nil
	}

	params := &paramBuilder{}
	params.args = append(params.args, msgs[0].SessionID)
	params.n = 1
	whereOrdinals := make([]string, len(ordinals))
	for i, ordinal := range ordinals {
		whereOrdinals[i] = params.add(ordinal)
	}

	query := fmt.Sprintf(`
		SELECT tool_call_message_ordinal, call_index,
			tool_use_id, agent_id, subagent_session_id,
			source, status, content, content_length,
			timestamp, event_index
		FROM tool_result_events
		WHERE session_id = $1
		  AND tool_call_message_ordinal IN (%s)
		ORDER BY tool_call_message_ordinal, call_index, event_index`,
		strings.Join(whereOrdinals, ","),
	)

	rows, err := s.pg.QueryContext(ctx, query, params.args...)
	if err != nil {
		return fmt.Errorf("querying tool_result_events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ordinal int
		var callIndex int
		var event db.ToolResultEvent
		var toolUseID sql.NullString
		var agentID sql.NullString
		var subagentSessionID sql.NullString
		var timestamp *time.Time
		if err := rows.Scan(
			&ordinal, &callIndex,
			&toolUseID, &agentID, &subagentSessionID,
			&event.Source, &event.Status, &event.Content, &event.ContentLength,
			&timestamp, &event.EventIndex,
		); err != nil {
			return fmt.Errorf("scanning tool_result_event: %w", err)
		}
		if toolUseID.Valid {
			event.ToolUseID = toolUseID.String
		}
		if agentID.Valid {
			event.AgentID = agentID.String
		}
		if subagentSessionID.Valid {
			event.SubagentSessionID = subagentSessionID.String
		}
		if timestamp != nil {
			event.Timestamp = FormatISO8601(*timestamp)
		}
		idx, ok := ordToIdx[ordinal]
		if !ok || callIndex < 0 || callIndex >= len(msgs[idx].ToolCalls) {
			continue
		}
		msgs[idx].ToolCalls[callIndex].ResultEvents = append(msgs[idx].ToolCalls[callIndex].ResultEvents, event)
	}
	return rows.Err()
}

func normalizeTokenUsage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func parseOptionalTimestamp(value string) any {
	ts, ok := ParseSQLiteTimestamp(value)
	if !ok {
		return nil
	}
	return ts
}

// ReplaceSessionMessages replaces a session's messages and related tool data atomically.
func (s *Store) ReplaceSessionMessages(sessionID string, msgs []db.Message) error {
	tx, err := s.pg.Begin()
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM tool_result_events WHERE session_id = $1`,
		sessionID,
	); err != nil {
		return fmt.Errorf("deleting old tool_result_events: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM tool_calls WHERE session_id = $1`,
		sessionID,
	); err != nil {
		return fmt.Errorf("deleting old tool_calls: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM messages WHERE session_id = $1`,
		sessionID,
	); err != nil {
		return fmt.Errorf("deleting old messages: %w", err)
	}

	if len(msgs) > 0 {
		msgStmt, err := tx.Prepare(`
			INSERT INTO messages (
				session_id, ordinal, role, content, timestamp,
				has_thinking, has_tool_use, content_length, is_system,
				model, token_usage, context_tokens, output_tokens
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13
			)`)
		if err != nil {
			return fmt.Errorf("preparing message insert: %w", err)
		}
		defer msgStmt.Close()

		callStmt, err := tx.Prepare(`
			INSERT INTO tool_calls (
				session_id, message_ordinal, call_index, tool_name, category,
				tool_use_id, input_json, skill_name, result_content_length,
				result_content, subagent_session_id
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11
			)`)
		if err != nil {
			return fmt.Errorf("preparing tool_call insert: %w", err)
		}
		defer callStmt.Close()

		eventStmt, err := tx.Prepare(`
			INSERT INTO tool_result_events (
				session_id, tool_call_message_ordinal, call_index,
				tool_use_id, agent_id, subagent_session_id,
				source, status, content, content_length,
				timestamp, event_index
			) VALUES (
				$1, $2, $3,
				$4, $5, $6,
				$7, $8, $9, $10,
				$11, $12
			)`)
		if err != nil {
			return fmt.Errorf("preparing tool_result_event insert: %w", err)
		}
		defer eventStmt.Close()

		for _, msg := range msgs {
			if _, err := msgStmt.Exec(
				sessionID, msg.Ordinal, msg.Role, msg.Content, parseOptionalTimestamp(msg.Timestamp),
				msg.HasThinking, msg.HasToolUse, msg.ContentLength, msg.IsSystem,
				msg.Model, normalizeTokenUsage(msg.TokenUsage), msg.ContextTokens, msg.OutputTokens,
			); err != nil {
				return fmt.Errorf("inserting message ord=%d: %w", msg.Ordinal, err)
			}

			for callIndex, call := range msg.ToolCalls {
				var resultContentLength any
				if call.ResultContentLength > 0 {
					resultContentLength = call.ResultContentLength
				}
				if _, err := callStmt.Exec(
					sessionID, msg.Ordinal, callIndex, call.ToolName, call.Category,
					call.ToolUseID, nullIfEmpty(call.InputJSON), nullIfEmpty(call.SkillName), resultContentLength,
					nullIfEmpty(call.ResultContent), nullIfEmpty(call.SubagentSessionID),
				); err != nil {
					return fmt.Errorf("inserting tool_call %q: %w", call.ToolName, err)
				}

				for eventIndex, event := range call.ResultEvents {
					contentLength := event.ContentLength
					if contentLength == 0 {
						contentLength = len(event.Content)
					}
					toolUseID := event.ToolUseID
					if toolUseID == "" {
						toolUseID = call.ToolUseID
					}
					subagentSessionID := event.SubagentSessionID
					if subagentSessionID == "" {
						subagentSessionID = call.SubagentSessionID
					}
					if _, err := eventStmt.Exec(
						sessionID, msg.Ordinal, callIndex,
						nullIfEmpty(toolUseID), nullIfEmpty(event.AgentID), nullIfEmpty(subagentSessionID),
						event.Source, event.Status, event.Content, contentLength,
						parseOptionalTimestamp(event.Timestamp), eventIndex,
					); err != nil {
						return fmt.Errorf("inserting tool_result_event %q/%q: %w", event.Source, event.Status, err)
					}
				}
			}
		}
	}

	return tx.Commit()
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
