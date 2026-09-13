#!/bin/sh

set -eu

REPO="jiacai2050/peekd"
BINARY_NAME="peekd"
GITHUB_URL="https://github.com/${REPO}"

VERSION="latest"
INSTALL_DIR="${HOME}/.local/bin"
CHINA=false

usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -v, --version <ver>      Release version (e.g. v1.0.0), default is latest"
    echo "  -p, --prefix <dir>       Directory to install binary, default is ~/.local/bin"
    echo "  --china                  Use proxy for downloads"
    echo "  -h, --help               Show this help message"
    exit 0
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version|-v)
            [ "$#" -ge 2 ] || { echo "Error: --version requires an argument" >&2; exit 1; }
            VERSION="$2"
            shift 2
            ;;
        --prefix|-p)
            [ "$#" -ge 2 ] || { echo "Error: --prefix requires an argument" >&2; exit 1; }
            INSTALL_DIR="$2"
            shift 2
            ;;
        --china)
            CHINA=true
            shift
            ;;
        --help|-h)
            usage
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage
            ;;
    esac
done

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux*) OS="linux" ;;
    darwin*) OS="darwin" ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
    echo "Resolving latest version..."
    VERSION=$(curl -fsSI "${GITHUB_URL}/releases/latest" |
        awk 'tolower($1) == "location:" { sub(".*/tag/", "", $2); gsub("\r", "", $2); print $2; exit }')
fi

if [ -z "$VERSION" ]; then
    echo "Could not resolve the latest version for ${REPO}." >&2
    echo "Specify a version explicitly with: $0 --version v1.0.0" >&2
    exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

FILE_NAME="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="${GITHUB_URL}/releases/download/${VERSION}/${FILE_NAME}"

if [ "$CHINA" = true ]; then
    DOWNLOAD_URL="https://api.liujiacai.net/proxy/${DOWNLOAD_URL}"
fi

echo "Downloading ${BINARY_NAME} ${VERSION} for ${OS}/${ARCH}..."
curl -fL "$DOWNLOAD_URL" -o "${TMP_DIR}/${FILE_NAME}"

tar -xzf "${TMP_DIR}/${FILE_NAME}" -C "$TMP_DIR" "$BINARY_NAME"
chmod +x "${TMP_DIR}/${BINARY_NAME}"

mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/"
else
    echo "Need sudo permissions to move binary to ${INSTALL_DIR}"
    sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/"
fi

echo "Successfully installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"

case ":${PATH}:" in
    *:"${INSTALL_DIR}":*) ;;
    *) echo "Warning: ${INSTALL_DIR} is not in your PATH." ;;
esac

"${INSTALL_DIR}/${BINARY_NAME}" -version
