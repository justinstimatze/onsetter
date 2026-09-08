---
name: onsetter
description: How to write, measure and wire an onsetter ask — prose in a fenced ask block inside an ordinary CLAUDE.md, injected in front of the Write or Edit that trips its gate. Read this before adding or changing an ask block, before picking a header (requires, in, not-in, on, not, has, untouched, added, removed, when, evokes, revisit, always, name, cues), and whenever a CLAUDE.md in the tree already contains ask fences. Covers where the block goes, the replay-before-wiring rule, why a wide gate now costs more than it used to, what replay cannot measure, and how one ask can cue another by name.
---

# Writing an onsetter ask

An ask is a fenced block in an ordinary `CLAUDE.md`, opened with the exact
keyword `ask`: headers, one blank line, then prose, all inside the fence.
Onsetter runs as a `PreToolUse` hook on Write and Edit. When a block's gates
match the pending edit, its body is injected in front of that edit.

Nothing calls onsetter directly — it fires on the tool call regardless of
whether anyone knows it's there, which is the point, and also why this skill
exists. The firing
side needs no discovery; the authoring side does.

## Get the header reference from the binary

```
onsetter headers
```

All fifteen headers with their haystacks, the gotchas, and a
what-you-want-to-catch table. It ships inside the binary, so it always
describes the version actually installed. Read it rather than working from
memory of this file — the trap that keeps catching people is `not:`, which is a
content regex and not a path exclusion, so `not: **/*_test.go` matches nothing
and the ask fires on the tests anyway. Path exclusion is `not-in:`.

Three of the fifteen, `name:`, `cues:` and `always:`, aren't gates at all:
`cues:` lets a fired ask also inject a second ask's prose by name, without
checking that second ask's own gate; `always:` skips the once-per-session
suppression entirely, so an ask whose corpus is suspect by construction can
fire on every matching edit rather than stopping after the first.
`onsetter headers` covers the semantics, the loop-safety, and the real
gotchas (a cue can only reach an ask its own `CLAUDE.md` chain would already
see; `always:` on an unnarrowed ask is the loudest injection this format can
produce).

## Where the block goes

The `CLAUDE.md` closest to the files the ask is about. Globs resolve against
that file's own directory, never the repo root, so an ask in `corpus/CLAUDE.md`
writes `chars/**` and not `corpus/chars/**`. Keeping the ask near its subject
is what lets `in:` stay short, and it means the ask moves when the content
moves.

One exception: asks in `<root>/.claude/CLAUDE.md` scope to `<root>`, because
Claude Code loads that file as the project's own. That exception needs a real
project root: an ask in the global `~/.claude/CLAUDE.md` scopes only to
`~/.claude/` itself — there's no project root above `$HOME` for its globs to
be relative to.

Never add an ask to a checkout you do not own.

## The loop, in order

1. Draft the block.
2. `onsetter lint` — a block that fails to parse is worse than no block,
   because the author believes it is watching.
3. `onsetter replay '<glob>'` against the files it claims to govern.
4. Narrow, and run replay again. Every first draft over-fires.

A gate tripping on more than a few percent of what it reaches is a tax on every
edit it touches. `onsetter list <path>` does the same for a single file and
names the gate that turned it away.

An `evokes:` ask has a fifth step: `onsetter warm` before it will ever fire.
Unlike every other header, it needs a local cache built ahead of time — a
phrase added since the last `warm` silently never fires, so re-run it after
drafting or editing one.

It also has no ground truth of its own the way a regex does, so `replay`'s
rate cannot tell you whether the threshold is right. `onsetter calib <ask>
<fires-glob> <not-glob>` is the equivalent for this one header: point it at
labeled positive and negative examples and it reports where they actually
land — including whether they overlap, in which case the fix is rewording
the phrases rather than picking a different number.

## What replay cannot tell you

**It measures cost, never catch.** The corpus has already been cleaned of the
defect the ask looks for, so a healthy ask and a dead one both read 0%. When a
row is 0%, read the funnel line to see which gate rejected: `in: ×38 · when: ×2`
means the glob reached two files and the regex matched neither, which is a
different bug from `in: ×40` alone.

**It cannot see `added:` or `removed:` at all.** Replay builds synthetic mints
from files on disk, so there is no old text: `added:` degrades to `when:` and
`removed:` never fires. A rate printed for one of those is silently about the
ask's *other* gates. Replay marks such rows. The honest check is to pipe two
JSON payloads through `onsetter hook` with an old/new pair.

A rate that agrees with whatever the ask replaced proves the migration was
faithful. It says nothing about whether the gate is good.

## Lifetime, and why width costs

The two kinds of block get opposite treatment without either declaring
itself. No content gate means nothing to quote: it fires once per session
however many files it governs, then goes quiet — a reminder, and a second
firing would be the literal same sentence. A `when:`, `added:`, `removed:`
or `evokes:` block is different: it always fires, on every occurrence, for
the whole session — marked `(asked N× already this session)` once a given
occurrence (the quote plus the edit around it, not the quote alone) recurs.
`has:` gates on the file as it stands but never contributes a quote, so a
`has:`-only block is a reminder too, same as a `cues:`-reached firing, which
never checks its own gate and so never has anything to quote either.

This is no suppression at all for the matched case, replaced by a count,
rather than a tightened once-per-session default. The old key hashed the quote
alone, so two unrelated occurrences sharing a short match silently
collapsed into one already-answered question; the count exists so a repeat
gets triaged by whoever's reading it, not hidden by a session file that
can't tell a genuine repeat from a different occurrence that looks similar.

The consequence when drafting: gate width is still expensive, more than
before. A pattern matching most of a corpus now costs one injection per
occurrence, with no once-per-session ceiling to cap it — the old "twenty
alternatives, twenty questions" ceiling no longer holds for a matched ask,
since the same string recurring in different edits counts as a fresh
occurrence each time, rather than one capped question.

`always: true` opts a matched ask out of the count — every firing looks
identical, for a near-check corpus where that's already expected. `revisit:`
is retired: its whole job was the widening that's unconditional now.
See `onsetter headers` for the full mechanics and cost.

## Writing the prose

The body is a question, not a rule. It is read by an agent mid-edit, and being
wrong costs one sentence — which is the whole reason a gate this loose is worth
having.

- Say what about the matched text is suspect. The injection already quotes it.
- End with the out: "…unless it is genuinely X, in which case continue."
- Say what to do, not only what is wrong.
- Do not write a linter. If the check is precise enough to block on, it belongs
  in CI, and someone will ask why this is not one.

Identity is a hash of the gate and the body, so inserting a paragraph above an
ask changes nothing and editing its prose re-arms it. To retire one, delete the
block.
