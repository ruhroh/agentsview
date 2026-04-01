# External Integrations

**Analysis Date:** 2026-03-31

## APIs & External Services

**Authentication:**
- Clerk - User authentication for remote/hosted deployments
  - SDK/Client: `github.com/clerk/clerk-sdk-go/v2` (backend), `svelte-clerk ^1.1.1` (frontend)
  - Backend verifier: `internal/server/clerk.go` — validates JWTs via JWKS endpoint
  - Frontend gate: `frontend/src/Root.svelte` wraps `App` in `ClerkProvider` when key is present
  - Auth env: `CLERK_SECRET_KEY`, `CLERK_PUBLISHABLE_KEY` / `VITE_CLERK_PUBLISHABLE_KEY`, `CLERK_AUTHORIZED_PARTIES`
  - Token delivery: `__session` cookie or `Authorization: Bearer` header
  - Optional: when `CLERK_SECRET_KEY` is absent, falls back to static bearer token (`auth_token` in config) or localhost-only open access

**Session Sharing (Self-Hosted):**
- Configurable share server - Sessions pushed from local instance to a hosted server
  - Push endpoint: HTTP POST to `Share.URL` with bearer token
  - Receive endpoint: `internal/server/share_write.go` — `handleUpsertShare`
  - Auth env: `AGENTSVIEW_SHARE_URL`, `AGENTSVIEW_SHARE_TOKEN`, `AGENTSVIEW_SHARE_PUBLISHER`

## Data Storage

**Primary Database:**
- SQLite with WAL mode and FTS5
  - Location: `~/.agentsview/sessions.db` (override via `AGENT_VIEWER_DATA_DIR`)
  - Client: `github.com/mattn/go-sqlite3` (CGO driver)
  - Schema: `internal/db/schema.sql` — sessions, messages, stats, pins, starred, shared tables
  - FTS: `messages_fts` virtual table with porter tokenizer, maintained by SQL triggers
  - Connection: separate writer + read-only pool; WAL mode, 5s busy timeout, 256MB mmap
  - Access interface: `internal/db/store.go` `Store` interface implemented by `internal/db/db.go`

**Optional Database:**
- PostgreSQL 16 (optional, multi-machine sync)
  - Client: `github.com/jackc/pgx/v5 v5.9.1`
  - Connection env: `AGENTSVIEW_PG_URL`, `AGENTSVIEW_PG_SCHEMA`, `AGENTSVIEW_PG_MACHINE`
  - Use: `pg push` syncs SQLite to PG; `pg serve` runs HTTP server backed by PG (read-only)
  - Test container: `postgres:16-alpine` via `docker-compose.test.yml` on port 5433

**File Storage:**
- Local filesystem only — session files read from agent directories on disk
- No cloud file storage

**Caching:**
- None — SQLite is the single source of truth; no Redis or in-memory cache layer

## Authentication & Identity

**Auth Provider:** Clerk (optional) or static bearer token or localhost-only open access

**Implementation:**
- `internal/server/auth.go` — `authMiddleware` applies to all `/api/` routes
- Precedence: Clerk JWT > static bearer token > localhost bypass (when remote access disabled)
- Clerk JWKS keys are fetched on first use and cached in-memory in `ClerkVerifier`
- Frontend reads publishable key from `<meta name="agentsview-clerk-publishable-key">` injected by Go server at runtime (`internal/server/server.go`), or falls back to `VITE_CLERK_PUBLISHABLE_KEY` build-time env var

## Monitoring & Observability

**Error Tracking:**
- None (no Sentry or similar)

**Coverage:**
- Codecov — CI uploads `coverage.out` via `codecov/codecov-action@v5.5.3` (`.github/workflows/ci.yml`); failures are warnings only, not blocking

**Logs:**
- `log` stdlib package — structured only via `log.Printf`; no structured logging library
- Request logging via custom `logMiddleware` in `internal/server/middleware.go`

## CI/CD & Deployment

**Hosting:**
- Local binary (default install to `~/.local/bin/agentsview`)
- Docker container on Railway — `docker-entrypoint.sh` reads `RAILWAY_PUBLIC_DOMAIN` to set `-public-url`
- Dockerfile: multi-stage build (`node:22-slim` → `golang:1.25-bookworm` → `debian:bookworm-slim`)

**CI Pipeline:**
- GitHub Actions (`.github/workflows/ci.yml`)
- Jobs: `lint` (golangci-lint), `test` (ubuntu + windows matrix, with race detector on Linux), `coverage` (Codecov upload), `integration` (PostgreSQL service container), `e2e` (Playwright, Chromium + WebKit)

**Desktop:**
- Tauri wrappers exist (Makefile targets: `desktop-macos-app`, `desktop-macos-dmg`, `desktop-windows-installer`, `desktop-linux-appimage`) — desktop directory not present in current branch

## Webhooks & Callbacks

**Incoming:**
- `POST /api/shares/{shareId}` — receives pushed shared sessions from local agentsview instances (`internal/server/share_write.go`)

**Outgoing:**
- Session share push: local instance POSTs to configured `Share.URL` with JSON payload (`internal/server/share.go`)

## Real-Time Updates

**SSE (Server-Sent Events):**
- `GET /api/sessions/{id}/watch` — streams session change events to frontend
- Implemented in `internal/server/server.go` using `text/event-stream` response type
- Auth: query param `?token=` accepted only on this endpoint for `EventSource` compatibility

## Reverse Proxy

**Managed Caddy (optional):**
- Invoked as subprocess when `-proxy=caddy` flag is set
- Implementation: `cmd/agentsview/managed_caddy.go`
- Supports TLS cert/key, custom bind host, CIDR subnet allowlists
- Public port defaults to 8443

## Agent Session Source Directories

The application reads session data from local filesystem directories for 13 supported AI agents. Each agent directory is configurable via env var (see STACK.md). Default paths relative to `$HOME`:

| Agent | Default Path | Env Var |
|-------|-------------|---------|
| Claude Code | `.claude/projects` | `CLAUDE_PROJECTS_DIR` |
| Codex | `.codex/sessions` | `CODEX_SESSIONS_DIR` |
| Copilot | `.copilot` | `COPILOT_DIR` |
| Gemini | `.gemini` | `GEMINI_DIR` |
| OpenCode | `.local/share/opencode` | `OPENCODE_DIR` |
| Cursor | `.cursor/projects` | `CURSOR_PROJECTS_DIR` |
| Amp | `.local/share/amp/threads` | `AMP_DIR` |
| Zencoder | `.zencoder/sessions` | `ZENCODER_DIR` |
| iFlow | `.iflow/projects` | `IFLOW_DIR` |
| VSCode Copilot | (platform-specific) | `VSCODE_COPILOT_DIR` |
| Pi | `.pi/agent/sessions` | `PI_DIR` |
| OpenClaw | `.openclaw/agents` | `OPENCLAW_DIR` |
| Kimi | `.kimi/sessions` | `KIMI_DIR` |

Agent registry defined in `internal/parser/types.go`.

---

*Integration audit: 2026-03-31*
