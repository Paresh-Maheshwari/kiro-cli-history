#!/bin/bash
set -e

BIN_NAME="kiro-cli-history"
INSTALL_DIR="$HOME/.local/bin"

echo "kiro-cli-history installer"
echo "========================="
echo ""

# Check Go
if ! command -v go &>/dev/null; then
    echo "ERROR: Go is required but not found."
    echo "Install from: https://go.dev/dl/"
    exit 1
fi

echo "Building..."
cd "$(dirname "$0")"
# The bundled SQLite C source triggers harmless const-qualifier warnings
# with newer GCC; hide them without overriding the user's own flags.
CGO_CFLAGS="${CGO_CFLAGS:--O2 -g} -Wno-discarded-qualifiers" go build -o "$BIN_NAME" .

# Install binary
mkdir -p "$INSTALL_DIR"
mv "$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo ""
echo "Installed to $INSTALL_DIR/$BIN_NAME"

# Check PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "NOTE: $INSTALL_DIR is not in your PATH."
    echo "Add this to your ~/.bashrc or ~/.zshrc:"
    echo ""
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
fi

echo ""
echo "Run: kiro-cli-history"
echo "Help: kiro-cli-history --help"
