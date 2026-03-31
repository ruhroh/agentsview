# Codebase Concerns

**Analysis Date:** 2026-03-25

## Tech Debt

### Database Schema Resync Complexity

**Issue:** Resync operation is complex with multiple failure modes. After a full re-sync, if the swap fails at line `os.Rename(tempPath, origPath)` in `internal/sync/engine.go` (line 799), the system attempts recovery by reopening the original database. However, if that reopen also fails, the system may end up in an inconsistent state with no clear recovery path.

**Files:** `internal/sync/engine.go` (lines 595-825), `internal/db/db.go` (lines 531-618)

**Impact:** Data loss risk during resync failures; operator intervention may be needed to manually fix database state.

**Fix approach:** Add defensive checks post-rename to verify database integrity before considering resync successful. Consider keeping backup of original database before swap to simplify recovery.

### PostgreSQL Push Synchronization Gap

**Issue:** Permanently deleted sessions from SQLite (via `EmptyTrash`) are never propagated as deletions to PostgreSQL because the local rows no longer exist at push time. Sessions soft-deleted with `deleted_at` are synced, but hard-deleted sessions leave orphaned records in PG.

**Files:** `internal/postgres/push.go` (lines 45-50)

**Impact:** PG database accumulates orphaned session records over time; manual cleanup via PG DELETE required; read-only PG serve will show stale sessions.

**Fix approach:** Track permanently deleted session IDs separately (e.g., in `skipped_files` table or dedicated `deleted_sessions` table) and push those deletions to PG. Or implement a deletion log that survives session pruning.

### Watermark Loss on Database Rebuild

**Issue:** When a full database rebuild (`ResyncAll`) completes, both local skip cache and PostgreSQL push watermarks become stale, requiring a full re-push even if only minor data changed. The system detects this (line 92-103 in `internal/postgres/push.go`) and forces a full push, but this can be inefficient on large datasets.

**Files:** `internal/postgres/push.go` (lines 68-109), `cmd/agentsview/pg.go`

**Impact:** After resync, next `pg push` will re-upload all sessions to PG, consuming bandwidth and causing duplicate writes.

**Fix approach:** Preserve push watermarks separately from the database during rebuild, or implement a more granular tracking mechanism that survives schema changes.

### Non-Fatal Error Suppression

**Issue:** Multiple operations in the resync flow suppress errors as "non-fatal," which can mask real issues:
- `CopyExcludedSessionsFrom` failure (line 649): deleted sessions may reappear
- `LinkSubagentSessions` failure (line 782): subagent relationships broken
- `CopySessionMetadataFrom` failure (line 791): user renames/soft-deletes lost

**Files:** `internal/sync/engine.go` (lines 647-792)

**Impact:** Silent data loss for renames, stars, pins; subagent session links broken without user awareness.

**Fix approach:** Make these operations blocking (abort resync if they fail) or implement a verification pass after resync to detect and report inconsistencies.

## Known Bugs

### Resync Abort Heuristic May Reject Valid Rebuilds

**Issue:** The resync abort logic (line 669-691 in `internal/sync/engine.go`) aborts swap if `stats.Failed > stats.filesOK`. This heuristic can reject valid rebuilds when:
1. New agent parser is stricter (e.g., rejects malformed sessions that old parser accepted)
2. Some source files have become inaccessible (permission changes)
3. A few large sessions have parsing errors

**Files:** `internal/sync/engine.go` (lines 669-691)

**Impact:** Resync fails even when new data is valid; system stays on stale database; requires manual override or debugging.

**Fix approach:** Refine abort criteria to distinguish between transient errors (permission denied) and permanent failures (parse errors). Allow operator to force completion with `--force-swap` flag.

### Concurrent Resync Interleaving

**Issue:** While a resync is in progress and engine.db points to newDB (line 653), API requests may read from the incomplete resync state. The switch back to origDB happens immediately (line 655), but between lines 653-655 there's a narrow window where queries hit the stale original DB while the resync is using newDB.

**Files:** `internal/sync/engine.go` (lines 652-655), `internal/db/db.go` (line 36-62)

**Impact:** During resync, API may briefly return no results or inconsistent data.

**Fix approach:** Hold a read lock during the critical section (653-655) or use a flag to signal API handlers to defer reads until resync completes.

## Security Considerations

### Cursor Secret Initialization

**Issue:** Cursor signing secret is generated as 32 random bytes (line 387 in `cmd/agentsview/main.go`). If the database file is copied or backed up without the corresponding secret, cursor tokens become invalidated and search results become inaccessible.

**Files:** `cmd/agentsview/main.go` (line 387), `internal/db/db.go` (lines 85-90)

**Impact:** Cursor tokens tied to single machine/database instance; backup/restore scenarios require manual secret management.

**Fix approach:** Document that database backups must be accompanied by secret exports, or store cursor secret separately in a config file.

### SQL Injection via Sort Parameter

**Issue:** While the codebase uses parameterized queries extensively, the sort parameter in search (tested in `internal/db/search_test.go` line 24) is tested for injection but may be vulnerable in edge cases. Need to verify all sort fields are validated.

