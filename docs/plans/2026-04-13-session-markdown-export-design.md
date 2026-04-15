# Session markdown export design

## Summary

Add `GET /api/v1/sessions/{id}/md` to serve session content as markdown for
reuse as agent context. Output should favor machine ingestion over visual
rendering while staying readable to humans.

This export is separate from existing HTML export and GitHub Gist publish flow.
Current system already supports:

- SPA links to `/sessions/{id}`
- HTML export at `GET /api/v1/sessions/{id}/export`
- HTML gist publish at `POST /api/v1/sessions/{id}/publish`

Current system does **not** support a markdown transcript endpoint or a stable
URL that serves session content as markdown.

## Goals

- Provide stable local URL for markdown transcript access
- Keep transcript readable by humans
- Keep structure easy for downstream agents to parse
- Preserve thinking and tool blocks with explicit XML tags
- Inline subagent descendants at logical spawn points when exact anchors exist
- Support optional descendant inclusion through query params

## Non-goals

- No new persistence or schema changes
- No public share or gist flow for markdown in this change
- No attempt to define or adopt an external universal transcript standard
- No frontend rendering work required for first implementation
- No change to existing HTML export behavior

## Standards and ecosystem findings

There is no broadly accepted standard for AI coding-agent transcripts serialized
as markdown with XML tags.

Closest references:

- **OpenInference** provides semantic conventions for agent, LLM, and tool
  tracing. Relevant field names include tool call identifiers, tool/function
  names, arguments, and tool-call/result linking.
- **MCP** defines structured concepts such as `tool_use` and `tool_result`, but
  not a markdown transcript format.
- **Anthropic** documents XML tags as a good way to structure prompts and notes
  there is no canonical tag vocabulary.
- **CommonMark** permits raw HTML/XML tags, but semantics for custom tags are
  application-defined.

Design implication: use standard markdown as container plus custom XML tags with
names influenced by OpenInference and MCP semantics.

## User-facing API

### Route

`GET /api/v1/sessions/{id}/md`

### Query params

- no `depth` param: current session only
- `depth=1`: include direct child sessions
- `depth=all`: include full descendant tree recursively

### Invalid params

If `depth` is present and not one of `1` or `all`, return `400 Bad Request`
with a clear error message.

### Handler behavior

Unlike existing HTML export, markdown export should use normal timeout-wrapped
route registration. Large recursive exports should still operate within regular
API timeout behavior for this first implementation.

### Error response shape

Error responses should reuse existing server JSON error behavior rather than
introducing a markdown-specific error body.

### Response headers

- `Content-Type: text/markdown; charset=utf-8`
- `Content-Disposition: inline; filename="<sanitized>.md"`

Reason: endpoint should be directly usable as URL-fed context without forced
attachment download.

## Export model

Implementation should build a normalized export tree before rendering markdown.
This keeps child placement logic, anchor resolution, and escaping in one place.

Suggested internal model:

- session metadata
- ordered messages
- ordered per-message segment stream
  - text segments
  - thinking segments
  - tool segments
  - code segments
  - skill segments
- anchored child sessions
- appended child sessions without anchors

Renderer contract for each message:

- include system messages rather than filtering them out
- derive ordered message segments using backend logic that matches frontend
  `parseContent()` plus `enrichSegments()` behavior from
  `frontend/src/lib/utils/content-parser.ts`
- when a tool segment has no usable inline body text, generate equivalent
  fallback body content using the same precedence as frontend tool rendering,
  including Task/Agent prompt handling and `generateFallbackContent()`-style
  behavior from `frontend/src/lib/utils/tool-params.ts`
- render segments in that exact order
- emit escaped text nodes for `text` segments
- emit `<code_block language="...">...</code_block>` for `code` segments
- emit `<skill name="...">...</skill>` for `skill` segments
- emit structured XML blocks for `thinking` and `tool` segments
- do not separately dump full raw `message.content` before or after the segment
  stream

This can later support multiple renderers if HTML export is migrated onto same
model, but initial scope only requires markdown rendering.

## Markdown structure

Top-level document remains markdown, but machine-significant structure is
expressed through XML tags.

Example skeleton:

