# Session Markdown Export Implementation Plan

> **For agentic workers:** REQUIRED: Use `/skill:orchestrator-implements` (in-session, orchestrator implements), `/skill:subagent-driven-development` (in-session, subagents implement), or `/skill:executing-plans` (parallel session) to implement this plan. Steps use checkbox syntax for tracking.

**Goal:** Add `GET /api/v1/sessions/{id}/md` that exports session transcripts as markdown with structured XML tags, optional descendant inclusion, and session-screen ordering semantics.

**Architecture:** Extend server export support with a markdown-specific handler and renderer in `internal/server/export.go`. Build markdown output from session/message data using backend logic that mirrors frontend content segmentation and tool-body fallback rules, then recurse through child sessions with anchored subagent placement or appended child-session blocks.

**Tech Stack:** Go, net/http, existing server/db layers, Go tests in `internal/server`, frontend parser semantics used as backend reference only.

---

### Task 1: Finalize spec + plan artifacts

**TDD scenario:** Trivial change — use judgment

**Files:**
- Modify: `docs/plans/2026-04-13-session-markdown-export-design.md`
- Create: `docs/plans/2026-04-13-session-markdown-export-implementation-plan.md`

- [ ] **Step 1: Re-read final spec and ensure implementation tasks cover every requirement**

Read:
- `docs/plans/2026-04-13-session-markdown-export-design.md`
- `internal/server/export.go`
- `frontend/src/lib/utils/content-parser.ts`
- `frontend/src/lib/utils/tool-params.ts`

Expected: clear mapping from spec requirements to implementation tasks below.

- [ ] **Step 2: Sanity-check docs formatting**

Run:
```bash
git diff --check -- docs/plans/2026-04-13-session-markdown-export-design.md docs/plans/2026-04-13-session-markdown-export-implementation-plan.md
```

Expected: no whitespace errors.

- [ ] **Step 3: Commit docs-only planning artifacts**

Run:
```bash
git add docs/plans/2026-04-13-session-markdown-export-design.md docs/plans/2026-04-13-session-markdown-export-implementation-plan.md
git commit -m "docs: finalize markdown export spec and plan"
```

Expected: planning docs committed before source changes.

### Task 2: Add failing markdown export formatter tests

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `internal/server/export_test.go`
- Modify: `internal/server/server_test.go`
- Reference: `internal/server/export.go`, `frontend/src/lib/utils/content-parser.ts`, `frontend/src/lib/utils/tool-params.ts`

- [ ] **Step 1: Write failing formatter tests for segment rendering**

Add table-driven tests covering:
- text segment escaping
- `<thinking>` rendering
- `<tool_call>`, `<tool_body>`, `<arguments>`, `<tool_result>` ordering
- Task/Agent prompt precedence over generic tool body
- legacy tool segment without structured `toolCall`
- `<code_block>` rendering
- `<skill>` rendering
- CDATA fallback on `]]>`
- omission of empty optional attributes

Example skeleton:

```go
func TestGenerateExportMarkdown_ToolOrdering(t *testing.T) {
	session := testSession()
	msgs := []db.Message{{
		SessionID: "test-id",
		Ordinal:   0,
		Role:      "assistant",
		Content:   "[Task]\nbody",
		HasToolUse: true,
		ToolCalls: []db.ToolCall{{
			ToolName:  "Task",
			Category:  "Task",
			ToolUseID: "toolu_1",
			InputJSON: `{"prompt":"inspect repo"}`,
			ResultContent: "done",
		}},
	}}

	out := generateExportMarkdown(session, msgs, exportMarkdownOptions{})
	assertOrdered(t, out,
		`<tool_call id="toolu_1" name="Task" category="Task">`,
		`<tool_body><![CDATA[`+"\ninspect repo\n"+`]]></tool_body>`,
		`<tool_result><![CDATA[`+"\ndone\n"+`]]></tool_result>`,
	)
}
```

