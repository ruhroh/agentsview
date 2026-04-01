# Technology Stack

**Analysis Date:** 2026-03-31

## Languages

**Primary:**
- Go 1.25.5 - Backend server, sync engine, parsers, HTTP API, CLI
- TypeScript 5.9.3 - Frontend SPA (strict mode, ESNext, `noUncheckedIndexedAccess`)

**Secondary:**
- SQL - SQLite schema (`internal/db/schema.sql`), FTS5 virtual tables, triggers
- TOML - Primary config format (`~/.agentsview/config.toml`)

## Runtime

**Environment:**
- Go runtime (CGO required — SQLite uses cgo bindings)
- Node.js >=20.19 (engines: `^20.19 || ^22.12 || >=24`) for frontend build only

**Package Manager:**
- Go modules (`go.mod`, `go.sum`)
- npm with lockfile (`frontend/package-lock.json`) for frontend

**Lockfile:** Present for both (`go.sum`, `frontend/package-lock.json`)

## Frameworks

**Core:**
- Svelte 5.55.0 - Frontend SPA component framework
- `net/http` (stdlib) - HTTP server with `http.ServeMux` (no third-party router)

**Testing:**
- Vitest 4.1.2 - Frontend unit tests (jsdom environment)
- Playwright 1.58.2 - E2E browser tests (Chromium + WebKit)
- `testing` (stdlib) + `github.com/stretchr/testify v1.11.1` - Go unit tests

**Build/Dev:**
- Vite 8.0.3 - Frontend build tool and dev server
- `@sveltejs/vite-plugin-svelte 7.0.0` - Svelte Vite integration
- Make - Primary build orchestrator (`Makefile`)
- golangci-lint v2.10.1 - Go linter (local + CI)
- prek 0.3.6+ - Pre-commit hook manager (`prek.toml`)

## Key Dependencies

**Critical Go:**
- `github.com/mattn/go-sqlite3 v1.14.37` - CGO SQLite driver; requires `CGO_ENABLED=1` and `-tags fts5` build tag
- `github.com/jackc/pgx/v5 v5.9.1` - PostgreSQL driver (optional PG sync feature)
- `github.com/clerk/clerk-sdk-go/v2 v2.5.1` - Clerk JWT verification for remote auth (`internal/server/clerk.go`)
- `github.com/BurntSushi/toml v1.6.0` - Config file parsing (`internal/config/config.go`)
- `github.com/fsnotify/fsnotify v1.9.0` - Cross-platform file system watcher for session directory sync
- `github.com/tidwall/gjson v1.18.0` - JSON path extraction (in `go.mod`)
- `github.com/google/shlex v0.0.0-20191202100458` - Shell-style string splitting
- `golang.org/x/mod v0.34.0` - Go module version utilities

**Critical Frontend:**
- `svelte-clerk ^1.1.1` - Clerk auth UI components (`ClerkProvider`, `SignIn`, `ClerkLoaded`) in `frontend/src/Root.svelte`
- `@tanstack/virtual-core 3.13.23` - Virtualized list rendering for session/message lists
- `dompurify 3.3.3` - HTML sanitization for rendered markdown content
- `marked 17.0.5` - Markdown-to-HTML renderer for message content

**Testing Only:**
- `github.com/google/go-cmp v0.7.0` - Deep equality assertions in Go tests (`internal/db/db_test.go`)
- `github.com/stretchr/testify v1.11.1` - Go test assertions and require helpers

## Configuration

**Environment Variables:**
- `AGENT_VIEWER_DATA_DIR` - Data directory override (default: `~/.agentsview/`)
- `CLAUDE_PROJECTS_DIR` - Override Claude session dir
- `CODEX_SESSIONS_DIR` - Override Codex session dir
- `COPILOT_DIR` - Override Copilot session dir
- `GEMINI_DIR` - Override Gemini session dir
- `OPENCODE_DIR` - Override OpenCode session dir
- `CURSOR_PROJECTS_DIR` - Override Cursor session dir
- `AMP_DIR` - Override Amp session dir
- `ZENCODER_DIR`, `IFLOW_DIR`, `VSCODE_COPILOT_DIR`, `PI_DIR`, `OPENCLAW_DIR`, `KIMI_DIR`
- `AGENTSVIEW_PG_URL` - PostgreSQL connection URL
- `AGENTSVIEW_PG_SCHEMA` - PostgreSQL schema name
- `AGENTSVIEW_PG_MACHINE` - Machine name for PG multi-machine sync
- `AGENTSVIEW_SHARE_URL`, `AGENTSVIEW_SHARE_TOKEN`, `AGENTSVIEW_SHARE_PUBLISHER`
- `CLERK_SECRET_KEY` - Backend Clerk secret for JWT verification
- `CLERK_PUBLISHABLE_KEY` / `VITE_CLERK_PUBLISHABLE_KEY` - Frontend Clerk key (baked into build at compile time)
- `CLERK_AUTHORIZED_PARTIES` - Comma-separated allowed origins for Clerk azp validation
- `PORT` - HTTP listen port (Docker/Railway)
- `RAILWAY_PUBLIC_DOMAIN` - Auto-set public URL on Railway deployments

**Config File:**
- `~/.agentsview/config.toml` (primary, auto-migrated from legacy `config.json`)
- Parsed by `internal/config/config.go` via `BurntSushi/toml`
- Layer order: defaults < config file < env vars < CLI flags

**Build:**
- Mandatory: `CGO_ENABLED=1 -tags fts5` for SQLite FTS5 support
- Frontend baked into binary via `//go:embed` in `internal/web/dist/`
- LDFLAGS inject `version`, `commit`, `buildDate` at build time

## Platform Requirements

**Development:**
- Go 1.25.5+ with CGO enabled
- C compiler (gcc/clang) for sqlite3 CGO binding
- Node.js >=20.19 for frontend development
- Optional: golangci-lint, prek, Docker (for PG integration tests)
- On Windows CI: MinGW64 (MSYS2 `mingw-w64-x86_64-gcc`)

**Production:**
- Single static binary + SQLite database file (`sessions.db` in `~/.agentsview/`)
- Docker image: `debian:bookworm-slim` base with `ca-certificates` (`Dockerfile`)
- Deployment targets: local binary, Docker container, Railway (via `docker-entrypoint.sh`)
- Optional: Caddy reverse proxy (managed mode via `-proxy=caddy` flag, `cmd/agentsview/managed_caddy.go`)
- Optional: PostgreSQL for multi-machine shared access (`pg push`/`pg serve` subcommands)

---

*Stack analysis: 2026-03-31*
