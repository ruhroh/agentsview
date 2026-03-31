# Coding Conventions

**Analysis Date:** 2026-03-25

## Naming Patterns

**Files:**
- Go files: lowercase with underscores (e.g., `sync_test.go`, `db.go`, `claude.go`)
- Test files: suffixed with `_test.go` in same package as code under test
- TypeScript/Svelte files: kebab-case for component files (e.g., `session-list-utils.ts`), camelCase for utilities (e.g., `content-parser.ts`)
- Test files: suffixed with `.test.ts` colocated with source (e.g., `sessions.svelte.js` and `sessions.test.ts`)

**Functions:**
- Go: exported functions PascalCase (e.g., `ParseClaudeSession`, `GetReader`, `Open`), private functions camelCase (e.g., `scanSessionRow`, `makeD`, `probeDatabase`)
- TypeScript: camelCase for both exported and internal functions (e.g., `createSessionsStore`, `getSession`, `listSessions`)

**Variables:**
- Go: camelCase for local variables and struct fields (e.g., `entries`, `subagentMap`, `filePath`)
- Go structs: exported fields PascalCase (e.g., `ID`, `FirstMessage`, `DeletedAt`)
- TypeScript: camelCase consistently (e.g., `sessionID`, `endedAt`, `messageCount`)
- Constants in Go: UPPER_SNAKE_CASE (e.g., `maxLineSize`, `forkThreshold`, `batchSize`)
- HTTP query parameters and JSON fields: snake_case (e.g., `next_cursor`, `user_message`, `session_count`)

**Types:**
- Go: exported types PascalCase (e.g., `Session`, `SearchFilter`, `Engine`, `DB`)
- TypeScript: exported types PascalCase or `Type` suffix (e.g., `SessionPage`, `Granularity`)
- Interface types in Go: suffixed with lowercase singular noun (e.g., `rowScanner`, `syncStateStore`)
- JSON struct tags: snake_case with omitempty for optional fields (e.g., `json:"display_name,omitempty"`)

## Code Style

**Formatting:**
- Go: standard `gofmt` formatting (run via `go fmt ./...`)
- TypeScript: Vite + Svelte 5 setup with strict TypeScript (`noUncheckedIndexedAccess`, `verbatimModuleSyntax`)
- Frontend: Svelte 5 reactive runes (.svelte.js files) for stores

**Linting:**
- Go: golangci-lint with these enabled linters: errcheck, govet, ineffassign, modernize, staticcheck, unused
- Go: errcheck is disabled for _test.go files (test error handling not enforced)
- Go: modernize linter excludes "omitzero:" rule
- TypeScript: svelte-check for Svelte type checking; no explicit ESLint config (relies on TS strict mode)

## Import Organization

**Order (Go):**
1. Standard library imports (`package`, `context`, `database/sql`, etc.)
2. Third-party imports (`github.com/`, other external packages)
3. Internal imports (`github.com/wesm/agentsview/internal/...`)
- Blank line between each group

**Order (TypeScript):**
1. Node built-ins (`"node:..."`)
2. npm packages (`@sveltejs`, `vitest`, etc.)
3. Local imports (`"../..."`, `"./"`)
- No blank lines required by convention; imports often grouped by feature

**Path Aliases:**
- Go: no aliases; use full import paths
- TypeScript: implicit `src` aliasing via Vite (imports from `src/` can use direct paths like `"../stores"`)

## Error Handling

**Patterns:**
- Go: errors wrapped with `fmt.Errorf("%w", err)` to preserve error chain
- Go: custom error types defined as `var ErrName = errors.New("message")` in package scope (see `ErrInvalidCursor`, `ErrSessionExcluded` in `internal/db/sessions.go`)
- Go: error checking done inline with early return (no error accumulation)
- TypeScript: Promise rejections caught in try/catch blocks; optional chaining (`?.`) used for safe property access

**Return values:**
- Go: errors always returned last; functions return `(result, error)` or `(result1, result2, error)`
- Go: `nil` returned for error-free execution
- TypeScript: Promises used for async operations; errors thrown for exceptional cases

## Logging

