# External Integrations

**Analysis Date:** 2026-03-25

## APIs & External Services

**GitHub:**
- Release update checking - Polls api.github.com for latest release metadata
  - SDK/Client: Standard library http.Client
  - Endpoint: `https://api.github.com/repos/wesm/agentsview/releases/latest`
  - Auth: Optional GitHub token via `github_token` config for higher rate limits
  - Used by: `internal/update/update.go` for version checks and `internal/server/export.go` for GitHub Gist export

**GitHub Gists:**
- Session export destination - Allows exporting session transcripts
  - SDK/Client: Standard library http.Client
  - Endpoint: `https://api.github.com/gists`
  - Auth: GitHub token required (stored in config as `github_token`)
  - Usage: `POST` to create gists with session content
  - See: `internal/server/export.go` for Gist creation logic

## Data Storage

**Databases:**

**SQLite (Local):**
- Type: Embedded SQL database with WAL mode
- Purpose: Primary data store for all session data
- Client: `github.com/mattn/go-sqlite3` v1.14.37 (CGO-based)
- Location: `~/.config/agentsview/agentsview.db` (or custom AGENT_VIEWER_DATA_DIR)
- Configuration:
  - Journal mode: WAL (Write-Ahead Logging)
  - Busy timeout: 5 seconds
  - Foreign keys: Enabled
  - Memory mapping: 256MB
  - Cache: 64MB
  - Build tag: `-tags fts5` required for full-text search (FTS5 virtual table)
- Connection pooling:
  - Write: Single connection with mutex serialization
  - Read: Pool with atomic pointer swapping for graceful reconnection
- Schema: See `internal/db/schema.sql`
  - Tables: sessions, messages, stats, tool_calls, insights, pinned_messages
  - FTS5 virtual table: messages_fts for full-text search
  - Triggers: Auto-update stats on insert/delete

**PostgreSQL (Optional - Multi-Machine):**
- Type: Relational database for shared access across machines
- Version: 16+ tested (docker-compose uses postgres:16-alpine)
- Purpose: Read-only view of session data synced from SQLite; optional shared analytics
- Client: `github.com/jackc/pgx/v5` v5.8.0 with standard library sql.DB adapter
- Connection setup:
  - Config: `internal/postgres/connect.go`
  - DSN parser: Uses pgx driver to resolve effective host and TLS
  - SSL enforcement: Required for non-loopback hosts (sslmode=require or verify-full)
  - Override: `allow_insecure = true` in [pg] config to disable SSL check (not recommended)
- Schema management:
  - Location: `internal/postgres/schema.go`
  - DDL creation at sync time
  - Schema name: Configurable (default: "public")
- Sync strategy:
  - Push-only: SQLite → PostgreSQL via `agentsview pg push` command
  - Non-destructive: Fingerprinting to detect changes (local insert > PG delete)
  - One-way: PostgreSQL is read-only for viewer access
- Test environment: docker-compose service on port 5433 with tmpfs (no persistence)

**File Storage:**
- Local filesystem only
- Session files read from configured agent directories
- Cache/state: ~/.config/agentsview/cache/ (created at runtime)

**Caching:**
- None - Direct SQLite/PostgreSQL reads with connection pooling
- File hash caching: Session files tracked by mtime + hash to detect changes
- Backend handles pagination and cursor signing for large result sets

## Authentication & Identity

**Auth Provider:**
- Custom token-based auth (optional)
  - Implementation: `auth_token` config field (TOML/CLI)
  - Mechanism: Bearer token in HTTP Authorization header
  - Used for: Remote access protection and machine identification
  - Machine name: `pg.machine_name` config for multi-host deployments
  - See: `internal/config/config.go` and `internal/server/` for middleware

