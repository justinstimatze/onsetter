# Security

## Reporting a vulnerability

Email **justin@justinstimatze.com** with `onsetter-security` in the subject.
Please do not file public issues for suspected vulnerabilities.

## Threat model

onsetter runs locally and sends nothing over the network, ever. It has no
configuration beyond the `CLAUDE.md` files already in your tree.

**What it reads.** The `CLAUDE.md` and `CLAUDE.local.md` files between an
edited file and its repository root, the pending tool call on stdin, and —
only when an ask sets `requires:` — whether a named binary resolves on
`$PATH`. That check never executes the binary; it only stats directories on
`$PATH`, the same as a shell resolving a command before running it.

**What it writes.** One file per session under `~/.cache/onsetter/sessions/`,
holding rule-identity hashes and nothing else — no paths, no file contents, no
prompt text. Entries older than fourteen days are removed when a new session
starts. `onsetter install` writes `~/.claude/settings.local.json` and leaves a
timestamped backup beside it.

**What it emits.** The prose of matching rules, plus the substring the content
gate matched, capped at 80 characters, injected as advisory context. A
`CLAUDE.md` in a repository you did not write can therefore put text of its
choosing in front of the model at edit time — the same trust you already extend
to that file by opening the repository in Claude Code. onsetter cannot block a
tool call and never executes anything from a rule.
