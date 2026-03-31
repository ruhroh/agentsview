# Architecture

**Analysis Date:** 2026-03-25

## Pattern Overview

**Overall:** Multi-agent session aggregator with file-watching sync engine, local SQLite storage, and embedded Svelte web UI

**Key Characteristics:**
- Pluggable parser system for 13+ AI agent platforms (Claude, Codex, Copilot, Gemini, Cursor, Amp, etc.)
- File-based discovery and incremental sync with skip caching for performance
- Command-line tool that doubles as an HTTP server with embedded SPA
- Optional PostgreSQL push-sync for multi-machine aggregation
- Real-time updates via Server-Sent Events (SSE)
- Full-text search across all session messages using SQLite FTS5

## Layers

**CLI Entry & Orchestration:**
- Purpose: Command routing, lifecycle management, startup coordination
- Location: `cmd/agentsview/main.go`, `cmd/agentsview/pg.go`, `cmd/agentsview/sync.go`
- Contains: Server startup, signal handling, file watcher coordination
- Depends on: config, db, sync, server, parser, postgres
- Used by: End user executing the `agentsview` binary

**Configuration Layer:**
- Purpose: Load and resolve agent directories, server settings, PostgreSQL credentials
- Location: `internal/config/config.go`
- Contains: TOML/JSON config parsing, environment variable override resolution, flag registration
- Depends on: Nothing external (stdlib only)
- Used by: CLI, server, sync engine

**Sync Engine:**
- Purpose: Orchestrate file discovery, parsing, and database insertion
- Location: `internal/sync/engine.go`, `internal/sync/watcher.go`, `internal/sync/discovery.go`
- Contains: Incremental vs. full resync logic, file watcher integration, per-agent discovery, skip cache
- Depends on: parser, db, config
- Used by: CLI startup, periodic background syncs, file watcher callbacks

**Parser Registry:**
- Purpose: Discover and parse agent-specific session formats
- Location: `internal/parser/` (types.go registry + agent-specific files: claude.go, codex.go, etc.)
- Contains: Per-agent discovery functions, file format parsers, content extraction, fork detection
- Depends on: Nothing external (stdlib only)
- Used by: sync engine, CLI prune/token-use commands

**Database Layer:**
- Purpose: Persist parsed sessions with FTS5 search, handle schema migrations, manage connection pools
- Location: `internal/db/db.go`, `internal/db/sessions.go`, `internal/db/messages.go`, `internal/db/search.go`
- Contains: SQLite operations (WAL mode, mmap, connection pooling), session/message CRUD, analytics queries
- Depends on: sqlite3 (CGO), nothing else
- Used by: sync engine, server, postgres push, CLI commands

**HTTP Server & API:**
- Purpose: Serve REST API endpoints and embedded Svelte 5 SPA
- Location: `internal/server/server.go`, plus ~50 handler files
- Contains: Request routing, session/search handlers, SSE event streaming, authentication, analytics
- Depends on: db, sync, config
- Used by: Browser clients, CLI tools via HTTP

**PostgreSQL Optional Layer:**
- Purpose: Push-sync local SQLite to PostgreSQL for multi-machine access
- Location: `internal/postgres/` (push.go, sync.go, store.go, etc.)
- Contains: Schema management, fingerprinting, push sync lifecycle, read-only store
- Depends on: db, config
- Used by: CLI `pg push` and `pg serve` commands, optional read-only server

**Frontend SPA:**
- Purpose: Interactive web UI for browsing, searching, and analyzing sessions
- Location: `frontend/src/` (Svelte 5, TypeScript)
- Contains: Components for session list, detail view, search, analytics, settings
- Depends on: REST API (fetch), embedded in binary at build time
- Used by: Browser via HTTP server

## Data Flow

**Session Discovery & Sync (File-Based Agents):**

1. CLI calls `sync.NewEngine()` with agent directories
2. Engine loads skip cache from database (tracked by file path + mtime)
3. On startup or periodic interval, `engine.SyncAll()` is called:
   - For each agent type: call agent-specific `DiscoverFunc` to walk directory tree
   - For each discovered file: check if in skip cache (if mtime unchanged, skip)
   - Parse file using agent-specific parser → returns `ParsedSession` struct
   - If parse fails or non-interactive: add to skip cache, continue
   - If successful: insert/update session and messages in database
4. File watcher monitors agent directories, debounced, calls `engine.SyncPaths(paths)` on changes
5. Periodic sync every 15 minutes catches unwatched directories or watcher misses

**Session Insertion Flow:**

1. Parser (e.g., `claudeParser()`) extracts messages from session file
2. Parser generates session ID, project name, timestamps, token counts
3. `sync.Engine` calls `db.UpsertSession(session)` → INSERT OR REPLACE into sessions table
4. For each message: `db.InsertMessage()` → INSERT into messages table
5. FTS5 triggers automatically index message content for full-text search
6. Database triggers update stats table (session/message counts)

**Read & Search Flow:**

1. Browser requests `/api/sessions` → `server.handleSessions()` queries `db.Sessions()` with filters
2. Query uses indexes on agent, project, ended_at, machine
3. Browser searches `/api/search?q=...` → `server.handleSearch()` calls `db.Search()`
4. Search uses FTS5 virtual table → returns ranked message results
5. Response includes session metadata and snippets

**PostgreSQL Push Sync:**

