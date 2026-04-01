# Coding Conventions

**Analysis Date:** 2026-03-31

## Naming Patterns

**Files:**
- Go: `snake_case.go` for all source files (e.g., `sessions.go`, `db_test.go`)
- Go platform variants: `<name>_<os>.go` (e.g., `boottime_darwin.go`, `procstart_linux.go`)
- Go test files: `<name>_test.go` co-located with the source file
- TypeScript/Svelte: `camelCase.ts` for utilities, `PascalCase.svelte` for components
- Frontend test files: `<name>.test.ts` co-located with the source file (e.g., `format.test.ts`)

**Packages:**
- Go: lowercase package names matching directory names (`package db`, `package server`)
- External test packages use `_test` suffix: `package server_test`, `package db_test`
- Internal test packages (white-box) use same package name: `package server`, `package db`
- Internal-only white-box test files named `<name>_internal_test.go` (e.g., `deadline_internal_test.go`, `helpers_internal_test.go`)

**Functions:**
- Go: exported functions use `PascalCase` (`GetSession`, `ListSessions`, `UpsertSession`)
- Go: unexported functions use `camelCase` (`buildSessionFilter`, `scanSessionRow`, `makeDSN`)
- TypeScript: `camelCase` for all functions (`formatTokenCount`, `sanitizeSnippet`, `listSessions`)

**Variables and Constants:**
- Go: exported constants use `PascalCase` (`DefaultSessionLimit`, `MaxSessionLimit`, `ErrInvalidCursor`)
- Go: unexported constants use `camelCase` (`dataVersion`, `schemaFTS`)
- Go: error sentinel variables named `Err<Description>` (e.g., `ErrInvalidCursor`, `ErrReadOnly`, `ErrSessionExcluded`)
- TypeScript: `camelCase` for variables, `PascalCase` for types and interfaces

**Types:**
- Go: exported types use `PascalCase` (`Session`, `SessionFilter`, `SessionPage`, `SearchFilter`)
- Go: interface types do not use `I` prefix — e.g., `Store`, not `IStore`
- TypeScript: `PascalCase` for type aliases and interfaces (`Session`, `ListSessionsParams`)

**Test helpers:**
- Go: helper functions prefixed with `require` for fatal assertions (`requireNoError`, `requireFTS`, `requireSessionExists`)
- Go: helper functions prefixed with `assert` for non-fatal assertions (`assertRecorderStatus`, `assertContentType`, `assertContainsAll`)
- Go: setup helpers named `setup`, `testDB`, `testServer`
- Go: seed/data builder helpers named descriptively (`userMsg`, `asstMsg`, `insertSession`, `seedSession`)

## Code Style

**Formatting:**
- Go: `go fmt ./...` required before commits (CLAUDE.md)
- Go: `go vet ./...` required before commits (CLAUDE.md)
- TypeScript: no separate formatter config; relies on TypeScript strict compiler checks

**Linting:**
- Go: `golangci-lint` with config at `.golangci.yml` (version 2)
  - Enabled: `errcheck`, `govet`, `ineffassign`, `modernize`, `staticcheck`, `unused`
  - `errcheck` excluded for `_test.go` files
  - Build tag `fts5` included in linter run
- TypeScript: `svelte-check` with `tsconfig.json`
  - `"strict": true`
  - `"noUncheckedIndexedAccess": true` — array index access is typed as potentially undefined
  - `"noImplicitOverride": true`
  - `"verbatimModuleSyntax": true`

## Import Organization

**Go order (standard goimports grouping):**
1. Standard library packages
2. Third-party packages (blank line separator)
3. Internal packages (e.g., `github.com/wesm/agentsview/internal/...`)

**Package alias usage:**
- Alias stdlib packages only when they conflict with internal names (e.g., `gosync "sync"` in `internal/server/server.go` to avoid clash with the `sync` package variable name)

**TypeScript import order:**
1. Vitest/testing imports (`describe`, `it`, `expect`, `vi`, `beforeEach`)
2. Module under test (named imports)
3. Related types (`import type { ... }`)

**Path aliases:**
- No custom TypeScript path aliases; all imports use relative paths
- ESM-compatible `.js` extension used in all TypeScript imports (e.g., `"./format.js"`, `"../api/types.js"`)

## Error Handling

