# Clerk Authentication Implementation Plan

## Overview

Add Clerk authentication to the hosted Railway deployment without
breaking the existing static-token flow used by non-Clerk/local
deployments.

The important constraint is deployment shape: on Railway, this repo
serves the Svelte SPA and the Go API from the same origin. That means
the hosted path should be cookie-first, not bearer-token-first.

- **Frontend**: use Clerk's official JavaScript SDK
  (`@clerk/clerk-js`) for sign-in UI and session lifecycle.
- **Hosted browser -> API requests**: rely on Clerk's same-origin
  `__session` cookie for normal `fetch()` and `EventSource` traffic.
- **Backend**: validate Clerk session tokens from the `__session`
  cookie, with `Authorization: Bearer` as a secondary path for
  explicit cross-origin callers.
- **Legacy mode**: if Clerk is not configured, keep the current
  static-token auth flow exactly as it is.

## Current State

- **Frontend**: `frontend/src/lib/api/client.ts` injects a static
  bearer token from `localStorage`. `App.svelte` shows a token-entry
  overlay when `settings.needsAuth` is true.
- **Backend**: `internal/server/auth.go` compares the provided token
  against `cfg.AuthToken`.
- **Hosted startup**: `cmd/agentsview/main.go` auto-enables remote
  access and generates a static auth token for internet-facing
  deployments.
- **No Clerk dependencies** exist today in either `go.mod` or
  `frontend/package.json`.

## Target Architecture

```text
Browser on https://<railway-domain>
  |
  v
Clerk JS SDK (@clerk/clerk-js)
  |-> renders sign-in UI
  |-> manages session on the app origin
  |
  v
fetch('/api/v1/...') / EventSource('/api/v1/.../watch')
  |   same-origin requests automatically include __session cookie
  |
  v
Go authMiddleware
  |-> if Clerk mode is enabled:
  |     - read __session cookie first
  |     - allow Authorization: Bearer as fallback
  |     - verify token with Clerk Go SDK + cached JWK
  |     - require authorized-party allowlist
  |     - set ctxKeyRemoteAuth + Clerk claims in context
  |
  v
Route handlers
```

The hosted Railway path should not depend on query-string tokens for
SSE and should not rewrite every API call around `getToken()`. Those
are cross-origin concerns, not same-origin concerns.

---

## Phase 1: Backend Clerk Verification

### 1.1 Add Clerk Go SDK

```bash
go get github.com/clerk/clerk-sdk-go/v2
```

Use Clerk's Go SDK instead of hand-rolling JWT parsing with a generic
JWT library. The SDK already provides the token decode/verify and JWK
fetching primitives this server needs.

### 1.2 Config changes

Add Clerk-specific config fields in `internal/config/config.go`:

```go
ClerkSecretKey          string   // env: CLERK_SECRET_KEY
ClerkAuthorizedParties  []string // env: CLERK_AUTHORIZED_PARTIES
```

Notes:

- `CLERK_SECRET_KEY` enables Clerk mode on the backend.
- `CLERK_AUTHORIZED_PARTIES` is required for hosted Clerk mode. It
  should be a comma-separated allowlist of browser origins, for
  example:

```text
CLERK_AUTHORIZED_PARTIES=https://server-production-80f5.up.railway.app
```

- Do not add `CLERK_DOMAIN`. With the Go SDK + secret key, the backend
  can use Clerk's supported verification flow directly.

### 1.3 Clerk verifier helper

Create `internal/server/clerk.go` with a small verifier wrapper around
Clerk's Go SDK:

```go
type ClerkVerifier struct {
    jwksClient *jwks.Client
    mu         sync.RWMutex
    keys       map[string]*clerk.JSONWebKey
    azp        []string
}

func NewClerkVerifier(secretKey string, authorizedParties []string) (*ClerkVerifier, error)
func (v *ClerkVerifier) VerifyRequest(r *http.Request) (*jwt.Claims, error)
```

Implementation requirements:

- Extract token from the `__session` cookie first.
- Fall back to `Authorization: Bearer ...` only if the cookie is not
  present.
- Decode token to get `kid`.
- Fetch/cache the matching JWK with the Clerk Go SDK.
- Verify standard claims (`exp`, `nbf`, etc.).
- Require `authorizedParties` configuration and verify the token's
  authorized party against it. Do not leave this as optional.
- Cache keys by `kid` in memory and refresh on cache miss.

