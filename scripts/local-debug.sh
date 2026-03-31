#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ ! -f .env ]; then
  echo "error: .env not found in $ROOT" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-8080}"
LOCAL_ORIGIN="http://${HOST}:${PORT}"

if [ -z "${VITE_CLERK_PUBLISHABLE_KEY:-}" ]; then
  echo "error: VITE_CLERK_PUBLISHABLE_KEY is required in .env" >&2
  exit 1
fi

# The backend runtime config injects the publishable key into index.html.
# Keep the runtime key aligned with the frontend build key for local debug.
export CLERK_PUBLISHABLE_KEY="${CLERK_PUBLISHABLE_KEY:-$VITE_CLERK_PUBLISHABLE_KEY}"

# The checked-in example uses a placeholder host. That is not a valid local
# Clerk azp value, so replace it for localhost debugging instead of silently
# pretending the placeholder will work.
if [ -z "${CLERK_AUTHORIZED_PARTIES:-}" ] ||
  [[ "${CLERK_AUTHORIZED_PARTIES}" == *"your-app-domain.example.com"* ]]; then
  export CLERK_AUTHORIZED_PARTIES="$LOCAL_ORIGIN"
  echo "Using local Clerk authorized party: $CLERK_AUTHORIZED_PARTIES"
fi

if [ ! -d frontend/node_modules ]; then
  npm --prefix frontend ci
fi

npm --prefix frontend run build
rm -rf internal/web/dist
cp -r frontend/dist internal/web/dist

CGO_ENABLED=1 go build -tags fts5 -o agentsview ./cmd/agentsview

exec ./agentsview serve -host "$HOST" -port "$PORT" -no-browser