**Go patterns:**
- Wrap errors with context using `fmt.Errorf("operation description: %w", err)`
- Sentinel errors for distinguishable cases: `var ErrFoo = errors.New("...")` at package level
- Custom error types implement `error` interface when needed (e.g., `errReadOnly struct{}` in `internal/db/store.go`)
- HTTP handlers delegate to helper functions for common error cases (all in `internal/server/response.go`):
  - `handleContextError(w, err)` — handles `context.Canceled` silently, `context.DeadlineExceeded` with 504
  - `handleReadOnly(w, err)` — handles `ErrReadOnly` with 501 Not Implemented
  - `writeError(w, status, "message")` — emits JSON `{"error":"..."}` with given status
  - `writeJSON(w, status, v)` — standard JSON response helper
- Context cancellation is handled silently (no response written) — assumed client disconnected
- `log.Fatal` only for unrecoverable startup errors; all other errors returned to caller

**TypeScript patterns:**
- Async functions return promises; errors bubble up naturally to store/component
- HTTP errors surfaced via `ApiError` class exported from `frontend/src/lib/api/client.ts`
- No defensive try/catch wrappers at every call site; boundary-level error handling in stores

## Logging

**Go:**
- `log.Printf` for informational runtime messages
- `log.Fatalf` only for unrecoverable startup failures
- Plain stdlib `log` package — no structured logging library
- Log messages: lowercase, no trailing punctuation (e.g., `"data version outdated; full resync required"`)
- No emojis in log output (project-wide rule from CLAUDE.md)

**TypeScript:**
- No logging framework; `console.error` for unexpected non-fatal browser errors

## Comments

**Go doc comments:**
- Exported types and functions always have a doc comment starting with the identifier name
- Example: `// scanSessionRow scans sessionBaseCols into a Session.`
- Unexported functions commented when behavior is non-obvious
- Constants with related semantics grouped under a single block comment
- Inline comments explain SQL query intent and non-obvious logic paths

**TypeScript:**
- JSDoc used for public methods in page objects (`frontend/e2e/pages/sessions-page.ts`)
- Inline comments for non-obvious logic; test file comments describe intent
- No requirement for JSDoc on all utility functions

## Function Design

**Size:** Keep functions focused. Complex SQL queries are stored as `const` strings or built by dedicated `build*` helpers (e.g., `buildSessionFilter` in `internal/db/sessions.go`).

**Parameters:**
- Go: `context.Context` always first for I/O-touching functions: `func (db *DB) GetSession(ctx context.Context, id string)`
- Go: functional options pattern for server construction: `opts ...Option` where `type Option func(*Server)`
- Go: test builder options via variadic `...func(*T)`: `func insertSession(t *testing.T, d *DB, id, project string, opts ...func(*Session))`

**Return Values:**
- Go: `(result, error)` for all fallible operations; never panic for recoverable errors
- Go: nullable domain objects returned as pointer: `GetSession` returns `(*Session, error)` where `nil, nil` means not found
- Go: `(bool, error)` pattern when presence and error must both be communicated

## Module Design

**Go package structure:**
- Each `internal/` subdirectory is a self-contained package with single responsibility
- `db.Store` interface in `internal/db/store.go` decouples HTTP handlers from SQLite
- Compile-time interface check pattern: `var _ Store = (*DB)(nil)` — used in `internal/db/store.go`
- Platform-specific code split into `_<os>.go` files with `//go:build` constraints

**Struct tags:**
- JSON tags on all exported struct fields using `snake_case`: `json:"field_name"`
- TOML tags on config structs: `toml:"field_name"`
- `omitempty` used selectively on optional/nullable fields
- Internal fields excluded from serialization with `json:"-"`
- Example from `internal/db/sessions.go`:

```go
type Session struct {
    ID           string  `json:"id"`
    Project      string  `json:"project"`
    DisplayName  *string `json:"display_name,omitempty"`
    DBPath       string  `json:"-" toml:"-"`
}
```

**TypeScript module design:**
- Svelte stores use factory function pattern returning a store instance with methods: `createSessionsStore()`
- API client functions are plain named exports from `frontend/src/lib/api/client.ts`
- Types separated into `frontend/src/lib/api/types.ts`
- Test-only exports prefixed with `_` to signal internal use: `_resetNonceCounter`

## Project-Specific Rules

From CLAUDE.md — these must always be followed:

- **Never drop, truncate, or recreate the database** to handle data version changes. Use non-destructive migrations (`ALTER TABLE`, `UPDATE`) or full resync via swap.
- **No emojis** in code, comments, log output, or user-facing text.
- **Prefer stdlib** over external dependencies.
- **Tests must be fast and isolated** — use `t.TempDir()` for all temporary directories.
- **Commit every turn** — commit at end of each working turn.
- **Markdown files** formatted with `mdformat --wrap 80` (requires `mdformat-tables` plugin).

---

*Convention analysis: 2026-03-31*
