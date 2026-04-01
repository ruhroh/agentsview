# Architecture

**Analysis Date:** 2026-03-31

## Pattern Overview

**Overall:** Monolithic server with embedded SPA, read-only REST API

**Key Characteristics:**
- Single Go binary embeds the compiled Svelte 5 SPA via `//go:embed` and serves it alongside the REST API
- SQLite (WAL mode, FTS5) is the local primary store; a read-only PostgreSQL store can be swapped in for hosted deployments via the `db.Store` interface
- The HTTP server depends only on `db.Store`, allowing SQLite and Postgres implementations to be substituted without changing handler code
- No sync engine in the hosted (`serve`) mode — binary is a pure read-only server + SPA

## Layers

**Entry Point / CLI:**
- Purpose: Parse flags, load config, open DB, wire server, handle signals
- Location: `cmd/agentsview/main.go`, `cmd/agentsview/serve_runtime.go`
- Contains: `runServe`, `mustLoadConfig`, `mustOpenDB`, port discovery, optional managed Caddy startup, startup lock and state file management
- Depends on: `internal/config`, `internal/db`, `internal/server`
- Used by: OS process

**Config:**
- Purpose: Unified configuration from TOML files, env vars, and CLI flags
- Location: `internal/config/config.go`
- Contains: `Config` struct, `Load`, `RegisterServeFlags`; supports per-agent directory maps, proxy settings, Clerk, PG, and share configs
- Depends on: `internal/parser` (for `AgentType` keys in `AgentDirs` map)
- Used by: `cmd/agentsview`, `internal/server`

**HTTP Server:**
- Purpose: Route all REST API calls, serve embedded SPA, enforce auth/CORS/CSP
- Location: `internal/server/server.go` (router + middleware), `internal/server/sessions.go`, `internal/server/messages.go`, `internal/server/search.go`, `internal/server/metadata.go`, `internal/server/starred.go`, `internal/server/share.go`, `internal/server/share_write.go`
- Contains: `Server` struct with `routes()`, `Handler()`, middleware chain, per-route handler funcs
- Depends on: `internal/db` (via `db.Store` interface), `internal/web` (embedded assets), `internal/config`
- Used by: `cmd/agentsview`

**Storage / DB:**
- Purpose: All SQLite reads, writes, migrations, and FTS5 search
- Location: `internal/db/db.go` (open/init/migrations), `internal/db/sessions.go`, `internal/db/messages.go`, `internal/db/search.go`, `internal/db/analytics.go`, `internal/db/insights.go`, `internal/db/starred.go`, `internal/db/shared.go`, `internal/db/store.go`
- Contains: `DB` struct (writer + reader pool via `atomic.Pointer[sql.DB]`), `Store` interface, schema SQL embedded via `//go:embed schema.sql`
- Depends on: `github.com/mattn/go-sqlite3` (CGO required)
- Used by: `internal/server`, `cmd/agentsview`

**Parser / Domain Types:**
- Purpose: Define agent types, domain types, and parse session files into `ParseResult`
- Location: `internal/parser/types.go`
- Contains: `AgentDef`, `Registry` (all supported agents), `ParsedSession`, `ParsedMessage`, `ParsedToolCall`, `ParseResult`, `InferRelationshipTypes`
- Depends on: nothing external
- Used by: config loader, sync engine (when present in local mode)

**Frontend SPA:**
- Purpose: Svelte 5 read-only browser UI — session browser, message viewer, search, analytics, settings
- Location: `frontend/src/` (source), `internal/web/dist/` (compiled, embedded into binary at build time)
- Contains: `App.svelte` (root), Svelte 5 rune-based class stores in `frontend/src/lib/stores/`, REST API client in `frontend/src/lib/api/client.ts`, typed API types in `frontend/src/lib/api/types/`, components grouped under `frontend/src/lib/components/{layout,sidebar,content,modals,command-palette}/`
- Depends on: Go server REST API at `/api/v1/`
- Used by: browser clients

**Embedded Web Assets:**
- Purpose: Bundle the compiled frontend into the Go binary without separate file serving
- Location: `internal/web/embed.go`
- Contains: `//go:embed all:dist` and `Assets() fs.FS` function
- Used by: `internal/server/server.go` via `web.Assets()`

## Data Flow

**Session List Request:**

1. Browser `GET /api/v1/sessions?project=…&cursor=…` hits `handleListSessions` in `internal/server/sessions.go`
2. Middleware chain runs in order: CSP → auth → host-check → CORS → log → handler
3. Handler parses query params via `internal/server/params.go` helpers, builds `db.SessionFilter`, calls `s.db.ListSessions(ctx, filter)`
4. `db.ListSessions` in `internal/db/sessions.go` executes parameterized SQL against the reader pool, returns `SessionPage` with cursor
5. Handler calls `writeJSON` in `internal/server/response.go` — response written

**SPA / Index Delivery:**

1. Any unmatched path falls to `handleSPA` in `internal/server/server.go`
2. `serveIndex` reads `index.html` from embedded `fs.FS`, injects `<meta name="agentsview-clerk-publishable-key">` and optional `<base href>` for reverse-proxy base-path deployments
3. Static assets (JS/CSS/fonts) are served directly from the embedded FS by `http.FileServerFS`

**Authentication Flow:**

1. `authMiddleware` in `internal/server/auth.go` runs on every `/api/` request
2. Localhost requests bypass auth when remote access is disabled
3. Remote requests must present either a Clerk session JWT (verified by `ClerkVerifier` in `internal/server/clerk.go` via JWKS) or a static Bearer token stored in config
4. Auth context key `ctxKeyRemoteAuth` is set to `true` on success; downstream host-check and CORS middleware relax restrictions for authenticated remote clients

