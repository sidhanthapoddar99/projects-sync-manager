#!/bin/sh
# Projects Sync Manager (PSM) — Universal Installer & Runner
# Works on Linux and macOS. Downloads the latest binary once, caches it, and runs it.
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.sh | sh
#   curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.sh | sh -s -- -d ~/projects
#   curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.sh | sh -s -- -d ~/projects -h 5

set -e

REPO="sidhanthapoddar99/projects-sync-manager"
CACHE_DIR="${TMPDIR:-/tmp}/psm-cache"
VERSION_FILE="$CACHE_DIR/version"

# --- Detect OS and architecture ---
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$OS" in
        linux*)  OS="linux" ;;
        darwin*) OS="darwin" ;;
        *)       echo "Error: Unsupported OS: $OS" >&2; exit 1 ;;
    esac

    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)  ARCH="arm64" ;;
        *)              echo "Error: Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac

    BINARY_NAME="psm-${OS}-${ARCH}"
    BINARY_PATH="$CACHE_DIR/$BINARY_NAME"
}

# --- Get latest release version from GitHub ---
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -sI "https://github.com/$REPO/releases/latest" 2>/dev/null | grep -i "^location:" | sed 's|.*/tag/||' | tr -d '\r\n'
    elif command -v wget >/dev/null 2>&1; then
        wget --spider --max-redirect=0 "https://github.com/$REPO/releases/latest" 2>&1 | grep "Location:" | sed 's|.*/tag/||' | tr -d '\r\n'
    else
        echo "Error: Neither curl nor wget found" >&2
        exit 1
    fi
}

# --- Download binary ---
download_binary() {
    local version="$1"
    local url="https://github.com/$REPO/releases/download/${version}/${BINARY_NAME}"

    echo "Downloading PSM $version for $OS/$ARCH..."

    mkdir -p "$CACHE_DIR"

    if command -v curl >/dev/null 2>&1; then
        curl -sL "$url" -o "$BINARY_PATH"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url" -O "$BINARY_PATH"
    fi

    chmod +x "$BINARY_PATH"
    echo "$version" > "$VERSION_FILE"
    echo "Cached at $BINARY_PATH"
}

# --- Main ---
detect_platform

echo "PSM — Detecting $OS/$ARCH"

LATEST=$(get_latest_version)

if [ -z "$LATEST" ]; then
    echo "Warning: Could not fetch latest version from GitHub" >&2
    if [ -f "$BINARY_PATH" ]; then
        echo "Using cached binary"
    else
        echo "Error: No cached binary and cannot reach GitHub" >&2
        exit 1
    fi
elif [ -f "$BINARY_PATH" ] && [ -f "$VERSION_FILE" ] && [ "$(cat "$VERSION_FILE")" = "$LATEST" ]; then
    echo "Already up to date ($LATEST)"
else
    download_binary "$LATEST"
fi

echo "---"
exec "$BINARY_PATH" "$@"
