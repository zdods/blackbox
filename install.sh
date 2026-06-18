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

# Action: install (default) or uninstall.
ACTION=install
case "${1:-}" in
  --uninstall | uninstall) ACTION=uninstall ;;
  "") ;;
  *) fail "unknown argument: $1 (supported: --uninstall)" ;;
esac

# --- Detect OS (needed for install and uninstall) -------------------------
OS=$(uname -s)
case "$OS" in
  Linux) OS=linux ;;
  Darwin) OS=darwin ;;
  MINGW* | MSYS* | CYGWIN*)
    fail "on Windows, download the zip from https://github.com/$REPO/releases and see packaging/windows/README.md" ;;
  *) fail "unsupported OS: $OS" ;;
esac

# --- Uninstall ------------------------------------------------------------
if [ "$ACTION" = uninstall ]; then
  BIN="$INSTALL_DIR/$BINARY"
  say "uninstalling $BINARY"

  # 1. Stop and remove the service unit if one was installed.
  if [ "$OS" = darwin ]; then
    PLIST="$HOME/Library/LaunchAgents/io.github.blackhaul.daemon.plist"
    if [ -f "$PLIST" ]; then
      launchctl unload "$PLIST" 2>/dev/null || true
      rm -f "$PLIST"
      say "  removed launchd agent"
    fi
  else
    if command -v systemctl >/dev/null 2>&1 &&
      systemctl list-unit-files 2>/dev/null | grep -q '^blackhaul-daemon\.service'; then
      sudo systemctl disable --now blackhaul-daemon 2>/dev/null || true
      sudo rm -f /etc/systemd/system/blackhaul-daemon.service
      sudo systemctl daemon-reload 2>/dev/null || true
      say "  removed systemd service"
    fi
  fi

  # 2. Clear local credentials (keyring token + config) BEFORE removing the
  #    binary, since the binary does this cleanup itself.
  if [ -x "$BIN" ]; then
    "$BIN" --reset || true
  else
    say "  $BIN not found; if the daemon is elsewhere, run '$BINARY --reset' to clear stored credentials"
  fi

  # 3. Remove the binary.
  if [ -e "$BIN" ]; then
    if [ -w "$INSTALL_DIR" ]; then rm -f "$BIN"; else sudo rm -f "$BIN"; fi
    say "  removed $BIN"
  fi

  say ""
  say "uninstalled. To fully revoke access, also delete this host in the blackhaul console."
  exit 0
fi

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

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
  # Match the exact filename and compare hashes directly. (Piping a grep into
  # `sha256sum -c` can silently pass on empty input on some platforms, so the
  # expected hash is asserted non-empty and compared explicitly.)
  expected=$(awk -v f="$ARCHIVE" '$2 == f {print $1}' checksums.txt)
  [ -n "$expected" ] || fail "no checksum listed for $ARCHIVE"
  actual=$($SHA_TOOL "$ARCHIVE" | awk '{print $1}')
  [ "$expected" = "$actual" ] || fail "checksum verification failed for $ARCHIVE"
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
say ""
say "to remove: curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sh -s -- --uninstall"
say "  (or '$BINARY --reset' to clear just the stored token + config)"