**Share Push Flow:**

1. Local instance calls `handleShareSession` in `internal/server/share.go` → pushes `sharePayload` (session + messages, local file metadata stripped) via HTTP PUT to the hosted server `/api/v1/shares/{shareId}`
2. Hosted server `handleUpsertShare` in `internal/server/share_write.go` validates the token, calls `s.db.UpsertSession` + `s.db.ReplaceSessionMessages`

**Frontend State Management:**

- Svelte 5 rune-based class stores (`.svelte.ts` files) in `frontend/src/lib/stores/`
- `sessions.svelte.ts` is the central store — manages active session, filter state, pagination cursor, child session map
- `sync.svelte.ts` is a stub in hosted viewer (no local sync); `watchSession` is a no-op
- `router.svelte.ts` manages URL-based routing with routes: `sessions`, `insights`, `pinned`, `trash`, `settings`
- All stores call `frontend/src/lib/api/client.ts` functions for HTTP requests; server URL and auth token stored in `localStorage`

## Key Abstractions

**`db.Store` (interface):**
- Purpose: Decouple HTTP handlers from the storage backend; both SQLite and PostgreSQL implement it
- Definition: `internal/db/store.go`
- Implementations: `*db.DB` (SQLite, `internal/db/db.go`), PostgreSQL read-only store (`internal/postgres/store.go`)
- Pattern: Write methods (stars, pins, shares, session management) return `db.ErrReadOnly` in the PG implementation, mapped to HTTP 501 by `handleReadOnly`

**`parser.AgentDef` + `parser.Registry`:**
- Purpose: Single source of truth for all supported agents — filesystem layout, env vars, ID prefixes, watch subdirs
- Location: `internal/parser/types.go`
- Pattern: `AgentByType(t)` and `AgentByPrefix(sessionID)` for lookup; `Registry` slice for iteration

**`db.DB` (dual-connection pool):**
- Purpose: Serialized writes (one connection, `sync.Mutex`) + concurrent reads (pool of 4, `atomic.Pointer[sql.DB]`) enabling lock-free concurrent reads by HTTP handlers
- Location: `internal/db/db.go`
- Pattern: `Update(fn func(*sql.Tx) error)` wraps all writes in a transaction; `Reader()` exposes the read pool; `Reopen()` swaps both connections atomically (used after a fresh DB swap)

**Middleware Chain (`internal/server/server.go`):**
- Pattern: Layered `http.Handler` wrappers applied in `Handler()`: `cspMiddleware` → `authMiddleware` → `hostCheckMiddleware` → `corsMiddleware` → `logMiddleware` → `s.mux`
- `withTimeout` wraps individual handlers with `http.TimeoutHandler` + JSON error body

## Entry Points

**`cmd/agentsview/main.go`:**
- Location: `cmd/agentsview/main.go`
- Triggers: `agentsview [serve] [flags]` CLI invocation
- Responsibilities: Dispatch to `runServe` → load config → open DB → write startup lock → create `server.Server` → auto-discover port → start HTTP listener → optionally start managed Caddy proxy → write state file

**`frontend/src/main.ts`:**
- Location: `frontend/src/main.ts`
- Triggers: Browser loads `index.html`
- Responsibilities: Mount `App.svelte`, initialize Svelte 5 reactive tree

**`internal/server/server.go` `routes()`:**
- Location: `internal/server/server.go:129`
- Triggers: Called inside `server.New()` constructor
- Responsibilities: Register all `/api/v1/` routes and the SPA fallback on `http.ServeMux`

## Error Handling

**Strategy:** Errors propagate as Go `error` returns; handlers use shared helpers from `internal/server/response.go`

**Patterns:**
- `writeError(w, status, msg)` writes `{"error": "..."}` JSON for all API error responses
- `handleContextError(w, err)` handles `context.Canceled` (client disconnected, silently ignored) and `context.DeadlineExceeded` (504 response)
- `handleReadOnly(w, err)` maps `db.ErrReadOnly` to HTTP 501 for write endpoints unavailable in remote/PG mode
- DB errors not matching above cases return HTTP 500; details logged to `debug.log`
- Fatal startup errors use `fatal(format, args...)` which writes to stderr (after log is redirected to the log file)

## Cross-Cutting Concerns

**Logging:** `log` stdlib, redirected to `~/.agentsview/debug.log` at startup (via `setupLogFile`); file truncated to 10 MB on each startup. Only `/api/` requests are logged via `logMiddleware`

**Validation:** Query parameter parsing centralized in `internal/server/params.go` — `parseIntParam`, `isValidDate`, `isValidTimestamp`, `clampLimit`. All handlers call these helpers before touching business logic

**Authentication:** `authMiddleware` in `internal/server/auth.go`; Clerk JWT via `ClerkVerifier` (`internal/server/clerk.go`) or static Bearer token. Localhost bypass when remote access is disabled

**Security:** DNS rebinding prevention via `hostCheckMiddleware`; CSRF prevention via `corsMiddleware` (blocks mutating requests with unrecognized Origin header); strict Content-Security-Policy injected into every non-API HTML response via `cspMiddleware`; `<meta name="agentsview-clerk-publishable-key">` injected at serve time (never hardcoded in source)

**Schema migrations:** Non-destructive. `db.Open` calls `probeDatabase` to check required columns (schema probe) and `user_version` (data version). Missing columns trigger a full DB drop+recreate. Stale `user_version` sets `dataStale=true` (triggers re-sync without data loss). `migrateColumns` idempotently adds new columns via `ALTER TABLE`

---

*Architecture analysis: 2026-03-31*
