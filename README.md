# onsetter

Somebody was editing `hook.go`.

It was a Tuesday, and the file was the one every `Write` and `Edit` in a
session runs through, and they had been at it an hour or so and were getting
on rather well.

Then they wrote a `panic()`, two calls downstream of a JSON field the caller
always sets. Always.

"Hallo," said a note. "Is this path reachable from the hook?"

"It can't be. The caller always sets that field."

"Always is a word about a caller you don't control," said the note. "If it's
wrong once, what happens here?"

So the somebody looked, which is more than most people do on a Tuesday. The
field came off disk, not off a struct literal, and nothing upstream of this
line had ever checked it was there.

"It's just for local testing," they said, hopefully.

It was not just for local testing.

---

The note is real. It lives in `CLAUDE.md`, seventeen lines into this
repository's own ordinary markdown file, and it has been sitting there since
the redesign that made a matched ask fire on every occurrence instead of once:

````markdown
```ask
in: cmd/**/*.go
not-in: **/main.go
not-in: **/*_test.go
when: os\.Exit\(|log\.Fatal|panic\(

Everything reachable from `onsetter hook` stands in front of every Write and
Edit in a session, and the only acceptable failure there is exit 0 with no
output — bad JSON, a missing path, an unparseable block, a panic. Is this path
reachable from the hook, and if it is, does something above it recover and exit
0? If this is a subcommand that only ever runs from a terminal, continue.
```
````

onsetter is the part that carried it to the edit. It is one `PreToolUse` hook.
It reads every `CLAUDE.md` above the file being written, matches the blocks
against the path and the incoming text, and prepends the ones that hit. Before
the write lands, Claude receives this:

```
onsetter — 1 ask for this edit. A repeat is marked, not hidden.

▸ CLAUDE.md:17 · matched "panic("
Everything reachable from `onsetter hook` stands in front of every Write and
Edit in a session, and the only acceptable failure there is exit 0 with no
output — bad JSON, a missing path, an unparseable block, a panic. Is this path
reachable from the hook, and if it is, does something above it recover and exit
0? If this is a subcommand that only ever runs from a terminal, continue.

To retire one, delete its block from the file named above.
```

