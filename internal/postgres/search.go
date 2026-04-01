package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

func stripFTSQuotes(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

func escapeLike(v string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(v)
}

// Search performs PostgreSQL FTS search across messages, with a trigram-backed
// name fallback branch for sessions whose names match but messages do not.
func (s *Store) Search(ctx context.Context, f db.SearchFilter) (db.SearchPage, error) {
	if f.Limit <= 0 || f.Limit > db.MaxSearchLimit {
		f.Limit = db.DefaultSearchLimit
	}

	plainQuery := stripFTSQuotes(strings.TrimSpace(f.Query))
	if plainQuery == "" {
		return db.SearchPage{}, nil
	}

	orderBy := "match_priority ASC, rank DESC, match_pos ASC, session_ended_at DESC NULLS LAST, session_id ASC"
	if f.Sort == "recency" {
		orderBy = "match_priority ASC, session_ended_at DESC NULLS LAST, rank DESC, match_pos ASC, session_id ASC"
	}

	args := []any{f.Query, plainQuery, "%" + escapeLike(plainQuery) + "%"}
	projectClause := ""
	nameProjectClause := ""
	if f.Project != "" {
		args = append(args, f.Project)
		projectArg := len(args)
		projectClause = fmt.Sprintf(" AND s.project = $%d", projectArg)
		nameProjectClause = fmt.Sprintf(" AND s.project = $%d", projectArg)
	}
	args = append(args, f.Limit+1, f.Cursor)
	limitArg := len(args) - 1
	cursorArg := len(args)

	query := fmt.Sprintf(`
		WITH query_terms AS (
			SELECT
				websearch_to_tsquery('english', $1) AS tsq,
				numnode(websearch_to_tsquery('english', $1)) AS tsq_nodes,
				$2::text AS plain_query
		),
		message_matches AS (
			SELECT DISTINCT ON (m.session_id)
				m.session_id,
				s.project,
				s.agent,
				COALESCE(s.display_name, s.first_message, '') AS name,
				COALESCE(s.ended_at, s.started_at, s.created_at) AS session_ended_at,
				m.ordinal,
				ts_headline(
					'english',
					m.content,
					qt.tsq,
					'StartSel=<mark>, StopSel=</mark>, MaxFragments=2, MaxWords=24, MinWords=8, FragmentDelimiter= ... '
				) AS snippet,
				ts_rank_cd(m.search_tsv, qt.tsq) AS rank,
				COALESCE(NULLIF(POSITION(LOWER(qt.plain_query) IN LOWER(m.content)), 0), 2147483647) AS match_pos
			FROM query_terms qt
			JOIN messages m
				ON qt.tsq_nodes > 0
			   AND m.search_tsv @@ qt.tsq
			JOIN sessions s
				ON s.id = m.session_id
			WHERE s.deleted_at IS NULL
			  AND m.is_system = FALSE
			  AND `+db.SystemPrefixSQL("m.content", "m.role")+`
			  %s
			ORDER BY
				m.session_id,
				ts_rank_cd(m.search_tsv, qt.tsq) DESC,
				COALESCE(NULLIF(POSITION(LOWER(qt.plain_query) IN LOWER(m.content)), 0), 2147483647) ASC,
				m.ordinal ASC,
				m.id ASC
		),
		name_matches AS (
			SELECT
				s.id AS session_id,
				s.project,
				s.agent,
				COALESCE(s.display_name, s.first_message, '') AS name,
				COALESCE(s.ended_at, s.started_at, s.created_at) AS session_ended_at,
				-1 AS ordinal,
				CASE
					WHEN s.display_name ILIKE $3 ESCAPE E'\\' THEN COALESCE(s.display_name, '')
					WHEN s.first_message ILIKE $3 ESCAPE E'\\' THEN COALESCE(s.first_message, '')
					ELSE COALESCE(s.display_name, s.first_message, '')
				END AS snippet,
				0.0::double precision AS rank,
				0 AS match_pos
			FROM sessions s
			WHERE (s.display_name ILIKE $3 ESCAPE E'\\'
			    OR s.first_message ILIKE $3 ESCAPE E'\\')
			  AND s.deleted_at IS NULL
			  AND EXISTS (
				SELECT 1
				FROM messages mx
				WHERE mx.session_id = s.id
				  AND mx.is_system = FALSE
				  AND `+db.SystemPrefixSQL("mx.content", "mx.role")+`
			  )
			  %s
			  AND NOT EXISTS (
				SELECT 1 FROM message_matches mm
				WHERE mm.session_id = s.id
			  )
		)
		SELECT session_id, project, agent, name,
			session_ended_at, ordinal, snippet, rank
		FROM (
			SELECT *, 1 AS match_priority FROM message_matches
			UNION ALL
			SELECT *, 2 AS match_priority FROM name_matches
		) combined
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		projectClause,
		nameProjectClause,
		orderBy,
		limitArg,
		cursorArg,
	)

	rows, err := s.pg.QueryContext(ctx, query, args...)
	if err != nil {
		return db.SearchPage{}, fmt.Errorf("searching: %w", err)
	}
	defer rows.Close()

	results := []db.SearchResult{}
	for rows.Next() {
		var result db.SearchResult
		var endedAt *time.Time
		if err := rows.Scan(
			&result.SessionID, &result.Project, &result.Agent, &result.Name,
			&endedAt, &result.Ordinal, &result.Snippet, &result.Rank,
		); err != nil {
			return db.SearchPage{}, fmt.Errorf("scanning search result: %w", err)
		}
		if endedAt != nil {
			result.SessionEndedAt = FormatISO8601(*endedAt)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return db.SearchPage{}, err
	}

	page := db.SearchPage{Results: results}
	if len(results) > f.Limit {
		page.Results = results[:f.Limit]
		page.NextCursor = f.Cursor + f.Limit
	}
	return page, nil
}

// SearchSession performs a substring search within a single session.
func (s *Store) SearchSession(ctx context.Context, sessionID, query string) ([]int, error) {
	if query == "" {
		return nil, nil
	}

	like := "%" + escapeLike(query) + "%"
	rows, err := s.pg.QueryContext(ctx, `
		SELECT DISTINCT m.ordinal
		FROM messages m
		LEFT JOIN tool_calls tc
			ON tc.session_id = m.session_id
		   AND tc.message_ordinal = m.ordinal
		WHERE m.session_id = $1
		  AND m.is_system = FALSE
		  AND `+db.SystemPrefixSQL("m.content", "m.role")+`
		  AND (
			m.content ILIKE $2 ESCAPE E'\\'
			OR tc.result_content ILIKE $2 ESCAPE E'\\'
		  )
		ORDER BY m.ordinal ASC`,
		sessionID, like,
	)
	if err != nil {
		return nil, fmt.Errorf("searching session: %w", err)
	}
	defer rows.Close()

	var ordinals []int
	for rows.Next() {
		var ordinal int
		if err := rows.Scan(&ordinal); err != nil {
			return nil, fmt.Errorf("scanning ordinal: %w", err)
		}
		ordinals = append(ordinals, ordinal)
	}
	return ordinals, rows.Err()
}
