# Codebase Structure

**Analysis Date:** 2026-03-31

## Directory Layout

```
agentsview/
├── cmd/
│   ├── agentsview/          # Go server binary entrypoint
│   │   ├── main.go          # CLI dispatch, server startup
│   │   ├── serve_runtime.go # Port discovery, Caddy wiring, wait loop
│   │   ├── managed_caddy.go # Managed Caddy reverse proxy
│   │   └── pg.go            # pg subcommand group (push, status, serve)
│   └── testfixture/         # Test data generator for E2E tests
├── internal/
│   ├── config/              # Config loading: TOML, env vars, CLI flags
│   ├── db/                  # SQLite store: schema, sessions, messages, search, analytics
│   │   └── schema.sql       # Embedded DDL (sessions, messages, tool_calls, etc.)
│   ├── dbtest/              # Shared test helpers (OpenTestDB, WriteTestFile)
│   ├── parser/              # Agent definitions + parsed domain types
│   │   └── types.go         # AgentDef, Registry, ParsedSession, ParsedMessage
│   ├── postgres/            # PostgreSQL: push sync, read-only store, schema
│   ├── server/              # HTTP handlers, middleware, auth, Clerk, share
│   ├── sync/                # Sync engine, file watcher, session discovery
│   ├── testjsonl/           # JSONL test fixture helpers
│   ├── timeutil/            # Time parsing utilities
│   └── web/                 # Embedded frontend (dist/ copied at build time)
│       ├── embed.go         # //go:embed all:dist
│       └── dist/            # Compiled Svelte SPA (generated, committed)
├── frontend/                # Svelte 5 SPA source
│   ├── src/
│   │   ├── main.ts          # SPA entry point
│   │   ├── App.svelte       # Root component
│   │   ├── Root.svelte      # Auth gate / outer shell
│   │   └── lib/
│   │       ├── api/         # REST client + TypeScript types
│   │       │   ├── client.ts
│   │       │   └── types/   # core.ts, analytics.ts, insights.ts, etc.
│   │       ├── components/  # Svelte UI components
│   │       │   ├── layout/      # AppHeader, ThreeColumnLayout, breadcrumb
│   │       │   ├── sidebar/     # SessionList, SessionItem
│   │       │   ├── content/     # MessageList, MessageContent, ToolBlock, etc.
│   │       │   ├── modals/      # AboutModal, ShortcutsModal, ConfirmDeleteModal
│   │       │   └── command-palette/ # CommandPalette
│   │       ├── stores/      # Svelte 5 rune-based state stores
│   │       ├── utils/       # Pure TypeScript helpers
│   │       └── virtual/     # Virtual list (createVirtualizer)
│   ├── e2e/                 # Playwright E2E test specs
│   └── dist/                # Frontend build output (source of truth for internal/web/dist)
├── scripts/                 # Release, install, E2E server, changelog scripts
├── docs/                    # Documentation files
├── .github/workflows/       # GitHub Actions CI
├── Makefile                 # Build, test, install, lint targets
├── go.mod                   # Go module (github.com/wesm/agentsview, go 1.25.5)
├── go.sum
├── CLAUDE.md                # Project instructions for AI agents
└── README.md
```

## Directory Purposes

**`cmd/agentsview/`:**
- Purpose: Binary entrypoint only — wiring, startup, shutdown, CLI subcommands
- Contains: `main.go`, `serve_runtime.go`, `managed_caddy.go`, `pg.go` and their test files
- Key files: `main.go` (startup sequence), `serve_runtime.go` (port discovery + Caddy lifecycle)
- Rule: No business logic here; delegate to `internal/` packages

**`internal/config/`:**
- Purpose: All configuration loading — TOML config file, env var overrides, CLI flag registration
- Key files: `config.go` — defines `Config` struct, `Load()`, `RegisterServeFlags()`
- Note: `AgentDirs` map uses `parser.AgentType` keys; zero value is safe (defaults applied during load)

