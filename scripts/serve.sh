#!/usr/bin/env bash
# Launches the fetched onsetter binary in serve mode, for .mcp.json's
# command entry.
#
# .mcp.json's command+args (both set, matching hooks.json's own exec-form
# convention) spawns directly rather than through a shell — no && / || is
# available inline the way hooks.json's shell-form command uses today, so
# that short-circuit lives here instead. The plugin data dir is passed as an
# argument, substituted by Claude Code before spawn, rather than read from
# an env var this script would otherwise have to assume is exported into an
# MCP server's own process the same way it's documented to be for hooks.
#
# A missing or non-executable binary (first install, or a session that
# starts mid-version-bump-refetch) exits 1 — a failed server connection,
# which Claude Code treats as a non-blocking error for every mcp_tool hook
# that names this server, not a session failure. That window is bounded and
# self-heals on the next session (fetch.sh has finished by then) — an
# accepted, documented gap, not chased with a retry loop here.
set -u
DATA_DIR="${1:-}"
BIN="$DATA_DIR/bin/onsetter"
if [ -n "$DATA_DIR" ] && [ -x "$BIN" ]; then
    exec "$BIN" serve
fi
exit 1
