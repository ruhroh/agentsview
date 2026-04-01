# Testing Patterns

**Analysis Date:** 2026-03-31

## Test Framework

**Go Runner:**
- Standard `testing` package — no third-party test runner
- `github.com/google/go-cmp/cmp` used for deep equality diffs (imported in `internal/db/db_test.go`)
- Build tag `fts5` required for FTS5 search tests
- Build tag `pgtest` required for PostgreSQL integration tests
- **Environment:** `CGO_ENABLED=1` required (SQLite3 driver needs C compilation)

**Frontend Unit Tests:**
- Vitest 4.1.2 with jsdom environment
- Config: `frontend/vite.config.ts` (test section)
- Run via `npm run test` (alias for `vitest run`)

**Frontend E2E Tests:**
- Playwright 1.58.2
- Config: `frontend/playwright.config.ts`
- Browsers: Chromium and WebKit
- Base URL: `http://127.0.0.1:8090`
- Web server: `bash ../scripts/e2e-server.sh` — starts a real agentsview binary with test fixtures

**Run Commands:**
```bash
make test           # Go unit tests (CGO_ENABLED=1 -tags fts5 ./... -v -count=1)
make test-short     # Fast tests only (-short flag)
make test-postgres  # PG integration tests (starts Docker container)
make e2e            # Playwright E2E tests
make lint           # golangci-lint
make vet            # go vet
```

```bash
cd frontend && npm run test   # Vitest unit tests
cd frontend && npm run e2e    # Playwright E2E tests
```

## Test File Organization

**Go:**
- Co-located with production code: `internal/db/sessions.go` → `internal/db/db_test.go`
- White-box tests (same package): `package db` — e.g., `internal/db/search_test.go`
- Black-box tests (external package): `package server_test` — e.g., `internal/server/server_test.go`
- Internal white-box test helpers named `helpers_internal_test.go`, `deadline_internal_test.go`
- Shared test helpers in separate `_test.go` files: `internal/db/db_test.go` contains all DB test helpers

**Frontend unit tests:**
- Co-located next to source: `frontend/src/lib/utils/format.ts` → `frontend/src/lib/utils/format.test.ts`

**Frontend E2E tests:**
- Separate directory: `frontend/e2e/`
- Page objects under: `frontend/e2e/pages/`
- Spec files: `frontend/e2e/*.spec.ts`

**Structure:**
```
internal/
  db/
    sessions.go
    search.go
    db_test.go          # shared test helpers (testDB, insertSession, etc.)
    search_test.go      # white-box tests
    shared_test.go      # share-related tests
  server/
    server.go
    server_test.go      # black-box integration tests (package server_test)
    helpers_internal_test.go  # white-box helpers (package server)
    search_test.go      # white-box unit tests (package server)
  dbtest/
    dbtest.go           # shared helpers for cross-package DB tests
frontend/
  src/lib/utils/
    format.ts
    format.test.ts
  e2e/
    session-list.spec.ts
    pages/sessions-page.ts
```

## Go Test Structure

**Table-driven tests (standard pattern):**
```go
func TestParseIntParam(t *testing.T) {
    tests := []struct {
        name       string
        query      string
        param      string
        wantVal    int
        wantOK     bool
        wantStatus int
    }{
        {
            name:       "absent param returns zero",
            query:      "",
            param:      "limit",
            wantVal:    0,
            wantOK:     true,
            wantStatus: http.StatusOK,
        },
        // ...more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test body
        })
    }
}
```

**Subtests with t.Parallel():**
```go
func TestSearch(t *testing.T) {
    d := testDB(t)
    // shared setup

    t.Run("deduplication: two messages in same session", func(t *testing.T) {
        // subtests may or may not call t.Parallel()
    })
}

// Top-level tests that are independent call t.Parallel():
func TestSearchEmptyQueryGuard(t *testing.T) {
    t.Parallel()
    // ...
}
```

**Assertion helpers pattern:**
- `require*` helpers call `t.Fatal` (stop test on failure)
- `assert*` helpers call `t.Error` (continue test on failure)
- All helpers call `t.Helper()` as first line
- Defined in `internal/db/db_test.go` and `internal/server/helpers_internal_test.go`

```go
func requireNoError(t *testing.T, err error, msg string) {
    t.Helper()
    if err != nil {
        t.Fatalf("%s: %v", msg, err)
    }
}

func assertRecorderStatus(t *testing.T, w *httptest.ResponseRecorder, code int) {
    t.Helper()
    if w.Code != code {
        t.Fatalf("expected status %d, got %d: %s", code, w.Code, w.Body.String())
    }
}
```

