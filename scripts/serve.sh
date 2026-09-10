#!/usr/bin/env bash
# Launches the fetched onsetter binary in serve mode, for .mcp.json's
# command entry.
#
# .mcp.json spawns this through `bash -c`, not directly, and reads both
# CLAUDE_PLUGIN_ROOT and $1 (the plugin data dir) from the real shell
# environment rather than from Claude Code's own `${...}` template
# substitution in the command/args fields. That substitution is documented
# to apply to a stdio server's command/args/env, and does on a session's
# first connect — but breaks on reconnect, spawning the literal unexpanded
# placeholder instead of a real path (confirmed live: a fresh
# `/plugin install` hit this immediately, not just a later reconnect;
# tracked upstream at anthropics/claude-code#65747 and #67483, both closed
# without a fix as of this writing). Reading CLAUDE_PLUGIN_ROOT from the
# process environment instead — verified directly by dumping a real spawned
# process's own env — sidesteps the same bug, since it never depends on
# Claude Code substituting anything inside a JSON string at all.
#
# A missing or non-executable binary (first install, or a session that
# starts mid-version-bump-refetch) exits 1 — a failed server connection,
# which Claude Code treats as a non-blocking, per-hook error rather than a
# session failure. That window is bounded and
# self-heals on the next session (fetch.sh has finished by then) — an
# accepted, documented gap, not chased with a retry loop here.
set -u
DATA_DIR="${1:-}"
BIN="$DATA_DIR/bin/onsetter"
if [ -n "$DATA_DIR" ] && [ -x "$BIN" ]; then
    exec "$BIN" serve
fi
exit 1