If a token arrives without an acceptable authorized party, reject it.
Do not silently weaken verification in hosted mode.

### 1.4 Modify `authMiddleware`

In `internal/server/auth.go`, add a Clerk code path:

```go
// If CLERK_SECRET_KEY is configured, use Clerk session verification.
// Otherwise, fall back to the existing static token comparison.
```

Decision logic:

- If `cfg.ClerkSecretKey != ""` -> Clerk mode
- Else -> existing static-token mode

Behavior in Clerk mode:

- Keep the existing localhost/public-access gating behavior.
- Accept same-origin API requests authenticated by the `__session`
  cookie.
- Accept bearer tokens only as an explicit fallback.
- Remove query-string token auth for Clerk-backed SSE.
- Set `ctxKeyRemoteAuth` on successful verification so downstream
  host-check/CORS behavior remains consistent.

### 1.5 Hosted startup behavior

Update `cmd/agentsview/main.go` so Railway startup does not generate or
print a static auth token when Clerk mode is enabled.

New rule:

- **Clerk configured**: enable remote access, but do not call
  `EnsureAuthToken()`.
- **Clerk not configured**: preserve current behavior and generate the
  static token as before.

This avoids running two unrelated auth systems in parallel for the
hosted deployment.

### 1.6 Request context

Store only claims that are actually stable in Clerk session tokens:

```go
type ClerkAuth struct {
    UserID           string
    SessionID        string
    AuthorizedParty  string
}
```

Do not assume `email` is present in the token. If email is needed
later, fetch it from Clerk's backend API or add explicit custom
claims.

---

## Phase 2: Frontend Clerk Integration

### 2.1 Install the official JS SDK

```bash
cd frontend && npm install @clerk/clerk-js
```

Do not use `@clerk/svelte`. That package is not available as an
official Clerk SDK and this repo is a plain Svelte SPA, not a
SvelteKit app using a community adapter.

### 2.2 Environment variable

Add `VITE_CLERK_PUBLISHABLE_KEY` to the frontend build.

In local `.env`:

```text
VITE_CLERK_PUBLISHABLE_KEY=pk_test_...
```

### 2.3 Add a thin Clerk wrapper

Create `frontend/src/lib/clerk.ts` to own Clerk initialization and
keep SDK-specific code out of `App.svelte`.

High-level shape:

```ts
import { Clerk } from "@clerk/clerk-js";

let clerkPromise: Promise<Clerk | null> | null = null;

export function getClerk(): Promise<Clerk | null> {
  const key = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;
  if (!key) return Promise.resolve(null);

  clerkPromise ??= (async () => {
    const clerk = new Clerk(key);
    await clerk.load();
    return clerk;
  })();

  return clerkPromise;
}
```

If Clerk's prebuilt UI needs additional setup from the JavaScript
quickstart, do that here. Keep it in one file.

### 2.4 Replace the token overlay with Clerk JS mounting

Do not import Svelte components from Clerk. Mount Clerk's JS UI into a
normal DOM node.

In `frontend/src/App.svelte`:

- If `VITE_CLERK_PUBLISHABLE_KEY` is missing, keep the existing
  token-overlay behavior.
- If the key is present:
  - initialize Clerk via `getClerk()`
  - mount Clerk's sign-in UI into the auth overlay when signed out
  - show the existing app shell when signed in
  - subscribe to Clerk auth state changes from the wrapper

This keeps local non-Clerk builds working while making hosted Clerk
mode real.

### 2.5 Leave same-origin API calls alone

Do **not** rewrite `authHeaders()` so every fetch awaits
`clerk.session.getToken()`.

For the Railway-hosted SPA:

- browser and API are same-origin
- Clerk's `__session` cookie is sent automatically
- `fetchJSON()` can remain synchronous
- existing request helpers should stay simple

Frontend change here should be minimal:

- In Clerk mode, normal hosted requests do not need an injected
  `Authorization` header.
- In legacy static-token mode, keep the current bearer-token header
  behavior.

If the product later needs true cross-origin Clerk API calls, add a
separate async helper for that case instead of contaminating the
hosted same-origin path.

### 2.6 SSE stays cookie-backed in Clerk mode

The current `watchSession()` uses `?token=` because `EventSource`
cannot send custom headers.

In Clerk mode on Railway:

- `EventSource("/api/v1/.../watch")` is same-origin
- the browser sends cookies automatically
- no query token is needed