**`internal/db/`:**
- Purpose: SQLite store — schema management, CRUD, FTS5 search, analytics, migrations
- Key files: `db.go` (open/init/migrate), `store.go` (Store interface), `schema.sql` (DDL), `sessions.go`, `messages.go`, `search.go`, `analytics.go`
- Tables: `sessions`, `messages`, `tool_calls`, `tool_result_events`, `insights`, `pinned_messages`, `starred_sessions`, `excluded_sessions`, `skipped_files`, `pg_sync_state`, `shared_sessions`
- `Store` interface in `store.go` must be implemented by any DB backend

**`internal/dbtest/`:**
- Purpose: Shared test helpers — `OpenTestDB(t)`, `WriteTestFile(t, path, content)`, `Ptr[T](v)`
- Used by: all packages with DB tests

**`internal/parser/`:**
- Purpose: Agent type definitions and parsed domain types; source file parsing logic
- Key file: `types.go` — `AgentDef`, `Registry` (all agents), `ParsedSession`, `ParsedMessage`, `ParseResult`
- Only one source file currently (`types.go`) — parsers (claude.go, codex.go, etc.) exist in local-mode build variants

**`internal/postgres/`:**
- Purpose: Optional PostgreSQL integration — read-only `Store` implementation, push sync from SQLite, schema management
- Key files: `store.go` (read-only Store), `push.go` (sync logic), `schema.go` (DDL), `connect.go` (DSN helpers)

**`internal/server/`:**
- Purpose: HTTP router, all REST API handlers, middleware, auth, Clerk integration, share push/receive
- Key files: `server.go` (router + middleware chain), `auth.go` (auth middleware), `clerk.go` (Clerk JWT verification), `sessions.go`, `messages.go`, `search.go`, `metadata.go`, `starred.go`, `share.go`, `share_write.go`, `response.go` (helpers), `params.go` (query param parsing), `statefile.go` (process state)

**`internal/sync/`:**
- Purpose: Sync engine, file watcher, session discovery (used in local mode builds only)
- Key files: `engine.go` (sync orchestration)

**`internal/web/`:**
- Purpose: Embed the compiled frontend into the Go binary
- Key files: `embed.go` (single `//go:embed all:dist` directive), `dist/` (generated, committed)
- Note: `dist/` is populated by `make frontend` and then committed; the binary reads it at startup

**`frontend/src/lib/stores/`:**
- Purpose: Svelte 5 rune-based class stores — all client-side reactive state
- Files: `sessions.svelte.ts`, `messages.svelte.ts`, `sync.svelte.ts`, `router.svelte.ts`, `starred.svelte.ts`, `search.svelte.ts`, `ui.svelte.ts`, `settings.svelte.ts`, `pins.svelte.ts`, `shared.svelte.ts`
- Pattern: Each store is a class with `$state` fields exported as a singleton (`export const sessions = new SessionsStore()`)

**`frontend/src/lib/api/`:**
- Purpose: All HTTP calls to the Go server
- Key files: `client.ts` (all API functions, auth token handling, base URL resolution), `types/core.ts` (Session, Message, ToolCall, etc.), `types/analytics.ts`, `types/insights.ts`

**`frontend/src/lib/components/`:**
- Grouped by UI region: `layout/` (shell chrome), `sidebar/` (session list), `content/` (message viewer), `modals/`, `command-palette/`

**`frontend/src/lib/utils/`:**
- Purpose: Pure TypeScript helpers — content parsing, formatting, markdown rendering, keyboard shortcuts, CSV export, clipboard, poll, transcript-mode, tool-params, model detection
- All files have colocated `.test.ts` test files

## Key File Locations

**Entry Points:**
- `cmd/agentsview/main.go`: Go binary startup, CLI dispatch
- `frontend/src/main.ts`: Svelte SPA mount
- `frontend/src/App.svelte`: Root Svelte component

**Configuration:**
- `internal/config/config.go`: `Config` struct and loader
- `.env.example`: Required environment variable documentation
- `Makefile`: All build/test/install targets

**Core Logic:**
- `internal/db/store.go`: `Store` interface — the contract between server and storage
- `internal/db/schema.sql`: Embedded DDL — source of truth for DB schema
- `internal/db/db.go`: DB open, init, migration, `Update()` / `Reader()`
- `internal/server/server.go`: HTTP router, middleware chain, `routes()`
- `internal/server/auth.go`: Auth middleware (Clerk + Bearer token)
- `internal/parser/types.go`: `AgentDef`, `Registry`, all domain types

