#!/bin/sh
set -eu

REPO="wso2/agent-manager"
BINARY="amctl"
TAG_PREFIX="amctl/v"

log() {
    printf '==> %s\n' "$*"
}

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

cleanup() {
    [ -n "${TMPDIR_CREATED:-}" ] && rm -rf "$TMPDIR_CREATED"
}
trap cleanup EXIT

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      fail "Unsupported OS: $OS. Only linux and darwin are supported." ;;
    esac

    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)       ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)            fail "Unsupported architecture: $ARCH. Only amd64 and arm64 are supported." ;;
    esac
}

fetch() {
    url="$1"
    output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$output" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$output" "$url"
    else
        fail "Neither curl nor wget found. Please install one of them."
    fi
}

resolve_version() {
    if [ -n "${AMCTL_VERSION:-}" ]; then
        VERSION="$AMCTL_VERSION"
        log "Using pinned version: $VERSION"
        return
    fi

    log "Resolving latest version..."
    TMPDIR_CREATED=$(mktemp -d)
    releases_json="$TMPDIR_CREATED/releases.json"
    fetch "https://api.github.com/repos/${REPO}/releases" "$releases_json"

    VERSION=$(grep -o "\"tag_name\": *\"${TAG_PREFIX}[^\"]*\"" "$releases_json" \
        | head -1 \
        | sed "s/.*\"${TAG_PREFIX}\([^\"]*\)\"/\1/")

    if [ -z "$VERSION" ]; then
        fail "Could not find any ${TAG_PREFIX}* release. Check https://github.com/${REPO}/releases"
    fi
    log "Latest version: $VERSION"
}

download_and_verify() {
    TMPDIR_CREATED=$(mktemp -d)
    ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
    BASE_URL="https://github.com/${REPO}/releases/download/${TAG_PREFIX}${VERSION}"

    log "Downloading $ARCHIVE..."
    fetch "${BASE_URL}/${ARCHIVE}" "${TMPDIR_CREATED}/${ARCHIVE}"

    log "Downloading checksums..."
    fetch "${BASE_URL}/checksums.txt" "${TMPDIR_CREATED}/checksums.txt"

    log "Verifying checksum..."
    cd "$TMPDIR_CREATED"
    if command -v sha256sum >/dev/null 2>&1; then
        grep "$ARCHIVE" checksums.txt | sha256sum -c --quiet
    elif command -v shasum >/dev/null 2>&1; then
        grep "$ARCHIVE" checksums.txt | shasum -a 256 -c --quiet
    else
        fail "Neither sha256sum nor shasum found. Cannot verify checksum."
    fi
    log "Checksum verified."
}

install_binary() {
    cd "$TMPDIR_CREATED"
    tar -xzf "$ARCHIVE"

    if [ -n "${AMCTL_INSTALL_DIR:-}" ]; then
        INSTALL_DIR="$AMCTL_INSTALL_DIR"
    elif [ -w /usr/local/bin ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="${HOME}/.local/bin"
    fi

    mkdir -p "$INSTALL_DIR"
    mv "$BINARY" "$INSTALL_DIR/$BINARY"
    chmod +x "$INSTALL_DIR/$BINARY"

    log "Installed $BINARY to $INSTALL_DIR/$BINARY"

    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *) log "Add $INSTALL_DIR to your PATH: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
    esac

    log "$("$INSTALL_DIR/$BINARY" version 2>/dev/null || echo "$BINARY $VERSION")"
}

detect_platform
resolve_version
download_and_verify
install_binary
