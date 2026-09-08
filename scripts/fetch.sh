#!/usr/bin/env bash
# Fetches the onsetter release binary matching this plugin's pinned version
# into CLAUDE_PLUGIN_DATA, verified against the release's checksums.txt.
# Runs from a SessionStart hook. Every failure path exits 0 — the same
# invariant onsetter's own PreToolUse hook holds itself to (see the ask block
# in this repo's CLAUDE.md gating cmd/**/*.go): a session must never fail to
# start because a binary fetch didn't land. Diagnostics go to fetch.log
# instead of stderr, so a real failure is still findable without breaking
# that invariant.
#
# checksums.txt is fetched from the same mutable GitHub release as the
# archive, so on its own it only catches a truncated download — anyone who
# can overwrite release assets can rewrite both together, and the sha the
# marketplace pins covers this script's own commit but not a binary fetched
# at runtime from a release. scripts/checksums-pin.txt closes that: it's
# committed to the repo (so it inherits the marketplace's commit-sha pin
# like any other tracked file) and names the *expected hash of
# checksums.txt itself* for each version, populated by `make pin-checksums`
# once a release is confirmed published. No entry for this version means no
# release has been vetted this way yet — refuse rather than trust an
# unpinned checksums.txt, which is a silent, harmless failure exactly like
# every other branch below.
set -u

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
DATA_DIR="${CLAUDE_PLUGIN_DATA:-}"
[ -n "$PLUGIN_ROOT" ] && [ -n "$DATA_DIR" ] || exit 0

mkdir -p "$DATA_DIR" 2>/dev/null || exit 0
exec >>"$DATA_DIR/fetch.log" 2>&1

VERSION=$(grep '"version"' "$PLUGIN_ROOT/.claude-plugin/plugin.json" 2>/dev/null | head -1 | sed -E 's/.*"version": *"([^"]+)".*/\1/')
echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "no usable version in plugin.json"; exit 0; }

BIN_DIR="$DATA_DIR/bin"
MARKER="$DATA_DIR/version"

# Already have this exact version cached, executable, and not a dangling
# symlink from a prior run's extraction — nothing to do.
if [ -f "$MARKER" ] && [ "$(cat "$MARKER" 2>/dev/null)" = "$VERSION" ] \
   && [ -x "$BIN_DIR/onsetter" ] && [ ! -L "$BIN_DIR/onsetter" ]; then
    exit 0
fi

case "$(uname -s)" in
    Linux) OS=linux ;;
    Darwin) OS=darwin ;;
    *) echo "unsupported OS: $(uname -s)"; exit 0 ;;
esac

case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) echo "unsupported arch: $(uname -m)"; exit 0 ;;
esac

command -v curl >/dev/null 2>&1 || { echo "no curl on PATH"; exit 0; }

ARCHIVE="onsetter_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/justinstimatze/onsetter/releases/download/v${VERSION}"

# Inside DATA_DIR, not /tmp: the eventual mv into BIN_DIR must be an atomic
# same-filesystem rename, not a cross-device streaming copy that leaves a
# growing, already-"+x" file visible at its final path mid-copy.
TMP_DIR=$(mktemp -d "$DATA_DIR/.fetch.XXXXXX") || exit 0
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM HUP

command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || { echo "no sha256 tool available"; exit 0; }

PIN_FILE="$PLUGIN_ROOT/scripts/checksums-pin.txt"
PIN=$([ -f "$PIN_FILE" ] && awk -v v="$VERSION" '$1 == v {print $2; exit}' "$PIN_FILE")
[ -n "$PIN" ] || { echo "no pinned checksums.txt hash for $VERSION in checksums-pin.txt — refusing an unpinned release"; exit 0; }

CURL_OPTS="-fsSL --connect-timeout 5 --max-time 30"
curl $CURL_OPTS -o "$TMP_DIR/$ARCHIVE" "$BASE_URL/$ARCHIVE" || { echo "download failed: $ARCHIVE"; exit 0; }
curl $CURL_OPTS -o "$TMP_DIR/checksums.txt" "$BASE_URL/checksums.txt" || { echo "download failed: checksums.txt"; exit 0; }

[ "$(sha256_of "$TMP_DIR/checksums.txt")" = "$PIN" ] || { echo "checksums.txt does not match the pinned hash for $VERSION — refusing to trust it"; exit 0; }

EXPECTED=$(awk -v target="$ARCHIVE" '{name=$2; sub(/^\*/, "", name); if (name == target) {print $1; exit}}' "$TMP_DIR/checksums.txt")
[ -n "$EXPECTED" ] || { echo "no checksum entry for $ARCHIVE"; exit 0; }
[ "$(sha256_of "$TMP_DIR/$ARCHIVE")" = "$EXPECTED" ] || { echo "checksum mismatch for $ARCHIVE"; exit 0; }

# Extract into its own subdir and demand exactly one real regular file named
# onsetter — not a symlink. A crafted or corrupted archive whose "onsetter"
# member is a symlink to an absolute path outside this dir would otherwise
# have that path chmod +x'd and then exec'd by every Write/Edit/Bash call.
EXTRACT_DIR="$TMP_DIR/extract"
mkdir -p "$EXTRACT_DIR" || exit 0
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$EXTRACT_DIR" onsetter || { echo "extraction failed"; exit 0; }

SRC="$EXTRACT_DIR/onsetter"
[ -f "$SRC" ] && [ ! -L "$SRC" ] && [ -s "$SRC" ] || { echo "extracted member is not a plain non-empty file"; exit 0; }

mkdir -p "$BIN_DIR" || exit 0
chmod +x "$SRC" && mv "$SRC" "$BIN_DIR/onsetter" || { echo "install into BIN_DIR failed"; exit 0; }
echo "$VERSION" > "$MARKER"
echo "installed onsetter $VERSION"
exit 0