**Testing:**
- `internal/dbtest/dbtest.go`: Shared DB test helpers
- `internal/testjsonl/testjsonl.go`: JSONL fixture helpers
- `frontend/e2e/`: Playwright E2E specs
- `scripts/e2e-server.sh`: E2E server launcher

## Naming Conventions

**Go Files:**
- `snake_case.go` for all Go source files
- Platform-specific: `boottime_darwin.go`, `process_unix.go` (GOOS suffix)
- Test files: `foo_test.go` (same package or `package foo_test`)
- Internal tests use `package server` (same package); external tests use `package server_test`

**Go Packages:**
- Single-word, lowercase, matching the directory name: `db`, `server`, `config`, `parser`, `sync`
- Test helper packages: `dbtest`, `testjsonl`

**TypeScript / Svelte Files:**
- Svelte stores: `camelCase.svelte.ts` (e.g., `sessions.svelte.ts`, `ui.svelte.ts`)
- Svelte components: `PascalCase.svelte` (e.g., `MessageList.svelte`, `SessionItem.svelte`)
- Utility modules: `kebab-case.ts` (e.g., `content-parser.ts`, `display-items.ts`)
- Test files colocated: `foo.test.ts` alongside `foo.ts`

**API Types:**
- TypeScript interfaces mirror Go struct field names exactly (snake_case JSON) to avoid manual mapping

## Where to Add New Code

**New REST API endpoint:**
- Handler function: `internal/server/` (add to appropriate file or create a new one)
- Register route: `internal/server/server.go` in `routes()`
- DB query (if needed): `internal/db/` in the relevant file
- Add to `Store` interface: `internal/db/store.go`
- TypeScript client function: `frontend/src/lib/api/client.ts`
- TypeScript type: `frontend/src/lib/api/types/core.ts` (or appropriate sub-file)

**New Svelte UI feature:**
- Store state: `frontend/src/lib/stores/` (add to existing store or create new `foo.svelte.ts`)
- Component: `frontend/src/lib/components/{layout,sidebar,content,modals}/` (choose appropriate group)
- Utility: `frontend/src/lib/utils/foo.ts` + colocated `foo.test.ts`

**New supported agent:**
- Add `AgentDef` entry to `Registry` in `internal/parser/types.go`
- Add parser file: `internal/parser/{agent}.go` (for local-mode file parsing)
- Add env var to `.env.example`

**New DB table or column:**
- Add to `internal/db/schema.sql` (new tables) or add `ALTER TABLE` migration to `migrateColumns` in `internal/db/db.go` (new columns)
- Update probe list in `needsSchemaRebuild` if the column is required for the schema to function
- Add corresponding query functions to the relevant `internal/db/*.go` file
- Add method to `Store` interface in `internal/db/store.go`
- Add stub returning `ErrReadOnly` to the PostgreSQL store in `internal/postgres/store.go`

**New config option:**
- Add field to `Config` in `internal/config/config.go`
- Register CLI flag in `RegisterServeFlags`
- Document in `.env.example` if env-var-backed

## Special Directories

**`internal/web/dist/`:**
- Purpose: Compiled Svelte SPA assets served by the Go binary
- Generated: Yes (by `make frontend` / `npm run build` in `frontend/`)
- Committed: Yes — the binary must embed the built frontend; CI builds from committed dist

**`frontend/dist/`:**
- Purpose: Vite build output (source of truth; copied into `internal/web/dist/`)
- Generated: Yes
- Committed: No (`.gitignore`d); used only as Vite's output directory before copying

**`.planning/codebase/`:**
- Purpose: AI-generated codebase analysis documents consumed by planning and execution tools
- Generated: Yes (by `gsd:map-codebase`)
- Committed: Yes

**`scripts/`:**
- Purpose: Release automation, install scripts, E2E server, changelog generation
- Key files: `e2e-server.sh` (starts server for Playwright), `release.sh`, `install.sh`

---

*Structure analysis: 2026-03-31*
