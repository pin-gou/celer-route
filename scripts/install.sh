#!/usr/bin/env bash
set -euo pipefail

# celer-route single-binary installer.
#
# Downloads the celer-route-http gateway binary for the current OS/arch from
# the GitHub release assets, verifies its SHA256 checksum, and installs it to
# PREFIX (default ~/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/pin-gou/celer-route/main/scripts/install.sh | bash
#   bash install.sh --version v1.2.3 --prefix /usr/local/bin

REPO="pin-gou/celer-route"
BIN_NAME="celer-route-http"
VERSION="latest"
PREFIX="${CELER_ROUTE_PREFIX:-${HOME}/.local/bin}"
MIRROR=""

usage() {
  cat <<'EOF'
Usage: install.sh [--version vX.Y.Z] [--prefix DIR] [--mirror URL]

Install the celer-route HTTP gateway binary.

Options:
  --version vX.Y.Z   Version to install (default: latest release)
  --prefix DIR       Install directory (default: ~/.local/bin)
  --mirror URL       Optional base URL of a binary mirror (used instead of GitHub release assets)

Examples:
  bash install.sh
  bash install.sh --version v1.2.3
  bash install.sh --prefix /usr/local/bin
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    --mirror) MIRROR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) os="windows" ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

# Only windows/amd64 is published
if [[ "$os" == "windows" && "$arch" != "amd64" ]]; then
  echo "unsupported windows architecture: $arch (only amd64 is published)" >&2
  exit 1
fi

ext=""
[[ "$os" == "windows" ]] && ext=".exe"

# Resolve the latest transports release if no explicit version was given
if [[ "$VERSION" == "latest" ]]; then
  refs="$(curl -fsSL "https://api.github.com/repos/${REPO}/git/matching-refs/tags/transports/v" 2>/dev/null || true)"
  VERSION="$(printf '%s' "$refs" | grep -o '"ref": *"refs/tags/transports/v[^"]*"' | sed 's/.*transports\///;s/"//' | sort -V | tail -1 || true)"
  if [[ -z "$VERSION" ]]; then
    echo "could not resolve the latest version from GitHub" >&2
    exit 1
  fi
  echo "latest version: $VERSION"
else
  VERSION="${VERSION#v}"
  VERSION="v${VERSION}"
fi

TAG="transports/${VERSION}"
ASSET="${BIN_NAME}-${os}-${arch}${ext}"
if [[ -n "$MIRROR" ]]; then
  URL="${MIRROR}/${VERSION}/${os}/${arch}/${BIN_NAME}${ext}"
else
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
fi

echo "Downloading ${URL} ..."
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fL --retry 3 -o "$TMP/$ASSET" "$URL" || { echo "download failed: $URL" >&2; exit 1; }

# Verify checksum unless a custom mirror is used (mirrors may not ship checksums)
if [[ -z "$MIRROR" ]]; then
  curl -fL --retry 3 -o "$TMP/$ASSET.sha256" "${URL}.sha256" || { echo "checksum download failed: ${URL}.sha256" >&2; exit 1; }
  (cd "$TMP" && (shasum -a 256 -c "$ASSET.sha256" 2>/dev/null || sha256sum -c "$ASSET.sha256")) \
    || { echo "SHA256 checksum verification failed" >&2; exit 1; }
  echo "✅ SHA256 checksum verified"
fi

chmod +x "$TMP/$ASSET"
mkdir -p "$PREFIX"
install -m 0755 "$TMP/$ASSET" "$PREFIX/$BIN_NAME$ext"

echo ""
echo "✅ installed $BIN_NAME v${VERSION} to $PREFIX/$BIN_NAME$ext"
echo ""
echo "Run it:"
echo "  $PREFIX/$BIN_NAME$ext -host 0.0.0.0 -port 8080"
echo ""
if [[ ":$PATH:" != *":$PREFIX:"* ]]; then
  echo "Add $PREFIX to your PATH if not already:"
  echo "  export PATH=\"$PREFIX:\$PATH\""
fi