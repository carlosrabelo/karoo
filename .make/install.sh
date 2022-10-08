#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="${BINARY_NAME:-karoo}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT_DIR/bin/$BINARY_NAME"

# Prefer the invoking user's home when running under sudo.
user_home() {
  if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    getent passwd "$SUDO_USER" | cut -d: -f6
    return
  fi
  echo "${HOME}"
}

# SYSTEM=1 or PREFIX=… → system-wide; else ~/.local/bin
if [ -n "${PREFIX:-}" ]; then
  INSTALL_DIR="${PREFIX%/}/bin"
elif [ "${SYSTEM:-0}" = "1" ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="$(user_home)/.local/bin"
fi

if [ ! -x "$SRC" ]; then
  echo "error: missing binary: $SRC" >&2
  echo "hint:  run make build first" >&2
  exit 1
fi

DEST="$INSTALL_DIR/$BINARY_NAME"
UH="$(user_home)"

# Under sudo: install into the user's home as that user so paths/ownership stay correct.
if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
  case "$INSTALL_DIR" in
    "$UH" | "$UH"/*)
      sudo -u "$SUDO_USER" install -D -m 755 "$SRC" "$DEST"
      ;;
    *)
      install -D -m 755 "$SRC" "$DEST"
      ;;
  esac
elif [ -w "$(dirname "$INSTALL_DIR")" ] 2>/dev/null || [ -w "$INSTALL_DIR" ] 2>/dev/null; then
  install -D -m 755 "$SRC" "$DEST"
else
  echo "Destination not writable; using sudo for install only..."
  sudo install -D -m 755 "$SRC" "$DEST"
fi

echo "Installed: $DEST"