```md
# Session: my-project

<session id="s1" project="my-project" agent="claude" started_at="2026-04-13T10:00:00Z">

<message role="user" ordinal="0" timestamp="2026-04-13T10:01:00Z">
How parser handle tool calls?
</message>

<message role="assistant" ordinal="1" timestamp="2026-04-13T10:01:05Z">
<thinking>
Need inspect export path first.
</thinking>

<tool_call id="toolu_123" name="Task" category="Task">
<arguments><![CDATA[
{"description":"inspect export path","subagent_type":"general-purpose"}
]]></arguments>
</tool_call>

<subagent_anchor session_id="agent-abc" tool_call_id="toolu_123" depth="1">
<subagent_session id="agent-abc" parent_session_id="s1" relationship="subagent">
...
</subagent_session>
</subagent_anchor>

Answer body here.
</message>

<child_session id="fork-1" parent_session_id="s1" relationship="fork">
...
</child_session>

</session>
```

## Tag vocabulary

### Required tags

- `<session ...>`
- `<message ...>`
- `<thinking>`
- `<tool_call ...>`
- `<tool_body>`
- `<arguments><![CDATA[...]]></arguments>`
- `<tool_result ...><![CDATA[...]]></tool_result>`
- `<subagent_anchor ...>`
- `<subagent_session ...>`
- `<child_session ...>`

### Optional tags

- `<skill ...>` for parsed skill segments
- `<code_block ...>` for parsed code segments

### Attributes

#### `session`

- `id`
- `project`
- `agent`
- `started_at` when available
- `ended_at` when available
- `message_count`

#### `message`

- `role`
- `ordinal`
- `timestamp` when available
- `is_system="true"` when persisted flag is set
- `has_thinking="true"` when applicable
- `has_tool_use="true"` when applicable

#### `tool_call`

- `id` from `tool_use_id` when available
- `name` from `tool_name`
- `category` from normalized category
- `subagent_session_id` when available

#### `tool_result`

- `tool_call_id` when available
- `source` when available
- `status` when available
- `agent_id` when available
- `subagent_session_id` when available
- `timestamp` when available

#### `subagent_anchor`

- `session_id`
- `tool_call_id` when available
- `depth` reflecting effective nesting level in export

#### `subagent_session` / `child_session`

Required:

- `id`
- `parent_session_id`

Optional:

- `relationship` when non-empty
- `project` when available
- `agent` when available
- `started_at` when available
- `ended_at` when available
- `message_count` when available

### Attribute serialization rule

For all tags, omit attributes whose values are absent or empty. Do not emit
empty-string attributes as placeholders. XML-escape all emitted attribute values.

## Placement rules

### Ordering source of truth

Markdown export should mirror the logical presentation order of the session
screen at `/sessions/{id}`, not invent a separate ordering model.

Concretely:

- message order follows stored `ordinal`
- within each message, ordering follows the same segment ordering derived from
  `frontend/src/lib/utils/content-parser.ts`
- within each tool block, ordering follows the same structure used by
  `frontend/src/lib/components/content/ToolBlock.svelte`

### Current session only

Without `depth`, export only requested session and its messages.

### Direct children

With `depth=1`, include direct children only.

### Full tree

With `depth=all`, recurse through descendants.

## Child session anchoring

### Anchored subagents

When a child session is linked from a tool call via
`tool_calls.subagent_session_id`, inline that child at the same logical point
where the session screen shows the inline subagent for that tool block, using:

```md
<subagent_anchor ...>
<subagent_session ...>
...
</subagent_session>
</subagent_anchor>
```

This is preferred because repository data already captures:

- `tool_calls.subagent_session_id`
- parent message containing the tool call
- tool call ordering within message
- tool result history for that call

In practice, this means anchored subagent content appears after the rest of that
rendered tool block, matching session screen behavior.

### Unanchored children

If a child has `parent_session_id` but no exact tool-call anchor, append it
after parent transcript in a `<child_session ...>` block ordered by
`started_at` ascending, then `id` ascending as tie-breaker.

This covers non-subagent descendants such as forks, continuations, or other
future relationship types.

### Recursive nesting

For `depth=all`, nested subagents should be recursively inlined at each child’s
own spawn point whenever anchors exist.

Traversal must maintain a visited set by session ID. Each session is emitted at
most once. If corrupt or unexpected archive data creates a cycle or duplicate
reachability, skip re-emitting already visited sessions.

## Message content formatting

### Plain text

Render `text` segments from the parsed segment stream as markdown/plain text in
place.

This export should include system messages too. They are not filtered out for
markdown export; instead they remain in transcript order and are explicitly
marked with `is_system="true"` on the enclosing `<message>` tag.

### Thinking blocks

Thinking content should be wrapped as:

