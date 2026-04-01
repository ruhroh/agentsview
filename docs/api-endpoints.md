# API Endpoint Catalog

This file catalogs the HTTP surface that is actually routed by the
current `server` branch. It is based on `internal/server/server.go`
and the handler implementations under `internal/server/`.

If you were assuming the frontend client defines the contract, that
assumption is wrong. `frontend/src/lib/api/client.ts` still carries
calls from `main` that are not implemented here.

## Base Path

- All API routes live under `/api/v1`.
- A reverse-proxy base path may prefix every route if the server is
  started with `--base-path`, for example `/agentsview/api/v1/...`.
- All non-API paths fall through to the embedded SPA.

## Auth Model

- Every `/api/` route passes through `authMiddleware`.
- Local loopback requests bypass auth only when remote access is off.
- Remote access requires either:
  - `Authorization: Bearer <token>`, or
  - Clerk session verification when Clerk is configured.
- `OPTIONS` preflight is handled by middleware, not explicit route
  registrations.
- Error responses are JSON in the form `{ "error": "..." }`.

## Live Routes

### Session browsing

#### `GET /api/v1/sessions`

List sessions with cursor pagination.

Query parameters:
- `project`
- `exclude_project`
- `machine`
- `agent`
- `date` as `YYYY-MM-DD`
- `date_from` as `YYYY-MM-DD`
- `date_to` as `YYYY-MM-DD`
- `active_since` as RFC3339
- `min_messages`
- `max_messages`
- `min_user_messages`
- `include_one_shot=true`
- `include_children=true`
- `cursor`
- `limit`

Notes:
- Invalid integer params return `400`.
- Invalid dates return `400`.
- `date_from > date_to` returns `400`.
- Invalid cursor returns `400`.
- `limit` defaults to `200` and is capped at `500`.

Response shape:

```json
{
  "sessions": [
    {
      "id": "s1",
      "project": "my-project",
      "machine": "host-a",
      "agent": "codex",
      "first_message": "first prompt",
      "display_name": "optional title",
      "started_at": "2025-01-15T10:00:00Z",
      "ended_at": "2025-01-15T11:00:00Z",
      "message_count": 12,
      "user_message_count": 4,
      "parent_session_id": "optional-parent",
      "relationship_type": "optional-type",
      "total_output_tokens": 1234,
      "peak_context_tokens": 5678,
      "created_at": "2025-01-15T10:00:00Z"
    }
  ],
  "next_cursor": "opaque-cursor",
  "total": 42
}
```

#### `GET /api/v1/sessions/{id}`

Fetch one session by id.

Responses:
- `200` with a `Session` object
- `404` if the session does not exist

#### `GET /api/v1/sessions/{id}/messages`

Fetch paginated messages for a session.

Query parameters:
- `from`
- `limit`
- `direction=asc|desc`

Notes:
- Any value other than `desc` is treated as ascending.
- Default `limit` is `100`; max `1000`.
- If `direction=desc` and `from` is omitted, the handler starts from
  a large sentinel value so the newest messages are returned first.

Response shape:

```json
{
  "messages": [
    {
      "id": 1,
      "session_id": "s1",
      "ordinal": 0,
      "role": "user",
      "content": "hello",
      "timestamp": "2025-01-15T10:00:00Z",
      "has_thinking": false,
      "has_tool_use": false,
      "content_length": 5,
      "model": "gpt-5",
      "token_usage": null,
      "context_tokens": 0,
      "output_tokens": 0,
      "tool_calls": [],
      "is_system": false
    }
  ],
  "count": 1
}
```

#### `GET /api/v1/sessions/{id}/children`

Fetch child sessions for a parent session.

Response:
- `200` with an array of `Session`
- Empty array when there are no children

#### `GET /api/v1/sessions/{id}/search`

Search within a single session and return matching message ordinals.

Query parameters:
- `q`

Notes:
- Empty `q` returns `200` with `{"ordinals":[]}`.

Response shape:

```json
{
  "ordinals": [3, 8, 21]
}
```

### Global search

#### `GET /api/v1/search`

Full-text search across sessions.

Query parameters:
- `q` required
- `project`
- `sort=relevance|recency`
- `cursor`
- `limit`

Notes:
- Empty `q` returns `400`.
- Invalid integer params return `400`.
- `limit` defaults to `50`; max `500`.
- Any `sort` other than `recency` is coerced to `relevance`.
- Returns `501` if FTS is unavailable.

Response shape:

```json
{
  "query": "login bug",
  "results": [
    {
      "session_id": "s1",
      "project": "my-project",
      "agent": "codex",
      "name": "Fix login flow",
      "ordinal": 12,
      "session_ended_at": "2025-01-15T11:00:00Z",
      "snippet": "...<mark>login bug</mark>...",
      "rank": -12.34
    }
  ],
  "count": 1,
  "next": 50
}
```

### Metadata

#### `GET /api/v1/projects`

List distinct projects with session counts.

Query parameters:
- `include_one_shot=true`

Response shape:

```json
{
  "projects": [
    {
      "name": "my-project",
      "session_count": 10
    }
  ]
}
```

#### `GET /api/v1/machines`

List distinct machine names.

Query parameters:
- `include_one_shot=true`

Response shape:

```json
{
  "machines": ["host-a", "host-b"]
}
```

#### `GET /api/v1/agents`

List distinct agent names with session counts.

Query parameters:
- `include_one_shot=true`

Response shape:

```json
{
  "agents": [
    {
      "name": "codex",
      "session_count": 10
    }
  ]
}
```