**Files:** `internal/db/search.go`, `internal/db/search_test.go`

**Impact:** Potential for crafted search requests to inject SQL; audit needed.

**Fix approach:** Run static analysis (sqlc, sqlLint) on all dynamic SQL; add allow-list for sort fields.

## Performance Bottlenecks

### Analytics Query Complexity

**Issue:** Analytics queries use chunked processing to stay under SQLite's bind-variable limit (500 vars per query, line 14 in `internal/db/analytics.go`), but with large session counts, this results in many sequential queries. For example, listing insights with 10,000+ sessions requires 20+ queries.

**Files:** `internal/db/analytics.go` (lines 14-40), `internal/postgres/analytics.go`

**Impact:** Slow dashboard loading with large session counts; N+1 query pattern.

**Fix approach:** Use temporary tables or CTEs instead of chunked IN clauses; pre-aggregate analytics in materialized views.

### Session Monitor Polling

**Issue:** SSE session monitor polls the database every 1.5 seconds (line 26 in `internal/server/events.go`) to detect message count changes. With many concurrent viewers, this creates O(N) polling load on SQLite.

**Files:** `internal/server/events.go` (lines 26-117)

**Impact:** Database lock contention under high viewer load; 100 viewers = 67 queries/sec.

**Fix approach:** Implement proper change notification (SQLite PRAGMA compile-time option or PG LISTEN/NOTIFY for PG backend) instead of polling.

### Worker Pool Saturation

**Issue:** File processing workers are capped at 8 (line 23 in `internal/sync/engine.go`), but on high-CPU machines this may under-utilize resources. However, going higher risks SQLite lock contention on the write side.

**Files:** `internal/sync/engine.go` (lines 21-24, 1073)

**Impact:** Sync performance plateaus on 16+ CPU systems; each worker may block on database writes.

**Fix approach:** Decouple parsing (CPU-bound, parallelizable) from database writing (I/O-bound, serialized); use a parse queue followed by a batch write.

## Fragile Areas

### Skip Cache Invalidation

**Issue:** Skip cache (line 44-51 in `internal/sync/engine.go`) stores file paths keyed by path with mtime values. If a file is deleted and recreated with the same path and happens to be created at the exact same mtime as the old file, it may remain in the skip cache and never re-parsed.

**Files:** `internal/sync/engine.go` (lines 44-51, 1145-1147), `internal/db/skipped.go`

**Impact:** New session data at same path with same mtime won't be detected; requires manual cache clear.

**Fix approach:** Use a more stable identifier than mtime alone (e.g., file inode + mtime, or content hash).

### Resync Temp Database Cleanup

**Issue:** If a resync crashes or is killed mid-process, the temp database (created at line 620 in `internal/sync/engine.go`) may be left behind. The code attempts cleanup at startup (line 209 in `cmd/agentsview/main.go`), but only if the file still exists and matches the naming pattern.

**Files:** `internal/sync/engine.go` (lines 595-630), `cmd/agentsview/main.go` (line 209)

**Impact:** Orphaned temp database files accumulate over time; disk space leak.

**Fix approach:** Use atomic operations or a lock file to ensure cleanup; add periodic cleanup task.

### FTS Initialization Failure Silent

**Issue:** Full-text search index initialization can fail silently if the fts5 module is not compiled in (line 509-516 in `internal/db/db.go`). The system logs the error but continues, making search functionality unavailable without user awareness.

**Files:** `internal/db/db.go` (lines 509-526)

**Impact:** Search endpoints return empty results without indicating FTS is missing; confusing user experience.

**Fix approach:** Make FTS required (fail fast) or add explicit FTS availability check to API responses.

### Concurrent Starred/Pinned Operations

**Issue:** Starred and pinned operations use INSERT ... SELECT ... WHERE EXISTS pattern (line 95 in `internal/db/starred.go`) to avoid TOCTOU races, but a race still exists if session is deleted between the check and insert.

**Files:** `internal/db/starred.go` (lines 11-100), `internal/db/pins.go`

**Impact:** Race condition can leave orphaned starred/pinned records pointing to deleted sessions.

**Fix approach:** Use foreign key constraints with ON DELETE CASCADE to clean up orphaned records; or use transactions with proper isolation level.

## Scaling Limits

### SQLite Write Serialization

**Issue:** All writes go through a single serialized write lock (line 40 in `internal/sync/engine.go` and line 63 in `internal/db/db.go`). This means all sync operations, API writes, and analytics aggregations compete for a single lock.

**Files:** `internal/sync/engine.go` (line 40), `internal/db/db.go` (lines 63-75)

**Impact:** Write throughput capped at SQLite's single-connection speed; at ~1000 writes/sec per file, sustained writes beyond that will queue.

**Fix approach:** Upgrade to PostgreSQL backend for write-heavy deployments (already supported via `internal/postgres/` but requires manual migration).

### Database File Growth