```md
<thinking>
...
</thinking>
```

Thinking detection must follow backend logic that matches
`frontend/src/lib/utils/content-parser.ts`, including avoidance of false
positives inside inline-code spans.

### Tool calls

Each `tool` segment should render as a structured block in the same position as
its corresponding tool block on the session screen:

```md
<tool_call id="..." name="..." category="...">
<arguments><![CDATA[
...
]]></arguments>
</tool_call>
<tool_body><![CDATA[
...
]]></tool_body>
```

Use raw `input_json` when available rather than lossy prettified summaries.

Tool body source precedence must match session screen behavior:

1. for Task/Agent-style tools, use prompt/task body equivalent to UI
   `taskPrompt` when present
2. otherwise use enriched segment body text from backend logic matching
   `enrichSegments()` when present
3. otherwise generate fallback body equivalent to frontend
   `generateFallbackContent()` behavior
4. if none of the above yield content, omit `<tool_body>`

For legacy/text-only tool segments without structured `toolCall` data:

- derive `name` from the parsed segment label
- derive `category` from normalized parsed tool name when possible
- omit `<arguments>` because no structured `input_json` exists
- still emit `<tool_body>` when the parsed segment contains body text

### Tool results

Tool results and result history should render as one or more tagged blocks:

```md
<tool_result tool_call_id="..." source="..." status="..."><![CDATA[
...
]]></tool_result>
```

If a tool has both main `result_content` and structured `result_events`, emit
both. No dedupe. Duplicate content is acceptable.

Ordering rule inside a rendered tool segment:

- tool call metadata first
- tool body next when present
- main `result_content` next when present
- `result_events` next in stored chronological order
- anchored subagent block last, matching session screen behavior

### Code segments

Render `code` segments as:

```md
<code_block language="go"><![CDATA[
...
]]></code_block>
```

If the language label is empty, omit the `language` attribute.

### Skill segments

Render `skill` segments as:

```md
<skill name="skill-name"><![CDATA[
...
]]></skill>
```

If the parsed skill label is empty, omit the `name` attribute.

### Escaping and CDATA

Use CDATA for:

- code block bodies
- skill bodies
- raw JSON arguments
- tool bodies
- command output
- diffs
- multi-line tool result content
- any content likely to contain markdown/XML special characters

If content contains `]]>` and would break CDATA, fall back to escaped text
inside the tag instead of CDATA.

Serialization matrix:

- `text` segment body: escaped text node inside `<message>`
- `thinking` body: CDATA when safe, else escaped text inside `<thinking>`
- `code_block` body: CDATA when safe, else escaped text inside `<code_block>`
- `skill` body: CDATA when safe, else escaped text inside `<skill>`
- `tool_body`: CDATA when safe, else escaped text inside `<tool_body>`
- `arguments`: CDATA when safe, else escaped text inside `<arguments>`
- `tool_result`: CDATA when safe, else escaped text inside `<tool_result>`

Plain message body text should still be XML-escaped as needed so malformed text
cannot break surrounding tags.

This endpoint prioritizes machine-ingestible structure over perfect markdown
viewer rendering. Raw XML blocks may not render like ordinary markdown in all
CommonMark viewers; that tradeoff is intentional.

## Ordering

### Sessions

- root session first
- anchored children inline at anchor point, matching session screen ordering
- anchored child sessions must not also be emitted later as appended
  `<child_session>` blocks
- unanchored children appended after parent transcript by `started_at`, then
  `id`
- top-level markdown heading (`# Session: ...`) appears only for root export,
  not nested child session containers

### Messages

Use stored message order (`ordinal` ascending).

If a persisted message is empty after segment derivation, still emit an empty
`<message ...></message>` block so transcript ordinals remain stable.

### Child session container shape

`<subagent_session>` and `<child_session>` contain:

- only the attributes explicitly listed in the tag attribute section above
- nested `<message>` blocks for that child session transcript
- nested descendant `<subagent_anchor>`, `<subagent_session>`, and
  `<child_session>` blocks as needed by depth and anchoring rules

They do not contain a repeated top-level markdown heading.

### Message segments

Render message segments in the same order produced by backend logic matching
`frontend/src/lib/utils/content-parser.ts`.

### Tool segments

Within a tool segment, mirror `ToolBlock.svelte` ordering:

- tool call block
- tool content/body
- main output (`result_content`) when present
- history/result events
- inline subagent block when `subagent_session_id` resolves and depth allows

