#!/bin/sh
# blackhaul-daemon installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/zdods/blackhaul/main/install.sh | sh
#
# Environment overrides:
#   BLACKHAUL_VERSION      release tag to install (default: latest, e.g. v0.1.0)
#   BLACKHAUL_INSTALL_DIR  install directory (default: /usr/local/bin)
set -eu

REPO="zdods/blackhaul"
BINARY="blackhaul-daemon"
INSTALL_DIR="${BLACKHAUL_INSTALL_DIR:-/usr/local/bin}"
VERSION="${BLACKHAUL_VERSION:-latest}"

say() { printf '%s\n' "$*" >&2; }
fail() { say "error: $*"; exit 1; }

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

# --- Detect OS/arch -------------------------------------------------------
OS=$(uname -s)
case "$OS" in
  Linux) OS=linux ;;
  Darwin) OS=darwin ;;
  MINGW* | MSYS* | CYGWIN*)
    fail "on Windows, download the zip from https://github.com/$REPO/releases and see packaging/windows/README.md" ;;
  *) fail "unsupported OS: $OS" ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *) fail "unsupported architecture: $ARCH" ;;
esac

# --- Resolve version ------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    grep '"tag_name":' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$VERSION" ] || fail "could not determine the latest release; pass BLACKHAUL_VERSION=vX.Y.Z"
fi
VNUM=${VERSION#v} # archive names use the version without the leading v

ARCHIVE="${BINARY}_${VNUM}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"

say "installing $BINARY $VERSION ($OS/$ARCH) to $INSTALL_DIR"

# --- Download + verify ----------------------------------------------------
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL -o "$TMP/$ARCHIVE" "$BASE_URL/$ARCHIVE" ||
  fail "download failed: $BASE_URL/$ARCHIVE"
curl -fsSL -o "$TMP/checksums.txt" "$BASE_URL/checksums.txt" ||
  fail "download failed: $BASE_URL/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  SHA_TOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_TOOL="shasum -a 256"
else
  fail "sha256sum or shasum is required to verify the download"
fi
(
  cd "$TMP"
  grep " $ARCHIVE\$" checksums.txt | $SHA_TOOL -c - >/dev/null 2>&1 ||
    fail "checksum verification failed for $ARCHIVE"
)
say "checksum verified"

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BINARY"

# --- Install --------------------------------------------------------------
if [ -w "$INSTALL_DIR" ]; then
  install -m 0755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
else
  say "$INSTALL_DIR is not writable; using sudo"
  sudo install -m 0755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
fi

say ""
say "$("$INSTALL_DIR/$BINARY" --version) installed"
say ""
say "next steps:"
say "  1. create a daemon token in the blackhaul console"
say "  2. run '$BINARY' for interactive setup (saves config + keyring token)"
if [ "$OS" = linux ]; then
  say "  3. (optional) run it as a service — systemd unit with instructions:"
  say "     https://github.com/$REPO/blob/main/packaging/systemd/blackhaul-daemon.service"
else
  say "  3. (optional) run it as a service — launchd agent with instructions:"
  say "     https://github.com/$REPO/blob/main/packaging/launchd/io.github.blackhaul.daemon.plist"
fi
