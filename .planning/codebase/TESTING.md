# Testing Patterns

**Analysis Date:** 2026-03-25

## Test Framework

**Go:**
- **Runner:** `testing` (stdlib)
- **Config:** Makefile targets in `/Users/alin/code/caption/agentsview/Makefile`
- **Build tag:** `-tags fts5` required for FTS5 full-text search support
- **Environment:** CGO_ENABLED=1 required (SQLite3 driver needs C compilation)

**TypeScript/Frontend:**
- **Runner:** Vitest 4.1.0
- **Assertion Library:** `@vitest/expect` (built-in)
- **Config:** `frontend/vite.config.ts` includes test configuration with jsdom environment
- **E2E:** Playwright 1.58.2 with config in `frontend/playwright.config.ts`

**Run Commands:**
```bash
# Go unit tests
make test       # Run all tests with -v and -count=1
make test-short # Run only tests marked with -short flag

# PostgreSQL integration tests (requires docker-compose)
make test-postgres   # Starts PG container, runs tests, leaves container running
make postgres-down   # Stop the test container

# Frontend tests
cd frontend && npm run test   # Run vitest
cd frontend && npm run e2e    # Run Playwright E2E tests

# Code quality
make vet   # go vet with -tags fts5
make lint  # golangci-lint (requires installation)
```

## Test File Organization

**Location:**
- Go: **colocated with source** - `*.go` and `*_test.go` in same directory
- TypeScript: **colocated with source** - `*.ts` and `*.test.ts` in same directory; E2E tests separate in `frontend/e2e/`

**Naming:**
- Go: `<filename>_test.go` (e.g., `sync_test.go`, `search_test.go`)
- TypeScript: `<filename>.test.ts` (e.g., `sessions.test.ts`, `content-parser.test.ts`)
- Playwright E2E: `<feature>.spec.ts` (e.g., `navigation.spec.ts`)

**Structure (by package):**
```
cmd/agentsview/
├── main.go
├── sync.go
└── sync_test.go          # Tests in same package (package main)

internal/db/
├── db.go
├── sessions.go
├── sessions_test.go
└── search_test.go

frontend/src/lib/stores/
├── sessions.svelte.js
├── sessions.test.ts      # Colocated test
└── messages.test.ts
```

## Test Structure

**Go Table-Driven Tests:**
```golang
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name      string          // Test case name
        input     string          // Input parameter
        wantOutput string         // Expected result
        wantErr   string          // Expected error substring
        check     func(t *testing.T, result Result) // Optional assertion helper
    }{
        {
            name: "description of case",
            input: "value",
            wantOutput: "expected",
            check: func(t *testing.T, result Result) {
                t.Helper()  // Mark helper to improve error reporting
                if result.Field != "expected" {
                    t.Errorf("Field = %q, want %q", result.Field, "expected")
                }
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test body
            result, err := FunctionName(tt.input)

            // Error handling
            if tt.wantErr != "" {
                if err == nil {
                    t.Fatalf("expected error containing %q", tt.wantErr)
                }
                if !strings.Contains(err.Error(), tt.wantErr) {
                    t.Fatalf("error %q missing %q", err, tt.wantErr)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }

            // Assertions
            if tt.check != nil {
                tt.check(t, result)
            }
        })
    }
}
```

**TypeScript/Vitest Tests:**
```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";

describe("FeatureName", () => {
    let state: ReturnType<typeof createStore>;

    beforeEach(() => {
        vi.clearAllMocks();
        // Setup
        state = createStore();
    });

    describe("functionality", () => {
        it("should do something", () => {
            state.action();
            expect(state.value).toBe(expected);
        });

        it("should handle error case", async () => {
            const result = await functionThatThrows();
            expect(result).toThrow(Error);
        });
    });
});
```

**Playwright E2E Tests:**
```typescript
import { test, expect } from "@playwright/test";

test.describe("Feature", () => {
    test.beforeEach(async ({ page }) => {
        await page.goto("/");
    });

    test("should navigate with keyboard", async ({ page }) => {
        await page.keyboard.press("]");
        expect(page.url()).toContain("/session/");
    });
});
```

## Mocking

**Go:**
- **Approach:** Interface-based mocking using embedded types
- **Pattern:** Create a spy struct that embeds the interface and overrides needed methods

**Example from `internal/server/search_test.go`:**
```golang
type searchSpy struct {
    db.Store                    // Embed interface to satisfy it
    filter   db.SearchFilter    // Capture arguments
}

func (s *searchSpy) HasFTS() bool { return true }

func (s *searchSpy) Search(
    _ context.Context, f db.SearchFilter,
) (db.SearchPage, error) {
    s.filter = f  // Capture for assertion
    return db.SearchPage{}, nil
}
```

**TypeScript:**
- **Framework:** `vi.mock()` from vitest
- **Pattern:** Mock modules at module level, use `vi.mocked()` to access mocked functions

**Example from `frontend/src/lib/stores/sessions.test.ts`:**
```typescript
vi.mock("../api/client.js", () => ({
    listSessions: vi.fn(),
    getSession: vi.fn(),
    getProjects: vi.fn(),
}));

beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listSessions).mockResolvedValue({
        sessions: [],
        total: 0,
    });
});
```

## What to Mock

**In Go:**
- ✓ Database operations via `db.Store` interface (use test database or spy)
- ✓ External HTTP calls via client interfaces
- ✓ File system operations via abstractions
- ✗ Standard library packages (avoid if possible)
- ✗ Internal package functions (test real implementation)

**In TypeScript:**
- ✓ HTTP API client functions
- ✓ External service dependencies
- ✓ Async operations that require specific behavior
- ✗ Store internal state (test real state mutations)
- ✗ Utility functions (test with real implementations)

