# Changelog

## Unreleased

## v0.2.0 — 2026-08-06

First release with a changelog. `v0.1.0` was cut fifteen commits earlier, before
most of the header set existed, and has no entry of its own; everything below
covers the tool as it stands.

- `ask` fenced blocks in `CLAUDE.md`, `CLAUDE.local.md`, or
  `<dir>/.claude/CLAUDE.md`, with `in`, `when`, `not`, `on`. Asks from the
  `.claude/` form scope to the parent directory — scoping them to `.claude/`
  would make every glob match nothing while the file looked wired.
- The keyword was `miniprompt` first. "prompt" reads as a chunk of text to
  prepend, which misses the gate, and the gate is the point. No alias: every
  deployed block was converted first, verified by sweeping every `CLAUDE.md` on
  disk, and a test asserts nothing but ```ask opens a block.
- `onsetter hook` — one `PreToolUse` dispatcher on `Write|Edit`. Fails open,
  fires once per ask per session, coalesces matches into a single block.
- `onsetter install` — writes the settings entry itself; re-running converges.
- `onsetter list` / `replay` / `lint`.
- `not-in:` excludes a path that `in:` would have matched. `not:` is a content
  regex, so `not: _test\.go` silently matches nothing — that had one migrated
  ask firing on nearly twice the files the script it replaced did.
- `added:` and `removed:` gate on the lines an edit introduces or deletes, via
  a real Myers diff of `old_string` against `new_string` (gotextdiff, the
  implementation gopls uses). `removed:` is the only gate that can see a
  deletion — nothing about taking an error check out appears in the text being
  written. `has:` gates on the file as it stands rather than on the edit, and
  reads the file only when some ask asks for it.
- `untouched:` gates on a glob no file written this session matched — the
  paired-file class, "you changed the schema and never went near a migration".
  The session store keeps the written-path set beside the fired-ask set. It
  sees only Write and Edit, so a file changed by a shell command still reads as
  untouched.
- Header names are Semgrep's (`in`/`not-in`/`when`/`not` for
  `paths.include`/`paths.exclude`/`patterns`/`pattern-not`), Danger's
  (`added`/`removed` for `.added`/`.deleted`) and pre-commit's (`on` for
  `stages`). Only `has:` had no precedent.
- Repeated `when:` is an AND, repeated `not:` an OR. RE2 has no lookahead, and
  "touches a fact_text block AND contains inference language" is a real gate.
- `Match` returns a `Result` naming the gate that rejected, not a bare false.
  `list` marks the offending header with `←` and prints `not-in:`, which it had
  never printed at all; `replay` gives every 0% ask a funnel line, so `in:`
  reaching nothing and `when:` matching nothing in everything it reached stop
  looking identical. With nine headers, "would not fire" meant deleting one at
  a time to find out which.
- `onsetter lint` could not report the failure it exists for. `discover.Sources`
  dropped the parse error and skipped any file with no ask that parsed, so a
  `CLAUDE.md` whose only block had an unclosed regex printed
  `0 ask(s) in 0 file(s).` and exited 0. Every caller now goes through one
  `discover.ParseSource`, which also fixes `lint` parsing `.claude/CLAUDE.md`
  unscoped and naming `.claude` as the directory an ask governs.
- A block written fence-then-prose does not parse — the blank line before the
  body is required even with no headers. The error says so now instead of
  reporting that a sentence is not `key: value`.
- `onsetter headers` prints the full reference — every header with its
  haystack, its gotchas, and a what-you-want-to-catch table. Embedded in the
  binary, because the reader is usually an agent mid-task with no path to a
  docs directory. A test asserts every header in the parser has a section and a
  table row, and that the doc's own example block parses.
- The README opens and closes on a scene rather than an argument, and the
  headline example is now the corpus ask that asks how the player could know
  something. A Go example about recorded-but-unread state invites the reader to
  argue a vet check could do it; a character asserting something the player
  never learned forecloses that in a line.
- This repo has a `CLAUDE.md` of its own, which it did not, while telling
  readers to write one. Two asks, both measured at one file in ten before being
  wired: the fail-open invariant on everything reachable from `onsetter hook`,
  and the three-places-must-agree coupling when a header is added. The README's
  replay walkthrough runs against them, so its numbers come from asks that
  actually ship.
