# Security

## Reporting a vulnerability

Email **justin@justinstimatze.com** with `onsetter-security` in the subject.
Please do not file public issues for suspected vulnerabilities.

## Threat model

onsetter runs locally. It has no configuration beyond the `CLAUDE.md` files
already in your tree, and it sends nothing to a network address beyond
`localhost` — the one exception, `evokes:`, is covered below.

**What it reads.** The `CLAUDE.md` and `CLAUDE.local.md` files between an
edited file and its repository root, the pending tool call on stdin, and —
only when an ask sets `requires:` — whether a named executable exists. That
name comes from the `CLAUDE.md`, the same trust boundary as everything else
here: a bare name is resolved by searching `$PATH`, the same as a shell
about to run a command, but a name containing a slash is checked directly
and `$PATH` is not consulted at all, so `requires:` can also report whether
an arbitrary file on disk exists and is executable. Either way this is a
stat and a permission check — `exec.LookPath` — never an execution.

**What it sends.** Only when an ask sets `evokes:`, and only to Ollama on
`localhost:11434` — the edit's content, sent to get back an embedding vector,
never to any address that isn't loopback. Nothing about the request or the
response leaves the machine, and a failure of any kind (Ollama not running,
the request timing out) makes the `evokes:` ask not fire; it never blocks the
edit or surfaces an error to the model.

**What it writes.** One file per session under `~/.cache/onsetter/sessions/`,
holding rule-identity hashes and nothing else — no paths, no file contents, no
prompt text. `onsetter install` writes `~/.claude/settings.local.json` and
leaves a timestamped backup beside it. `onsetter warm` writes
`~/.cache/onsetter/embeddings.json`: the literal text of every `evokes:`
phrase in your `CLAUDE.md` files, plus its embedding vector — never edit
content, which is only ever held in memory for the one comparison it is used
for. Session files older than fourteen days are removed when a new session
starts; the embed cache has no such sweep and grows with the phrases you
write.

**What it emits.** The prose of matching rules, plus the substring the content
gate matched, capped at 80 characters, injected as advisory context. A
`CLAUDE.md` in a repository you did not write can therefore put text of its
choosing in front of the model at edit time — the same trust you already extend
to that file by opening the repository in Claude Code. onsetter cannot block a
tool call and never executes anything from a rule.