That is the whole product. Adding an ask is writing a paragraph in a file you
already have. There is nothing to install per ask and no second source of
truth. It fires on every occurrence, marking a repeat instead of hiding it —
see [Design](#design) for what that means and why. To be rid of it,
delete the block.

[The failure it answers](#the-failure-it-answers) ·
[Install](#install) ·
[Writing an ask](#writing-an-ask) ·
[Commands](#commands) ·
[Design](#design) ·
[Where it sits](#where-it-sits) ·
[Prior art](#prior-art)

## The failure it answers

You write the convention down, it loads at the top of the session with
everything else, and it gets followed a bit better than half the time — 55.0%,
across six models, when the rules are sitting right there in the context window
([Zhou et al. 2026](https://arxiv.org/abs/2606.13174); the full table is under
[prior art](#prior-art)). The convention was never wrong. Rereading it just
never happens, because nothing makes it happen at the moment it applies.

## Install

There are two independent install paths — pick one, since running both wires
the hook twice, and `onsetter status` will tell you if that's happened.

**As a Claude Code plugin.** No Go toolchain needed: a `SessionStart` hook
fetches a release binary and verifies it against a checksum pinned in this
repo before ever running it.

```
/plugin marketplace add justinstimatze/onsetter
/plugin install onsetter@onsetter
```

The plugin wires the hook as a persistent MCP server (`onsetter serve`,
`type: "mcp_tool"` in `hooks/hooks.json`), not a fresh process per call — the
same `Write`/`Edit`/`Bash` matcher, plus `Read` unconditionally, since the
cost that used to make `Read` an opt-in extra (a process spawn and a full
`CLAUDE.md` re-parse on every call) doesn't apply to a connection Claude Code
keeps warm for the session. `skills/onsetter/SKILL.md` already ships with the
plugin, so nothing else to run.

Rarely — the very first session after installing, or one that starts mid an
update's binary re-fetch — the server can fail to connect before the fetch
finishes; `onsetter status` reports the wiring as configured but won't claim
a live connection it can't see, since it's a one-shot process itself. If an
ask stops firing right after install or an update, reconnect the server from
`/mcp`, or just start a new session once the fetch has had time to finish.

**As a Go binary**, if you'd rather build it yourself (this path still gates
`on: read` behind `--read`, since a manually-wired hook is still one process
per call):

```
go install github.com/justinstimatze/onsetter/cmd/onsetter@latest
onsetter install
```

`onsetter install` writes the one settings entry into
`~/.claude/settings.local.json`. Re-running converges instead of stacking, and
it leaves every other project's hooks alone. From a clone, `make wire` does
both and stamps the version from the git tag. It also writes
`~/.claude/skills/onsetter/SKILL.md` itself, which is how *writing* an ask
becomes discoverable — the hook needs no such thing, since it fires on the
tool call whether or not anything knows it exists, but drafting a block does:
there is no tool named onsetter in an agent's list, so without the skill the
format is only findable by reading this file. The skill is indexed by its
description and loads when an ask is being written, and it defers the header
table to `onsetter headers` rather than copying it, so the reference an agent
reads is always the one the installed parser implements. Pass `--read` to
also wire an `on: read` matcher — see [Writing an ask](#writing-an-ask) for
what that trades off.

Hook settings are read per session, so a session already running will not pick
this up. Start a new one.

## Why a question and not a check

A check that blocks needs high precision. A linter wrong one time in three gets
turned off inside a week, and it deserves to be. Every convention that cannot
be made that precise has had exactly one home available: a document, read once
at the top of a session and never again at the moment it applied.

A question has almost no precision floor. Being wrong costs one sentence —
*it's just for local testing, continuing* — which is why every ask here
ends with that escape hatch. The asymmetry is the design. It opens a band
nothing has served: conventions worth checking that can never be enforced.

What lives in that band is the slow leak. Nothing these asks catch turns CI
red. The build is green, the code reads fine, the value is recorded — and the
cost arrives months later as a field nobody renders, a state nobody
persisted, an assumption the caller could not have verified, a regression
lock that has never once been observed to fail.

It is newly affordable because the reader is an agent. A person who meets a
check that is often wrong learns to scroll past it, and then past the ones that
were right. An agent mid-edit reads one sentence and either changes course or
dismisses it in a clause. Advice too imprecise to automate costs almost nothing
when the recipient was going to process the tokens anyway.

That asymmetry has a floor: a question and a permission prompt fail the same
way, both handing the decision back to whoever might skip it — right when
they're mid-task and likely to skip it a second time. If the actual failure is
*the rule was right there and got ignored anyway*, arriving at a better
moment doesn't remove that dependency — it aims the same mechanism more
precisely. Removing it takes a check nothing gets a vote on. That's a
different tool; see [stull](https://github.com/justinstimatze/stull), below.

## The kinds of ask this holds

The set below spans a few shapes of ordinary software convention — an error
string, a handler's context lifetime, an accessible label, a comment's
certainty, a regression lock — the kind of thing every codebase accumulates
and nothing compiles.

**Taste.**

```ask
in: **/*.go
not-in: **/*_test.go
when: (?i)"(invalid input|bad request|something went wrong)"

An error message a caller has to guess the meaning of is worse than none. Name
the field, the constraint it violated, and what a valid value looks like. If
this string already carries all three, continue.
```

**A design principle.**

```ask
in: **/*handler*.go
when: context\.Background\(\)|context\.TODO\(\)

A handler that starts its own context instead of taking the caller's ignores
a client's cancellation and any deadline this request is already running
against. If this genuinely has to outlive the request — a background job the
handler only kicks off — continue.
```

**Accessibility**, where `not:` carves out the one legitimate shape.

```ask
in: **/*.{tsx,jsx,html}
when: <img\b
not: alt=

An `<img>` with no `alt` reads as nothing to a screen reader — not "image,"
nothing. If this is genuinely decorative, `role="presentation"` says so on
purpose instead of by omission. If alt text is already here, continue.
```

**Epistemics**, where two `when:` lines are an AND — *names a symptom the code
actually observed* and *asserts why it happened* are two conditions over the
same span, and one regex cannot say it.

```ask
when: (?i)\b(timed out|timeout|connection reset|EOF)\b
when: (?i)\b(because|caused by|due to|the server (was|is))\b

The code observed a symptom — a timeout, a reset connection — not a cause. A
comment or log line asserting *why* it happened claims something the code
never checked. If this traces back to a specific condition verified two lines
up, continue.
```

**A standard for the tests themselves**, gated on nothing but the filename,
because every edit to a file matching it is the thing it is about:

```ask
in: **/*_regression_test.go

This is a regression lock. Before trusting it green, reintroduce the bug it
guards and watch it go red — a lock you have not seen fail is theater.

Anchor on something distinctive and deterministic: a real fixture value, a
specific error type, a specific line of output. Never a substring so common it
would also match the next bug in the same function.
```

*A lock you have not seen fail is theater* is the standard the whole set is
trying to meet. An ask that can name what it keeps catching has earned its
place; the rest are on probation.

## Writing an ask

Headers, one blank line, then the prose, all inside the fence — and the blank
line is required even when there are no headers, or the first sentence is read
as one. Every header names the text it matches against, and every one is
optional. They are listed here in the order they are applied — the same
order `replay` reports a funnel in:

| header | matches | default |
|---|---|---|
| `requires` | a binary resolving on `$PATH` — a fact about the machine, not the file | none |
| `in` | the path — glob relative to **this** `CLAUDE.md`'s directory. `**`, `{a,b}`, `?`, `[...]` | everything below the file |
| `not-in` | the path — excludes something `in` would have matched | none |
| `on` | `any`, `mint` (the file does not exist yet), `edit`, or `read` (a Read call, never a write) | `any` |
| `not` | the incoming text — suppresses the ask | none |
| `has` | the file as it stands, ignoring the edit | — |
| `untouched` | glob — no file matching it was written this session | — |
| `added` | only the lines this edit introduces | — |
| `removed` | only the lines this edit deletes | — |
| `when` | the incoming text. `(?i)` for case-insensitive | any content |
| `evokes` | the incoming text, semantically — not a regex | none |
| `revisit` | nothing — retired, parses but does nothing; `lint` flags it | `false` |
| `always` | nothing — skips the repeat count on a matched ask, or the once-per-session suppression on a reminder | `false` |
| `name` | nothing — gives another ask something to cue | none |
| `cues` | nothing — fires a second, named ask in the same injection | none |

The last four never gate — none of them can turn a pending edit away, and
none appears in `replay`'s funnel.

`when` and `not` see `content` on a Write and `new_string` on an Edit — the
replacement span, not the whole file. Repeat `when`, `added`, `removed`,
`has`, or `requires` for an AND; repeat `not` for an OR of suppressors.

`any`, `mint`, and `edit` all mean "a pending Write or Edit." `on: read` is
the opposite — a Read call, never a write — so it can never be paired with
`when`, `added`, `removed`, `not`, or `evokes`: none of those ever see
incoming text on a Read, and `onsetter lint` rejects the combination. `has`
still works, since it reads disk content either way. The plugin install wires
`on: read` unconditionally — a Read happens far more often than a write, but
the plugin's persistent server pays the discovery walk once per file, not
once per call, so there's no per-Read cost left to gate behind a flag. The
manual Go-binary install still gates it: `onsetter install --read` adds it,
since that path is still one process per call and a Read happening far more
often than a write means real added cost there — `onsetter status` flags an
`on: read` ask sitting unreachable if you forget, on either install path.

`requires` is checked before every other header — it is a fact about the
machine running onsetter, not the file or the edit, so a rejection on a
machine without the tool names the real reason instead of a misleading path
or content mismatch. It never executes anything; `exec.LookPath` only stats
`$PATH`.

`onsetter headers` prints the full reference: every header with its haystack,
its gotchas, and a table mapping *what you want to catch* to the gate that
catches it. It ships inside the binary rather than living at a path you have to
already know, because the reader who needs it is usually an agent mid-task.

Use `not-in` and never `not` to exclude a path: `not` is a content regex, so
`not: _test\.go` silently matches nothing, which had one ask firing on nearly
twice the files the script it replaced did.

The same silent-nothing trap catches a leading `./` on `in:`. The path onsetter
matches against comes from `filepath.Rel`, which never carries one, so
`in: ./cmd/**/*.go` matches no file `in: cmd/**/*.go` wouldn't already match on
its own — write the glob relative to the CLAUDE.md's own directory with no
leading `./`.

`added` and `removed` come from a real line diff of `old_string` against
`new_string`, so an edit that replaces a span already containing the pattern
does not read as introducing it. Only `removed` can see a deletion at all —
nothing about taking an error check out appears in the text being written,
which is why that class survives review.

```ask
in: **/*.go
removed: if err != nil

This edit deletes an error check. Is the error now handled somewhere else, or
did it just stop being checked? If the call genuinely cannot fail here, say so
in a comment and continue.
```

`has` is the one that needed inventing: it gates on the file as it stands
rather than on the edit, which is how you write *this file already has a mutex
and you are adding a second locking scheme*. The file is read only when some
ask actually asks for it.

`untouched` gates on the paired-file case — *you changed one half of this and
not the other*:

```ask
in: internal/db/schema.go
untouched: migrations/*.sql

The schema changed and no migration has been touched this session. Is the
column backfilled somewhere, or does this ship a table the running code cannot
read? If the migration is already merged, continue.
```

It knows only what onsetter saw go past. `onsetter install` also wires a
`Bash` observer alongside the main hook: it parses the command's actual
shell syntax and marks a path touched when the command clearly writes to
it — a redirect, `tee`, an in-place `sed`/`sd`, or `cp`/`mv`'s destination —
trusting only arguments that resolve to a plain literal, never a guess at
what a variable or command substitution might produce. It never runs
anything; it only reads the command text. This narrows the gap rather than
closing it: a program invoked *by* the Bash call, writing files through its
own logic, is still invisible — dismissible in a sentence, but worth knowing
before you write one. The path in hand is recorded *after* matching, so an
edit never satisfies an `untouched:` gate about itself.

`evokes` is a fuzzy trigger phrase rather than a regex or a glob. Repeating it
means an OR — it fires on any one of them — where every other repeatable
header means an AND:

```ask
evokes: committing without asking the user first
evokes: pushing straight to the main branch

Never commit or push without explicit confirmation.
```

It needs setup nothing else here does. `onsetter warm` embeds every `evokes:`
phrase into a local cache ahead of time — the hook never fills a cache miss
itself, so a phrase added since the last `warm` silently never fires. Running
[Ollama](https://ollama.com) locally with an embedding model pulled is
required too; missing either one degrades to "this ask does not fire," never
an error. And it has almost no signal on a whole code file: a true paraphrase
scores well above an unrelated sentence when the relevant text stands alone,
but bury it in a page of syntax and the score drops to barely above noise.
`evokes` is for a topic or a shape of reasoning in comments, commit messages,
or prose files — the regex headers above it already own code-shaped
triggers, and that split came from measurement.

A regex header gets `replay` for free — its own match or no-match is its own
ground truth, no labels needed. `evokes` doesn't have that: whether a fuzzy
score *should* fire is not self-evident from the number. `onsetter calib`
is `replay`'s counterpart for this one header — point it at an ask and two
globs of files you've labeled fires and not, and it reports where the true
positives and true negatives actually land relative to the threshold:

```
$ onsetter calib CLAUDE.md 'fires/**' 'not/**'
3 positive example(s), 2 negative example(s), threshold 0.48

positive scores (weakest first):
    fires/subtle.md                          0.563
    fires/paraphrase.md                      0.577
    fires/direct.md                          0.732

negative scores (strongest first):
    not/close.md                             0.586   ← fires (false positive)
    not/unrelated.md                         0.475

POS floor 0.563 (fires/subtle.md)   NEG ceiling 0.586 (not/close.md)
overlap 0.024 — no single threshold separates every example given;
the evokes: phrases or the examples themselves need rework, not just a number
```

Five examples were enough to catch this: "reviewed the PR carefully and left
three comments" scored above the weakest genuine match, meaning no threshold
separates that pair — the phrases need rewording, not a different number,
which is the whole reason this exists as a real tool rather than a hand-run
scratch test: the number that matters is whichever example set an author
actually built, not the two or three pairs it shipped measured against.

`name` and `cues` let one ask fire a second one by name — its gate is never
consulted:

```ask
when: fetch\(.*credentials
cues: check-token-scope

Fetching with a credential in scope — does the token this uses have write
access it doesn't need here?
```

```ask
name: check-token-scope
not-in: **

Cued-only prose lives here, reachable only by name.
```

A cued firing never quotes anything, so it fires once per session like any
other no-content-gate ask, and it can only ever reach a `name:` declared in a
`CLAUDE.md` that is an ancestor of (or the same file as) the citing ask's own
— never one nested below it. A cycle or a diamond of cues is safe: each ask
is visited at most once per edit. `onsetter headers` has the full mechanics
and the one real gotcha (the ancestor-scope rule above).

A matched ask already fires on every occurrence. `always` opts it out of the
*count* that marks a repeat, for a corpus where every matching edit is
suspect by construction and a running "asked 4× already" number would just
be noise.

```ask
in: corpus/**
when: (?i)\bnever\b
always: true

Every matching edit here is worth a second look, not just the first one.
```

Without `always:`, this ask would fire on every match too — that's the
default now — just with an `(asked N× already this session)` marker once a
given occurrence recurs. `always:` keeps every firing looking identical
instead. Width still costs here, more than most other asks: an unnarrowed
one is one of the loudest injections this format can produce, marked or
not, with no once-per-session amortization to fall back on for the matched
class at all anymore.

Most blocks are one header and a paragraph. An ask in `corpus/CLAUDE.md` with
no `in:` governs everything under `corpus/`, which is the scope its author can
actually reason about.

The body is injected verbatim, so notes *about* the ask — why the regex is
shaped that way, what an earlier draft measured — belong in ordinary markdown
outside the fence. They are still read by anyone going through the file top to
bottom; they just do not arrive mid-edit, where they are not the question.

`CLAUDE.local.md` and `<dir>/.claude/CLAUDE.md` are read alongside `CLAUDE.md`.
The `.claude/` form is for repos that gitignore `.claude/*` to keep their
conventions out of the tree they publish. Its asks scope to the parent
directory, not to `.claude/`, so `in: internal/**/*.go` in
`<root>/.claude/CLAUDE.md` means what it looks like it means.

### Replay before you wire

The ask at the top of this page is one of this repo's own two — both live in
the `CLAUDE.md` at the root, so there's a real corpus to replay against
without inventing one. Add a third, written the way a first draft gets
written:

````markdown
```ask
in: **/*.go
not-in: **/*_test.go
when: \bappend\(|\bRecord[A-Z]|\bStore[A-Z]

This records state — trace the read side now: does the value have a reader
that reaches the user — a rendered field, an API response, a log someone
greps? If it is internal plumbing feeding another writer, confirm that writer
exists and continue.
```
````

Then ask what all three cost:

```
$ onsetter replay 'cmd/**/*.go' 'internal/**/*.go'
Replayed 25 file(s).

CLAUDE.md:49                           9/25      36.0%
    cmd/onsetter/bash.go  "append("
    cmd/onsetter/calib.go  "append("
    cmd/onsetter/hook.go  "append("
CLAUDE.md:17                           1/25       4.0%
    cmd/onsetter/hook.go  "os.Exit("
CLAUDE.md:36                           0/25       0.0%
    turned away at  in: ask/ask.go ×25

A gate tripping on more than a few percent of what it matches is a tax.
Narrow it, or move the ask closer to the files it is about.
```

The new one fires on over a third of the Go files in this glob, where the two
beside it either barely fire or — the third line — can't reach a single one of
these 25, because its own `in:` scopes it to one file elsewhere in the repo on
purpose. This is an ask that gets scrolled past by Thursday, and it reads
perfectly well. In a repo with two hundred packages and the same `in:`
narrowed to the five that hold mutable state, it is a good ask. Same regex,
same prose, different blast radius — and nothing but a rate tells
you which one you have.

That is the whole discipline. Every first draft over-fires:

| ask | draft 1 | draft 2 | final |
|---|---|---|---|
| does this new claim need a debunking pass? | 100.0% | 12.2% | 2.8% |
| is this citation pinned to an edition? | 43.7% | 0.9% | 0% |
| is this name doing too much work? | 0% | 0% | 0% |

The first gated on the file carrying a citation, which is what every file in
that corpus *is* — a property of the type, not a signal that anything changed.
Reading the regex never catches that.

Replaying an ask against the script it replaces is stronger still, when there
is one. Migrating a repo's hook set, every ask but one ended up agreeing with
its script on every file in the corpus; the exception was narrowed on purpose.
The disagreements that got them there were two regexes paraphrased instead of
copied and one path filter written as a content filter — none of which reading
the ask would have caught.

`replay` measures what a gate **costs**. It cannot measure what a gate
**catches**: the corpus has already been cleaned of the defect the ask looks
for, so a healthy ask and a dead one both read as zero — a fact about the
corpus itself.

It can say *where* a zero came from, which is the next best thing. Every ask
that fired on nothing gets a funnel line naming the gates that turned its files
away, in the order they were applied:

```
CLAUDE.md:160                          0/27       0.0%
    turned away at  in: internal/{state,engine,worldstate}*/*.go ×27
CLAUDE.md:175                          0/27       0.0%
    turned away at  when: "aliases" ×27
```

The first glob reached no files at all. The second reached all 27 and the regex
matched none of them — a live ask pointed at a corpus that does not have the
thing. Same 0.0% either way, and only one of them is a typo.

## Recipes

These are meant to be copied whole. The worked examples above teach shape
for one corpus; a recipe is a block anyone can drop in as-is.

**Point an `always`/`never` rule at a compiled guard instead of leaving it as
prose.** [stull](https://github.com/justinstimatze/stull) compiles exactly
this kind of judgment into a hook mesh with a fuel-bounded guarantee that it
halts; a `CLAUDE.md` line saying "always" or "never" is often that same
judgment, minus the guarantee. `requires: stull` means this only ever fires
on a machine that actually has it — nobody without stull gets asked about it.

```ask
requires: stull
in: **/CLAUDE.md
when: (?i)\b(always|never)\b

This reads like an enforceable rule, not a description. If it names a tool
call whose path and content could carry it, a hook fires on every matching
call — this line only fires when the file happens to be in context. See
stull's CLAUDE.md, under "Adding a machine". If this is judgment, attitude,
or something no guard could check, continue.
```

## Using `ask` as a library

Everything above assumes onsetter's own hook: a file path and a pending
`content`/`new_string`. The matching engine underneath is a separate package,
`github.com/justinstimatze/onsetter/ask`, for a second `PreToolUse` hook that
wants the same block format against a tool call that is not a `Write` or an
`Edit` — an MCP tool argument, say, which has no file path at all.

```go
import "github.com/justinstimatze/onsetter/ask"

asks, _ := ask.ParseFile("CLAUDE.md")
for _, a := range asks {
    res := a.Match(ask.Edit{New: note}) // no Path — this call has none
    if res.OK {
        // res.Matched is the text the gate hit
    }
}
```

`Edit.Path` can be empty. `when:`, `not:`, and `has:` never looked at a path
and run exactly as they do against a real file. `in:`, `not-in:`, and
`untouched:` do — an ask that sets one of those rejects with a `Result`
saying it needs a path, rather than silently never firing. Write asks meant
for a path-less caller without them, or expect them to reject every time.

`discover.Roots`, which finds the `CLAUDE.md`s governing a path, is
unchanged and still needs a real one. A path-less caller has no file to climb
from, so it has to pick which `CLAUDE.md` governs a call itself — a fixed
location, a store's own root — rather than discovering it the way `onsetter
hook` does for a `Write` or `Edit`.

The motivating caller gates an MCP tool's own argument the way this hook
gates a file write — no path, so `Edit.Path` can be empty, and the caller
picks which `CLAUDE.md` governs the call itself rather than discovering it.
`Ask`, `Edit`, `Result`, `Match`, and `Parse*` are the exported names. The
package moved out of `internal/` because that caller needed to import it. The
surface hasn't settled against more than one consumer yet, so there's no
stability guarantee.

## Commands

```
onsetter hook              the dispatcher; the only thing settings.json runs
onsetter install [--read]  wire ~/.claude/settings.local.json, write the skill
onsetter list [path]       what governs this path, and what would fire now
onsetter replay <glob>...  fire rate of every ask against a corpus
onsetter lint [dir]        parse every block; refuse the ones that say nothing
onsetter status [dir]      is it actually wired, parsing, warm, and satisfied
onsetter warm [dir]        embed every evokes: phrase under dir into the cache
onsetter calib <ask> <fires-glob> <not-glob>
                           measure one evokes: ask against labeled examples
onsetter headers           the full reference for writing one
onsetter --version
```

`list` answers "what is watching this file", which is the question you have
when an ask surprises you:

```
$ onsetter list cmd/onsetter/hook.go
2 ask(s) govern cmd/onsetter/hook.go

▸ CLAUDE.md:17
    in:        cmd/**/*.go   (relative to .)
    not-in:    **/main.go
    not-in:    **/*_test.go
    when:      os\.Exit\(|log\.Fatal|panic\(
    → would fire, matching "os.Exit("
    Everything reachable from `onsetter hook` stands in front of every Write
    and Edit in a session, and the only acceptable failure there is exit 0 ...
```

It also answers the harder question, which is why an ask you just wrote is
*not* firing. There are fifteen headers, and the one that rejected gets marked
— unless a `cues:` from elsewhere reached it anyway, in which case `list`
says so instead:

```
$ onsetter list cmd/onsetter/warm.go
2 ask(s) govern cmd/onsetter/warm.go

▸ CLAUDE.md:17
    in:        cmd/**/*.go   (relative to .)
    not-in:    **/main.go
    not-in:    **/*_test.go
    when:      os\.Exit\(|log\.Fatal|panic\(   ← nothing like it in the incoming text
    → would not fire · turned away at when:
    Everything reachable from `onsetter hook` stands in front of every Write
    and Edit in a session, and the only acceptable failure there is exit 0 ...
```

Without the marker the loop is delete a header, rebuild, rerun, repeat.

## Design

**A matched ask always fires, and marks a repeat instead of hiding it.**
State lives in `~/.cache/onsetter/sessions/<session_id>`. Two kinds of block
live in this format and want opposite treatment, and neither has to declare
which it is. A block with no content gate is a reminder — you need to know
the standard exists, and once you do, saying the identical sentence again is
noise. It quotes nothing, so it fires once per session across every file it
governs, then stays quiet. A block with a content gate is an inspection —
*is this narrator overreach* is a different question about `you nod` in one
file than the same three words in another — and it fires on every
occurrence, forever, marked `(asked 2× already this session)` once it's
recurred. An occurrence is the quote together with the edit around it: two
edits sharing a short match are two different questions, and a
byte-identical repeat of the same edit is still the same occurrence, still
worth a fresh count.

That's not the tightened version of "once per session" — it's the opposite
of it, and it wasn't the original design. The old key hashed the quote
alone, so two unrelated occurrences sharing a short match silently collapsed
into one already-answered question. Nothing measurable distinguishes an ask
that changed an edit from one that was skimmed and ignored — see "no decay
model" below — and treating "already fired once" as "already handled" was
exactly that unverifiable guess, just an invisible one. Counting instead of
suppressing hands the actual judgment call to the thing suited to make it:
whoever's reading the marker, not a session file that can't tell a genuine
repeat from a different occurrence that happens to look similar.

`always: true` opts a matched ask out of the count entirely, for the case
where every firing is expected to look the same — a corpus where every
matching edit is already suspect by construction, where a running number
would be pure noise. Gating on content and adding `always:` are both
declarations; there is no header for the once-per-session reminder default,
since that's what having nothing to quote already means.

Ask identity underneath is a hash of the gate and the prose, so inserting a
paragraph above an ask does not re-fire everything below it.

**No decay model.** Nothing measurable distinguishes an ask that changed an
edit from one that was skimmed. A decay algorithm built on a proxy nobody
trusts is worse than none, so retirement is manual: every injection names its
source as `path:line`, and the closing line says to delete the block. Asks are
pruned by the person they just annoyed.

**Fail open.** One hook standing in front of every Write and Edit is a single
point of failure, and the only acceptable failure is silent and harmless. Bad
JSON, a missing path, an unparseable block, a panic — all exit 0 with no
output. The tests assert this.

**One output shape.** `PreToolUse` is the only hook event that accepts
`additionalContext`, and it is emitted from exactly one place. A sibling
project shipped a `PreCompact` hook using the same field, where it is not
supported, and the block silently failed schema validation on the first real
compaction. A test pins the shape.

**Quote the match back.** A question about a string the author can see costs
one sentence to dismiss. A question about nothing in particular costs an
investigation.

**Gate on path and content, not path alone.** `lint` rejects an ask with no
content gate, no `in:` narrowing, and no mode — that is a banner.

**No cache.** Parsing a handful of `CLAUDE.md` files is well under a
millisecond; process spawn dominates. A cache would be a second source of truth
for no measurable gain.

## What it is bad at

Anything a linter already does, because that is free and this costs attention.
Anything that has to block, because it only ever advises. And any ask whose
honest answer is always yes — a matched ask fires on every occurrence now,
so a poorly narrowed one becomes constant wallpaper instead of an occasional
repeat; `replay` exists specifically to catch this before it's wired, and its
own warning
("a gate tripping on more than a few percent of what it matches is a tax")
is about exactly this cost.

## Where it sits

- **onsetter** — file edits. A deterministic gate, a question injected.
- [**weir**](https://github.com/justinstimatze/weir) — shell commands.
  Capability probe and antipattern rewriter.
- [**stull**](https://github.com/justinstimatze/stull) — anything needing a
  state machine, a fuel budget, a model in the loop, or an actual block.
  Where onsetter can only ask, stull can refuse outright — not a
  better-worded question, a check nothing gets a vote on. onsetter is
  deliberately not a mesh: one static hook, no branching, no LLM, no block.
- [**crystal**](https://github.com/justinstimatze/crystal) — moving work that
  *executes* onto deterministic tiers behind a verifier. Different payload;
  asks add a question where no call existed.

## Prior art

The header vocabulary is borrowed rather than invented. **Semgrep** rules are
`paths.include` / `paths.exclude` / `patterns` / `pattern-not` plus a `message`,
which is `in` / `not-in` / repeated `when` / `not` plus the body, under shorter
names. **Danger** reviews a diff through `.added` and `.deleted`, which is
`added` and `removed`; the diff itself is
[gotextdiff](https://github.com/hexops/gotextdiff), the Myers implementation
gopls uses, rather than a hand-rolled one. **pre-commit** has `files` /
`exclude` / `stages`, and `stages` is `on`. Only `has` had no precedent to
take, because none of the three sees a pending write and the current file at
the same time.

Closest in spirit: a `CODEOWNERS` file that asks the question itself instead of
summoning the reviewer, and an `.eslintrc` cascade whose rules are prose and
whose fix is judgement.

Closest in mechanism: Anthropic's own `security-guidance` plugin, which ships
enabled by default in Claude Code and gates `PostToolUse` on path *and*
pending content, injecting advisory `additionalContext` from author-supplied
rules — never blocking, the same shape this project's own header vocabulary
takes. The difference is `PostToolUse` versus `PreToolUse`: its note arrives
after the write has already landed, this one before.

The 55.0% at the top of this page is from **TRACE** — *Getting Better at
Working With You: Compiling User Corrections into Runtime Enforcement for
Coding Agents*, Zhou et al., Notre Dame / IBM Research / Tencent AI Lab
([arXiv:2606.13174](https://arxiv.org/abs/2606.13174), 11 June 2026). They took
29 hand-curated natural-language rules drawn from anonymized real-user friction
cases, and measured how often a coding agent's final response or workspace
state actually satisfied them. Averaged over six models:

| what the agent was given | complied |
|---|---|
| the task alone, no rules | 31.6% |
| corrections retrieved from a Mem0 memory backend | 42.5% |
| only the rules relevant to this task, in context | 54.0% |
| **every rule, in context** — the `CLAUDE.md` case | **55.0%** |
| the same rules compiled into checks that gate completion | 70.1% |

Read the last two rows against each other before believing onsetter works.
Narrowing to the relevant rules bought one point. Compiling them into
enforceable checks bought fifteen: evidence for enforcement, and onsetter
does not enforce.

What TRACE did not vary is *when*. Every condition there hands over its rules
at prompt time, and onsetter's entire claim is about arrival at the tool call
instead — a variable nobody has isolated. So the paper establishes the problem
and leaves this particular fix unproven. The honest scope of what is measured
here is narrower: fire rates, and blocks that agreed with the shell scripts
they replaced. [`EVAL.md`](EVAL.md) documents a direct attempt to close that
gap against TRACE's own evaluation harness, and the mismatch it surfaced
instead. [`CUSTOM_EVAL.md`](CUSTOM_EVAL.md) is the follow-up that fixed the
instrument and ran the comparison for real: same rule content delivered at
the tool call versus always in context, on 36 purpose-built scenarios —
onsetter's own channel wins, p ≈ 0.0007.

What onsetter adds beyond either is that the ask and the content it governs
live in the same directory, so the ask moves when the content does.

---

Somebody was editing a different file, some months later.

"Hallo," said a note. "This file already has a mutex."

"It doesn't."

"It did in March."

There was a pause of the sort you get when both parties are right about
different years.

"Where do you live?"

"`internal/store/CLAUDE.md`, line 41," said the note, which is written at the bottom
of everything it says, for exactly this.

And that was the end of that.

## License

MIT. See `LICENSE`.
