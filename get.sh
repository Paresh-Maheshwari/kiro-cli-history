#!/bin/bash
set -e

REPO="Paresh-Maheshwari/kiro-cli-history"
BIN_NAME="kiro-cli-history"
INSTALL_DIR="$HOME/.local/bin"

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

ASSET="${BIN_NAME}-${OS}-${ARCH}"
echo "Detected: ${OS}/${ARCH}"

# Get latest release download URL
DOWNLOAD_URL=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep "browser_download_url.*${ASSET}" \
    | head -1 \
    | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: No release found for ${ASSET}"
    echo "Available at: https://github.com/${REPO}/releases"
    exit 1
fi

echo "Downloading ${ASSET}..."
mkdir -p "$INSTALL_DIR"
curl -sL "$DOWNLOAD_URL" -o "${INSTALL_DIR}/${BIN_NAME}"
chmod +x "${INSTALL_DIR}/${BIN_NAME}"

echo "Installed to ${INSTALL_DIR}/${BIN_NAME}"
echo ""

# Verify
"${INSTALL_DIR}/${BIN_NAME}" --version 2>/dev/null || true

# Check PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "Add to your PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

echo ""
echo "Run: ${BIN_NAME}"
