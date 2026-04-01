# Codebase Concerns

**Analysis Date:** 2026-03-31

## Tech Debt

**Frontend API client references many endpoints absent from `server` branch:**
- Issue: `frontend/src/lib/api/client.ts` calls ~30 API routes that do not exist in the
  current `internal/server/server.go` route table. Missing routes include
  `/api/v1/analytics/*`, `/api/v1/insights`, `/api/v1/insights/{id}`,
  `/api/v1/sessions/{id}/pins`, `/api/v1/pins`, `/api/v1/trash`,
  `/api/v1/sessions/{id}/resume`, `/api/v1/sessions/{id}/export`,
  `/api/v1/sessions/{id}/watch`, `/api/v1/sessions/{id}/publish`,
  `/api/v1/sessions/{id}/rename`, `/api/v1/sessions/{id}/open`,
  `/api/v1/sessions/{id}/directory`, `/api/v1/sync`, `/api/v1/resync`,
  `/api/v1/sync/status`, `/api/v1/update/check`, `/api/v1/config/github`,
  `/api/v1/config/terminal`, `/api/v1/settings`, `/api/v1/openers`.
- Files: `internal/server/server.go` (routes function, lines 129-163),
  `frontend/src/lib/api/client.ts`
- Impact: All analytics charts, insights, session management (rename, trash, pins,
  resume), sync controls, and settings UI are non-functional. The UI may call
  these endpoints and receive 404s or SPA fallback HTML.
- Fix approach: The `server` branch appears to be a stripped-down hosted read-only
  server. Missing routes either need to be implemented or the frontend needs to
  hide feature sections when endpoints return 404. Check the `main` branch for
  the full handler set.

**Dual migration systems with drift risk:**
- Issue: Schema changes are applied via two parallel mechanisms: `needsSchemaRebuild`
  probes in `db.go` (triggers destructive DB drop) and `migrateColumns` ALTER TABLE
  migrations. New columns added to `schema.sql` are not automatically probed in
  `needsSchemaRebuild`. Adding a new table or column without also updating the probe
  list may silently leave old databases with missing structures that never trigger a
  rebuild.
- Files: `internal/db/db.go` lines 206-239 (probe list), `internal/db/db.go`
  lines 260-338 (migrations), `internal/db/schema.sql`
- Impact: Users upgrading from old releases may silently miss new columns.
- Fix approach: Consolidate: use only `migrateColumns` for additive changes, and
  update `needsSchemaRebuild` probes only for genuinely incompatible changes. Add
  a test that validates all schema.sql columns appear in either a probe or migration.

**In-memory timezone filtering forces full message table scan:**
- Issue: `filteredSessionIDs` in `internal/db/analytics.go` (line 187) loads all
  messages with timestamps for every analytics query that uses hour-of-day or
  day-of-week filters. This is called 8 times across analytics functions
  (lines 327, 512, 702, 893, 1211, 1376, 1722, 2022). The filter cannot be pushed
  into SQL because SQLite lacks timezone-aware datetime functions.
- Files: `internal/db/analytics.go`
- Impact: On large databases (100k+ messages), hour/dow-filtered analytics queries
  load the full messages table into Go memory on every request. Latency grows
  linearly with database size.
- Fix approach: Cache the filtered ID set per (filter params hash) in memory with a
  short TTL (e.g. 60 seconds), or store a `local_hour` / `local_dow` column computed
  at sync time for a configurable timezone.

**`analytics.go` is 2100 lines with duplicated query patterns:**
- Issue: `internal/db/analytics.go` contains 8+ analytics query functions that each
  independently repeat the same pattern: call `filteredSessionIDs` if time filter is
  set, fetch sessions, apply in-Go date range filtering, chunk-query related tables.
  The same `buildWhere`, `localDate`, `inDateRange` logic is repeated inline.
- Files: `internal/db/analytics.go`
- Impact: High maintenance cost; any change to filter logic must be replicated across
  all functions. Test coverage is harder to achieve.
- Fix approach: Extract a common `analyticsSessionRows` helper that returns pre-filtered
  `(id, date, agent)` rows given an `AnalyticsFilter`, reducing per-function boilerplate.

**Schema rebuild drops database on missing column, destroying unarchived sessions:**
- Issue: When `needsSchemaRebuild` detects a missing probe column, `dropDatabase` is
  called unconditionally (line 128 in `db.go`). The non-destructive orphan copy path
  (`CopyOrphanedDataFrom`) is only invoked during a data-version resync, not during
  schema rebuild.
