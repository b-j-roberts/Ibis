#!/usr/bin/env bash
set -euo pipefail

# Ibis installer script
# Usage: curl -sSL https://raw.githubusercontent.com/b-j-roberts/ibis/main/scripts/install.sh | bash

REPO="b-j-roberts/ibis"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="ibis"

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Get latest release tag
LATEST=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$LATEST" ]; then
    echo "Failed to fetch latest release"
    exit 1
fi

echo "Installing ibis ${LATEST} (${OS}/${ARCH})..."

# Download binary
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST}/${BINARY}_${OS}_${ARCH}"
TMP=$(mktemp)
curl -sSL -o "$TMP" "$DOWNLOAD_URL"
chmod +x "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
    echo "Need sudo to install to ${INSTALL_DIR}"
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ibis to ${INSTALL_DIR}/${BINARY}"
ibis --help