- [ ] **Step 2: Write failing handler tests for route/query/header behavior**

Add tests for:
- `GET /api/v1/sessions/{id}/md`
- `Content-Type: text/markdown`
- inline `Content-Disposition` filename
- invalid `depth` => `400`
- not found => `404`
- route reachable through normal timeout-wrapped server registration

Example skeleton:

```go
func TestMarkdownSessionExport(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 1)
	te.seedMessages(t, "s1", 1)

	w := te.get(t, "/api/v1/sessions/s1/md")
	assertStatus(t, w, http.StatusOK)
	assertHeaderContains(t, w, "Content-Type", "text/markdown")
	assertHeaderContains(t, w, "Content-Disposition", "inline")
}
```

- [ ] **Step 3: Run focused tests and verify they fail for feature-missing reasons**

Run:
```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server -run 'Test(MarkdownSessionExport|GenerateExportMarkdown)' -count=1
```

Expected: FAIL with undefined handler/renderer behavior or missing assertions for markdown export, not syntax/import errors.

- [ ] **Step 4: Commit failing tests**

Run:
```bash
git add internal/server/export_test.go internal/server/server_test.go
git commit -m "test: add markdown export coverage"
```

Expected: red tests captured in history.

### Task 3: Implement markdown segment parsing and rendering

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `internal/server/export.go`
- Reference: `frontend/src/lib/utils/content-parser.ts`
- Reference: `frontend/src/lib/utils/tool-params.ts`
- Test: `internal/server/export_test.go`

- [ ] **Step 1: Add backend markdown export options and route-facing renderer entrypoints**

Implement minimal types/helpers, for example:

```go
type exportMarkdownOptions struct {
	Depth string
}

type markdownRenderState struct {
	visited map[string]bool
}

func generateExportMarkdown(
	session *db.Session,
	msgs []db.Message,
	opts exportMarkdownOptions,
) string {
	// build root heading + session container + rendered messages
}
```

- [ ] **Step 2: Port frontend segment parsing semantics needed by export**

Add backend helpers in `internal/server/export.go` for:
- thinking detection with marked + legacy forms
- skill block detection
- tool block detection with alias normalization
- code block detection
- inline-code false-positive avoidance
- overlap resolution and segment building
- structured tool-call enrichment including appended structured-only tools

Suggested helper names:

```go
func parseMarkdownExportSegments(m db.Message) []exportSegment
func enrichMarkdownToolSegments(segs []exportSegment, calls []db.ToolCall) []exportSegment
```

- [ ] **Step 3: Add safe XML/CDATA serialization helpers**

Implement helpers such as:

```go
func escapeXMLText(s string) string
func escapeXMLAttr(s string) string
func wrapXMLText(tag string, attrs map[string]string, body string) string
func wrapXMLCDATAOrEscaped(tag string, attrs map[string]string, body string) string
```

Rules:
- omit empty attrs
- CDATA when safe
- escaped text fallback on `]]>`
- keep top-level heading root-only

- [ ] **Step 4: Implement segment rendering in session-screen order**

For each `<message>`:
- emit empty block even when no visible segments remain
- `text` => escaped text node
- `thinking` => `<thinking>`
- `code` => `<code_block>`
- `skill` => `<skill>`
- `tool` => `<tool_call>`, optional `<arguments>`, optional `<tool_body>`, optional `<tool_result>` main output, optional result history, optional trailing `<subagent_anchor>` placeholder to be filled by recursion task

Tool body precedence must match spec:
1. Task/Agent prompt
2. enriched segment content
3. generated fallback body
4. omit when empty

- [ ] **Step 5: Run focused formatter tests and verify they pass**

Run:
```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server -run 'TestGenerateExportMarkdown' -count=1
```

Expected: PASS for formatter-focused tests.

- [ ] **Step 6: Commit renderer implementation**

Run:
```bash
git add internal/server/export.go internal/server/export_test.go
git commit -m "feat: add markdown export renderer"
```