- Files: `internal/db/db.go` lines 127-133
- Impact: A user upgrading across a schema-breaking release loses sessions whose source
  files no longer exist on disk. The CLAUDE.md policy explicitly prohibits this pattern.
- Fix approach: On schema rebuild, open the old DB read-only before dropping, copy
  orphaned sessions and user metadata into the new DB after init, consistent with the
  data-version resync path.

**`orphanSessionCols` builds column list dynamically per migration:**
- Issue: `orphanSessionCols` in `internal/db/orphaned.go` (line 391) probes for
  `display_name` and `deleted_at` column presence every time orphan data is copied.
  Similarly, `oldDBHasColumn` and `oldDBHasTable` are called in hot loops. Each call
  issues `PRAGMA old_db.table_info(...)` queries inside the copy transaction.
- Files: `internal/db/orphaned.go`
- Impact: Slow orphan copy for databases with many tables; brittle if additional
  columns are added in future without updating orphan copy code.
- Fix approach: Probe all columns once before the transaction and pass results to
  the copy functions.

## Security Considerations

**Static bearer token compared with `!=` (not constant-time):**
- Risk: The static auth token comparison at `internal/server/auth.go` line 155 uses
  `provided != token` (Go string inequality), which is not constant-time. A remote
  attacker making many requests could theoretically measure response latency to
  perform a timing attack and recover the token character by character.
- Files: `internal/server/auth.go` line 155
- Current mitigation: Tokens are 32-byte random base64 strings (long enough to make
  statistical timing attacks impractical over a network). The risk is low in practice.
- Recommendations: Replace with `subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1`
  from `crypto/subtle` to eliminate the attack vector entirely.

**Auth token exposed in URL query param for SSE watch endpoint:**
- Risk: When the `watchSession` function in `frontend/src/lib/api/client.ts` uses
  the legacy token flow, it appends `?token=<auth-token>` to the EventSource URL.
  This token appears in browser history, access logs, and any proxy/CDN log.
- Files: `frontend/src/lib/api/client.ts` lines 409-430, `internal/server/auth.go`
  line 148
- Current mitigation: The query-param fallback is restricted to paths ending in `/watch`
  only. A comment in the client acknowledges the leak risk and notes that Clerk-backed
  deployments avoid it.
- Recommendations: When Clerk is not used, consider a fetch-based SSE polyfill with
  custom headers instead of native `EventSource` to avoid token in URL.

**Incoming share payload decoded without body size limit:**
- Risk: `handleUpsertShare` in `internal/server/share_write.go` (line 53) uses
  `json.NewDecoder(r.Body).Decode(&body)` with no `http.MaxBytesReader` guard. A
  client with a valid token could push arbitrarily large JSON payloads.
- Files: `internal/server/share_write.go` lines 43-111,
  `internal/server/starred.go` line 82 (same pattern in `handleBulkStar`)
- Current mitigation: Both endpoints require a valid auth token.
- Recommendations: Add `r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)`
  before decoding. A 10 MB limit would cover legitimate session payloads.

**Clerk JWKS cache has no TTL:**
- Risk: `ClerkVerifier.storeKey` caches JWKS keys indefinitely in a
  `map[string]*clerkapi.JSONWebKey`. If Clerk rotates a signing key, the cached key
  is never invalidated until process restart, causing all new tokens signed with the
  new key to fail verification until restart.
- Files: `internal/server/clerk.go` lines 148-163
- Current mitigation: Clerk key rotation is infrequent. Token verification fails
  closed (returns 401) rather than open.
- Recommendations: Add a TTL to cached keys (e.g. 24 hours) and evict on lookup.

**Auth token and secrets stored in config.toml as plaintext:**
- Risk: `AuthToken`, `ClerkSecretKey`, `GithubToken`, and `Share.Token` are stored
  in plaintext in `~/.agentsview/config.toml` with `json:` and `toml:` tags. If the
  config file is accidentally committed, shared, or included in a backup, all secrets
  are exposed.
- Files: `internal/config/config.go` lines 83-88
- Current mitigation: Config file is written with mode `0o600`. No API endpoint
  serializes the `Config` struct directly.
