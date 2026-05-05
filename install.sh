#!/bin/sh
# install.sh — download and install the latest gh-peek release binary.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sud0whoami/gh-peek/main/install.sh | sh
#   VERSION=1.2.3 curl -fsSL ... | sh   # pin a specific version
set -eu

REPO="sud0whoami/gh-peek"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Resolve OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Resolve arch
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Resolve version (latest if not pinned)
if [ -z "${VERSION:-}" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/')"
fi

ARCHIVE="gh-peek_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/v${VERSION}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading gh-peek v${VERSION} (${OS}/${ARCH})..."
curl -fsSL "${BASE}/${ARCHIVE}" -o "${TMP}/${ARCHIVE}"

echo "Verifying checksum..."
curl -fsSL "${BASE}/checksums.txt" | grep "${ARCHIVE}" > "${TMP}/checksum.txt"
# sha256sum is standard on Linux; shasum ships with macOS (and most Debian/Ubuntu).
# Alpine and other minimal images only have sha256sum.
if command -v sha256sum > /dev/null 2>&1; then
  (cd "$TMP" && sha256sum --check checksum.txt)
else
  (cd "$TMP" && shasum -a 256 -c checksum.txt)
fi

echo "Installing to ${INSTALL_DIR}..."
tar -xzf "${TMP}/${ARCHIVE}" -C "${TMP}" gh-peek
if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "${TMP}/gh-peek" "${INSTALL_DIR}/"
else
  sudo install -m 755 "${TMP}/gh-peek" "${INSTALL_DIR}/"
fi

echo "Installed gh-peek v${VERSION} to ${INSTALL_DIR}/gh-peek"
