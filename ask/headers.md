# Writing an ask

An ask is a fenced block in an ordinary `CLAUDE.md`. Headers, one blank line,
then the prose — all inside the fence.

```ask
in: corpus/{locations,chars}/**
when: you (nod|realize|decide|turn away)

The narrator describes the world; the player decides what they do and what it
means. Rewrite to observable world-state — unless the match is genuinely
sensory, in which case continue.
```

The body is injected verbatim in front of the Write or Edit that trips the
gate.

Three rules the format will not forgive:

- The fence keyword is exactly `ask`, lowercase. ` ```asks `, ` ```Ask ` and
  ` ```miniprompt ` are ordinary markdown and are skipped in silence — that is
  what makes the fence an opt-in rather than a guess about prose.
- One blank line separates the headers from the body, **including when there
  are no headers**. Fence, then prose, with nothing between them, reads the
  first sentence as a header and fails to parse.
- Header values are trimmed. `when: foo ` is the pattern `foo`; to match a
  trailing space write `\s` or `[ ]`.

Every header is optional. An ask with no headers fires on every edit below its
`CLAUDE.md`, which `onsetter lint` reports as a banner. Run `onsetter lint`
after writing one: a block that fails to parse is worse than no block, because
the author believes it is watching.


## Where the block goes

Onsetter reads `CLAUDE.md`, `CLAUDE.local.md` and `<dir>/.claude/CLAUDE.md` in
every directory from the file being edited up to the one holding `.git`. Asks
from all of them apply, outermost first, so a repo-wide ask and a
directory-local one both fire.

Put the ask in the `CLAUDE.md` closest to the files it is about. That is what
lets `in:` stay short, and it means the ask moves when the content does.

**The one exception to glob scoping:** asks in `<root>/.claude/CLAUDE.md` scope
to `<root>`, not to `.claude/`. Claude Code loads that file as the project's
own, and scoping its globs to `.claude/` would make every one of them match
nothing while the file looked wired. So `in: internal/**/*.go` there means what
it looks like it means.

That exception is scoped to a real project root, not to `$HOME/.claude/`. An
ask in the global `~/.claude/CLAUDE.md` scopes to `~/.claude/` itself — the
same directory-name collision doesn't apply, because `$HOME` isn't a project
root for anything to be relative to. `in: projects/**/memory/feedback_*.md`
there means `~/.claude/projects/**/memory/feedback_*.md`; without the fix,
`in: feedback_*.md` alone would resolve against `$HOME` and never reach a real
memory file.


## The fourteen headers

Listed in the order `Match` applies them, which is the order `onsetter replay`
reports a funnel in — except the last three. `revisit`, `name` and `cues` are
never a reason a pending edit gets turned away; none of the three appears in
that funnel at all.

| Header      | Matches                                   | Repeat means |
|-------------|-------------------------------------------|--------------|
| `requires`  | a binary resolving on `$PATH`             | AND          |
| `in`        | the path, as a glob                       | last wins    |
| `not-in`    | the path, as a glob — excludes            | OR           |
| `on`        | one of: any, mint, edit                   | last wins    |
| `not`       | the incoming text — suppresses            | OR           |
| `has`       | the file as it stands on disk             | AND          |
| `untouched` | paths written this session — suppresses   | AND          |
| `added`     | the lines this edit introduces            | AND          |
| `removed`   | the lines this edit deletes               | AND          |
| `when`      | the incoming text                         | AND          |
| `evokes`    | the incoming text, fuzzily — not a regex  | OR           |
| `revisit`   | nothing — widens the session key instead  | last wins    |
| `name`      | nothing — gives another ask something to cue | last wins |
| `cues`      | nothing — fires a second ask by name      | cue each     |

Globs are [doublestar](https://github.com/bmatcuk/doublestar) and resolve
against the directory of the `CLAUDE.md` the ask lives in — never the repo root
and never the working directory, with the `.claude/` exception noted above.
Braces and `**` both work: `{locations,chars}/**/*.yaml`.

Patterns are Go RE2. **There is no lookahead and no backreference.** To require
two things, repeat the header — `when: fact_text` plus `when: you realize` is
the conjunction, and there is no way to write it as one pattern.


### `requires:` — a companion tool

```
requires: stull
```

Fires only when the named binary resolves on `$PATH` — a fact about the
machine running onsetter, not about the file or the edit. Checked first,
before any path or content gate, so a rejection on a machine without the tool
names the real reason instead of a misleading glob or regex mismatch.

Nothing is ever executed — this is `exec.LookPath`, a stat and a permission
check. A bare name searches `$PATH`; a name containing a slash is checked
directly and `$PATH` is not consulted. Repeat for an AND: `requires: stull`
plus `requires: git` means both. The same ask reports a different rate on a
teammate's machine that does not have the tool; that is the tradeoff for
gating on something outside the repo.

Two things worth knowing before this rejects and you cannot see why:

- **It resolves against the `$PATH` onsetter's own process sees, which is not
  always your shell's.** A tool installed via a GUI app, a version manager
  (`nvm`, `rbenv`, `goenv`), or into `$GOPATH/bin` can be on the `$PATH` your
  terminal shows you and absent from the narrower one a non-interactive or
  GUI-launched process inherits. If `onsetter list` says a tool is missing
  and `which <tool>` in your terminal disagrees, that gap is the first thing
  to check, not a broken installation.
- **It is checked before every other gate, so a rejection here can hide a
  second, unrelated one.** Installing the tool and still not firing means a
  different header turned it away; re-run `onsetter list` rather than
  assuming the fix landed.

### `in:` — which files

```
in: internal/store/**
```

Default is `**`: everything below this `CLAUDE.md`, recursively. The path is
made relative to the `CLAUDE.md`'s directory before matching, so an ask in
`corpus/CLAUDE.md` writes `chars/**`, not `corpus/chars/**`.

### `not-in:` — which files not

```
in: internal/**
not-in: **/*_test.go
```

Use this, never `not:`, to exclude a path. `not:` is a content regex, so
`not: _test\.go` matches nothing and the ask fires on the tests anyway.

### `on:` — new file or existing file

```
on: mint
```

`mint` fires only when the file does not exist yet; `edit` only when it does;
`any` is the default. Mint-only is the single most effective narrowing
available: one ask that fired on every file in its corpus on content alone
fired on 2.8% of them once it also required the file to be new.

### `not:` — call it off

```
when: TODO
not: TODO\(\w+\)
```

If any `not:` matches the incoming text, the ask stays quiet. This is the
escape hatch for the case the author already thought about.

### `has:` — the file already does something

```
has: sync\.Mutex
when: sync\.RWMutex
```

`has:` reads the file **as it stands on disk**, before this edit. Everything
else here looks at the edit. That difference is the whole point: it expresses
"this file already does X and you are adding a second way to do it", which
nothing that sees only the pending write can say.

### `untouched:` — the paired file

```
in: internal/store/schema.go
untouched: migrations/**
```

Fires when you change one file and have not been near its partner this
session. Globs resolve like `in:`.

Its knowledge is only what onsetter saw. A file rewritten by a shell command
never reaches a `PreToolUse` hook on Write or Edit, so it still counts as
untouched and the ask fires anyway. Dismissible in a sentence, but real.

### `added:` and `removed:` — the diff

```
removed: if err != nil
```

These run a real Myers diff of `old_string` against `new_string` and match only
the inserted or deleted lines. Replacing a span that already contained the
pattern therefore does not read as adding it.

`removed:` is the class nothing else here can see: an edit that takes something
out leaves no trace in the text being written.

Two shapes to know. A Write has no old text, so `added:` sees the whole file and
`removed:` can never fire.

And `onsetter list` and `onsetter replay` construct synthetic edits from files on
disk, so `added:` degrades to `when:` and `removed:` reports nothing in either of
them. A rate printed for one of these gates is therefore about the ask's *other*
gates and says nothing about the one you care about — `replay` marks any such row
rather than leaving the number to be read as if it meant something. The honest
check is to drive `onsetter hook` with an old/new pair.

### `when:` — the incoming text

```
when: \bpanic\(
```

The catch-all content gate. **On an Edit this is `new_string` — the replacement
span, not the whole file.** An ask that needs to see the rest of the file wants
`has:`, not `when:`.

### `evokes:` — a fuzzy trigger phrase

```
evokes: committing without asking first
evokes: pushing straight to main
```

Not a regex. Fires when the edit's content is a semantic match for *any* one
of these phrases, even without sharing a word with it — the opposite of
`when:`'s exact AND, because these are independent conceptual cues rather
than conditions that must all hold at once. Checked last, after every other
gate: it is the fuzziest header here, so only an edit every crisp glob and
regex already let through pays for it.

This needs setup the other headers don't. `onsetter warm` has to run first,
embedding every `evokes:` phrase under a directory into a local cache —
`onsetter hook` never fills a cache miss itself, so a phrase added since the
last `warm` silently never fires, the same way a `requires:` binary that
is not installed silently never fires. Run `onsetter warm` again after
adding or editing an `evokes:` line. It also needs Ollama running locally
with the embedding model pulled; either one being unavailable degrades to
"this ask does not fire," never an error.

The similarity threshold has real measurement behind it but is still a first
draft, not a calibrated one — one machine, one model, a handful of data
points, and there is no equivalent yet of the file-by-file rate `onsetter
replay` gives every other header. Treat a freshly written `evokes:` ask the
same as any other first draft: replay it before trusting the rate.

**It has almost no signal on a whole code file, only on prose.** A sentence
that clearly evokes a phrase scores well above an unrelated one when matched
in isolation — but embed the whole file it lives in, syntax and all, and the
score drops to barely above what an unrelated file scores. The surrounding
code dilutes the match almost to noise. `evokes:` is for a topic or a shape
of reasoning in comments, commit messages, or prose files; the regex headers
above it already own code-shaped triggers precisely, and that division is
not a style preference — it is where this header's signal actually lives.

### `revisit:` — do not let a stale dismissal cover a new state

```
when: TODO
revisit: true
```

Every other content-gated ask keys its once-per-session firing on the text it
quoted back, described in full under **Lifetime** below. `revisit: true`
widens that key to the quote plus the whole edit that produced it, so a later
edit that reintroduces the identical string is a new question rather than one
already answered — see **Lifetime** for what this changes and what it costs.

Not a gate: it cannot make an ask fire or turn one away, so it never appears
in a `replay` funnel. And it needs a quote to widen: pairing it with an ask
that has nothing to quote back does nothing at all. That is `added:`,
`removed:`, `when:`, or `evokes:` — not `has:`, which gates on the file as it
stands but, like a path-only ask, never contributes to what gets quoted.


### `name:` — a handle other asks can cue

```
name: check-token-scope
```

Gives this ask a stable, human-chosen slug another ask's `cues:` can point
at. Not a gate — Match never reads it, and `onsetter replay` never reports
a rate for it.

Not `ID()`. `ID()` is a content hash that changes every time this ask's
prose or gates are edited, and is never printed anywhere — `onsetter list`
and `onsetter replay` show `Where()`, not `ID()`. Citing it directly from
another ask would silently break on the next wording tweak. `name:` is the
address that survives that.

Must be unique across the whole tree; `onsetter lint` and `onsetter status`
both fail loudly on a collision, because a silent second `name:` means
whichever cue meant the first ask now reaches the second one instead.

### `cues:` — fire a second ask by name

```
when: fetch\(.*credentials
cues: check-token-scope
```

When this ask fires, it also injects the prose of the ask (or asks —
repeat the header to cue more than one) named by `cues:`, in the same
block, **without checking that ask's own gate at all**. That is the entire
point: it fires because it was cued, not because its own `in:`/`when:`/etc.
also matched this edit. Every other gate on the cued ask is bypassed except
`requires:`, which still has to resolve — a cued ask naming an uninstalled
tool still shouldn't inject.

A cued firing never quotes anything, even if the target ask has its own
`when:` or similar — there was never a match to quote. That makes a cued
firing behave like any other no-content-gate ask for session purposes: it
fires once per session, full stop, and `revisit:` on the target does
nothing for a firing reached this way.

**A cue can only ever reach an ask that this file's own `CLAUDE.md` chain
could already see.** Concretely: the target's `name:` must be declared in a
`CLAUDE.md` that is an ancestor of (or the same file as) the citing ask's
own `CLAUDE.md` — never in one nested below it, unless the citing ask's own
`in:` is scoped to that same subtree. `onsetter lint` flags the common
mistake shape (a target declared below the citer) as a heuristic, and
`onsetter replay` will show zero cued fires in a corpus where the author
expected otherwise if the scope is wrong in a way the heuristic missed.

Loop-safe by construction: a cascade visits each ask at most once per edit,
so a cycle (A cues B, B cues A) or a diamond (A and B both cue C) both fire
every ask exactly once rather than looping or double-injecting.

The idiom for prose that should only ever fire by being cued, never on its
own: `not-in: **`. `in:` defaults to matching everything, and a `not-in:
**` unconditionally excludes every path, so the ask can never pass its own
gate — only a `cues:` from elsewhere can ever reach it.

## Choosing a gate

| You want to catch                          | Reach for                |
|--------------------------------------------|--------------------------|
| a phrase appearing anywhere in a write      | `when:`                  |
| a phrase being *introduced*                 | `added:`                 |
| a safety check being *taken out*            | `removed:`               |
| a second way to do something already there  | `has:` + `when:`         |
| a file created without its counterpart      | `untouched:`             |
| a convention that only applies to new files | `on: mint`               |
| anything at all under one directory         | `in:` alone (no content) |
| an ask that only makes sense with a companion tool installed | `requires:` |
| a topic or a shape of reasoning, not a fixed string | `evokes:`         |
| one ask's prose should also pull in a second one    | `cues:` (with `name:` on the target) |

If a gate would need lookahead, split it across two `when:` lines. If it would
need to exclude a directory, that is `not-in:`, not `not:`.


## Writing the prose

The body is a question, not a rule. It is read by an agent mid-edit, and being
wrong costs one sentence — which is the entire reason a gate this loose is
worth having. So:

- **Quote what you are asking about.** The injection already quotes the text the
  gate matched; the prose should say what about it is suspect.
- **End with the out.** "…unless it is genuinely X, in which case continue." A
  wrong ask should cost one sentence, not an investigation.
- **Say what to do, not just what is wrong.** "Rewrite to observable
  world-state" beats "this is second person".
- **Do not write a linter.** If the check is precise enough to block on, it
  belongs in CI, and someone will ask why this is not one.


## Before you wire it

```
onsetter replay 'corpus/**/*.md'
```

Every first draft over-fires. One draft fired on every file in its corpus
because it gated on the file carrying a citation, which is what every file in
that corpus is. A gate tripping on more than a few percent of what it matches
is a tax on every edit it touches.

Replay cannot measure what a gate *catches* — the corpus has been cleaned of
exactly the defect the ask looks for, so a healthy ask and a dead one both read
as 0%. When one reads 0%, the funnel line says which gate turned the files away:
`in: corpus/** ×38 · when: \bTODO\b ×2` means the glob reached two files and the
regex matched neither. A lone `in: ×40` means the glob reached nothing, which is
a different bug.

`onsetter list <path>` does the same for one file and names the gate that
rejected it.


## Lifetime

An ask asks each question once per session, and a question is the ask plus the
text it quoted back.

That means the two kinds of block get opposite treatment without either one
declaring itself. A block with no content gate quotes nothing, so it fires once
per session however many files it governs — it is a reminder, and repeating it
is noise. A block with a `when:`, `added:`, `removed:` or `evokes:` keys on
what it matched, so a new match asks again and a repeat of the same match
stays quiet. `has:` gates on the file as it stands but never contributes a
quote, so a `has:`-only block is a reminder too. Its ceiling is set by its own
pattern: an ask can fire at most once per
distinct string its regex can match, so twenty alternatives means at most
twenty questions.

If a content-gated ask only ever fires once, its pattern is matching a property
of the file type rather than a signal that something changed. That is worth
knowing — see the funnel note under **Before you wire it**.

That default is right for most asks: the quote is the question, and a repeat
of the same quote has already been answered. It is wrong for one still true —
an unresolved TODO an ask flagged once, still sitting there five edits later
to the same file, reads as already-answered because the string never changed,
even though the agent has had five more chances to deal with it and has not.
`revisit: true` says this ask's question is not "have you seen this exact
string" but "is this still here", and widens its key from the quote alone to
the quote plus the edit that produced it — so touching the file again with
the same violation still present asks again, and only a genuine no-op re-edit
(byte-identical to one already asked about) stays quiet.

The idea is [treadiehq/codecut](https://github.com/treadiehq/codecut)'s: its
`verification-evidence` rule keys a passing test to the diff fingerprint it
actually ran against, so a stale pass from before the last edit does not
count. Onsetter never sees a test run — only the `Write`/`Edit` it already
watches — so this is that idea's narrower shadow: fingerprinting the edit
rather than a verification event, because an edit is the only state onsetter
has.

Turning it on changes every deployed ask's identity, `revisit: true` or not:
`ID()` now folds the header in, so this release re-arms every ask once per
in-flight session, the one-time cost the v0.3.0 key-format change also paid.

`cues:` paid that same one-time cost when it shipped, for the same reason:
`ID()` folds the `cues:` list in (so a freshly wired cue gets a chance to
walk a session where the citer already fired once), but leaves `name:` out
of the hash entirely — renaming an ask to fix a collision, or just to make
its cue wiring read better, should not re-arm every already-answered
session instance of it. A cued firing's own key never carries `revisit:`
either way: it never quotes anything, so there is nothing for `revisit:` to
widen.

Identity is a hash of the gate and the body, not the line number, so inserting
a paragraph above an ask changes nothing and editing its prose re-arms it. To
retire one, delete the block.