- Recommendations: Document the risk. Consider scrubbing secrets from `Config.MarshalJSON`
  if the struct is ever serialized to a log or API response.

## Performance Bottlenecks

**Analytics queries load all session rows into Go memory:**
- Problem: Most analytics functions fetch all sessions matching date filters from
  the database into Go slices before applying in-memory timezone bucketing. With
  large session counts (10k+), this creates large allocations per request.
- Files: `internal/db/analytics.go` — functions `GetAnalyticsSummary` (line 316),
  `GetAnalyticsActivity` (line 499), `GetAnalyticsHeatmap` (line 695),
  `GetAnalyticsProjects` (line 895), `GetAnalyticsVelocity` (line 1710),
  `GetAnalyticsTopSessions` (line 2030)
- Cause: SQLite lacks timezone-aware date functions, forcing in-Go conversion after
  a padded UTC range query. The ±14-hour UTC padding results in fetching up to 28
  extra hours of sessions that are then discarded.
- Improvement path: Store a materialized local date as a generated column or at sync
  time. Ensure the padded UTC range uses the indexed `started_at` column rather than
  the `COALESCE(NULLIF(started_at,''), created_at)` expression which cannot use the
  `idx_sessions_started` index efficiently.

**`filteredSessionIDs` issues a full message JOIN for time-filtered queries:**
- Problem: When hour-of-day or day-of-week filters are active, `filteredSessionIDs`
  joins the full `sessions` and `messages` tables (lines 194-197 in `analytics.go`),
  loading all message timestamps into Go memory for timezone conversion. This call
  happens once per analytics function per request — up to 8 times for a single
  page load that exercises multiple analytics endpoints simultaneously.
- Files: `internal/db/analytics.go` lines 187-232
- Cause: No index on `messages.timestamp` for range queries; the query performs a
  full messages scan for the date-padded window.
- Improvement path: Add `CREATE INDEX idx_messages_timestamp ON messages(timestamp)`.
  Cache `filteredSessionIDs` results within a single request context or short-lived
  in-process cache.

**`serveIndex` allocates a new string per request:**
- Problem: `serveIndex` in `internal/server/server.go` (lines 197-243) opens and
  reads the embedded `index.html` entry and applies string replacements on every
  non-asset HTTP request. The base-path replacement creates a new string allocation
  on each call.
- Files: `internal/server/server.go` lines 197-243
- Cause: No caching of the mutated index HTML.
- Improvement path: Compute the final index HTML once at startup (when `basePath` and
  `ClerkPublishableKey` are known and do not change) and serve the cached bytes
  directly.

## Fragile Areas

**Search query uses manually ordered argument lists:**
- Files: `internal/db/search.go` lines 145-270
- Why fragile: The `Search` function builds a complex SQL query with a UNION of FTS
  and name branches. Arguments are assembled in a precisely counted order documented
  with a comment table (lines 148-163). A second copy of the argument array (`args2`)
  is then prepended. Any change to the SQL template requires careful re-counting of
  argument positions. A mismatch silently passes the wrong value to the wrong
  placeholder.
- Safe modification: When changing the query, update the comment table first, then
  verify `len(args) == number of ? placeholders` in the SQL string by counting
  question marks manually or adding an assertion.
- Test coverage: `internal/db/search_test.go` covers main cases but not all
  argument-position edge cases.

**`resolveToolCalls` panics on programmer error:**
- Files: `internal/db/messages.go` lines 735-746
- Why fragile: The function panics if `len(ids) != len(msgs)`. While this is a
  legitimate programmer error guard, panics in database layer functions crash
  the server process rather than returning an HTTP 500.
- Safe modification: Keep the invariant check but return an error instead of
  panicking, or recover in the HTTP handler layer with a deferred recover.
- Test coverage: Tested via the panic recovery path in `db_test.go` line 2638.

**`http.Server` has no `WriteTimeout`:**
- Files: `internal/server/server.go` lines 674-679
- Why fragile: `ListenAndServe` configures `ReadTimeout: 10 * time.Second` and
  `IdleTimeout: 120 * time.Second` but no `WriteTimeout`. The per-handler
  `WriteTimeout` is applied via `withTimeout` middleware (30 seconds by default),
  but if a handler panics or exits unexpectedly before the middleware timeout fires,
  the server-level TCP write timeout never applies.
