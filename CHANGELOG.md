# Changelog

## v0.5.0 — 2026-08-26

- New header: `revisit: true`. Every content-gated ask fires once per
  distinct string it quotes back, for the life of the session — editing the
  same file five more times without touching that string stays silent, which
  is correct for a fixed reminder and wrong for one still sitting there
  unresolved. `revisit: true` widens the fired-set key from the quoted
  substring alone to the substring plus the whole edit that produced it
  (`internal/session.KeyRevisit`), so a later, different edit that
  reintroduces the same literal text asks again instead of reading as
  already-answered. Not a gate — it never appears in a `replay` funnel, and
  pairing it with an ask that gates on nothing does nothing at all.
  Prompted by reading treadiehq/codecut's `stateFingerprint` mechanism, which
  solves the analogous problem for its own verification-evidence rule by
  keying a passing test's evidence to the diff it actually ran against. The
  translation here is narrower: onsetter never sees a Bash call or a test
  result, only the Write/Edit it already watches, so this fingerprints the
  edit rather than a verification event.
  Every ask's `ID()` now folds in `Revisit`, so this release re-arms every
  deployed ask once per in-flight session, the same one-time cost v0.3.0's
  key-format change paid.
- Fixed: an ask in the global `~/.claude/CLAUDE.md` scoped its globs to
  `$HOME` instead of `~/.claude/`, because `scopeOf` treated any `CLAUDE.md`
  sitting in a directory named `.claude` as the project-local
  `<root>/.claude/CLAUDE.md` case. `$HOME` is not a project root, so
  `in: feedback_*.md` there silently never matched a real memory file under
  `~/.claude/projects/**/memory/`. Caught installing the first-ever ask
  directly in the global file: it parsed, linted, and would never have fired.
  `scopeOf` now excludes `$HOME/.claude` from the parent-scoping rule.
- `internal/ask` is now `ask` — exported so a second module can call the same
  `Match` logic against its own tool calls, not just onsetter's own
  `Write`/`Edit` hook. First consumer in mind is winze-agent's
  `capture-guard`, which gates an MCP call (`winze_remember`) with no file
  path.
- `Match` accepts `Edit.Path == ""`. `when:`/`not:`/`has:` run as before; an
  ask that also sets `in:`, `not-in:`, or `untouched:` rejects and says the
  ask needs a path, instead of the misleading "the file is not under this
  CLAUDE.md's directory" a path-less call used to get.
- New header: `requires:`. Fires only when a named binary resolves on
  `$PATH` — a fact about the machine, not the file or the edit — and is
  checked before every other gate, so a rejection on a machine without the
  tool names the real reason. Nothing is executed; it is `exec.LookPath`.
  First use: gating README's stull recipe with `requires: stull`, so it only
  ever asks on a machine that has stull installed.
- New header: `evokes:`. A fuzzy trigger phrase, not a regex — fires on any
  one of a repeated list, the opposite of `when:`'s AND. Backed by a local
  Ollama embedding call and a warm, on-disk cache (`onsetter warm` builds it;
  `onsetter hook` never fills a cache miss itself), with a same-turn score
  as the whole decision — no deferred judge pass, on the reasoning that every
  onsetter ask is already built to be cheap to dismiss. Measured directly
  against real content rather than borrowing a threshold from elsewhere: has
  real signal on prose-shaped edits (comments, commit messages, docs) and
  close to none on a whole code file, where syntax dilutes the match almost
  to noise — the regex headers already own that case precisely. New command:
  `onsetter warm [dir]`.
- New command: `onsetter calib <ask> <fires-glob> <not-glob>` — `replay`'s
  counterpart for `evokes:`, the one header whose fire/no-fire is not its own
  ground truth. Reports the weakest true positive against the strongest
  false-positive risk from an author-supplied labeled example set, and says
  plainly when the two overlap: no threshold separates them, and the fix is
  rewording the phrases, not the number. Built after a cross-project
  exchange showed a threshold measured against a handful of hand-picked
  pairs does not reliably generalize even to an adjacent matching regime.