## Fixtures and Factories

**Go Test Helpers:**
- **Location:** Inline in test files or in test helper packages
- **Pattern:** Helper functions prefixed with `test` (e.g., `testDB(t)`, `testConfig()`)
- **Temp directories:** Use `t.TempDir()` for isolated file system access (see `cmd/agentsview/sync_test.go`)
- **Database:** Use `testDB(t)` to create isolated SQLite database for each test

**TypeScript Helpers:**
- **Helpers:** Defined as functions in test files (e.g., `mockListSessions()`, `mockGetProjects()` in `sessions.test.ts`)
- **Location:** Colocated with test file
- **Data:** Test fixtures defined as inline objects or helper functions that return test data

## Async Testing

**Go:**
- Use goroutines with `sync.WaitGroup` to test concurrent code
- Use `time.Sleep()` sparingly; prefer channels or condition variables
- Test timeout handling with context.WithTimeout

**TypeScript:**
```typescript
it("should handle async operations", async () => {
    const result = await asyncFunction();
    expect(result).toBe(expected);
});

it("should reject on error", async () => {
    await expect(asyncFunction()).rejects.toThrow("error message");
});
```

## Error Testing

**Go:**
- Check error presence with `if err != nil`
- Match error substrings with `strings.Contains(err.Error(), substring)`
- Use `errors.Is()` for wrapped errors that match a sentinel

**Example from `internal/server/search_test.go`:**
```golang
if tt.wantErr != "" {
    if err == nil {
        t.Fatalf("expected error containing %q", tt.wantErr)
    }
    if !strings.Contains(err.Error(), tt.wantErr) {
        t.Fatalf("error %q missing %q", err, tt.wantErr)
    }
    return
}
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
```

**TypeScript:**
- Use `expect().toThrow()` for thrown errors
- Use `expect().rejects` for rejected promises

## Coverage

**Requirements:**
- No explicit coverage % enforced in CI
- New tests required for all features and bug fixes (per CLAUDE.md)
- Focus on critical paths, error cases, and integration points

**View Coverage:**
```bash
# Go coverage
go test -tags fts5 -cover ./...
go test -tags fts5 -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Frontend coverage (not configured by default)
# Can be added to vitest.config.ts coverage settings
```

## Test Types

**Unit Tests:**
- **Scope:** Individual functions or methods
- **Approach:** Table-driven tests in Go, vitest describe/it in TypeScript
- **Isolation:** Mock external dependencies, use test doubles for interfaces
- **Example:** `TestValidateSort`, `TestPrepareFTSQuery` in `internal/server/search_test.go`

**Integration Tests:**
- **Scope:** Multiple components working together
- **Approach:** Use real database (test db in tempdir), real file system
- **Location:** In same test file as unit tests, marked with comments or separated
- **Example:** Parser tests with real JSONL files, sync engine tests with real DB

**PostgreSQL Integration Tests:**
- **Setup:** Requires real PostgreSQL via docker-compose or manual instance
- **Build tag:** `-tags pgtest` in addition to `fts5`
- **Connection:** TEST_PG_URL env var specifies database
- **Cleanup:** Tests create/drop schema automatically; use dedicated database
- **Run:** `make test-postgres` handles container lifecycle

**E2E Tests:**
- **Framework:** Playwright 1.58.2 in `frontend/e2e/`
- **Scope:** Full user workflows (navigation, interactions, state)
- **Setup:** Playwright config starts agentsview server automatically on port 8090
- **Browser:** Chromium only (configured in `frontend/playwright.config.ts`)
- **Example:** `frontend/e2e/navigation.spec.ts` tests keyboard navigation between sessions

## Common Patterns

**Parallel Tests (Go):**
- Use `t.Parallel()` in test functions to enable parallel test execution
- Example: `search_test.go` marks tests with `t.Parallel()`
- Caveat: Database tests should not use parallelism (see database test setup)

**Test Isolation (Go):**
```golang
func TestSomething(t *testing.T) {
    t.Parallel()

    // Use temporary directory for file operations
    tempDir := t.TempDir()

    // Use isolated database connection
    db := testDB(t)
    defer db.Close()

    // Test code here
}
```

**Helper Functions with t.Helper():**
```golang
func expectError(t *testing.T, got error, want string) {
    t.Helper()  // Mark as helper to improve error location reporting
    if !strings.Contains(got.Error(), want) {
        t.Errorf("error %q missing %q", got, want)
    }
}

// In test:
expectError(t, err, "expected message")
```

**Mock Function Verification (TypeScript):**
```typescript
// Verify mock was called with specific arguments
expect(api.listSessions).toHaveBeenLastCalledWith(
    expect.objectContaining({ project: "myproj" })
);
```

**Before/After Setup:**

Go: No built-in before/after; use helper functions or setUp code at test start:
```golang
func TestSuite(t *testing.T) {
    // Setup
    db := testDB(t)
    defer db.Close()  // Cleanup via defer

    // Test body
}
```

TypeScript: Use `beforeEach`/`afterEach`:
```typescript
beforeEach(() => {
    vi.clearAllMocks();
    mockListSessions();
});

afterEach(() => {
    // Additional cleanup if needed
});
```

## Testing Best Practices

**Do:**
- Write test cases for edge cases and error conditions
- Use descriptive test case names that explain what is being tested
- Isolate tests (no shared state between test functions)
- Use table-driven tests for parameter variations
- Clear mocks between tests

**Don't:**
- Write tests that depend on execution order
- Share test data across test functions
- Mock more than necessary
- Test private implementation details (test behavior instead)
- Commit tests with `t.Skip()` or skipped test suites

---

*Testing analysis: 2026-03-25*
