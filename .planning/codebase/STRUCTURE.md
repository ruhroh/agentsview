# Codebase Structure

**Analysis Date:** 2026-03-25

## Directory Layout

```
agentsview/
├── cmd/                        # Executable entry points
│   ├── agentsview/             # Main binary (server, sync, pg commands)
│   └── testfixture/            # Test data generator for E2E tests
├── internal/                   # Core packages (not externally importable)
│   ├── config/                 # Configuration loading (TOML, flags, env vars)
│   ├── db/                     # SQLite database operations
│   ├── postgres/               # PostgreSQL integration (optional push-sync)
│   ├── parser/                 # Session file parsers (13+ agent types)
│   ├── server/                 # HTTP server and REST API handlers
│   ├── sync/                   # Sync engine, file watcher, discovery
│   ├── insight/                # Analytics and insights generation
│   ├── timeutil/               # Time parsing utilities
│   ├── web/                    # Embedded frontend assets
│   ├── dbtest/                 # Test database helpers
│   ├── testjsonl/              # Test JSON-L utilities
│   └── update/                 # Self-update logic
├── frontend/                   # Svelte 5 SPA (Vite-based)
│   ├── src/                    # TypeScript/Svelte source
│   │   ├── lib/                # Reusable components and utilities
│   │   │   ├── components/     # UI components (40+ files)
│   │   │   ├── stores/         # Svelte stores (state management)
│   │   │   ├── api/            # API client code
│   │   │   └── utils/          # Shared utilities
│   │   ├── App.svelte          # Root component
│   │   └── main.ts             # Entry point
│   ├── e2e/                    # Playwright E2E tests
│   ├── public/                 # Static assets
│   └── dist/                   # Build output (embedded in binary)
├── scripts/                    # Utility scripts
│   ├── e2e-server.sh           # E2E test server
│   └── changelog.sh            # Changelog generator
├── docs/                       # Documentation
├── .github/                    # GitHub Actions CI/CD
├── .githooks/                  # Pre-commit git hooks
├── go.mod, go.sum              # Go module dependencies
├── Makefile                    # Build automation
├── CLAUDE.md                   # Project conventions and setup
└── .roborev.toml               # RoboRev task definitions
```

## Directory Purposes

**`cmd/agentsview/`:**
- Purpose: Main binary entry point with CLI subcommands
- Contains: main.go (router), pg.go (PostgreSQL commands), sync.go (sync orchestration), serve_runtime.go (server startup)
- Key files: `main.go` (runServe, runSync, runPG routing)
- Tests: *_test.go files for each command module

**`cmd/testfixture/`:**
- Purpose: Generate test session data for E2E/integration tests
- Contains: Creates synthetic Claude/Codex/Copilot sessions in memory
- Used by: E2E tests to seed database

**`internal/config/`:**
- Purpose: Load and resolve application configuration
- Contains: Config struct (65+ fields), TOML parsing, environment variable resolution, flag registration
- Key files: `config.go` (main config logic), `persistence_test.go` (config persistence tests)
- Env vars: CLAUDE_PROJECTS_DIR, CODEX_SESSIONS_DIR, AGENT_VIEWER_DATA_DIR, etc.

**`internal/db/`:**
- Purpose: All SQLite operations and schema management
- Contains: ~10,000 lines across 20 files covering sessions, messages, search, analytics
- Key files:
  - `db.go`: Database open/close, connection pooling, migration
  - `schema.sql`: Schema definition (sessions, messages, FTS5, indexes, triggers)
  - `sessions.go`: Session CRUD, batch upsert, parent-child relationships
  - `messages.go`: Message insert, ordering, filtering
  - `search.go`: FTS5 full-text search queries
  - `analytics.go`: Analytics queries (agent breakdown, token usage, trends)
- Data version: Tracks schema changes; version 6 as of 2026-03
- Tests: 100+ test cases in db_test.go covering all major operations

**`internal/parser/`:**
- Purpose: Parse 13+ agent-specific session formats
- Contains: 47 files, 35K lines covering discovery, parsing, fork detection
- Agent-specific modules:
  - `claude.go`: Claude Code projects (JSON format)
  - `codex.go`: Codex sessions (JSON)
  - `copilot.go`: Copilot CLI sessions
  - `cursor.go`: Cursor editor projects
  - `gemini.go`: Google Gemini CLI
  - `amp.go`: Amp threads
  - `iflow.go`, `kimi.go`, `pi.go`, `openclaw.go`, `vscode_copilot.go`, `zencoder.go`, `opencode.go`
- Registry: `types.go` defines AgentDef and centralized Registry slice
- Utilities:
  - `discovery.go`: File system traversal and discovery per agent
  - `fork_test.go`: Fork detection logic
  - `content.go`: Content extraction and formatting
  - `project.go`: Project metadata extraction (git info, branch)
- Tests: 15+ test files with 30K+ lines of test cases

