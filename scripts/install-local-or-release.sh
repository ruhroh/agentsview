#!/bin/bash
# Install agentsview either from the local repo checkout or the latest release.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
LOCAL_REPO_ROOT="${AGENTSVIEW_REPO_ROOT:-$REPO_ROOT}"

# Reuse the existing release installer helpers instead of duplicating them.
# shellcheck source=install.sh
source "${SCRIPT_DIR}/install.sh"

usage() {
    cat <<EOF
Usage: $(basename "$0") [local|release]

Install agentsview from:
  local    Build and install from this repo checkout
  release  Install the latest GitHub release

Environment:
  AGENTSVIEW_REPO_ROOT   Override the repo path used for local installs
EOF
}

install_local() {
    info "Installing agentsview from local source..."
    info "Repo: ${LOCAL_REPO_ROOT}"
    echo

    if [ ! -d "${LOCAL_REPO_ROOT}" ]; then
        error "Repo directory does not exist: ${LOCAL_REPO_ROOT}"
    fi
    if [ ! -f "${LOCAL_REPO_ROOT}/Makefile" ]; then
        error "Makefile not found in repo: ${LOCAL_REPO_ROOT}"
    fi

    (
        cd "${LOCAL_REPO_ROOT}"
        make install
    )

    # This only affects the current shell when the script is sourced.
    hash -r 2>/dev/null || true

    echo
    info "Local install complete"
    if command -v agentsview >/dev/null 2>&1; then
        agentsview version
    else
        warn "agentsview is not on PATH in this shell"
    fi
}

install_release() {
    info "Installing agentsview from latest GitHub release..."
    echo

    local os
    os="$(detect_os)"
    local arch
    arch="$(detect_arch)"
    local install_dir
    install_dir="$(find_install_dir)"

    info "Platform: ${os}/${arch}"
    info "Install directory: ${install_dir}"
    echo

    if install_from_release "${os}" "${arch}" "${install_dir}"; then
        info "Installed from GitHub release"
    else
        error "Installation failed. Please check https://github.com/${REPO}/releases for available builds."
    fi

    # This only affects the current shell when the script is sourced.
    hash -r 2>/dev/null || true

    echo
    info "Installation complete"
    if command -v agentsview >/dev/null 2>&1; then
        agentsview version
    else
        warn "agentsview is not on PATH in this shell"
    fi

    if ! echo "$PATH" | grep -q "$install_dir"; then
        echo
        warn "Add this to your shell profile:"
        echo "  export PATH=\"\$PATH:${install_dir}\""
    fi
}

choose_mode() {
    local choice
    echo "Choose agentsview install source:"
    echo "  1) Local repo build (${LOCAL_REPO_ROOT})"
    echo "  2) Latest GitHub release"
    printf "> "
    read -r choice

    case "${choice}" in
        1|local|Local|LOCAL) echo "local" ;;
        2|release|Release|RELEASE) echo "release" ;;
        *) error "Invalid choice: ${choice}" ;;
    esac
}

main() {
    local mode="${1:-}"

    case "${mode}" in
        "")
            mode="$(choose_mode)"
            ;;
        local|release)
            ;;
        -h|--help|help)
            usage
            return 0
            ;;
        *)
            error "Unknown option: ${mode}"
            ;;
    esac

    case "${mode}" in
        local)
            install_local
            ;;
        release)
            install_release
            ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