- Safe modification: Add `WriteTimeout: 35 * time.Second` (slightly above the
  middleware handler timeout) to ensure server-level connection cleanup.

**Caddy subprocess not explicitly killed on error before start:**
- Files: `cmd/agentsview/managed_caddy.go`
- Why fragile: The managed Caddy process is started via `exec.CommandContext`. If
  Caddy fails to start within `managedCaddyStartGrace` (300ms), the process may
  still be alive and holding the public port. Subsequent server restarts will fail
  to bind to the same port.
- Safe modification: On startup failure, call `cmd.Process.Kill()` explicitly before
  returning the error.

## Scaling Limits

**SQLite single writer:**
- Current capacity: One concurrent write at a time due to `db.mu` mutex and
  `writer.SetMaxOpenConns(1)` in `openAndInit` at `internal/db/db.go` line 366.
- Limit: High-write scenarios (many concurrent share uploads or session syncs)
  serialize behind the write mutex.
- Scaling path: Acceptable for single-user local use. For multi-user hosted scenarios,
  the PostgreSQL backend (`PGConfig`) removes this limit.

**Analytics queries fetch entire result sets into memory:**
- Current capacity: Works well for collections up to ~10k sessions.
- Limit: At 50k+ sessions, analytics responses may exceed 100ms and allocate
  multi-MB buffers per request.
- Scaling path: Paginate or stream analytics aggregations in SQL rather than Go;
  add a materialized summary table updated incrementally at sync time.

## Dependencies at Risk

**`github.com/mattn/go-sqlite3` requires CGO:**
- Risk: The sqlite3 driver requires `CGO_ENABLED=1` and a C compiler at build time.
  This complicates cross-compilation, CI, and container builds (see `Makefile`
  requirement for `CGO_ENABLED=1 -tags fts5`).
- Impact: Build failures in pure-Go or CGO-disabled environments. No binary
  distribution without matching host platform build.
- Migration plan: `modernc.org/sqlite` is a pure-Go SQLite implementation with FTS5
  support that does not require CGO, though it has different performance characteristics.

**`github.com/BurntSushi/toml` used only in config loading:**
- Risk: Low-activity dependency; potential for abandonment.
- Impact: TOML config parsing in `internal/config/config.go`.
- Migration plan: `github.com/pelletier/go-toml/v2` or standard library alternative
  if the library becomes unmaintained.

## Test Coverage Gaps

**No unit tests for `cmd/agentsview/main.go` server startup path:**
- What's not tested: `runServe` startup sequence, `mustOpenDB`, `setupLogFile`,
  `truncateLogFile` edge cases (file is a symlink, no permission).
- Files: `cmd/agentsview/main.go`
- Risk: Startup regressions (wrong DB path, log file permission errors) go undetected.
- Priority: Low (startup is exercised by E2E tests in `frontend/e2e/`).

**`internal/server/share.go` lacks tests for push failure scenarios:**
- What's not tested: `pushShare` timeout behavior, partial failures where push
  succeeds but local `RecordShare` fails (leaving share in inconsistent state).
- Files: `internal/server/share.go`, `internal/server/share_write.go`
- Risk: Users may see a 502 error after a successful remote push, leaving remote and
  local records inconsistent.
- Priority: Medium.

**No test verifies `needsSchemaRebuild` probe list completeness:**
- What's not tested: No test verifies that every column in `schema.sql` either
  appears in the `needsSchemaRebuild` probe list or in `migrateColumns`.
- Files: `internal/db/db.go`, `internal/db/schema.sql`
- Risk: Adding a new column to `schema.sql` without updating probes leaves old
  databases silently missing the column, potentially causing runtime errors only
  discovered by end users.
- Priority: High.

**Analytics time-filter edge cases (DST transitions, sub-hour offsets) not tested:**
- What's not tested: `filteredSessionIDs` timezone conversion for locations with
  30-minute or 45-minute UTC offsets (e.g. `Asia/Kolkata` UTC+5:30, `Australia/Lord_Howe`
  UTC+10:30). The ±14-hour UTC padding covers all standard offsets but these are not
  exercised in tests.
- Files: `internal/db/analytics.go` lines 73-90, `internal/db/analytics_test.go`
- Risk: Users in non-standard-offset timezones may see sessions dropped from
  analytics when filtering by day-of-week or hour-of-day.
- Priority: Medium.

---

*Concerns audit: 2026-03-31*