## Database Test Helpers

**`testDB(t)` — creates an isolated SQLite instance:**
```go
func testDB(t *testing.T) *DB {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "test.db")
    d, err := Open(path)
    requireNoError(t, err, "opening test db")
    t.Cleanup(func() { d.Close() })
    return d
}
```

Used within `internal/db` package. For cross-package tests, use `dbtest.OpenTestDB(t)` from `internal/dbtest/dbtest.go`.

**`insertSession` — creates a session with functional option overrides:**
```go
func insertSession(
    t *testing.T, d *DB, id, project string,
    opts ...func(*Session),
) {
    t.Helper()
    s := Session{
        ID:           id,
        Project:      project,
        Machine:      defaultMachine,
        Agent:        defaultAgent,
        MessageCount: 1,
    }
    for _, opt := range opts {
        opt(&s)
    }
    if err := d.UpsertSession(s); err != nil {
        t.Fatalf("insertSession %s: %v", id, err)
    }
}

// Usage:
insertSession(t, d, "s1", "proj-a", func(s *Session) {
    s.Agent = "claude"
    s.StartedAt = Ptr("2024-01-01T10:00:00Z")
})
```

**Message factory helpers:**
```go
func userMsg(sid string, ordinal int, content string) Message { ... }
func asstMsg(sid string, ordinal int, content string) Message { ... }
func userMsgAt(sid string, ordinal int, content, ts string) Message { ... }
func asstMsgAt(sid string, ordinal int, content, ts string) Message { ... }
```

**Cross-package helpers in `internal/dbtest/dbtest.go`:**
- `dbtest.OpenTestDB(t)` — open temp SQLite DB
- `dbtest.SeedSession(t, d, id, project, opts...)` — same functional-option pattern
- `dbtest.SeedMessages(t, d, msgs...)` — insert messages with fatal on error
- `dbtest.UserMsg(sid, ordinal, content)` / `dbtest.AsstMsg(...)` — message builders
- `dbtest.Ptr[T](v T)` — generic pointer helper

**FTS guard:**
```go
func requireFTS(t *testing.T, d *DB) {
    t.Helper()
    if !d.HasFTS() {
        t.Skip("no FTS support")
    }
}
```

## HTTP Handler Test Helpers

Defined in `internal/server/helpers_internal_test.go` (white-box, `package server`):

**`testServer(t, writeTimeout, opts...)` — full server with real DB:**
```go
func testServer(t *testing.T, writeTimeout time.Duration, opts ...Option) *Server {
    t.Helper()
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "test.db")
    database, err := db.Open(dbPath)
    // ...
    t.Cleanup(func() { database.Close() })
    return New(cfg, database, opts...)
}
```

**`newTestRequest(t, query)` — lightweight GET request helper:**
```go
func newTestRequest(t *testing.T, query string) (*httptest.ResponseRecorder, *http.Request) {
    // returns recorder + httptest.NewRequest
}
```

**`testEnv` — full integration test environment (black-box, `package server_test`):**
```go
type testEnv struct {
    srv     *server.Server
    handler http.Handler
    db      *db.DB
    dataDir string
}

func setup(t *testing.T, opts ...setupOption) *testEnv { ... }
func (te *testEnv) seedSession(t *testing.T, id, project string, msgCount int, opts ...func(*db.Session))
func (te *testEnv) seedMessages(t *testing.T, sessionID string, count int, mods ...func(i int, m *db.Message))
func (te *testEnv) listenAndServe(t *testing.T) string // starts real server, returns base URL
```

**`setupOption` functional options for `setup()`:**
```go
func withWriteTimeout(d time.Duration) setupOption { ... }
func withPublicOrigins(origins ...string) setupOption { ... }
func withPublicURL(url string) setupOption { ... }
```

## Mocking

**Go:**
- No mocking framework; mocks are hand-written structs implementing the `db.Store` interface
- Example spy pattern from `internal/server/search_test.go`:
```go
type searchSpy struct {
    db.Store
    filter db.SearchFilter
}
func (s *searchSpy) HasFTS() bool { return true }
func (s *searchSpy) Search(_ context.Context, f db.SearchFilter) (db.SearchPage, error) {
    s.filter = f
    return db.SearchPage{}, nil
}
```

**TypeScript (Vitest):**
- `vi.mock("../api/client.js", () => ({...}))` — full module mock
- `vi.fn()` for individual function mocks
- `vi.stubGlobal("fetch", vi.fn().mockResolvedValue({...}))` — global stub for fetch
- `vi.mocked(api.listSessions).mockResolvedValue({...})` — typed mock return values
- `vi.clearAllMocks()` in `beforeEach` to reset state between tests

