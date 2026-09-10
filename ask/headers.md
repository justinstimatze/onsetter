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


## The eighteen headers

Listed in the order `Match` applies them, which is the order `onsetter replay`
reports a funnel in — except the last seven. `revisit`, `always`, `block`,
`name`, `cues`, `fires-on` and `silent-on` are never a reason a pending edit
gets turned away; none of the seven appears in that funnel at all.

| Header       | Matches                                   | Repeat means |
|--------------|--------------------------------------------|--------------|
| `requires`   | a binary resolving on `$PATH`             | AND          |
| `in`         | the path, as a glob                       | last wins    |
| `not-in`     | the path, as a glob — excludes            | OR           |
| `on`         | one of: any, mint, edit                   | last wins    |
| `not`        | the incoming text — suppresses            | OR           |
| `has`        | the file as it stands on disk             | AND          |
| `untouched`  | paths written this session — suppresses   | AND          |
| `added`      | the lines this edit introduces            | AND          |
| `removed`    | the lines this edit deletes               | AND          |
| `when`       | the incoming text                         | AND          |
| `evokes`     | the incoming text, fuzzily — not a regex  | OR           |
| `revisit`    | nothing — widens the session key instead  | last wins    |
| `always`     | nothing — skips the session key entirely  | last wins    |
| `block`      | nothing — denies the write instead of only informing about it | last wins |
| `name`       | nothing — gives another ask something to cue | last wins |
| `cues`       | nothing — fires a second ask by name      | cue each     |
| `fires-on`   | nothing — `lint` asserts the glob's files fire | AND     |
| `silent-on`  | nothing — `lint` asserts the glob's files stay silent | AND |

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

### `on:` — new file, existing file, or a Read

```
on: mint
```

`mint` fires only when the file does not exist yet; `edit` only when it does;
`any` is the default. Mint-only is the single most effective narrowing
available: one ask that fired on every file in its corpus on content alone
fired on 2.8% of them once it also required the file to be new.

`any`, `mint`, and `edit` all mean "a pending `Write` or `Edit`." `read` is
the fourth value, and it means the opposite: a `Read` tool call, never a
pending write. No ask matches both kinds — an `on: read` ask never fires on
a `Write` or `Edit`, and the other three never fire on a `Read`.

```
in: corpus/**
on: read

Never read the corpus directly before generating — query the index instead.
```

Two guardrails specific to this value:

- **A `Read` call has no incoming text.** `when:`, `added:`, `removed:`,
  `not:`, and `evokes:` all match against text a pending write carries — a
  `Read` never has any, so combining `on: read` with any of them can never
  fire. `onsetter lint` rejects the combination and names which gate is
  dead. `has:` still works: it reads the file as it stands on disk, which a
  `Read` call has just as much as a write does.
- **`on: read` support has to be wired separately, and it isn't by
  default.** `onsetter install` wires `Write`, `Edit`, and `Bash` — not
  `Read` — because a `Read` happens far more often than a write in a typical
  session, and unlike the other two, every `Read` pays the full `CLAUDE.md`
  discovery walk. `onsetter install --read` wires it; `onsetter status`
  reports plainly when an `on: read` ask exists with no `Read` wiring to
  ever reach it, the same way it already catches an unwarmed `evokes:`
  phrase or a missing `requires:` binary.

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

Its knowledge is only what onsetter saw. A companion `PreToolUse` hook on
`Bash`, wired by `onsetter install` alongside the main one, narrows this:
it parses the command's actual shell syntax and marks a path touched when
the command clearly writes to it — an output redirect (`>`, `>>`, `&>`,
`&>>`), `tee`'s target, an in-place `sed -i` or `sd` (which rewrites in
place by default), or `cp`/`mv`'s destination. It only trusts an argument
that resolves to a plain literal at parse time — one built from a variable
or command substitution contributes nothing, on purpose: a wrong guess here
would wrongly suppress a real `untouched:` ask, which is worse than the miss
it would replace. It never runs the command; it only reads its text.

This narrows the gap, not closes it. A program invoked *by* the Bash call —
`python generate.py`, `make`, a custom tool — writing files through its own
logic stays invisible, the same as it always was. Dismissible in a sentence,
but real.

### `added:` and `removed:` — the diff

```
removed: if err != nil
```

These run a real Myers diff of `old_string` against `new_string` and match only
the inserted or deleted lines. Replacing a span that already contained the
pattern therefore does not read as adding it.

`removed:` is the class nothing else here can see: an edit that takes something
out leaves no trace in the text being written.

Three shapes to know. A Write has no old text, so `added:` sees the whole file and
`removed:` can never fire.

The matched lines are joined into one string before the pattern runs against
them, and that string is compiled with `(?m)`, so `^` and `$` anchor to each
line rather than to the start and end of the whole join — `added: ^\s*[-*]`
means "a line starting with a bullet," not "the first added line, whatever it
is." Writing `(?m)` yourself is harmless and redundant, never an error.

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

### `revisit:` — retired

```
when: TODO
revisit: true
```