**GitHub Token:**
- Purpose: Optional enhancement for GitHub API access
- Storage: Stored in `~/.config/agentsview/config.toml` under `github_token`
- Config handling: `GithubToken` field in Config struct
- API endpoints: `GET /api/v1/config/github` (read), `POST /api/v1/config/github` (set)
- Rate limit: Allows higher GitHub API rate limits when provided
- Security: Not included in version/debug output

**Cursor Secret:**
- Purpose: HMAC signing for pagination cursors to prevent tampering
- Storage: Base64-encoded in config as `cursor_secret`
- Length: Cryptographically random bytes
- Thread-safe: `cursorMu` RWMutex in `internal/db/db.go`

## Monitoring & Observability

**Error Tracking:**
- None - No external error tracking service

**Logs:**
- Approach: Standard Go logging to stdout/stderr
- File logging: Optional log file in data directory (setup in main.go)
- No external log aggregation

**Metrics:**
- None - No metrics collection service integrated

## CI/CD & Deployment

**Hosting:**
- Self-hosted single binary
- Default: Local HTTP server (127.0.0.1:8080)
- Optional: Behind managed reverse proxy (Caddy)
- Alternative: PostgreSQL read-only mode for remote viewing (`agentsview pg serve`)

**Reverse Proxy (Optional):**
- Implementation: Managed Caddy support
  - Mode: `proxy=caddy` flag
  - Config: `ProxyConfig` struct with TLS cert/key, bind host, public port
  - Purpose: HTTPS termination and hostname-based access
  - Binary: Auto-detected or via `-caddy-bin` flag
  - See: `cmd/agentsview/managed_caddy.go`

**CI Pipeline:**
- GitHub Actions (referenced in code comments)
- Test environment: Starts PostgreSQL service container (port 5433)
- Build: go build with -tags fts5 -ldflags for version info
- Tests: CGO_ENABLED=1, integration tests with pgtest tag

**Deployment Targets:**
- macOS: Binary or desktop app bundle
- Linux: Single binary
- Windows: Single binary (experimental)
- Container: Docker image possible but not official

## Environment Configuration

**Required env vars for agent discovery:**
- `AGENT_VIEWER_DATA_DIR` - Base directory for SQLite and cache (default: ~/.config/agentsview)
- At least one agent directory env var:
  - `CLAUDE_PROJECTS_DIR`
  - `CODEX_SESSIONS_DIR`
  - `COPILOT_DIR`
  - `GEMINI_DIR`
  - `OPENCODE_DIR`
  - `AMP_DIR`

**Optional env vars:**
- None directly consumed; all configuration via config file or CLI flags

**Secrets location:**
- Config file: `~/.config/agentsview/config.toml`
- Fields: `github_token`, `auth_token`, `cursor_secret`, `pg.url`
- File permissions: 0600 (read-write owner only)
- See: `internal/config/config.go` for persistence layer

## Webhooks & Callbacks

**Incoming:**
- None - No webhook receivers

**Outgoing:**
- None - No webhook senders

## Session File Parsers

**Integrated Agent Support:**
All session file parsing internal (no external APIs):
- Claude Code (`.claude.md` files)
- Codex (Cursor integration)
- Copilot CLI
- Gemini CLI
- OpenCode
- Amp

Parsers located in `internal/parser/`:
- `claude.go` - Claude Code markdown parser
- `codex.go` - Codex session parser
- `copilot.go` - Copilot CLI parser
- `amp.go` - Amp session parser
- Message extraction uses gjson for structured JSON parsing

## Multi-Machine Architecture

**PostgreSQL Sync System:**
- Command: `agentsview pg push` - Push local SQLite to PostgreSQL
- Command: `agentsview pg status` - Show sync state
- Command: `agentsview pg serve` - Read-only HTTP server from PostgreSQL
- Fingerprinting: Content-based change detection to avoid re-uploading unchanged data
- Machine tracking: `machine` field in sessions table for multi-host deployments
- Schema versioning: `user_version` pragma to detect parser updates requiring full resync

---

*Integration audit: 2026-03-25*