**What to mock:**
- Go: mock `db.Store` interface for server handler tests that don't need real DB
- TypeScript: mock `../api/client.js` module in store tests; mock `fetch` globally in API client tests

**What NOT to mock:**
- Go: prefer real `testDB(t)` over mocking for database logic tests — tests real SQL
- TypeScript: do not mock the module under test; mock only its external dependencies

## Fixtures and Factories

**Go test data:**
- Timestamp constants defined in `internal/db/db_test.go`:
```go
const (
    defaultMachine = "local"
    defaultAgent   = "claude"
    tsZero    = "2024-01-01T00:00:00Z"
    tsHour1   = "2024-01-01T01:00:00Z"
    tsMidYear = "2024-06-01T10:00:00Z"
)
```

**TypeScript test factories:**
- Local factory functions within test files, e.g.:
```typescript
function makeMsg(overrides: Partial<Message> & { content: string }): Message {
    const defaults: Message = { id: 1, session_id: "s1", ordinal: 0, ... };
    return { ...defaults, ...overrides };
}
```

**E2E test fixtures:**
- Real session data injected via `cmd/testfixture/main.go`
- Server started by `scripts/e2e-server.sh` with fixture data
- Fixed session counts declared as constants in spec files:
```typescript
const TOTAL_SESSIONS = 8;
const ALPHA_SESSIONS = 2;
```

## Coverage

**Requirements:** No enforced coverage target.

**View coverage:**
```bash
CGO_ENABLED=1 go test -tags fts5 ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Types

**Go unit tests:**
- Scope: single function or small group of related functions
- Use `testDB(t)` for any test touching SQLite
- Use `httptest.NewRecorder()` + `httptest.NewRequest()` for handler tests without a full server
- Table-driven with meaningful `name` fields

**Go integration tests:**
- Scope: full HTTP request → handler → database → response cycle
- Use `testEnv` with `setup(t)` for full server wiring
- Some tests use `listenAndServe(t)` for real TCP connection tests (CORS, bind checks)

**PostgreSQL integration tests:**
- Build tag: `pgtest`
- Require `TEST_PG_URL` environment variable
- Run with `make test-postgres` (auto-starts Docker container)
- Tests create/drop `agentsview` schema — use a dedicated test database

**Frontend unit tests (Vitest):**
- Scope: single utility function, store, or component
- jsdom environment for DOM APIs
- No network — all HTTP mocked via `vi.mock` or `vi.stubGlobal("fetch", ...)`

**Frontend E2E tests (Playwright):**
- Scope: full user flows in a real browser against a running server
- Page Object Model: `frontend/e2e/pages/sessions-page.ts` encapsulates selectors
- Two browsers: Chromium and WebKit
- Server auto-started by `webServer` config in `playwright.config.ts`

## Common Patterns

**Async error handling in Go tests:**
```go
page, err := d.Search(context.Background(), SearchFilter{
    Query: "alpha", Limit: 10,
})
if err != nil {
    t.Fatalf("Search: %v", err)
}
```

**Context cancellation testing:**
```go
func canceledCtx() context.Context {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    return ctx
}

func requireCanceledErr(t *testing.T, err error) {
    t.Helper()
    if !errors.Is(err, context.Canceled) {
        t.Errorf("expected context.Canceled, got: %v", err)
    }
}
```

**Parallel sub-tests with shared setup:**
```go
func TestSearchSession(t *testing.T) {
    t.Parallel()
    d := testDB(t)
    insertSession(t, d, "s1", "proj")
    insertMessages(t, d, /* many messages */)

    tests := []struct { name, sessionID, query string; want []int }{ ... }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // safe because d is read-only in subtests
            got, err := d.SearchSession(context.Background(), tt.sessionID, tt.query)
            // assertions
        })
    }
}
```

**Vitest parameterized tests:**
```typescript
it.each([
    ["case description", input, expected],
    // ...
])("%s", (_name, input, expected) => {
    expect(sanitizeSnippet(input)).toBe(expected);
});
```

**Playwright page object usage:**
```typescript
test.describe("Session list", () => {
    let sp: SessionsPage;

    test.beforeEach(async ({ page }) => {
        sp = new SessionsPage(page);
        await sp.goto();
    });

    test("sessions load and display", async () => {
        await expect(sp.sessionItems).toHaveCount(TOTAL_SESSIONS);
    });
});
```

---

*Testing analysis: 2026-03-31*
