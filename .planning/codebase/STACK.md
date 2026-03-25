# Technology Stack

**Analysis Date:** 2026-03-25

## Languages

**Primary:**
- Go 1.25.5 - Backend server, CLI, sync engine, PostgreSQL integration
- TypeScript 5.9.3 - Frontend application logic and type safety
- JavaScript - Frontend build and utilities

**Secondary:**
- SQL - Database schema and queries (SQLite, PostgreSQL)
- HTML/CSS - Frontend templates and styling (Svelte components)

## Runtime

**Environment:**
- Go 1.25.5 runtime (embedded HTTP server, file watcher, sync orchestration)
- Node.js 20.19+ (frontend build only, not required at runtime)
  - Supported: ^20.19 || ^22.12 || >=24

**Package Manager:**
- Go Modules (go.mod/go.sum) - Backend dependencies
- npm - Frontend dependencies (no lock file specified in Makefile)

## Frameworks

**Core Backend:**
- Standard library (net/http, database/sql, sync, flag, etc.) - HTTP server, database abstraction, concurrency
- No external web framework (raw http.ServeMux routing)

**Frontend:**
- Svelte 5.54.0 - Component framework with reactivity
- Vite 8.0.1 - Development server and build bundler

**Testing:**
- Go: testify (v1.11.1) for assertions and mocking
- Frontend: Vitest 4.1.0 with jsdom - Unit tests
- Playwright 1.58.2 - E2E tests (baseURL: http://127.0.0.1:8090)

**Build/Dev:**
- Makefile - Build orchestration
- golangci-lint - Go linting and analysis (.golangci.yml at v2)
- svelte-check 4.4.5 - Svelte type checking

## Key Dependencies

**Critical Backend:**
- github.com/jackc/pgx/v5 (v5.8.0) - PostgreSQL driver with connection pooling
- github.com/mattn/go-sqlite3 (v1.14.37) - SQLite3 driver via CGO
- github.com/fsnotify/fsnotify (v1.9.0) - File system watcher for session directory changes
- github.com/tidwall/gjson (v1.18.0) - JSON parsing for session file extraction

**Infrastructure:**
- github.com/BurntSushi/toml (v1.6.0) - Configuration file parsing
- github.com/google/shlex (v0.0.0-20191202100458-e7afc7fbc510) - Shell argument parsing for terminal launching
- golang.org/x/mod (v0.34.0) - Module version parsing utilities

**Frontend:**
- @tanstack/virtual-core (3.13.23) - Virtualization for long message lists
- dompurify (3.3.3) - XSS prevention for rendered content
- marked (17.0.4) - Markdown parsing
- @sveltejs/vite-plugin-svelte (7.0.0) - Svelte integration with Vite

## Configuration

**Environment:**
Session discovery via directory configuration:
- `AGENT_VIEWER_DATA_DIR` - Data directory for SQLite DB and cache (default: ~/.config/agentsview)
- `CLAUDE_PROJECTS_DIR` - Claude Code session files
- `CODEX_SESSIONS_DIR` - Codex session files
- `COPILOT_DIR` - Copilot CLI session files
- `GEMINI_DIR` - Gemini CLI session files
- `OPENCODE_DIR` - OpenCode session files
- `AMP_DIR` - Amp session files

Server and feature flags via CLI:
- `-host` (default 127.0.0.1) - HTTP server bind address
- `-port` (default 8080) - HTTP server port
- `-public-url` - For hostname/proxy access
- `-public-origin` - Trusted browser origin for CORS
- `-proxy` - Managed reverse proxy mode (caddy)
- `-tls-cert`, `-tls-key` - HTTPS certificate paths
- `-no-browser` - Disable auto-launch

**Config File:**
- Location: `~/.config/agentsview/config.toml` (TOML format)
- Contains: host, port, auth tokens, PostgreSQL connection, proxy settings, terminal preferences
- Migration from JSON legacy format supported

**Build Configuration:**
- `.golangci.yml` - Linter configuration (fts5 build tag required)
- `frontend/tsconfig.json` - TypeScript configuration
- `frontend/vite.config.ts` - Build output to `dist/`, dev proxy to :8080
- `frontend/svelte.config.js` - Svelte preprocessing with Vite plugin
- `frontend/playwright.config.ts` - E2E test timeout: 20s, baseURL: http://127.0.0.1:8090

## Platform Requirements

**Development:**
- Go 1.25.5 with CGO_ENABLED=1 (sqlite3 driver requires C compilation)
- Node.js 20.19+ for frontend build
- Build tags: `-tags fts5` for SQLite full-text search

**Build Requirements:**
- Go compiler with CGO support
- C compiler (gcc/clang)
- SQLite3 development headers

**Production:**
- Single compiled binary (CGO disabled in release build)
- HTTP server embedded with Svelte SPA (internal/web/dist embedded via go:embed)
- Optional: PostgreSQL 16+ for multi-machine shared access (tested with postgres:16-alpine)

**CI/CD:**
- Docker: postgres:16-alpine for integration tests (docker-compose.test.yml)
- Test environment: PostgreSQL test instance on port 5433 with tmpfs storage
- Build tags: `fts5` required for all tests
- Makefile targets: build, build-release, test, test-postgres, e2e, lint

---

*Stack analysis: 2026-03-25*
