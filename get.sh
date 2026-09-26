#!/bin/bash
# Install the latest kiro-cli-history release binary (Linux / macOS).
set -euo pipefail

REPO="Paresh-Maheshwari/kiro-cli-history"
BIN_NAME="kiro-cli-history"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
case "$OS" in
    linux|darwin) ;;
    mingw*|msys*|cygwin*)
        echo "On Windows, download ${BIN_NAME}-windows-${ARCH}.exe from"
        echo "https://github.com/${REPO}/releases/latest"
        exit 1 ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

ASSET="${BIN_NAME}-${OS}-${ARCH}"
BASE="https://github.com/${REPO}/releases/latest/download"
echo "Detected: ${OS}/${ARCH}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${ASSET}..."
if ! curl -fsSL "${BASE}/${ASSET}" -o "${TMP}/${ASSET}"; then
    echo "Error: no release found for ${ASSET}"
    echo "See: https://github.com/${REPO}/releases"
    exit 1
fi

if curl -fsSL "${BASE}/checksums.txt" -o "${TMP}/checksums.txt"; then
    want=$(awk -v f="$ASSET" '$2 == f || $2 == "*" f {print $1}' "${TMP}/checksums.txt")
    if command -v sha256sum >/dev/null; then
        got=$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')
    else
        got=$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')
    fi
    if [ -z "$want" ] || [ "$want" != "$got" ]; then
        echo "Error: checksum verification failed for ${ASSET}"
        exit 1
    fi
    echo "Checksum verified."
else
    echo "Warning: release has no checksums.txt; skipping verification."
fi

mkdir -p "$INSTALL_DIR"
chmod +x "${TMP}/${ASSET}"
mv -f "${TMP}/${ASSET}" "${INSTALL_DIR}/${BIN_NAME}"
echo "Installed to ${INSTALL_DIR}/${BIN_NAME}"
echo ""

"${INSTALL_DIR}/${BIN_NAME}" --version 2>/dev/null || true

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "Add to your PATH:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
fi

echo ""
echo "Run: ${BIN_NAME}    Update later: ${BIN_NAME} update"
