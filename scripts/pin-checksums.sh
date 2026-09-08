#!/usr/bin/env bash
# Pins the sha256 of a published release's checksums.txt into
# scripts/checksums-pin.txt, so fetch.sh can trust it instead of just the
# mutable GitHub release it came from — see the comment at the top of
# fetch.sh for why that distinction matters. Run this after `gh release view
# vX.Y.Z` confirms goreleaser actually published assets for the tag; this
# step is what makes that version installable by the plugin at all.
#
# Usage: scripts/pin-checksums.sh 0.8.0
set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    echo "usage: $0 <version, e.g. 0.8.0 or v0.8.0>" >&2
    exit 1
fi
# The Makefile's own $(VERSION) (git describe) carries a v-prefix; plugin.json
# and checksums-pin.txt don't. Accept either so `make pin-checksums` without
# an explicit override doesn't silently look for release "vv0.8.0".
VERSION="${VERSION#v}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PIN_FILE="$ROOT/scripts/checksums-pin.txt"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

gh release download "v$VERSION" --repo justinstimatze/onsetter \
    --pattern checksums.txt --dir "$TMP_DIR" --clobber

if command -v sha256sum >/dev/null 2>&1; then
    HASH=$(sha256sum "$TMP_DIR/checksums.txt" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    HASH=$(shasum -a 256 "$TMP_DIR/checksums.txt" | awk '{print $1}')
else
    echo "no sha256 tool available" >&2
    exit 1
fi

grep -v "^$VERSION " "$PIN_FILE" > "$PIN_FILE.tmp" || true
mv "$PIN_FILE.tmp" "$PIN_FILE"
echo "$VERSION $HASH" >> "$PIN_FILE"

echo "pinned $VERSION -> $HASH"
echo "review the diff in scripts/checksums-pin.txt, then commit it"
