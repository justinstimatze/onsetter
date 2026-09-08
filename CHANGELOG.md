# Changelog

## v0.9.1 — 2026-09-08

A second adversarial pass — this one over the plugin packaging as a whole
rather than one script — found a gap that made v0.9.0 not actually
self-installable, plus a real leak already live on this public repo.

- **`.claude-plugin/marketplace.json`**, missing from v0.9.0 entirely.
  Without it, `/plugin marketplace add` has nothing to add — that command
  always targets a marketplace catalog, never a bare plugin repo, so
  `justinstimatze/onsetter` alone was not actually installable by anyone,
  from any commit before this one. `source: "./"` because the plugin
  already lives at the repo root; verified for real, not just against
  `claude plugin validate`: added this repo as its own marketplace and
  installed `onsetter@onsetter` into an isolated `HOME`, confirmed the
  version, the cache layout, and that the `skills/onsetter/SKILL.md`
  symlink survives the copy into `~/.claude/plugins/cache`.
- **`INTEGRATIONS.md` deleted.** It named `~/.claude/CLAUDE.md` by path and
  section — private, global, and not this repo's to reference — and cited
  file:line detail from a project with no public repo at that name, none
  of it verifiable by a reader. Already live on a public repo, not just a
  pre-submission concern. The one still-relevant fact (the `ask` package's
  path-less `Edit` support exists because a second `PreToolUse` caller
  needed it) is folded into the *Using `ask` as a library* section, without
  naming what that caller is.
- **`.gitignore` now excludes the three session-handoff notes** another
  Claude Code session had dropped at the repo root, un-ignored, one
  `git add -A` from landing in a commit: conversation-trace prose
  ("written by a session, at Justin's request"), a competitor's star
  count, cross-project detail. Named explicitly rather than by a glob, so
  a real future root-level doc isn't silently swallowed by the same
  pattern.
- **README's `## Install` section now documents the plugin path** —
  previously it only ever described `go install` + `onsetter install`,
  meaning the one set of instructions a marketplace visitor would actually
  follow led straight to the double-wiring `onsetter status` was built
  last release to detect. Also notes that `on: read` has no plugin
  equivalent yet (the plugin's `hooks.json` wires
  `Write`/`Edit`/`Bash` only), and that the plugin ships its own
  `skills/onsetter/SKILL.md` rather than writing one the way
  `onsetter install` does.
- **CI now runs `golangci-lint`**, closing a real gap: `make check` (and
  `.golangci.yml`, already committed) required a tool CI never ran, so
  errcheck/ineffassign/unused could pass CI while failing the local gate.
  `release.yml`'s `actions/checkout`/`actions/setup-go` versions now match
  `ci.yml`'s.
- Plugin/marketplace `description` no longer says "one paragraph, one
  time" — inaccurate since the always-fires-and-marks redesign shipped a
  release ago — and now names `Bash`, which the hook has matched since
  `onsetter install`'s `Bash` observer landed.

## v0.9.0 — 2026-09-08

Packages onsetter for distribution as a Claude Code plugin, and fixes two
concurrency/visibility gaps a fresh adversarial review found in that
packaging before it shipped.

- **Plugin packaging**: `.claude-plugin/plugin.json`, `hooks/hooks.json`
  (its `PreToolUse` entry is the same shape `install.go` already writes into
  `settings.local.json`; its `SessionStart` entry is new, plugin-only —
  `install.go` never wired anything to that event), and
  `skills/onsetter/SKILL.md` as a relative symlink to `ask/skill.md` — one
  source of truth, still `//go:embed`-ded into the binary for the
  manual-install path. `scripts/fetch.sh` runs from `SessionStart`,
  downloading the release binary matching `plugin.json`'s pinned version
  into `${CLAUDE_PLUGIN_DATA}`, verified before it's ever `chmod +x`'d or
  run. `.goreleaser.yaml` and `.github/workflows/release.yml`, both modeled
  on `hindcast`'s, give this repo a release pipeline for the first time —
  `-X main.version={{ .Tag }}`, not `{{ .Version }}`, so a goreleaser-built
  binary self-reports the same string a local `make build` does for the
  same commit.
