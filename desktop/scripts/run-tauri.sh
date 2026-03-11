#!/usr/bin/env bash
set -euo pipefail

# Wrapper that restores tauri.conf.json after `tauri` exits,
# undoing the version patch applied by prepare-sidecar.sh.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONF="$SCRIPT_DIR/../src-tauri/tauri.conf.json"

cleanup() {
  git checkout -- "$CONF" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

tauri "$@"