**Framework:** Go `log` package (stdlib) - specifically `log.Printf()` for non-fatal messages

**Patterns:**
- Go: `log.Printf()` used for informational and warning messages in startup/sync logic (see `internal/db/db.go`, `internal/sync/engine.go`)
- Go: debug logs rare; logging focused on significant state transitions
- TypeScript: console logging minimal; mostly used in test setup/teardown

**What to log:**
- Database version mismatches or schema changes
- Sync engine state transitions
- Configuration loading issues
- Parser errors and skipped files (rare)

**What NOT to log:**
- Sensitive credentials or tokens
- Full file contents or large data structures
- Per-request HTTP details

## Comments

**When to Comment:**
- Complex algorithms requiring explanation (DAG fork detection in `claude.go`)
- Exported functions (Go convention: comment above function name)
- Non-obvious state management or synchronization logic
- Constants with special meaning (see `dataVersion` comment in `internal/db/db.go`)
- Business logic constraints (see `forkThreshold` in `internal/parser/claude.go`)

**JSDoc/Doc Comments:**
- Go: comment strings placed directly above exported items; follow Go documentation conventions
- Go: example: `// Open creates or opens a SQLite database at the given path.` followed by implementation details
- TypeScript: minimal; mostly rely on TS type hints rather than comments
- Frontend: component comments for complex Svelte state

**Comment style:**
- Start with subject (e.g., "// ParseClaudeSession parses...")
- Use second person sparingly; prefer active voice
- Multi-line comments: separate related thoughts with blank comment lines

**ABOUTME comments:**
- Used in package files for high-level package documentation (e.g., `// ABOUTME: Parses Claude Code JSONL session files...` in `internal/parser/claude.go`)
- Provides context for what the entire file/package does

## Function Design

**Size:**
- Go: prefer functions under 100 lines; break complex logic into helpers
- TypeScript: prefer small store functions; use composition for complex state logic

**Parameters:**
- Go: pass configurations as structs when many related options (e.g., `EngineConfig` in `internal/sync/engine.go`)
- Go: use Option pattern for optional parameters on constructors (see `Option func(*Server)` in `internal/server/server.go`)
- TypeScript: use object parameters with destructuring for optional flags (e.g., `overrides?: Partial<...>` in tests)

**Return Values:**
- Go: always return error as last value if present
- Go: use named return values only for clarity on large functions (most functions use unnamed returns)
- TypeScript: return Promises for async; use tuple types sparingly

## Module Design

**Exports:**
- Go: packages export top-level New/Create functions; constructors initialize all needed state
- Go: example: `func New(cfg config.Config, database db.Store, ...) *Server` in `internal/server/server.go`
- Go: use interfaces for dependencies to enable mocking (see `db.Store` interface used by handlers)
- TypeScript: export default or named exports depending on module purpose

**Barrel Files:**
- Go: no barrel files; each package is self-contained
- TypeScript: `types.js` and `client.js` files serve as barrels for API types/functions

**Package organization:**
- Go: one responsibility per package (`db` handles database ops, `sync` handles file discovery, `parser` handles session parsing)
- Go: internal packages prefixed with `internal/` to prevent external imports
- TypeScript: stores organized by feature (`sessions.svelte.js`, `messages.test.ts`, etc.)

## Testing Conventions

**Test helpers:**
- Go: use `t.Helper()` in helper functions to improve error reporting (see `sync_test.go`)
- Go: table-driven tests preferred for parameterized testing (see `TestParseSyncFlags` in `sync_test.go`)
- TypeScript: use `beforeEach` to set up mocks and clear state before each test

**Type assertions and mocking:**
- Go: mock database stores using embedded interfaces (e.g., `searchSpy` in `internal/server/search_test.go` embeds `db.Store` and implements only needed methods)
- TypeScript: `vi.mock()` used to mock API client; `vi.mocked()` to access mocked functions

**Test structure:**
- Go: test functions named `Test<FunctionName>` or `Test<FunctionName><Case>`
- TypeScript: vitest describe/it structure; tests organized by feature store

---

*Convention analysis: 2026-03-25*