## v0.4.0 — 2026-08-07

Nothing here changes how an existing ask fires. Re-run `onsetter install` to
pick up the skill; the hook entry it rewrites is the one it already wrote.

- `install` writes `~/.claude/skills/onsetter/SKILL.md` beside the settings
  entry it wires. The hook half of this tool needs no advertisement — it fires
  on the tool call whether or not the agent it fires at knows the tool exists.
  The authoring half had nothing: no tool named onsetter appears in an agent's
  list, so the only route to the format was a line of prose in someone's
  always-on `CLAUDE.md`, which costs tokens on every unrelated turn and is
  still missed. A skill is that prose, indexed by its description and loaded
  when an ask is actually being written.

  It defers the header table to `onsetter headers` instead of copying it. Two
  copies of one reference drift, and the copy inside the binary is the one that
  cannot disagree with the parser shipped beside it. The skill carries what the
  reference does not: where the block goes, the replay loop, the two things
  replay cannot measure, and why a wide gate costs more since v0.3.0.

  Regenerated on every install rather than merged, and a test asserts a
  hand-edit does not survive one. Adding a header is now four places, not
  three — the fourth is the header list in the skill's frontmatter, which is
  the entire retrieval surface, so a header missing there is a header whose
  questions never reach the guide. The test names the exact list.

- `replay` marks any row whose rate does not measure the gate the author cares
  about. It builds synthetic edits from files on disk, so there is no old text:
  `added:` degrades to `when:` and `removed:` never fires. A real
  `added: "aliases"` printed 100.0% while gating on nothing replay can see, and
  the number was silently about the ask's `in:` instead. The row now says so and
  points at `onsetter hook`, which is the honest check.

  Not building `replay --since <rev>` to fix this properly. Across every
  deployed ask set there is exactly one using a diff header and none using
  `removed:`, so an hour of git-history walking would serve a single gate that
  two piped JSON payloads verify in ninety seconds. If diff-gated asks get
  common the feature earns itself then.

## v0.3.0 — 2026-08-07

Behaviour change for anyone already running v0.2.0: an ask that gates on content
now asks more than once in a session. See the first entry.

- The fired-set keys on the ask **and the text it quoted back**, so an ask that
  gates on content asks again when there is something new to look at. Editing
  forty files in one session used to produce one question; now a new match is a
  new question and a repeat of the same match stays quiet.

  Two kinds of block live in this format and want opposite treatment, and the
  block already says which it is. No content gate means nothing to quote, so the
  key is the bare ask ID and it fires once per session — the reminder case,
  unchanged. A content gate means it is asking about a specific string, and *is
  this narrator overreach* is a different question about `you nod` than about
  `you find yourself`. So the rule is derived rather than configured, and there
  is no new header.

  The ceiling is authored: an ask fires at most once per distinct string its own
  regex can match. A content-gated ask that still only fires once is telling you
  its `when:` matches a property of the file type rather than a signal that
  anything changed.

  The match is hashed into the key rather than appended — it can be 80 bytes of
  arbitrary text including newlines, and the store is one key per line. Sessions
  in flight across the change hold bare IDs, so reminders keep suppressing and
  content asks re-arm once.
- `internal/session` had no tests and now has six, including one asserting that a
  match containing newlines cannot write two lines into a one-key-per-line store.
- The `ci` workflow pins `GITHUB_TOKEN` to `contents: read`. CodeQL's actions
  analysis flagged it the moment code scanning was switched on: with no
  permissions block the job takes whatever the repository default grants, which
  is a write token handed to every action in a build that only reads.
- Dependabot PRs auto-merge once `ci` goes green, for non-major bumps only. A
  title the parser cannot read is left for a human.
- Repository settings: secret scanning, push protection, Dependabot alerts,
  private vulnerability reporting, and CodeQL default setup over `go` and
  `actions` on the extended suite.

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
