---
name: onsetter
description: How to write, measure and wire an onsetter ask — prose in a fenced ask block inside an ordinary CLAUDE.md, injected in front of the Write or Edit that trips its gate. Read this before adding or changing an ask block, before picking a header (in, not-in, on, not, has, untouched, added, removed, when), and whenever a CLAUDE.md in the tree already contains ask fences. Covers where the block goes, the replay-before-wiring rule, the two things replay cannot measure, and why a wide gate now costs more than it used to.
---

# Writing an onsetter ask

An ask is a fenced block in an ordinary `CLAUDE.md`, opened with the exact
keyword `ask`: headers, one blank line, then prose, all inside the fence.
Onsetter runs as a `PreToolUse` hook on Write and Edit. When a block's gates
match the pending edit, its body is injected in front of that edit.

Nothing calls onsetter. It fires on the tool call whether or not anyone knows
it is there — which is the point, and also why this skill exists. The firing
side needs no discovery; the authoring side does.

## Get the header reference from the binary

```
onsetter headers
```

All nine headers with their haystacks, the gotchas, and a
what-you-want-to-catch table. It ships inside the binary, so it always
describes the version actually installed. Read it rather than working from
memory of this file — the trap that keeps catching people is `not:`, which is a
content regex and not a path exclusion, so `not: **/*_test.go` matches nothing
and the ask fires on the tests anyway. Path exclusion is `not-in:`.

## Where the block goes

The `CLAUDE.md` closest to the files the ask is about. Globs resolve against
that file's own directory, never the repo root, so an ask in `corpus/CLAUDE.md`
writes `chars/**` and not `corpus/chars/**`. Keeping the ask near its subject
is what lets `in:` stay short, and it means the ask moves when the content
moves.

One exception: asks in `<root>/.claude/CLAUDE.md` scope to `<root>`, because
Claude Code loads that file as the project's own.

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

An ask asks each question once per session, and a question is the ask plus the
text it quoted back.

So the two kinds of block get opposite treatment without either declaring
itself. No content gate means nothing to quote: it fires once per session
however many files it governs — a reminder, and repeating it is noise. A
`when:`, `has:`, `added:` or `removed:` keys on what it matched, so a new match
asks again and a repeat of the same match stays quiet.

The consequence when drafting: gate width is expensive. A pattern matching most
of a corpus used to cost one injection per session and now costs one per
distinct match. An ask whose own regex admits twenty alternatives has a ceiling
of twenty questions.

If a content-gated ask only ever fires once, its pattern is matching a property
of the file type rather than a signal that anything changed.

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