**Issue:** The database uses WAL mode (line 95 in `internal/db/db.go`), which can leave behind large WAL files if not regularly checkpointed. No automatic checkpoint/VACUUM is configured.

**Files:** `internal/db/db.go` (lines 92-100)

**Impact:** Database + WAL files can grow to 2-3x session data size; no space recovery on delete/prune.

**Fix approach:** Add periodic VACUUM and checkpoint operations; or enable auto-checkpoint via pragma.

## Dependencies at Risk

### Deprecated Type Definition Warning

**Issue:** `dompurify` package.json (frontend) shows deprecated stub type definition warning. Types should come from `dompurify` directly, not `@types/dompurify`.

**Files:** `frontend/package.json` (line 19), `frontend/package-lock.json`

**Impact:** Type safety compromised; potential for future breaking changes when stub is removed.

**Fix approach:** Remove `@types/dompurify` from devDependencies and update imports to use types from main package.

### Parser Maintenance Burden

**Issue:** Codebase supports 8+ agent types (Claude, Codex, Copilot, Gemini, OpenCode, Amp, Kimi, iFlow) with separate parsers. Each parser must handle format changes independently; no parser version tracking.

**Files:** `internal/parser/*.go` (55+ files)

**Impact:** When agent changes format (e.g., Claude Code messages schema), parser must be updated reactively; no version compatibility layer.

**Fix approach:** Add agent format version tracking to schema; implement compatibility layer for old formats.

## Missing Critical Features

### No Automatic Database Maintenance

**Issue:** No automatic VACUUM, checkpoint, or WAL cleanup scheduled. Operator must manually run maintenance or database will grow unbounded.

**Files:** `internal/db/db.go`, `cmd/agentsview/sync.go`

**Impact:** Long-running instances accumulate database bloat; disk fills up silently.

**Fix approach:** Add `prune --vacuum` subcommand; document manual maintenance in README; or implement background maintenance task.

### No Session Deduplication

**Issue:** If the same session file is symlinked or copied to multiple agent directories, it will be parsed and stored multiple times with different IDs, leading to duplicate session data.

**Files:** `internal/sync/engine.go`, `internal/parser/discovery.go`

**Impact:** Duplicate sessions in database; misleading analytics and session counts.

**Fix approach:** Use content hash or inode-based deduplication to detect and merge duplicate sessions.

## Test Coverage Gaps

### Concurrent Resync and API Access

**Issue:** `TestResyncConcurrentReads` (line 2246 in `internal/sync/engine_integration_test.go`) tests basic concurrent access, but doesn't verify that all API responses are consistent during resync.

**Files:** `internal/sync/engine_integration_test.go` (lines 2246-2300)

**Impact:** Edge cases where API returns partial data during resync may exist but are untested.

**Fix approach:** Add test that verifies all API responses complete before resync completes, or gracefully degrade during resync.

### PostgreSQL Push Failure Scenarios

**Issue:** PG push tests in `internal/postgres/push_pgtest_test.go` don't cover network failure scenarios (dropped connections, partial writes, PG restart mid-push).

**Files:** `internal/postgres/push_pgtest_test.go`, `internal/postgres/sync_test.go`

**Impact:** Push failures may leave PG in inconsistent state (partial session written, watermark not updated).

**Fix approach:** Add chaos engineering tests (simulate connection drops, slow queries, PG restarts).

### Permission and File System Edge Cases

**Issue:** Parser tests don't cover all file system edge cases (sparse files, symlink loops, very large files >1GB, files deleted during parsing).

**Files:** `internal/parser/discovery_test.go`, `internal/sync/engine_integration_test.go`

**Impact:** Parsing may fail or hang on unusual file system scenarios.

**Fix approach:** Add integration tests with fuse filesystem or similar to simulate edge cases.

### Frontend Accessibility Tests

**Issue:** No accessibility testing (a11y) for Svelte components. VirtualList component may have ARIA issues.

**Files:** `frontend/src/lib/virtual/`, `frontend/e2e/`

**Impact:** Screen reader users may have difficulty navigating search results or session details.

**Fix approach:** Add axe-playwright or similar a11y testing to E2E suite.

## Data Integrity Risks

### Orphaned Tool Calls

**Issue:** If a message is deleted or updated, its associated tool_calls records may become orphaned if the deletion/update transaction partially fails.

**Files:** `internal/db/messages.go` (lines 375-410)

**Impact:** Orphaned tool_call records pointing to deleted messages consume space and cause analytics to double-count.

**Fix approach:** Add foreign key constraint `REFERENCES messages(id) ON DELETE CASCADE` to ensure cascade cleanup.

### Missing Transaction Boundaries

**Issue:** Some multi-step operations (e.g., inserting a session + its messages + tool_calls) span multiple `Update()` calls, creating transaction boundaries and race windows.

**Files:** `internal/db/db.go` (lines 620-637), callers in sync/server

**Impact:** Partial writes on error; database inconsistency.

**Fix approach:** Combine related operations into single transaction.

---

*Concerns audit: 2026-03-25*