`revisit: true` used to widen a matched ask's session key from the quote
alone to the quote plus the whole edit that produced it, so a different
occurrence sharing the same short quote wouldn't silently collapse into an
already-answered question. That widening is what every matched ask does
now, unconditionally — see **Lifetime** below. `revisit:` has nothing left
to widen.

The header still parses, so an existing `revisit: true` block does not
suddenly fail to load — it just stops doing anything. `onsetter lint` flags
it as redundant and names what replaced it; there's no reason to leave it
in a block once lint says so, but nothing breaks if you do.

### `always:` — skip the repeat count

```
in: corpus/**
when: (?i)\bnever\b
always: true
```

Every matched ask fires on every occurrence now and marks a repeat with a
running count — see **Lifetime**. `always: true` opts a specific ask out of
that count: it never checks the session store and never gets a marker, so
every firing looks identical regardless of how many times it's already
asked. For the *near-check* class this was built for — a corpus where every
matching edit is suspect by construction, "never do X" over a directory
where X is always worth a second look — that's usually what you want: the
count is real information most of the time, and pure noise for an ask that
expects to fire on nearly everything anyway.

The injected line marks it, the same way a `cues:`-reached ask says `cued
by`: `matched "never" · always`, replacing what would otherwise be an
`(asked N× already this session)` count.

On a no-content-gate ask (a reminder — nothing quoted, see **Lifetime**),
`always:` means what it always has: skip the once-per-session suppression
entirely and fire on every matching edit, not just the first. There's no
count to skip there, since a reminder never gets one.

Not a gate — Match never reads it, and `onsetter replay` never reports a
rate for it.

### `block:` — deny the write, not just inform about it

```
added: \bprint\(
block: true
```

`PreToolUse` fires after the model has already committed to a tool call's
exact arguments — every other ask here can only inform a *future* call, never
correct the one that tripped it. A stray `print(` written and gated in the
same `Write` shows up in `additionalContext` for the next edit, never for
this one. `block: true` closes that gap: a matched firing sets
`permissionDecision: deny` with this ask's own prose as the reason, Claude
Code blocks the tool call, and the model gets a chance to retry with the
correction already in front of it, in the same turn.

Requires `added:` or `removed:` — this parses only alongside one of them, and
`onsetter lint` cannot check it for you after the fact because a bad
combination never gets the chance to exist. That restriction is the whole
design: `added:`/`removed:` are the two gates that hand back the exact text
in the edit, not a guess from a path or a fuzzy phrase, so denying on their
match means denying on something concrete the model can see quoted back to
it. A `when:`-only or `has:`-only ask stays advisory no matter what — there
is nothing here to point at as the reason for a denial.

Only a direct match blocks. A `block: true` ask reached by `cues:` never
denies, even if its own gate would have matched something elsewhere — a cued
hit never checks its own gate, so it has no quoted text to justify a denial
with, the same reasoning `has:`-only and reminder asks fall under.

Batching is unchanged: every matched ask in one call still coalesces into a
single `additionalContext` block, blocking and advisory together, so denying
on one ask's match never hides what the others found. The injected line
marks a blocking hit the same way `always:` marks its own: `matched
"print(" · blocks`.

A real positioning shift, not a free add-on: onsetter's whole design has been
advisory, never blocking, on purpose (see the README's own **Design**
section). `block:` is opt-in, per ask, and scoped to the one case where
onsetter already has the offending text in hand — reach for it only when a
reminder arriving one call too late has demonstrably cost something.
`CUSTOM_EVAL.md`'s two real misses are that case: a `Write` that embeds a
violation in the same call that creates the file, where an advisory hook has
no later call in the round to land a correction on — see its "Two real
misses" section, and `evals/block-denies-write-embed/` for the reproduction.

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
tool still shouldn't inject. `on:` is one of the bypassed gates: an `on: any`
ask can cue an `on: read` target, and the reverse, with no special handling
— the cued ask's prose injects on whichever call reached its citer, `read`
or otherwise.

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

### `fires-on:` and `silent-on:` — prove a gate isn't dark

```
fires-on: fixtures/bad_error.go
silent-on: fixtures/good_error.go
```

A gate has two independent ways to be dark: a broken pattern that never
matches anything, and a glob narrow enough that nothing ever reaches the
pattern to try. `onsetter replay`'s rate reads the same — low, or zero —
either way, and a low rate from a clean corpus is indistinguishable from a
low rate from a dead ask. `fires-on:` and `silent-on:` close that gap by
pointing at a real file whose current content is a known example, and
`onsetter lint` checks the claim: every `fires-on:` glob's matches must make
`Match` return true, every `silent-on:` glob's matches must make it return
false. A mismatch is a lint failure, not a rate to eyeball — `this ask
cannot fire on its own stated example` is a different, sharper claim than
`this ask's rate looks low`.

Repeat either header for more than one fixture; each glob is checked
independently, and every file it matches has to agree. Globs resolve
against the ask's own directory, the same as `in:` and `untouched:` — a
fixture doesn't have to live in the same file `in:` reaches, only under the
same `CLAUDE.md`'s directory.