1. CLI calls `pg push` → reads local SQLite sessions/messages
2. Computes fingerprints for each message (hash of content + metadata)
3. Queries PostgreSQL for existing fingerprints → skip known messages
4. Upserts new sessions/messages to PostgreSQL via prepared statements
5. On `pg serve`: server runs in read-only mode, queries PostgreSQL instead of SQLite

**State Management:**

- **Startup lock**: Written before DB open, removed when server is ready (prevents token-use race)
- **State file**: Contains host, port, PID; read by token-use to connect to running server
- **Skip cache**: In-memory map (path → mtime) persisted to database, survives process restart
- **Session data**: Immutable once parsed (messages never deleted, only soft-deleted via sessions.deleted_at)

## Key Abstractions

**AgentDef & Registry:**
- Purpose: Decouple agent-specific logic from sync/server code
- Examples: `internal/parser/types.go:Registry` lists all 13+ agent definitions
- Pattern: Each agent type implements `DiscoverFunc()` and `FindSourceFunc()`, registered in `Registry` slice
- Allows: Sync engine to iterate agents without hardcoding agent types

**Parser Interface (implicit):**
- Purpose: Standardize session parsing across different agent formats
- Pattern: Each agent module (claude.go, codex.go, etc.) exports a Parse function that returns `parser.Session` struct
- Contains: Extraction of messages, metadata, token counts, fork relationships

**db.Store Interface:**
- Purpose: Allow switching between SQLite (`*db.DB`) and PostgreSQL (`*postgres.Store`)
- Examples: Both implement methods like `Sessions()`, `Search()`, `GetSession()`
- Used by: Server accepts `db.Store` parameter, works with either backend
- Location: `internal/db/store.go` (interface definition)

**SSE Event Streaming:**
- Purpose: Notify browser of sync progress in real-time
- Pattern: `server.handleEvents()` accepts HTTP request, keeps connection open
- Sends JSON-encoded `sync.Progress` events every 100ms during sync
- Browser reconnects on close, reads latest progress

**Fork Detection:**
- Purpose: Model branching relationships in Claude sessions
- Pattern: Parser analyzes conversation history, identifies where branches occur
- Stores: `parent_session_id` and `relationship_type` (e.g., "branch") in database
- Used by: Frontend to show session trees, analytics to count session variations

## Entry Points

**CLI Commands:**
- Location: `cmd/agentsview/main.go`
- `agentsview` (default): Start server with sync
- `agentsview serve [flags]`: Explicit serve mode
- `agentsview sync [flags]`: One-shot sync without server
- `agentsview pg push|status|serve`: PostgreSQL commands
- `agentsview token-use <id>`: Token accounting for single session
- `agentsview prune [filters]`: Delete sessions matching criteria

**HTTP Server Startup:**
- Location: `cmd/agentsview/main.go:runServe()`
- Triggers: CLI entry, or default when no subcommand
- Responsibilities:
  1. Load config (env vars, TOML, CLI flags)
  2. Open database (or create if first run)
  3. Create sync engine with agent directories
  4. Start file watcher on agent directories
  5. Run initial/incremental sync
  6. Start HTTP server
  7. Open browser (if -no-browser not set)

**HTTP Request Entry:**
- Location: `internal/server/server.go:routes()`
- Routing: `http.ServeMux` with pattern-based routing (Go 1.22+ style)
- Patterns: `/api/sessions`, `/api/search`, `/api/events`, `/app/*`, static files
- Each pattern has dedicated handler function

**WebSocket/SSE:**
- Location: `internal/server/events.go`
- Endpoint: `/api/events`
- Triggers: Browser opens EventSource connection
- Sends: `sync.Progress` JSON events during sync operations

## Error Handling

**Strategy:** Errors are logged but rarely fatal; sync continues, skipping problematic sessions

**Patterns:**
- Parse errors: File added to skip cache, logged to `debug.log`, sync continues
- Database errors: Returned up stack, may abort sync batch
- File watcher errors: Falls back to polling every 2 minutes
- Network errors (PG push): Logged, sync continues with local data

**Recovery:**
- Stale database (data version): Triggers non-destructive resync (mtime reset, skip cache cleared)
- Crashed resync: Temp files cleaned up at startup
- Startup lock: If state file missing but lock present, waits for lock removal

## Cross-Cutting Concerns

**Logging:**
- Setup: `log` package to `debug.log` in data directory (truncated at 10MB)
- Level: Info/debug via printf, errors via log package
- Rotation: Automatic truncation on startup if exceeds 10MB

**Validation:**
- Agent directories: Checked at startup, warns if missing
- Session ID: Must be non-empty, unique within database
- Message content: Stored as-is, FTS5 handles encoding

**Authentication:**
- When `-public-url` or `-public-origin` set: Optional bearer token generated
- Token stored in config, sent as Authorization header by browser
- Checked by middleware before HTTP handlers

**Performance:**
- Incremental sync: Skip cache keyed by (path, mtime) avoids re-parsing unchanged files
- Batch insert: 100 sessions at a time to reduce transaction overhead
- Worker pool: Up to 8 concurrent file parsers
- FTS5 indexes: Async triggers, tokenized on porter stemmer + unicode61
- Connection pooling: SQLite reader pool with atomic.Pointer swapping

---

*Architecture analysis: 2026-03-25*
