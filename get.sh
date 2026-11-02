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
echo "Detected: ${OS}/${ARCH}"

TMP=$(mktemp -d)
STAGED=""
trap 'rm -rf "$TMP"; if [ -n "$STAGED" ]; then rm -f "$STAGED"; fi' EXIT

VERSION="${KIRO_CLI_HISTORY_VERSION:-}"
if [ -n "${KIRO_CLI_HISTORY_RELEASE_DIR:-}" ]; then
    # Used to validate the exact build artifacts before publication.
    [ -n "$VERSION" ] || { echo "Error: staged installation requires KIRO_CLI_HISTORY_VERSION"; exit 1; }
    cp "${KIRO_CLI_HISTORY_RELEASE_DIR}/${ASSET}" "${TMP}/${ASSET}"
    cp "${KIRO_CLI_HISTORY_RELEASE_DIR}/checksums.txt" "${TMP}/checksums.txt"
else
    if [ -z "$VERSION" ]; then
        release=$(curl -fIsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")
        case "$release" in
            "https://github.com/${REPO}/releases/tag/"*) VERSION="${release##*/}" ;;
            *) echo "Error: could not resolve the latest release"; exit 1 ;;
        esac
    fi
    case "$VERSION" in
        *[!a-zA-Z0-9.+-]*|"") echo "Error: invalid release version"; exit 1 ;;
    esac
    VERSION="${VERSION#v}"
    BASE="https://github.com/${REPO}/releases/download/v${VERSION}"
    echo "Downloading ${ASSET} (${VERSION})..."
    curl -fsSL "${BASE}/${ASSET}" -o "${TMP}/${ASSET}"
    # A missing checksum is a failed installation, never an unverified fallback.
    curl -fsSL "${BASE}/checksums.txt" -o "${TMP}/checksums.txt"
fi
VERSION="${VERSION#v}"

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

mkdir -p "$INSTALL_DIR"
# Stage beside the destination so replacement is an atomic rename.
STAGED=$(mktemp "${INSTALL_DIR}/.${BIN_NAME}.XXXXXX")
cp "${TMP}/${ASSET}" "$STAGED"
chmod 755 "$STAGED"
output=$("$STAGED" --version)
if [ "${output%%$'\n'*}" != "${BIN_NAME} ${VERSION}" ]; then
    echo "Error: downloaded binary reported an unexpected version"
    exit 1
fi
mv -f "$STAGED" "${INSTALL_DIR}/${BIN_NAME}"
STAGED=""
echo "Installed to ${INSTALL_DIR}/${BIN_NAME}"
echo ""
printf '%s\n' "$output"

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "Add to your PATH:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
fi

echo ""
echo "Run: ${BIN_NAME}    Update later: ${BIN_NAME} update"
