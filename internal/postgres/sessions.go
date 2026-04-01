package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

const pgSessionCols = `id, project, machine, agent,
	first_message, display_name, started_at, ended_at,
	message_count, user_message_count,
	parent_session_id, relationship_type,
	total_output_tokens, peak_context_tokens,
	deleted_at, created_at`

type paramBuilder struct {
	n    int
	args []any
}

func (pb *paramBuilder) add(v any) string {
	pb.n++
	pb.args = append(pb.args, v)
	return fmt.Sprintf("$%d", pb.n)
}

func scanPGSession(rs interface{ Scan(...any) error }) (db.Session, error) {
	var s db.Session
	var startedAt, endedAt, deletedAt *time.Time
	var createdAt time.Time
	err := rs.Scan(
		&s.ID, &s.Project, &s.Machine, &s.Agent,
		&s.FirstMessage, &s.DisplayName,
		&startedAt, &endedAt,
		&s.MessageCount, &s.UserMessageCount,
		&s.ParentSessionID, &s.RelationshipType,
		&s.TotalOutputTokens, &s.PeakContextTokens,
		&deletedAt, &createdAt,
	)
	if err != nil {
		return s, err
	}
	if startedAt != nil {
		v := FormatISO8601(*startedAt)
		s.StartedAt = &v
	}
	if endedAt != nil {
		v := FormatISO8601(*endedAt)
		s.EndedAt = &v
	}
	if deletedAt != nil {
		v := FormatISO8601(*deletedAt)
		s.DeletedAt = &v
	}
	s.CreatedAt = FormatISO8601(createdAt)
	return s, nil
}

