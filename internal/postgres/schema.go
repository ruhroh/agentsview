package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaDDL = `
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS sessions (
    id                  TEXT PRIMARY KEY,
    project             TEXT NOT NULL,
    machine             TEXT NOT NULL,
    agent               TEXT NOT NULL,
    first_message       TEXT,
    display_name        TEXT,
    started_at          TIMESTAMPTZ,
    ended_at            TIMESTAMPTZ,
    message_count       INT NOT NULL DEFAULT 0,
    user_message_count  INT NOT NULL DEFAULT 0,
    parent_session_id   TEXT,
    relationship_type   TEXT NOT NULL DEFAULT '',
    total_output_tokens INT NOT NULL DEFAULT 0,
    peak_context_tokens INT NOT NULL DEFAULT 0,
    deleted_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS messages (
    id              BIGSERIAL PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal         INT NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    timestamp       TIMESTAMPTZ,
    has_thinking    BOOLEAN NOT NULL DEFAULT FALSE,
    has_tool_use    BOOLEAN NOT NULL DEFAULT FALSE,
    content_length  INT NOT NULL DEFAULT 0,
    is_system       BOOLEAN NOT NULL DEFAULT FALSE,
    model           TEXT NOT NULL DEFAULT '',
    token_usage     TEXT NOT NULL DEFAULT '',
    context_tokens  INT NOT NULL DEFAULT 0,
    output_tokens   INT NOT NULL DEFAULT 0,
    search_tsv      TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english', COALESCE(content, ''))
    ) STORED,
    UNIQUE (session_id, ordinal)
);

CREATE TABLE IF NOT EXISTS tool_calls (
    id                    BIGSERIAL PRIMARY KEY,
    session_id            TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    message_ordinal       INT NOT NULL,
    call_index            INT NOT NULL DEFAULT 0,
    tool_name             TEXT NOT NULL,
    category              TEXT NOT NULL,
    tool_use_id           TEXT NOT NULL DEFAULT '',
    input_json            TEXT,
    skill_name            TEXT,
    result_content_length INT,
    result_content        TEXT,
    subagent_session_id   TEXT,
    UNIQUE (session_id, message_ordinal, call_index)
);

CREATE TABLE IF NOT EXISTS tool_result_events (
    id                        BIGSERIAL PRIMARY KEY,
    session_id                TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    tool_call_message_ordinal INT NOT NULL,
    call_index                INT NOT NULL DEFAULT 0,
    tool_use_id               TEXT,
    agent_id                  TEXT,
    subagent_session_id       TEXT,
    source                    TEXT NOT NULL,
    status                    TEXT NOT NULL,
    content                   TEXT NOT NULL,
    content_length            INT NOT NULL DEFAULT 0,
    timestamp                 TIMESTAMPTZ,
    event_index               INT NOT NULL DEFAULT 0,
    UNIQUE (
        session_id,
        tool_call_message_ordinal,
        call_index,
        event_index
    )
);

CREATE TABLE IF NOT EXISTS starred_sessions (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS shared_sessions (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    share_id   TEXT NOT NULL UNIQUE,
    server_url TEXT NOT NULL,
    shared_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_sort
    ON sessions ((COALESCE(ended_at, started_at, created_at)) DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_project
    ON sessions (project);

CREATE INDEX IF NOT EXISTS idx_sessions_machine
    ON sessions (machine);

CREATE INDEX IF NOT EXISTS idx_sessions_agent
    ON sessions (agent);

CREATE INDEX IF NOT EXISTS idx_sessions_parent
    ON sessions (parent_session_id)
    WHERE parent_session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_session_ordinal
    ON messages (session_id, ordinal);

CREATE INDEX IF NOT EXISTS idx_messages_session_role
    ON messages (session_id, role);

CREATE INDEX IF NOT EXISTS idx_messages_search_tsv
    ON messages USING GIN (search_tsv);

CREATE INDEX IF NOT EXISTS idx_sessions_display_name_trgm
    ON sessions USING GIN (display_name gin_trgm_ops)
    WHERE display_name IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_first_message_trgm
    ON sessions USING GIN (first_message gin_trgm_ops)
    WHERE first_message IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tool_calls_session
    ON tool_calls (session_id);

CREATE INDEX IF NOT EXISTS idx_tool_calls_result_content_trgm
    ON tool_calls USING GIN (result_content gin_trgm_ops)
    WHERE result_content IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tool_result_events_session
    ON tool_result_events (session_id);

CREATE INDEX IF NOT EXISTS idx_starred_sessions_created
    ON starred_sessions (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_shared_sessions_shared_at
    ON shared_sessions (shared_at DESC);
`

// EnsureSchema creates the target schema and required tables/extensions/indexes.
func EnsureSchema(ctx context.Context, db *sql.DB, schema string) error {
	quoted, err := quoteIdentifier(schema)
	if err != nil {
		return fmt.Errorf("invalid schema name: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoted); err != nil {
		return fmt.Errorf("creating pg schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, schemaDDL); err != nil {
		return fmt.Errorf("creating pg schema objects: %w", err)
	}
	return nil
}