- **`fetch.sh` hardening**, from an adversarial review that ran the failure
  modes rather than reasoning about them: a symlink named `onsetter` inside
  the release archive used to survive extraction and get `chmod +x`'d at
  its real target — fixed by extracting into an isolated subdir and
  requiring a real, non-symlink, non-empty file before touching
  permissions. `mktemp` used to land outside `CLAUDE_PLUGIN_DATA`, making
  the final `mv` into place a cross-filesystem streaming copy that left a
  growing, already-executable file visible at its destination mid-copy —
  fixed by keeping the temp dir on the same filesystem, so the move is an
  atomic rename. Both curls now carry `--connect-timeout`/`--max-time`, the
  `SessionStart` hook entry carries an explicit `timeout`, and the cleanup
  trap covers `INT`/`TERM`/`HUP`, not just `EXIT` — a hook timeout kill is
  exactly `SIGTERM`, which an `EXIT`-only trap never sees.
  `scripts/checksums-pin.txt` (populated by `make pin-checksums
  VERSION=x.y.z` after a release is confirmed published) closes the
  remaining gap: `checksums.txt` is fetched from the same mutable release
  as the binary it verifies, so on its own it only catches a truncated
  download, not a compromised release. `fetch.sh` now refuses to trust an
  unpinned `checksums.txt` at all — same silent, harmless failure as every
  other branch, not a new exception to the invariant.
- **`internal/session`'s `Record` is now safe against concurrent writers.**
  Two onsetter processes sharing a session id — a plugin install wiring
  the hook alongside a manual one, or a batch of parallel tool calls each
  spawning their own process — used to silently lose each other's counts:
  `Record` wrote its own in-memory snapshot from whenever it called `Open`,
  clobbering whatever another process had written in between. `Record` now
  takes an `flock` on a sibling `.lock` file and rereads the on-disk state
  fresh inside that critical section before merging its own increments in.
  A new test drives eight real concurrent `Store`s racing 200 increments
  against one key and asserts none are lost, with the race detector on.
- **`onsetter status` can now see a plugin-installed onsetter**, the second
  of the two independent ways the hook gets wired — a plugin's
  `hooks.json` is never written into `settings.local.json`, so before this
  a plugin-only install read as "not found, run `onsetter install`," which
  was wrong advice for someone who never needed to run it. It also flags
  the case the concurrency fix above makes survivable but still wasteful:
  a manual install wired alongside a plugin install, running `onsetter
  hook` twice on every `Write`/`Edit`/`Bash` call.
- **README's primary worked examples are no longer drawn from a specific
  content-authoring pipeline.** The opening story, the five "kinds of ask"
  examples, and the `Commands` section's `list` output now use onsetter's
  own real, currently-wired ask (verbatim, from this repo's own
  `CLAUDE.md`) and a handful of illustrative examples spanning ordinary
  software conventions — an error string, a handler's context lifetime, an
  accessible label, a comment's certainty, a regression lock — instead of
  a specific game-narrative corpus. The "Replay before you wire" section's
  demo numbers are regenerated against the real repo as it stands today,
  not carried over stale from when the repo had fewer files.

## 2026-09-02 (shipped in the v0.9.0 tag — no v0.8.0 tag was ever cut)

Three features from a sibling session's feedback on real onsetter usage,
plus a breaking change — the first in this project's history — that grew
out of reviewing the first of the three.