Expected: renderer work committed separately from routing.

### Task 4: Implement recursive child-session export and HTTP handler

**TDD scenario:** Modifying tested code — run existing tests first

**Files:**
- Modify: `internal/server/export.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Reference: `internal/db/sessions.go`, `internal/db/messages.go`

- [ ] **Step 1: Run existing server tests touching export routes before changing handler wiring**

Run:
```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server -run 'Test(ExportSession|ExportSession_NotFound|ExportSession_HTMLContent)' -count=1
```

Expected: PASS before routing changes.

- [ ] **Step 2: Add markdown handler and depth validation**

Implement:

```go
func (s *Server) handleMarkdownSession(
	w http.ResponseWriter, r *http.Request,
) {
	// validate depth, fetch root, render markdown,
	// set inline markdown headers, write body
}
```

Wire in `internal/server/server.go` using normal timeout wrapper:

```go
s.mux.Handle(
	"GET /api/v1/sessions/{id}/md",
	s.withTimeout(s.handleMarkdownSession),
)
```

- [ ] **Step 3: Implement descendant loading + placement rules**

Add helpers roughly like:

```go
func (s *Server) loadMarkdownExportTree(
	ctx context.Context,
	rootID string,
	depth string,
) (*exportSessionTree, error)
```

Rules:
- default: no descendants
- `depth=1`: direct children only
- `depth=all`: recurse with visited set
- anchored child: match by `toolCall.subagent_session_id`, inline once only
- unanchored child: append as `<child_session>` after transcript ordered by `started_at`, then `id`
- missing child target: skip safely

- [ ] **Step 4: Run handler-focused tests and verify they pass**

Run:
```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server -run 'TestMarkdownSessionExport' -count=1
```

Expected: PASS for route/query/header behavior and descendant export tests.

- [ ] **Step 5: Commit route + recursion work**

Run:
```bash
git add internal/server/export.go internal/server/server.go internal/server/server_test.go
git commit -m "feat: add markdown session export endpoint"
```

Expected: endpoint and recursion behavior committed.

### Task 5: Verify full server package and prepare PR

**TDD scenario:** Modifying tested code — run existing tests before and after

**Files:**
- Modify: any review-fix files from previous tasks
- Test: `internal/server/export_test.go`, `internal/server/server_test.go`

- [ ] **Step 1: Run full relevant test suite**

Run:
```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server/... -count=1
```

Expected: PASS with 0 failures.

- [ ] **Step 2: Run lightweight diff verification**

Run:
```bash
git diff --check
```

Expected: no whitespace or conflict-marker issues.

- [ ] **Step 3: Review final diff for PR summary inputs**

Run:
```bash
git status --short
git diff --stat HEAD~3..HEAD
```

Expected: only markdown export–related files changed.

- [ ] **Step 4: Commit any final review-driven fixes**

Run:
```bash
git add -A
git commit -m "fix: polish markdown session export"
```

Expected: only if post-test/review fixes exist.

- [ ] **Step 5: Push branch and open PR**

Run:
```bash
git push -u origin abiding-almanac
gh pr create --title "feat: add markdown session export" --body "## Summary
- add /api/v1/sessions/{id}/md markdown export endpoint
- render session-screen ordered markdown/XML transcript with optional descendants
- cover route, formatting, and recursion with server tests

## Validation
- CGO_ENABLED=1 go test -tags fts5 ./internal/server/... -count=1
- git diff --check"
```

Expected: branch pushed, PR URL returned.

## Self-review

- Spec coverage: route, headers, depth handling, ordering, descendant placement,
  escaping, CDATA fallback, optional attrs, system messages, and recursion
  safety are all mapped into Tasks 2-5.
- Placeholder scan: no TODO/TBD steps; each task has concrete files and
  commands.
- Consistency: plan uses one renderer in `internal/server/export.go`, one route
  in `internal/server/server.go`, and one verification command family in
  `internal/server` tests.
