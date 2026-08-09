# stull and winze-agent: two integration questions

Two ways onsetter's ask mechanism could extend past its own repo: pointing
"always"/"never" language in a `CLAUDE.md` at a stull hook instead of leaving
it as prose, and gating winze-agent's `winze_remember` the same way
`Write`/`Edit` are gated today.

An earlier draft of this idea proposed routing onsetter's questions through
an LLM classification call, using stull's `Cell`. That's wrong for this
codebase: `onsetter hook` has no model call anywhere. Every ask is a
deterministic regex match against `content`/`new_string`, governed by
`E-INJECT`-style discipline (fail open, exit 0, prepend static prose) — an
invariant `cmd/onsetter/hook.go`'s own ask block already polices. A model
call inside the hook would break it.

## 1. "always"/"never" in a `CLAUDE.md` → point at stull instead of prose

Needs zero onsetter code. One ask block:

````
```ask
in: CLAUDE.md
when: (?i)\b(always|never)\b

This reads like an enforceable rule, not a description. If it names a tool
call whose path and content could carry it, a hook fires on every matching
call — this line only fires when the file happens to be in context. See
stull (github.com/justinstimatze/stull)'s "Adding a machine" recipe. If this
is judgment, attitude, or something no guard could check, continue.
```
````

`discover.Roots` stops climbing at the first `.git` it finds
(`internal/discover/discover.go:43-45`) — "stopping at the repo root is what
makes `in:` default to everything below this file, instead of everything on
the disk," per that file's own comment. A project `CLAUDE.md` is almost
always inside a git repo, so the walk never reaches `$HOME`: an ask sitting
only in `~/.claude/CLAUDE.md` never fires for a write to
`~/Documents/<project>/CLAUDE.md`. The block above has to be pasted into each
project's own `CLAUDE.md` to take effect there — the same thing
`~/.claude/CLAUDE.md`'s "Contextual miniprompts" section already does by
hand (author a project-scoped miniprompt whenever a feedback memory carries a
tool-call surface), turned into something copy-pasteable instead of
freeform.

A second ask source that survives the `.git` boundary would let this fire
everywhere without per-repo pasting, but it cuts directly against `Roots`'
own stated design principle that an ask's blast radius is the directory it
lives in (`internal/discover/discover.go:24-27`). Don't build it: an author
who writes `in: CLAUDE.md` is reasoning about one repo, not every project on
the machine, and a global ask firing from `~/.claude/CLAUDE.md` would
silently apply that judgment to repos they weren't looking at when they
wrote it. Per-repo paste keeps the blast radius legible; that cost is the
point, not a gap.

## 2. Gating `winze_remember`, not just `Write`/`Edit`

`onsetter hook` only fires on `Write` and `Edit` — the README says so
directly ("reads every `CLAUDE.md` above the file being written"), and every
ask header (`in`, `on`, `added`, `removed`...) assumes a file path and a
`content`/`new_string` span. `winze_remember(note, role?, title?, force?)`
(`winze/docs/agent.md:34`) is an MCP tool call: no path, and the text lands
in `note`, not `content`. onsetter's matcher has nothing to point at it.

Widening `onsetter hook`'s `PreToolUse` matcher to know about arbitrary MCP
tools and their argument shapes isn't the right shape — it would mean
special-casing `winze_remember`'s `note` field today and every other MCP
tool's own field name later, one at a time, forever. `winze-agent` already
runs its own `PreToolUse` surface (`winze-agent capture-guard`, per
`winze/docs/agent.md:24`) sitting directly in front of its own tool calls —
that's the natural place to apply the same ask-block matching, not a reason
to teach onsetter about winze specifically.

The blocker was that `internal/ask` was importable only from inside
`github.com/justinstimatze/onsetter` — `winze-agent` is a separate module.
`Ask`, `Edit`, `Result`, `Match`, and `Parse*` were already capital-exported,
so the fix was mechanical: a directory move (`internal/ask` → `ask`) and
five import-line edits. It's exported now, at `github.com/justinstimatze/onsetter/ask`.

Two things were worth being precise about before `winze-agent` calls it.

`capture_guard.go:31-68` inspects neither `note` nor any write content
today — it reads `tool_name` and `tool_input.file_path`, checks the path
against `isNativeMemoryPath` (contains `/.claude/projects/` and `/memory/`),
and blocks only if `storeRootConfigured()` also holds. No content field is
parsed anywhere in the file, so calling `ask.Match` against `note` is new
gating logic in `winze-agent`, not a widened check on something already
there.

`ask.Match(e Edit) Result` resolved `filepath.Rel(r.Dir, e.Path)` against
`In`/`NotIn` as its first check, unconditionally, before `when:`/`not:`/
`has:` ever ran — and `discover.Roots(path)` climbs from `filepath.Dir(path)`
to find the governing `CLAUDE.md`s in the first place. `winze_remember(note,
...)` has no path for either call. `discover.Roots` still needs one — a
`winze-agent` caller has to pick which `CLAUDE.md` governs a note-only call
(the store's own, or its working directory's) rather than discovering it.
`ask.Match` no longer does: it now accepts `Edit.Path == ""` directly. If the
ask sets `In` beyond its default (`**`), `NotIn`, or `Untouched`, it rejects
with a `Result` that says the ask needs a path and this call had none;
otherwise `when:`/`not:`/`has:` run exactly as they do on a real file, since
none of the three ever touched a path. `on: mint`/`on: edit` were already
path-independent — they read `Edit.Exists` directly, which a `winze-agent`
caller can set from its own notion of new-vs-revised memory.

## Where stull sits in both

Not in the interception path either way. stull is the compile target for
whatever an ask surfaces as mechanical — the thing reached for after
deciding a rule is a guard, never the thing doing the deciding. No third
project needed for either integration: (1) is content for a `CLAUDE.md`, (2)
is an export plus a second caller inside two Go modules that already exist.