#### `GET /api/v1/stats`

Fetch database-wide summary stats.

Query parameters:
- `include_one_shot=true`

Response shape:

```json
{
  "session_count": 42,
  "message_count": 420,
  "project_count": 7,
  "machine_count": 3,
  "earliest_session": "2025-01-01T00:00:00Z"
}
```

#### `GET /api/v1/version`

Fetch build metadata.

Response shape:

```json
{
  "version": "dev",
  "commit": "abcdef1",
  "build_date": "2026-04-01T00:00:00Z",
  "read_only": false
}
```

### Star management

#### `GET /api/v1/starred`

List starred session ids.

Response shape:

```json
{
  "session_ids": ["s1", "s2"]
}
```

#### `PUT /api/v1/sessions/{id}/star`

Star a session.

Responses:
- `204` on success
- `404` if the session does not exist
- `501` on read-only backends

#### `DELETE /api/v1/sessions/{id}/star`

Remove a star from a session.

Responses:
- `204` on success
- `501` on read-only backends

#### `POST /api/v1/starred/bulk`

Star many sessions from a JSON body.

Request body:

```json
{
  "session_ids": ["s1", "s2"]
}
```

Responses:
- `204` on success
- `400` for invalid JSON
- `501` on read-only backends

### Share management for local clients

#### `GET /api/v1/shared`

List locally shared session ids.

Response shape:

```json
{
  "session_ids": ["s1", "s2"]
}
```

#### `PUT /api/v1/sessions/{id}/share`

Push a local session to the configured remote share server and record
the share locally.

Required server config:
- `share.url`
- `share.token`
- `share.publisher`

Responses:
- `200` with a `SharedSession` object
- `400` if share config is missing
- `404` if the session does not exist
- `502` if the remote share server call fails
- `501` on read-only backends

Response shape:

```json
{
  "session_id": "s1",
  "share_id": "work-laptop:s1",
  "server_url": "https://viewer.example.com",
  "shared_at": "2026-04-01T12:00:00.000Z",
  "updated_at": "2026-04-01T12:00:00.000Z"
}
```

#### `DELETE /api/v1/sessions/{id}/share`

Delete a local share record and attempt remote deletion best-effort.

Responses:
- `204` on success
- `404` if the session is not shared
- `501` on read-only backends

### Share replication endpoints on the hosted server

These are the endpoints that other agentsview instances call when
they push or remove a share on this server.

#### `PUT /api/v1/shares/{shareId}`

Upsert a shared session payload.

Request body shape:

```json
{
  "share_id": "publisher:s1",
  "session": {
    "id": "s1",
    "project": "my-project",
    "machine": "publisher",
    "agent": "codex",
    "first_message": "first prompt",
    "display_name": "optional title",
    "started_at": "2025-01-15T10:00:00Z",
    "ended_at": "2025-01-15T11:00:00Z",
    "message_count": 12,
    "user_message_count": 4,
    "parent_session_id": null,
    "relationship_type": "",
    "total_output_tokens": 1234,
    "peak_context_tokens": 5678
  },
  "messages": []
}
```

Notes:
- If `share_id` exists in the body, it must match the path.
- Stored session id is rewritten to the path `shareId`.
- Incoming messages have `session_id` rewritten to `shareId`.

Responses:
- `204` on success
- `400` for missing path id, invalid JSON, or mismatched body `share_id`
- `501` on read-only backends

#### `DELETE /api/v1/shares/{shareId}`

Soft-delete a hosted shared session.

Responses:
- `204` on success
- `404` if the share does not exist
- `501` on read-only backends

## SPA Fallback

#### `GET /`

Any non-API route is handled by the embedded SPA server:

- Existing static asset paths are served directly.
- Unknown non-API paths return `index.html`.
- The server injects runtime metadata into `index.html`, including:
  - Clerk publishable key meta tag
  - optional `<base href>` for reverse-proxy deployments

## Not Implemented On This Branch

The frontend client still references these paths, but this branch does
not register handlers for them:

- `GET /api/v1/update/check`
- `GET /api/v1/sync/status`
- `POST /api/v1/sync`
- `POST /api/v1/resync`
- `GET /api/v1/sessions/{id}/activity`
- `GET /api/v1/sessions/{id}/watch`
- `GET /api/v1/sessions/{id}/export`
- `POST /api/v1/sessions/{id}/resume`
- `POST /api/v1/sessions/{id}/publish`
- `GET /api/v1/sessions/{id}/directory`
- `GET /api/v1/openers`
- `POST /api/v1/sessions/{id}/open`
- `GET /api/v1/config/github`
- `POST /api/v1/config/github`
- `GET /api/v1/config/terminal`
- `POST /api/v1/config/terminal`
- `GET /api/v1/settings`
- `PUT /api/v1/settings`
- `GET /api/v1/analytics/summary`
- `GET /api/v1/analytics/activity`
- `GET /api/v1/analytics/heatmap`
- `GET /api/v1/analytics/projects`
- `GET /api/v1/analytics/hour-of-week`
- `GET /api/v1/analytics/sessions`
- `GET /api/v1/analytics/velocity`
- `GET /api/v1/analytics/tools`
- `GET /api/v1/analytics/top-sessions`
- `GET /api/v1/insights`
- `GET /api/v1/insights/{id}`
- `DELETE /api/v1/insights/{id}`
- `POST /api/v1/insights/generate`

If you need those endpoints, switch back to `main` or reintroduce
them intentionally. They are not present in this branch's route table.
