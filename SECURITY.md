# Security

## Reporting a vulnerability

Email **justin@justinstimatze.com** with `onsetter-security` in the subject.
Please do not file public issues for suspected vulnerabilities.

## Threat model

onsetter runs locally, with no configuration beyond the `CLAUDE.md` files
already in your tree. It sends nothing to a network address beyond
`localhost` — the one exception, `evokes:`, is covered below.

The plugin install runs the hook as `onsetter serve`, a process Claude Code
keeps alive for the whole session rather than spawning fresh per call — it
holds every governing `CLAUDE.md`'s already-parsed rules in memory for as
long as the session lasts, instead of nothing surviving between calls. It
never reads anything beyond what the sections below already name, and stdio
is still the only channel in or out; a session boundary is what ends its
lifetime, same as `Write`/`Edit`/`Bash`/`Read` remain the same trust
boundary either way. The manual Go-binary install is unaffected — still one
process per call.

**What it reads.** The `CLAUDE.md` and `CLAUDE.local.md` files between an
edited file and its repository root, the pending tool call on stdin, and —
only when an ask sets `requires:` — whether a named executable exists. That
name comes from the `CLAUDE.md`, the same trust boundary as everything else
here: a bare name is resolved by searching `$PATH`, the same as a shell
about to run a command, but a name containing a slash is checked directly
and `$PATH` is not consulted at all, so `requires:` can also report whether
an arbitrary file on disk exists and is executable. Either way this is a
stat and a permission check — `exec.LookPath` — never an execution.

A `Bash` call is also read, by the touch-observer `onsetter install` wires
alongside the main hook: its `command` text, parsed with a real shell
grammar ([`mvdan.cc/sh`](https://github.com/mvdan/sh), the parser behind
`shfmt`) purely to find the paths it looks like it writes to, for
`untouched:`'s benefit. That parser only ever builds a syntax tree from the
text — it does not execute the command, look at its output, or know whether
it ran at all.

With `onsetter install --read`, a second, opt-in wiring, it also reads the
path of every file the model reads via `Read` — not the file's content,
only the path (an `on: read` ask's `has:`, if it has one, is what reads the
file, the same as it already does for a pending write). This is strictly
less than what a `Write`/`Edit` call already hands it, which includes the
pending content.

**What it sends.** Only when an ask sets `evokes:`, and only to Ollama on
`localhost:11434` — the edit's content, sent to get back an embedding vector,
never to any address that isn't loopback. Nothing about the request or the
response leaves the machine, and a failure of any kind (Ollama not running,
the request timing out) makes the `evokes:` ask not fire; it never blocks the
edit or surfaces an error to the model.

**What it writes.** Two files per session under `~/.cache/onsetter/sessions/`:
one holding rule-identity hashes and how many times each has fired, and a
`.paths` sidecar holding the absolute
path of every file `Write`, `Edit`, or the `Bash` touch-observer has seen
written this session — no file contents, no prompt text, no shell command
text, just the paths themselves, kept only so `untouched:` can answer "was
this file's counterpart touched." `onsetter install` writes
`~/.claude/settings.local.json` and
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

A rule can also name a second rule via `cues:`, which injects that second
rule's prose in the same block — with no substring at all, since a cued rule
never checks its own content gate. Same threat category and the same cap:
still only prose from a `CLAUDE.md` you already opened, never anything
executed, just a second path by which that prose reaches the model.
