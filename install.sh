#!/bin/bash
# Build kiro-cli-history from source and install it (Linux / macOS).
# For a prebuilt binary without Go, use get.sh instead:
#   curl -sL https://raw.githubusercontent.com/Paresh-Maheshwari/kiro-cli-history/main/get.sh | bash
set -euo pipefail

BIN_NAME="kiro-cli-history"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

echo "kiro-cli-history installer (from source)"
echo "========================================"
echo ""

if ! command -v go &>/dev/null; then
    echo "ERROR: Go is required but not found. Install it from https://go.dev/dl/"
    echo "Or install the prebuilt binary:"
    echo "  curl -sL https://raw.githubusercontent.com/Paresh-Maheshwari/kiro-cli-history/main/get.sh | bash"
    exit 1
fi
echo "Using $(go version)"

cd "$(dirname "$0")"

# SQLite (classic-mode chats) needs a C compiler. Without one, build without
# it: everything else works, classic chats are skipped.
if command -v "${CC:-cc}" &>/dev/null || command -v gcc &>/dev/null || command -v clang &>/dev/null; then
    export CGO_ENABLED=1
    # The bundled SQLite C source triggers harmless const-qualifier warnings
    # with newer GCC; hide them without overriding the user's own flags.
    export CGO_CFLAGS="${CGO_CFLAGS:--O2 -g} -Wno-discarded-qualifiers"
else
    export CGO_ENABLED=0
    echo "NOTE: no C compiler found; building without classic-mode (SQLite) chat support."
    echo "      Install gcc (e.g. 'sudo dnf install gcc' / 'sudo apt install gcc') and re-run to enable it."
fi

echo "Building..."
TMP_BIN=$(mktemp "${TMPDIR:-/tmp}/${BIN_NAME}.XXXXXX")
trap 'rm -f "$TMP_BIN"' EXIT
go build -trimpath -ldflags="-s -w" -o "$TMP_BIN" .

mkdir -p "$INSTALL_DIR"
chmod +x "$TMP_BIN"
mv -f "$TMP_BIN" "$INSTALL_DIR/$BIN_NAME"
trap - EXIT

echo ""
echo "Installed to $INSTALL_DIR/$BIN_NAME"
"$INSTALL_DIR/$BIN_NAME" --version | head -1

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    case "$(basename "${SHELL:-bash}")" in
        zsh)  RC="~/.zshrc" ;;
        fish) RC="~/.config/fish/config.fish" ;;
        *)    RC="~/.bashrc" ;;
    esac
    echo ""
    echo "NOTE: $INSTALL_DIR is not in your PATH. Add this to $RC:"
    echo ""
    if [ "$RC" = "~/.config/fish/config.fish" ]; then
        echo "  fish_add_path $INSTALL_DIR"
    else
        echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
fi

echo ""
echo "Run: $BIN_NAME    Help: $BIN_NAME --help    Update: $BIN_NAME update"