**`internal/server/`:**
- Purpose: HTTP server and REST API
- Contains: 54 files covering handlers, middleware, state management
- Key files:
  - `server.go`: Router setup, handler registration, graceful shutdown
  - `sessions.go`: GET /api/sessions, session list with filters
  - `search.go`: GET /api/search, full-text search
  - `events.go`: GET /api/events, SSE stream for sync progress
  - `analytics.go`: Analytics endpoints (trends, breakdowns)
  - `export.go`: Session export (JSON, CSV)
  - `resume.go`: Resume sessions in terminal/IDE
  - `auth.go`: Bearer token validation
  - `statefile.go`: State file management (host, port, PID)
- Handler patterns: Each handler is a named function (e.g., `handleSessions()`)
- Middleware: `middleware.go` handles CORS, auth, timeout wrapping

**`internal/sync/`:**
- Purpose: Sync orchestration, file watcher, discovery
- Contains: 14 files covering engine, watcher, discovery, progress tracking
- Key files:
  - `engine.go`: SyncAll(), SyncPaths(), ResyncAll(), skip cache management
  - `watcher.go`: File system watcher (fsnotify-based), debouncing
  - `discovery.go`: Session file discovery per agent type
  - `progress.go`: Progress tracking and reporting
  - `hash.go`: File hashing for change detection
- Skip cache: In-memory map (path → mtime), persisted to database
- Batch size: 100 sessions per transaction
- Max workers: 8 concurrent file parsers

**`internal/postgres/`:**
- Purpose: Optional PostgreSQL support for multi-machine sync
- Contains: 11 files covering connection, schema, push logic, read-only store
- Key files:
  - `push.go`: Push sync logic (fingerprinting, upsert)
  - `sync.go`: Full push sync lifecycle
  - `store.go`: PostgreSQL read-only store (implements db.Store interface)
  - `schema.go`: PostgreSQL schema (DDL)
  - `connect.go`: Connection setup, SSL handling
  - `sessions.go`, `messages.go`, `analytics.go`: Query implementations
- Used by: CLI `pg push` and `pg serve` commands

**`internal/web/`:**
- Purpose: Embedded frontend assets
- Contains: Compiled frontend at `dist/` (embedded via go:embed)
- Build process: Vite builds frontend → copied to `internal/web/dist/` → embedded in binary

**`frontend/src/`:**
- Purpose: Svelte 5 SPA source code
- Structure:
  - `App.svelte`: Root component with routing
  - `lib/components/`: 40+ components organized by feature
    - `layout/`: Header, sidebar, main container
    - `sidebar/`: Session list, filters
    - `content/`: Message display, syntax highlighting
    - `analytics/`: Charts, insights
    - `modals/`: Search, settings, confirm dialogs
    - `command-palette/`: Keyboard shortcuts
    - `pinned/`: Starred sessions
    - `trash/`: Soft-deleted sessions
  - `lib/stores/`: Svelte stores for state (sessions, current selection, filters)
  - `lib/api/`: HTTP client with fetch-based API calls
  - `lib/utils/`: Formatting, date parsing, search utilities

**`frontend/e2e/`:**
- Purpose: Playwright E2E tests
- Contains: Tests for main UI flows (session browsing, search, export)

**`internal/insight/`:**
- Purpose: Analytics and AI-powered insights
- Contains: Analytics queries, insight generation via LLM streaming
- Key files: `analytics.go` (analytics data), `insights.go` (insight generation)

**`internal/timeutil/`:**
- Purpose: Time parsing utilities
- Contains: Helpers for parsing agent-specific timestamp formats

**`scripts/`:**
- Purpose: Build and testing utilities
- Files:
  - `e2e-server.sh`: Start server in E2E test mode
  - `changelog.sh`: Generate changelog from git log

## Key File Locations

**Entry Points:**
- `cmd/agentsview/main.go`: CLI routing and server startup
- `cmd/agentsview/pg.go`: PostgreSQL command handlers
- `internal/server/server.go`: HTTP server setup and routing
- `frontend/src/main.ts`: Frontend entry point

**Configuration:**
- `internal/config/config.go`: Config struct and loading logic
- `~/.agentsview/config.toml`: User config file (TOML format)
- Env vars: `CLAUDE_PROJECTS_DIR`, `CODEX_SESSIONS_DIR`, `AGENT_VIEWER_DATA_DIR`, etc.

**Core Logic:**
- `internal/sync/engine.go`: Sync orchestration
- `internal/parser/types.go`: Agent registry and types
- `internal/parser/claude.go`: Claude parser (most complex agent)
- `internal/db/db.go`: Database operations
- `internal/db/schema.sql`: Schema definition