- **Breaking: a matched ask now always fires.** A matched ask
  (`when:`/`added:`/`removed:`/`evokes:`) fires on every occurrence, for
  the whole session, instead of going quiet after the first sighting —
  marked `(asked N× already this session)` once a given occurrence
  recurs. An occurrence is the quote plus the edit around it, not the
  quote alone: `"you nod"` in `betty.md` and `"you nod"` in
  `art_callahan.md` are two different occurrences and both fire on their
  own first sighting, unmarked; the same edit repeated byte-for-byte is
  the same occurrence and its count keeps climbing instead of staying
  silent. A reminder (no content gate), a `has:`-only block, and a
  `cues:`-reached firing are all unchanged: none of them ever had
  anything to quote, so none of them get a count, and all three still
  fire once per session then go quiet.
  This closes a real bug, not just a stance change: the old session key
  hashed the ask ID and the matched substring alone — no file, no edit —
  so two genuinely unrelated occurrences sharing a short match (the "you
  nod" example above, for real) silently collapsed into one
  already-answered question and the second one went quiet with nothing to
  show for it. The fix hands the actual judgment call — is a repeat worth
  a fresh look, or the same answer as last time — to whoever's reading the
  marker, instead of a session file that couldn't tell those two cases
  apart.
  New header `always: true` opts a matched ask out of the marker entirely
  — every firing looks identical, for a near-check corpus where every
  matching edit is already expected to be suspect and a running count
  would be noise. On a reminder, `always:` means what a plain
  once-per-session default can't: bypass that suppression and fire on
  every matching file, not just the first.
  `revisit: true` is retired: its whole job was widening the session key
  from the quote alone to the quote plus the edit, which is unconditional
  now. It still parses, so an existing `revisit: true` block does not go
  dark — `onsetter lint` flags it as redundant and names what replaced it,
  informational, not fatal.
  `internal/session`'s on-disk format moves from one bare key per line to
  `key:count`. Self-healing: a session file written before this release
  fails to parse as the new format and that session starts counting
  fresh, the same fail-soft shape this package already used everywhere
  else — nothing to migrate, and the existing 14-day sweep already clears
  old session files regardless.
  Prompted by feedback from a sibling session building a repo template on
  onsetter's asks, which named the gap directly: the once-per-session
  default has no header for "no session memory at all," for a near-check
  class of ask where every edit to a corpus is suspect.
- `onsetter install` now also wires a `PreToolUse` observer on `Bash`,
  narrowing `untouched:`'s biggest known blind spot: a file rewritten by a
  shell command — `sed -i`, a heredoc, `tee` — used to read as untouched
  forever, no matter how it was rewritten. The observer parses the
  command's actual shell syntax with a real parser
  ([`mvdan.cc/sh`](https://github.com/mvdan/sh), the parser behind
  `shfmt`) and marks a path touched when the command clearly writes to it:
  an output redirect (`>`, `>>`, `&>`, `&>>`), `tee`'s target(s), an
  in-place `sed -i`/`sd` (which rewrites in place by default), or
  `cp`/`mv`'s destination. It trusts only an argument that resolves to a
  plain literal at parse time — a path built from a shell variable or a
  command substitution contributes nothing rather than a guess, since a
  wrong guess here would wrongly suppress a real `untouched:` ask, which is
  worse than the miss it replaces. `cp`/`mv`'s destination is the one
  position where an earlier unresolved argument (a variable source, the
  common shape) doesn't block it: the destination's position is fixed
  regardless of what the source resolves to. `sed`/`sd` need every argument
  to resolve, since the flag/expression/files boundary depends on all of
  them together, not on one fixed position. This is bookkeeping only — no
  ask fires from a `Bash` call, and the default matcher's cost past the one
  extra process spawn stays small: no `CLAUDE.md` tree walk, no parsing,
  the same as it was before for everything but `Write`/`Edit`.
  This narrows the gap; it does not close it. A program invoked *by* the
  Bash call — `python generate.py`, `make`, a custom tool — writing files
  through its own logic is still invisible, same as always.
  `SECURITY.md`'s "What it writes" section also gets a standing correction
  here, independent of this feature: it claimed the session store held "no
  paths," which was already false — the `.paths` sidecar `untouched:` has
  always used already holds one absolute path per line.
  Prompted by the same sibling-session feedback, which named the specific
  harness shape that trips it: an agent told to prefer Bash (`sed`,
  heredocs, short scripts) over the dedicated Write/Edit tools for file
  changes.
- New `on:` value: `read`. Every ask before this only ever saw a pending
  `Write` or `Edit`; `on: read` reacts to a `Read` tool call instead — the
  motivating case was "never read the corpus directly before generating,"
  a rule about a read, not a write. `on:` is a strict four-way partition
  now: `any`/`mint`/`edit` only ever match a Write or Edit, `read` only
  ever matches a Read, and no ask matches both kinds. `Edit` gained a
  fourth field, `IsRead bool`, defaulting to `false`, so every existing
  caller of the `ask` package — every construction site in this repo is
  already keyed, `ask_external_test.go`'s included — keeps compiling and
  behaving exactly as it does today.
  A Read call carries no incoming text, so `when:`/`added:`/`removed:`/
  `not:`/`evokes:` can never match one; `onsetter lint` now rejects `on:
  read` combined with any of them, naming the dead gate. `has:` still
  works, since it reads disk content either way. `cues:` already bypasses
  every gate but `requires:` for a cued target, so an `on: any` ask can cue
  an `on: read` one and the reverse, with no code change.
  A Read never marks its path touched, and never satisfies an `untouched:`
  gate — reading a file is not writing it. `cmdList`/`cmdReplay` build a
  second, Read-shaped synthetic edit and use it for any `on: read` ask,
  the same honesty bar `cues:` set for a cue-only ask: left write-shaped,
  every `on: read` ask would misreport as permanently dead.
  Wiring is opt-in: `onsetter install --read` adds a second `PreToolUse`
  entry, matcher `Read`, alongside the default `Write|Edit|Bash` one — not
  bundled into the default, because a Read happens far more often than a
  write in a typical session and, unlike the Bash observer, still runs the
  full `CLAUDE.md` discovery walk on every call. A plain re-install without
  `--read` drops the entry again, the same convergence behavior the base
  entry already has. `onsetter status` gained a sixth check: an `on: read`
  ask defined with no `Read` wiring to ever reach it now reports plainly,
  the same shape as an unwarmed `evokes:` phrase or a missing `requires:`
  binary — a silent dead ask instead of a firing that never happens.

## v0.7.0 — 2026-08-29

- New headers: `name:` and `cues:`. A fired ask can now cue a second, named
  ask into the same injection — `cues: check-token-scope` fires the ask
  declared `name: check-token-scope` alongside its own, without that second
  ask's own gate being checked at all. Every gate but `requires:` is
  bypassed for a cued firing; `requires:` still has to resolve, since it's
  a fact about the machine rather than the edit. A cued firing never
  quotes anything, so it fires once per session the same way any other
  no-content-gate ask does, and `revisit:` on the target does nothing for a
  firing reached this way. Loop-safe by construction: a cascade visits
  each ask at most once per edit, so a cycle (A cues B, B cues A) or a
  diamond (A and B both cue C) both fire every ask exactly once.
  `cues:` can only ever reach a `name:` declared in a `CLAUDE.md` that is
  an ancestor of (or the same file as) the citing ask's own — the same
  chain `discover.Asks` already walks for a real edit — never one nested
  below it. `onsetter lint` and `onsetter status` both flag the common
  mistake shape (a target declared below its citer) as a heuristic; a cue
  scoped wrong will still show as zero cued fires in `onsetter replay`
  where the author expected otherwise. The idiom for prose meant to only
  ever fire by being cued, never on its own: `not-in: **`.
  `ID()` folds `cues:` into its hash (so a freshly wired cue gets a chance
  to walk a session where the citer already fired once) but leaves `name:`
  out entirely, the same treatment `requires:` already gets — renaming an
  ask to fix a collision shouldn't re-arm every already-answered session
  instance of it. This re-arms every deployed ask once per in-flight
  session regardless of whether it uses `cues:`, the same one-time cost
  `revisit:` and the v0.3.0 key-format change both paid.
  Prompted by the user's own comparison to habit-stacking in *Atomic
  Habits* — one habit's completion becoming the next one's cue.

## v0.6.0 — 2026-08-26

- New command: `onsetter status [dir]`. Read-only, four sections: is the
  `PreToolUse` hook actually registered in the settings file `install` would
  have written to, does every ask under `dir` still parse, is every
  `evokes:` phrase warmed (`Cache.Warm`, a pure lookup — no Ollama call, so
  this runs even when nothing is serving one locally), and does every
  `requires:` binary still resolve on `$PATH`. Each ask can go silently dead
  in a different way — a hook that stopped being registered, a block that
  no longer parses, a phrase added since the last `warm`, a tool that got
  uninstalled — and none of them announce themselves; the file that would
  have tripped the ask just never gets a question. Exits non-zero on any
  problem, the same convention `lint` already uses.
  Checks only the default settings path `install` resolves to (the
  `CLAUDE_SETTINGS` env var, else `~/.claude/settings.local.json`) — not
  every location Claude Code merges settings from. A custom install target
  or a project-local `settings.json` reads as "not found" here even when
  Claude Code is loading it fine. Scanning every location was the other
  option; not building it because nothing today is known to install
  anywhere but the default path, and it would have meant resolving a
  project root and reading up to four files for a case with no known user.
  If that changes, this is the first thing to widen.

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