Built from the same synthetic edit `onsetter list` and `onsetter replay`
already construct from a file's on-disk content — no new fixture format,
no new mechanism to keep in sync with `Match`. That reuse carries the same
blind spot `replay`'s own warning names: a synthetic edit has no old text,
so `removed:` can never pass on one — not degrade, *never*, since there is
no diff to find a removed line in. An ask with a `removed:` header is
skipped by this check entirely, with a note printed instead of a false
result either direction; check a `removed:` gate by driving `onsetter hook`
with a real old/new pair. `added:` has no such gap — it degrades to
"matches anywhere in the file," which is exactly the question a
deliberately-written fixture is answering.

Not a gate — Match never reads either header, and `onsetter replay` never
reports a rate for them.

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
| a corpus where every matching edit is suspect, not just the first | `always: true` |
| a Write/Edit that should be forced to retry with the fix in hand | `block: true` (with `added:`/`removed:`) |

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

Every first draft over-fires — one fired on every file in its corpus because
it gated on the file carrying a citation, which is what every file in that
corpus is. A gate tripping on more than a few percent of what it matches
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

The two kinds of block get opposite treatment without either one declaring
itself, and a question is the ask plus the text it quoted back.

A block with no content gate quotes nothing — it's a reminder, and it fires
once per session however many files it governs, then stays quiet: a second
firing would be the literal same sentence, with nothing new in it. `has:`
gates on the file as it stands but never contributes a quote either, so a
`has:`-only block is a reminder too. So is a firing reached through
`cues:` — a cued hit never checks its own gate, so it never has anything to
quote, whatever the target ask's own `when:` might otherwise have matched.

A block with a `when:`, `added:`, `removed:` or `evokes:` is different: it
always fires, on every occurrence, for the whole session. What used to be a
session-suppression key (the ask, the quote, and — with `revisit: true` — the
edit that produced it) is now purely a *repeat counter*: the same occurrence
firing again gets marked, `matched "TODO" (asked 2× already this session)`,
instead of going quiet. An occurrence is the quote *and* the edit around it —
"you nod" in one file and "you nod" in an unrelated one are different
occurrences and both fire on their own first sighting, unmarked; the same
edit repeated byte-for-byte is the same occurrence and its count keeps
climbing rather than resetting.

That's a real change from every earlier release, not a tightening of the old
default: the old key hashed the quote alone, so two different occurrences
sharing a short match collapsed into one already-answered question and the
second one went silently quiet. Nothing measurable distinguishes an ask that
changed an edit from one that was skimmed and ignored — this package has
never trusted a decay model built on that guess — and treating "already
fired once" as "already handled" was exactly that guess, just an invisible
one. Counting instead of suppressing hands the actual judgment call — is a
repeat worth a fresh look, or the same answer as last time — to the one
thing in this loop suited to make it: the model reading the marker, not a
session file that can't tell those two cases apart.

One invariant this changes: a reminder still has a ceiling of exactly one
firing per session, since it has only ever had one possible key. A matched
ask no longer does — "an ask can fire at most once per distinct string its
regex can match" was true when the key was the string alone; now the key
includes the edit, so the same string recurring in twenty different edits
counts as twenty occurrences. What's left as a ceiling is authored by the
corpus rather than the pattern: how many times a real edit will ever contain a
match, which `replay` still measures as a rate, just no longer one a session
key could distort.

`always: true` opts a matched ask out of the counter entirely — every firing
looks identical, `matched "TODO" · always`, no matter how many times it's
already asked. Right for the near-check class this exists for: a corpus
where every matching edit is already expected to be suspect, where a
running count would just be noise. On a reminder, `always:` means what it
always has — skip the once-per-session suppression and fire on every file,
not just the first; there's no counter there to opt out of.

`revisit:` is retired — its whole job was widening the key past the bare
quote, and that widening is unconditional now. It still parses, so an
existing block doesn't go dark; `onsetter lint` flags it as redundant.

`block: true` doesn't change any of the counting above — it rides on
whatever firing the ask would have had anyway, matched or cued, counted or
not. What it adds is a second, independent effect on a direct match only:
the response also denies the tool call, with this ask's prose as the reason,
and the injected marker reads `matched "print(" · blocks` instead of the
plain quote.

The idea that a repeat still worth a fresh look, rather than the same
answered question, is [treadiehq/codecut](https://github.com/treadiehq/codecut)'s
`verification-evidence` rule: it keys a passing test to the diff fingerprint
it actually ran against, so a stale pass from before the last edit doesn't
count. Onsetter never sees a test run — only the `Write`/`Edit`/`Read` it
already watches — so this is that idea's narrower shadow: fingerprinting the
edit rather than a verification event, because an edit is the only state
onsetter has.

This release re-arms every deployed ask once per in-flight session, whether
or not it ever used `revisit:` — `ID()`'s hash shape changed, not just one
header's contribution to it — the same one-time cost the v0.3.0 key-format
change, and `cues:`'s own release, both already paid. `cues:` still leaves
`name:` out of the hash entirely — renaming an ask to fix a collision, or
just to make its cue wiring read better, should not re-arm every
already-answered session instance of it.

Identity is a hash of the gate and the body rather than the line number, so
inserting a paragraph above an ask changes nothing and editing its prose
re-arms it. To
retire one, delete the block.