**Testing:**
- `cmd/agentsview/sync_test.go`: Sync integration tests
- `internal/db/db_test.go`: Comprehensive database tests (120K+ lines)
- `internal/parser/parser_test.go`: Parser unit tests
- `internal/server/server_test.go`: Server/API tests (70K+ lines)
- `frontend/e2e/`: Playwright E2E tests

## Naming Conventions

**Files:**
- Go source: `snake_case.go` (e.g., `session_mgmt.go`)
- Tests: `*_test.go` suffix (e.g., `parser_test.go`)
- Integration tests: `*_integration_test.go` suffix
- Platform-specific: `*_unix.go`, `*_windows.go`, `*_darwin.go` suffixes
- Svelte: `PascalCase.svelte` for components (e.g., `App.svelte`, `SessionList.svelte`)

**Directories:**
- Go packages: lowercase, no underscores (e.g., `internal/parser`, `internal/server`)
- Feature directories: plural nouns (e.g., `components/`, `stores/`, `utils/`)
- Test utilities: `*test/` suffix (e.g., `internal/dbtest/`)

**Go Functions & Variables:**
- Public (exported): PascalCase (e.g., `SyncAll()`, `ParseSession()`)
- Private (unexported): camelCase (e.g., `syncWorker()`, `parseMessages()`)
- Constants: UPPER_SNAKE_CASE (e.g., `maxWorkers`, `periodicSyncInterval`)
- Interfaces: PascalCase ending in `-er` or `-able` (e.g., `Store`, `rowScanner`)

**TypeScript/Svelte:**
- Components: PascalCase (e.g., `SessionList`, `SearchModal`)
- Functions: camelCase (e.g., `fetchSessions()`, `formatDate()`)
- Constants: UPPER_SNAKE_CASE (e.g., `API_BASE_URL`)
- Types: PascalCase (e.g., `Session`, `SearchResult`)
- Stores: camelCase with `store` suffix (e.g., `sessionStore`, `filterStore`)

## Where to Add New Code

**New Parser (for new agent type):**
- File: `internal/parser/newagent.go`
- Pattern: Implement `Discover<Agent>()` and `<Agent>Parser()` functions
- Register: Add to `Registry` slice in `internal/parser/types.go`
- Tests: Create `internal/parser/newagent_test.go` with sample session files

**New API Endpoint:**
- Handler file: `internal/server/newfeature.go` (if grouping related handlers)
- Or: Add to existing handler file if logically related (e.g., add to `sessions.go`)
- Route: Register in `internal/server/server.go:routes()` via `mux.HandleFunc()`
- Tests: Add to `internal/server/server_test.go`

**New Frontend Feature:**
- Component: `frontend/src/lib/components/feature/<Component>.svelte`
- Store (if needed): `frontend/src/lib/stores/featureStore.ts`
- API integration: `frontend/src/lib/api/featureApi.ts` (if new API calls)
- Tests: Colocated `*.test.ts` file or Playwright spec in `frontend/e2e/`

**New Database Query:**
- File: `internal/db/newqueries.go` (group by domain)
- Method receiver: `func (db *DB) QueryName() { ... }`
- Tests: Add to `internal/db/db_test.go` with table-driven tests

**New CLI Command:**
- Handler file: `cmd/agentsview/newcmd.go`
- Entry: Add case in `main.go` switch statement
- Flag setup: Use `flag.NewFlagSet()` pattern
- Tests: Create `cmd/agentsview/newcmd_test.go`

**Configuration Keys:**
- TOML: Add field to `Config` struct in `internal/config/config.go`
- Env var: Define in `internal/config/config.go:Load()` resolution logic
- Flag: Register in `RegisterServeFlags()` or appropriate function

**Utilities:**
- Shared helpers: `internal/utilities/` or add to existing `internal/timeutil/`
- Math, string utils: Add functions directly to relevant package

## Special Directories

**`.planning/`:**
- Purpose: Generated GSD analysis documents (this directory)
- Contains: ARCHITECTURE.md, STRUCTURE.md, CONVENTIONS.md, TESTING.md, STACK.md, INTEGRATIONS.md, CONCERNS.md
- Generated: Yes (via GSD mapping tools)
- Committed: Yes (documents are version-controlled)

**`frontend/dist/`:**
- Purpose: Built frontend SPA (HTML, CSS, JS)
- Generated: Yes (by `make frontend`)
- Committed: No (.gitignore excludes it)
- Embedded: Copied into Go binary at build time via `go:embed` in `internal/web/web.go`

**`internal/parser/testdata/`:**
- Purpose: Sample session files for parser tests
- Contains: Real/synthetic session data for each agent type
- Used by: Parser tests to validate parsing logic
- Committed: Yes

**`scripts/`:**
- Purpose: Build and test automation
- Committed: Yes (bash scripts)

**`.githooks/`:**
- Purpose: Pre-commit hooks (installed via `make install-hooks`)
- Contains: Code formatting, linting checks
- Used by: Git pre-commit trigger

---

*Structure analysis: 2026-03-25*
