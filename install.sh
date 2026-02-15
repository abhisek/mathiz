#!/bin/sh
set -e

REPO="abhisek/mathiz"
INSTALL_DIR="/usr/local/bin"
BINARY="mathiz"

# Detect OS and architecture.
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) os="windows" ;;
  *) echo "Error: unsupported operating system: $OS" >&2; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64)  arch="arm64" ;;
  *) echo "Error: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

if [ "$os" = "windows" ]; then
  BINARY="mathiz.exe"
fi

# Fetch latest release tag.
echo "Fetching latest release..."
tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//')
if [ -z "$tag" ]; then
  echo "Error: could not determine latest release" >&2
  exit 1
fi
echo "Latest release: $tag"

# Build download URL.
asset="mathiz-${os}-${arch}"
if [ "$os" = "windows" ]; then
  asset="${asset}.exe"
fi
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"

# Download binary.
echo "Downloading ${asset}..."
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
if ! curl -fsSL -o "${tmpdir}/${BINARY}" "$url"; then
  echo "Error: binary not available for ${os}/${arch}" >&2
  echo "Check available assets at https://github.com/${REPO}/releases/tag/${tag}" >&2
  exit 1
fi

# Install.
if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo install -m 755 "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed mathiz ${tag} to ${INSTALL_DIR}/${BINARY}"

# Verify the install directory is in PATH.
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Warning: ${INSTALL_DIR} is not in your PATH. Add it with:" >&2
     echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" >&2 ;;
esac