New rule:

- **Clerk mode**: plain `/watch` URL, cookie auth only
- **Legacy static-token mode**: keep the existing `?token=` fallback

This removes URL-token leakage from the hosted Clerk path.

---

## Phase 3: Deployment Config (Railway)

### 3.1 Railway env vars

Set these in Railway:

| Variable | Value |
|----------|-------|
| `CLERK_SECRET_KEY` | `sk_live_...` |
| `CLERK_AUTHORIZED_PARTIES` | `https://server-production-80f5.up.railway.app` |
| `VITE_CLERK_PUBLISHABLE_KEY` | `pk_live_...` |

`VITE_CLERK_PUBLISHABLE_KEY` must be available at frontend build time,
so keep the Docker build-arg pattern:

```dockerfile
ARG VITE_CLERK_PUBLISHABLE_KEY
ENV VITE_CLERK_PUBLISHABLE_KEY=$VITE_CLERK_PUBLISHABLE_KEY
RUN npm run build
```

`CLERK_SECRET_KEY` and `CLERK_AUTHORIZED_PARTIES` are runtime env vars
used by the Go server.

### 3.2 Clerk dashboard setup

- Create a Clerk application.
- Set the application / redirect URLs for the Railway domain.
- Configure desired sign-in methods.
- Use the Railway browser origin in the authorized-party allowlist.

The backend plan no longer depends on a manually copied JWKS domain.

---

## Phase 4: Testing

### 4.1 Backend tests

Add `internal/server/clerk_test.go` covering:

- token accepted from `__session` cookie
- bearer token accepted as fallback
- expired token rejected
- invalid signature rejected
- unknown `kid` refresh path
- token with unauthorized `azp` rejected
- fallback to static-token auth when Clerk is not configured
- legacy SSE query token still works in static-token mode
- SSE query token is not required in Clerk mode

### 4.2 Frontend tests

- `getClerk()` returns `null` when no publishable key is set
- `App.svelte` uses Clerk auth state when the key is set
- hosted same-origin fetch helpers do not inject Clerk bearer tokens
- legacy static-token header injection still works when Clerk is off

### 4.3 Integration tests

Manual staging checks:

- deploy to Railway with Clerk enabled
- sign in through Clerk UI
- verify normal API requests succeed without custom auth headers
- verify session watch SSE works without `?token=`
- verify invalid/expired session is rejected
- verify requests from an origin outside
  `CLERK_AUTHORIZED_PARTIES` are rejected

---

## Migration / Backward Compatibility

- **Legacy mode**: no Clerk env vars set. Backend keeps static-token
  auth. Frontend keeps the existing token overlay.
- **Hosted Clerk mode**: `CLERK_SECRET_KEY`,
  `CLERK_AUTHORIZED_PARTIES`, and
  `VITE_CLERK_PUBLISHABLE_KEY` are set. Frontend shows Clerk sign-in.
  Backend validates Clerk sessions from cookies/bearer tokens.
- **Remote URL + token workflow**: keep the current static-token path
  for explicit cross-origin remote connections. Do not partially
  convert that workflow to Clerk in this phase.

Rollout order:

1. ship backend Clerk support behind env vars
2. ship frontend `@clerk/clerk-js` integration
3. enable Clerk env vars in Railway
4. verify same-origin cookie auth and SSE in staging

---

## Files Changed

| File | Change |
|------|--------|
| `go.mod` | Add `github.com/clerk/clerk-sdk-go/v2` |
| `internal/config/config.go` | Add Clerk env-backed config fields |
| `internal/server/clerk.go` | New: Clerk verifier helper + JWK cache |
| `internal/server/clerk_test.go` | New: Clerk backend tests |
| `internal/server/auth.go` | Add Clerk cookie/bearer auth path |
| `cmd/agentsview/main.go` | Skip static token generation when Clerk is enabled |
| `frontend/package.json` | Add `@clerk/clerk-js` |
| `frontend/src/lib/clerk.ts` | New: Clerk JS wrapper |
| `frontend/src/App.svelte` | Mount Clerk sign-in UI when Clerk mode is enabled |
| `frontend/src/lib/api/client.ts` | Keep same-origin fetch path simple; preserve static-token fallback |
| `frontend/src/lib/api/client.test.ts` | Update auth behavior tests |
| `Dockerfile` | Keep publishable-key build arg for Vite |
