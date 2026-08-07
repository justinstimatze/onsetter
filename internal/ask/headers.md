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


## The nine headers

Listed in the order `Match` applies them, which is the order `onsetter replay`
reports a funnel in.

| Header      | Matches                                   | Repeat means |
|-------------|-------------------------------------------|--------------|
| `in`        | the path, as a glob                       | last wins    |
| `not-in`    | the path, as a glob — excludes            | OR           |
| `on`        | one of: any, mint, edit                   | last wins    |
| `not`       | the incoming text — suppresses            | OR           |
| `has`       | the file as it stands on disk             | AND          |
| `untouched` | paths written this session — suppresses   | AND          |
| `added`     | the lines this edit introduces            | AND          |
| `removed`   | the lines this edit deletes               | AND          |
| `when`      | the incoming text                         | AND          |

Globs are [doublestar](https://github.com/bmatcuk/doublestar) and resolve
against the directory of the `CLAUDE.md` the ask lives in — never the repo root
and never the working directory, with the `.claude/` exception noted above.
Braces and `**` both work: `{locations,chars}/**/*.yaml`.

Patterns are Go RE2. **There is no lookahead and no backreference.** To require
two things, repeat the header — `when: fact_text` plus `when: you realize` is
the conjunction, and there is no way to write it as one pattern.


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
is noise. A block with a `when:`, `has:`, `added:` or `removed:` keys on what
it matched, so a new match asks again and a repeat of the same match stays
quiet. Its ceiling is set by its own pattern: an ask can fire at most once per
distinct string its regex can match, so twenty alternatives means at most
twenty questions.

If a content-gated ask only ever fires once, its pattern is matching a property
of the file type rather than a signal that something changed. That is worth
knowing — see the funnel note under **Before you wire it**.

Identity is a hash of the gate and the body, not the line number, so inserting
a paragraph above an ask changes nothing and editing its prose re-arms it. To
retire one, delete the block.