func scanPGSessionRows(rows *sql.Rows) ([]db.Session, error) {
	sessions := []db.Session{}
	for rows.Next() {
		s, err := scanPGSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func buildPGSessionFilter(f db.SessionFilter) (string, []any) {
	pb := &paramBuilder{}
	basePreds := []string{
		"message_count > 0",
		"deleted_at IS NULL",
	}
	if !f.IncludeChildren {
		basePreds = append(basePreds, "relationship_type NOT IN ('subagent', 'fork')")
	}

	var filterPreds []string
	if f.Project != "" {
		filterPreds = append(filterPreds, "project = "+pb.add(f.Project))
	}
	if f.ExcludeProject != "" {
		filterPreds = append(filterPreds, "project != "+pb.add(f.ExcludeProject))
	}
	if f.Machine != "" {
		machines := strings.Split(f.Machine, ",")
		if len(machines) == 1 {
			filterPreds = append(filterPreds, "machine = "+pb.add(machines[0]))
		} else {
			placeholders := make([]string, len(machines))
			for i, machine := range machines {
				placeholders[i] = pb.add(machine)
			}
			filterPreds = append(filterPreds, "machine IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	if f.Agent != "" {
		agents := strings.Split(f.Agent, ",")
		if len(agents) == 1 {
			filterPreds = append(filterPreds, "agent = "+pb.add(agents[0]))
		} else {
			placeholders := make([]string, len(agents))
			for i, agent := range agents {
				placeholders[i] = pb.add(agent)
			}
			filterPreds = append(filterPreds, "agent IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	if f.Date != "" {
		filterPreds = append(filterPreds, "DATE(COALESCE(started_at, created_at) AT TIME ZONE 'UTC') = "+pb.add(f.Date)+"::date")
	}
	if f.DateFrom != "" {
		filterPreds = append(filterPreds, "DATE(COALESCE(started_at, created_at) AT TIME ZONE 'UTC') >= "+pb.add(f.DateFrom)+"::date")
	}
	if f.DateTo != "" {
		filterPreds = append(filterPreds, "DATE(COALESCE(started_at, created_at) AT TIME ZONE 'UTC') <= "+pb.add(f.DateTo)+"::date")
	}
	if f.ActiveSince != "" {
		filterPreds = append(filterPreds, "COALESCE(ended_at, started_at, created_at) >= "+pb.add(f.ActiveSince)+"::timestamptz")
	}
	if f.MinMessages > 0 {
		filterPreds = append(filterPreds, "message_count >= "+pb.add(f.MinMessages))
	}
	if f.MaxMessages > 0 {
		filterPreds = append(filterPreds, "message_count <= "+pb.add(f.MaxMessages))
	}
	if f.MinUserMessages > 0 {
		filterPreds = append(filterPreds, "user_message_count >= "+pb.add(f.MinUserMessages))
	}

	oneShotPred := ""
	if f.ExcludeOneShot {
		if f.IncludeChildren {
			oneShotPred = "user_message_count > 1"
		} else {
			filterPreds = append(filterPreds, "user_message_count > 1")
		}
	}

	hasFilters := len(filterPreds) > 0 || oneShotPred != ""
	if !f.IncludeChildren || !hasFilters {
		return strings.Join(append(basePreds, filterPreds...), " AND "), pb.args
	}

	baseWhere := strings.Join(basePreds, " AND ")
	rootMatchParts := append([]string{}, filterPreds...)
	if oneShotPred != "" {
		rootMatchParts = append(rootMatchParts, oneShotPred)
	}
	rootMatch := strings.Join(rootMatchParts, " AND ")
	subqWhere := "message_count > 0 AND deleted_at IS NULL"
	if rootMatch != "" {
		subqWhere += " AND " + rootMatch
	}

	where := baseWhere + " AND (" + rootMatch + " OR parent_session_id IN (SELECT id FROM sessions WHERE " + subqWhere + "))"
	return where, append(append([]any{}, pb.args...), pb.args...)
}

// ListSessions returns a cursor-paginated list of sessions.
func (s *Store) ListSessions(ctx context.Context, f db.SessionFilter) (db.SessionPage, error) {
	if f.Limit <= 0 || f.Limit > db.MaxSessionLimit {
		f.Limit = db.DefaultSessionLimit
	}

	where, args := buildPGSessionFilter(f)
	var total int
	var cur db.SessionCursor
	if f.Cursor != "" {
		var err error
		cur, err = s.DecodeCursor(f.Cursor)
		if err != nil {
			return db.SessionPage{}, err
		}
		total = cur.Total
	}
	if total <= 0 {
		query := "SELECT COUNT(*) FROM sessions WHERE " + where
		if err := s.pg.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
			return db.SessionPage{}, fmt.Errorf("counting sessions: %w", err)
		}
	}

	cursorWhere := where
	cursorArgs := append([]any{}, args...)
	if f.Cursor != "" {
		cursorWhere += fmt.Sprintf(" AND (COALESCE(ended_at, started_at, created_at), id) < (%s, %s)",
			fmt.Sprintf("$%d", len(cursorArgs)+1),
			fmt.Sprintf("$%d", len(cursorArgs)+2),
		)
		cursorArgs = append(cursorArgs, cur.EndedAt, cur.ID)
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM sessions
		WHERE %s
		ORDER BY COALESCE(ended_at, started_at, created_at) DESC, id DESC
		LIMIT $%d`,
		pgSessionCols,
		cursorWhere,
		len(cursorArgs)+1,
	)
	cursorArgs = append(cursorArgs, f.Limit+1)

	rows, err := s.pg.QueryContext(ctx, query, cursorArgs...)
	if err != nil {
		return db.SessionPage{}, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	sessions, err := scanPGSessionRows(rows)
	if err != nil {
		return db.SessionPage{}, err
	}

	page := db.SessionPage{Sessions: sessions, Total: total}
	if len(sessions) > f.Limit {
		page.Sessions = sessions[:f.Limit]
		last := page.Sessions[f.Limit-1]
		endedAt := last.CreatedAt
		if last.StartedAt != nil && *last.StartedAt != "" {
			endedAt = *last.StartedAt
		}
		if last.EndedAt != nil && *last.EndedAt != "" {
			endedAt = *last.EndedAt
		}
		page.NextCursor = s.EncodeCursor(endedAt, last.ID, total)
	}
	return page, nil
}

// GetSession returns a single visible session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (*db.Session, error) {
	row := s.pg.QueryRowContext(ctx, "SELECT "+pgSessionCols+" FROM sessions WHERE id = $1 AND deleted_at IS NULL", id)
	session, err := scanPGSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session %s: %w", id, err)
	}
	return &session, nil
}

// GetChildSessions returns child sessions for a parent.
func (s *Store) GetChildSessions(ctx context.Context, parentID string) ([]db.Session, error) {
	rows, err := s.pg.QueryContext(ctx,
		"SELECT "+pgSessionCols+" FROM sessions WHERE parent_session_id = $1 ORDER BY started_at ASC NULLS LAST, id ASC",
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying child sessions for %s: %w", parentID, err)
	}
	defer rows.Close()
	return scanPGSessionRows(rows)
}

const pgRootSessionFilter = `message_count > 0
	AND relationship_type NOT IN ('subagent', 'fork')
	AND deleted_at IS NULL`

// GetStats returns database statistics for visible root sessions.
func (s *Store) GetStats(ctx context.Context, excludeOneShot bool) (db.Stats, error) {
	filter := pgRootSessionFilter
	if excludeOneShot {
		filter += " AND user_message_count > 1"
	}
	query := fmt.Sprintf(`
		SELECT
			(SELECT COUNT(*) FROM sessions WHERE %s),
			(SELECT COALESCE(SUM(message_count), 0) FROM sessions WHERE %s),
			(SELECT COUNT(DISTINCT project) FROM sessions WHERE %s),
			(SELECT COUNT(DISTINCT machine) FROM sessions WHERE %s),
			(SELECT MIN(COALESCE(started_at, created_at)) FROM sessions WHERE %s)`,
		filter, filter, filter, filter, filter,
	)

	var stats db.Stats
	var earliest *time.Time
	if err := s.pg.QueryRowContext(ctx, query).Scan(
		&stats.SessionCount,
		&stats.MessageCount,
		&stats.ProjectCount,
		&stats.MachineCount,
		&earliest,
	); err != nil {
		return db.Stats{}, fmt.Errorf("fetching stats: %w", err)
	}
	if earliest != nil {
		v := FormatISO8601(*earliest)
		stats.EarliestSession = &v
	}
	return stats, nil
}

// GetProjects returns project names with session counts.
func (s *Store) GetProjects(ctx context.Context, excludeOneShot bool) ([]db.ProjectInfo, error) {
	query := `SELECT project, COUNT(*) AS session_count
		FROM sessions
		WHERE message_count > 0
		  AND relationship_type NOT IN ('subagent', 'fork')
		  AND deleted_at IS NULL`
	if excludeOneShot {
		query += " AND user_message_count > 1"
	}
	query += " GROUP BY project ORDER BY project"

	rows, err := s.pg.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying projects: %w", err)
	}
	defer rows.Close()

	var projects []db.ProjectInfo
	for rows.Next() {
		var project db.ProjectInfo
		if err := rows.Scan(&project.Name, &project.SessionCount); err != nil {
			return nil, fmt.Errorf("scanning project: %w", err)
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

// GetAgents returns distinct agent names with session counts.
func (s *Store) GetAgents(ctx context.Context, excludeOneShot bool) ([]db.AgentInfo, error) {
	query := `SELECT agent, COUNT(*) AS session_count
		FROM sessions
		WHERE message_count > 0
		  AND agent <> ''
		  AND deleted_at IS NULL
		  AND relationship_type NOT IN ('subagent', 'fork')`
	if excludeOneShot {
		query += " AND user_message_count > 1"
	}
	query += " GROUP BY agent ORDER BY agent"

	rows, err := s.pg.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer rows.Close()

	agents := []db.AgentInfo{}
	for rows.Next() {
		var agent db.AgentInfo
		if err := rows.Scan(&agent.Name, &agent.SessionCount); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

// GetMachines returns distinct machine names.
func (s *Store) GetMachines(ctx context.Context, excludeOneShot bool) ([]string, error) {
	query := "SELECT DISTINCT machine FROM sessions WHERE deleted_at IS NULL"
	if excludeOneShot {
		query += " AND user_message_count > 1"
	}
	query += " ORDER BY machine"

	rows, err := s.pg.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying machines: %w", err)
	}
	defer rows.Close()

	var machines []string
	for rows.Next() {
		var machine string
		if err := rows.Scan(&machine); err != nil {
			return nil, fmt.Errorf("scanning machine: %w", err)
		}
		machines = append(machines, machine)
	}
	return machines, rows.Err()
}