## Data sources in current codebase

Existing data already supports this feature without schema changes:

- session and message retrieval via `getSessionWithMessages()` in
  `internal/server/export.go`
- child session retrieval via `GetChildSessions()` in DB and server layers
- subagent links through `tool_calls.subagent_session_id`
- tool result history through `tool_result_events`
- child relationship via `sessions.parent_session_id`

Likely implementation files:

- `internal/server/server.go` for new route registration
- `internal/server/export.go` for handler and markdown generator
- `internal/server/server_test.go` for endpoint tests
- `internal/server/export_test.go` for markdown formatting tests
- `frontend/src/lib/utils/content-parser.ts` as semantic reference for
  `parseContent()` and `enrichSegments()` behavior only; no frontend changes
  required for this first implementation
- `frontend/src/lib/utils/tool-params.ts` as semantic reference for fallback
  tool-body generation only; no frontend changes required for this first
  implementation

## Error handling

- unknown session ID: `404 Not Found`
- invalid `depth` value: `400 Bad Request`
- DB failures: `500 Internal Server Error`
- empty child list: export succeeds without child blocks
- malformed timestamps: preserve raw string where existing helpers already do so
- CDATA-unsafe content: fall back to escaped text content inside XML tags

## Security and safety

- Keep existing escaping discipline from HTML export logic, adapted for
  markdown/XML output
- Ensure arbitrary message content cannot inject fake structural tags
- Do not expose auth tokens or remote credentials in generated URL shape
- Preserve existing local-only behavior; this endpoint is not a public publish
  mechanism

## Test plan

Add focused unit and handler tests for:

1. route exists at `/api/v1/sessions/{id}/md`
2. content type is `text/markdown`
3. route uses normal timeout-wrapped registration
4. default export returns current session only
5. `depth=1` includes direct children only
6. `depth=all` includes recursive descendants
7. anchored subagent child is inserted under matching `<subagent_anchor>` in
   same relative position as session screen inline subagent rendering
8. anchored child is not also duplicated as appended `<child_session>`
9. unanchored child is appended as `<child_session>` after transcript
10. thinking blocks render as `<thinking>` tags
11. inline-code text that mentions `[Thinking]` or tool markers does not create
    false structured blocks
12. tool calls include arguments block with preserved raw JSON
13. tool results render in same order as session screen semantics
14. code segments render as `<code_block>` and skill segments render as
    `<skill>` in segment order
15. empty relationship type omits `relationship` attribute rather than
    inventing a value
16. empty optional attributes are omitted and emitted attributes are escaped
17. empty persisted messages still emit empty `<message>` blocks
18. system messages are included with `is_system="true"`
19. invalid `depth` returns `400`
20. missing session returns `404`
21. invalid `input_json` falls back without dropping tool segment
22. legacy tool segment without structured `toolCall` still emits deterministic
    `<tool_call>` shape without `<arguments>`
23. missing `subagent_session_id` target does not break export
24. children with absent or equal `started_at` values use deterministic fallback
    ordering
25. text containing XML-like content is safely escaped
26. cyclic or duplicate descendant references do not recurse forever or re-emit
    same session twice

## Open questions resolved

- Endpoint path: `/api/v1/sessions/{id}/md`
- Descendant control: `depth` query param
- Default depth: current session only
- Descendant strategy: inline anchored subagents; append unanchored children
- Nested subagents with `depth=all`: recurse inline
- Tag style: custom XML tags in markdown, not fenced-only representation
- System messages: include them and tag with `is_system="true"`
- Message ordering: mirror session screen segment ordering rather than dumping
  full raw message content separately
- Tool output merge: emit both `result_content` and `result_events`; no dedupe
- Anchor ordering: same as session screen inline subagent ordering
- CDATA failure: fall back to escaped text instead of CDATA
- Route wiring: use normal timeout wrapper
- Code segments: render as `<code_block>`
- Skill segments: render as `<skill>`
- Empty relationship type: omit `relationship` attribute
- Empty persisted messages: still emit empty `<message>` blocks

## Recommended implementation sequence

1. add route and request validation
2. build recursive session export tree with anchor resolution
3. add markdown renderer
4. add endpoint tests
5. add formatter tests for thinking, tool calls, tool results, and child session
   placement

## Planning handoff

Implementation plan should focus on:

- handler and query parsing
- export tree construction
- markdown rendering helpers
- recursive descendant loading
- tests for route, formatting, and tree placement
