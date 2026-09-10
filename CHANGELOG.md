# Changelog

## 2026-09-10 (not yet in a tagged release)

- **`AUDIT.md`: an independent, adversarial pressure-test of
  `CUSTOM_EVAL.md`'s 36-scenario benchmark, run by two agents dispatched
  together with no visibility into each other's prompt or findings, against
  data already collected — zero new `claude_cli`/API calls. The headline
  numbers (0/72, 55/72, 70/72), call count, and cost all re-derive exactly
  from raw round data, and both named P4 misses are confirmed real. But the
  audit found and corrected three real problems in the write-up: (1)
  `check_preferences.py` was claimed "copied byte-identical from the
  canonical source" — false; no canonical source exists anywhere public to
  copy from, and the script is original, authored by the same hand and in
  the same commit as the rule prose it scores; (2) the P4-miss section
  attributed both losses to pure timing structure, when the round records'
  own decision logs show the model reasoned about the hook's exception
  clause and chose wrong, post-hoc, rather than never having a chance to
  react at all; (3) the reported 97.2%/76.4% gap concentrates almost
  entirely in 2 of the 5 nominal P1-P5 categories, with 3 of the 5 asks
  never exercising onsetter's actual content-matched delivery mechanism.
  Also fixed two smaller factual slips (a case-sensitivity bug attributed to
  the wrong condition, a six-vs-seven scenario undercount) and surfaced one
  unaddressed confound (the harness tells the model which experimental arm
  it's in, by name, every round). None of this changes the qualitative
  conclusion, and `CUSTOM_EVAL.md` now carries all of these caveats inline.
- **The audit's own P4 finding turned into two real fixes, mined from the
  same failure data rather than a bigger run.** Both P4 misses' decision
  logs show the identical mechanism: the model quotes the ask's own "unless
  this is example output being quoted verbatim inside a doc" exception
  verbatim to justify keeping a `print()` call in a snippet it just wrote
  itself — not quoted from anywhere, freshly authored. `ask/headers.md`'s
  "End with the out" authoring guidance now names this pitfall: an
  exception clause is only as reliable as "one sentence" when it asks the
  model to judge pre-existing state; asking it to judge content it's
  authoring in the same call is where the sentence becomes a place to
  rationalize instead of reconsider. The fix is `block: true`, since no
  wording of "unless" closes that gap. Then confirmed for real: `block: true` was wired onto
  the eval's own P4 ask in `trace_exp` and both affected scenarios were
  re-run end to end (8 real `claude_cli` calls, $1.73) — both previously-
  losing held-out rounds now pass. See `CUSTOM_EVAL.md`'s 2026-09-10
  update and `AUDIT.md`.
- **`CUSTOM_EVAL.md`'s named `removed:`/`untouched:` coverage gap: two new
  scenarios authored, a real pilot that measured nothing, then a redesign
  grounded in four directly-read sources.** First pass (`riverstone_analytics`
  for `R1`/`removed:`, `palisade_grid` for `R2`/`untouched:`) ran for real
  twice — 24 `claude_cli` calls each, $4.58 and $4.44 — and came back
  looking clean because `onsetter_native_cc` never actually engaged:
  `condition_diagnostics.onsetter_hook_events` reads `0` on every round of
  both scenarios in both runs (confirmed against `block_true_recheck`,
  where the same field reads `1` with real rule IDs). Root cause: the
  round text stated each rule directly, so the model got it right
  unprompted from round one and `observe_correction()` never recorded a
  violation for onsetter to act on — a bug in the eval, not a finding
  about onsetter. Redesigned against TRACE's own stated methodology
  (arXiv:2606.13174 §3.1, "the underlying preference is removed from each
  task prompt"), ClawArena's own paper (arXiv:2604.04202 §2.3, confirming
  its native preference dimension is purely stylistic and `R1`/`R2` test
  something it never covers), a minimal-edit-fidelity study
  (arXiv:2609.04061, over-editing under a real modification request is the
  norm, quantified) and a specification-gaming eval-design paper
  (arXiv:2605.02269 §3, a real "hacking opportunity" needs a structural
  reason the easy edit and the correct edit diverge). Two independent bugs
  fixed alongside it: `check_R2` hardcoded a single config key regardless
  of which one a round changed; `R1`'s `removed:` regex matched exact
  wording that the redesign's evolving bound legitimately changes round to
  round. Mechanically validated (`oracle_dry_run`, 24/24, no crashes)
  before any further spend.
- **A third `R1`/`R2` bug, caught live 8 rounds into the real pilot rather
  than after all 24.** `no_memory` failed `riverstone_analytics` q3/q4 with
  the guard still at q1's $500 bound, not q2's $1,000 — `train` and
  `id_test` turned out to be separate seeded workspace copies (confirmed by
  diffing `audit_export.py` between them mid-run), so code edits never
  cross that boundary, only a condition's own delivered correction *text*
  does. `R1`'s redesign had quietly assumed file state persisted the same
  way the original P1-P5 pack's stateless output-format preferences never
  needed to. Stopped the run at 8/24 ($1.53, well under the full $9) before
  it could finish on half-uninterpretable data; `q3`/`q4` now restate the $1,000 bound as a
  plain task fact, never the rule itself, the same pattern `vantorex_ops`
  already uses for its own per-round numbers. `palisade_grid` was never
  affected — every round sets an absolute value with no prior-round
  dependency. Full writeup in `CUSTOM_EVAL.md`; results pending the
  re-run.
- **The re-run (`r1r2_pilot_v4`, 24/24 real calls, $4.84) came back clean
  again — and mining its own transcripts found why, without a fifth real
  call.** `pref_pass: true` on every round, all three conditions, both
  scenarios, both adversarial `q4`s; `onsetter_native_cc`'s
  `compiled_rule_count` stayed `0` throughout, same as the first pass, but
  this time the redesign had already fixed the round-text-states-the-rule
  bug. Read all six `q4` transcripts' `decision_log` fields directly instead
  of taking the clean result at face value: every one names the exact reason
  it didn't drop the guard, in language lifted from the scenario's own
  design rationale — *"the harness freezes learned state at the train->eval
  boundary so no new rule should be introduced."* Root cause:
  `scenario.description` (`questions.json`'s `desc` field, written as
  documentation for a human, not the model under test) gets serialized
  straight into the prompt payload every round (`prompts.py:29`), alongside
  `condition.name` (`prompts.py:27`, `AUDIT.md`'s confound #3). Not scoped
  to the new scenarios — `vantorex_ops/questions.json`'s own `desc` carries
  the same "never introduce a new rule after the train split" language, part
  of the already-published 36-scenario pack. It didn't null that pack's
  result (P4 still shows real differential violations per `AUDIT.md`),
  plausibly because P4's violation mechanism doesn't map onto the leaked
  language the way R1/R2's does — but that leaves the 36-scenario headline's
  independence from this leak an open question, still to verify. Fixed:
  `prompts.py` no longer sends `scenario_description` or `condition`;
  confirmed at the payload-construction level (`build_round_prompt` called
  directly, grepped the rendered output — neither key present, no instance
  of "adversarial") and re-validated mechanically (`oracle_dry_run`, exit 0).
  A fresh real pilot under the fixed payload hasn't been run yet. Full
  writeup in `CUSTOM_EVAL.md` and `AUDIT.md`.
- **The fixed-payload pilot (`r1r2_pilot_v5_clean`, 24/24 real calls, $4.63)
  found the first real, harness-bug-free `R1`/`R2` violation — and, one
  round later, the first direct proof the compile-on-violation pipeline
  works.** `onsetter_native_cc`/`palisade_grid` q2 (train): the model ran
  the workspace's own `bump_threshold.sh`, noticed the paired rationale doc
  went stale, and deliberately left it per editing discipline — a real `R2`
  miss. `hook_events` stayed `0`, correctly: `compiled_rule_count` was still
  `0` at the moment of that call, since `OnsetterNativeCcCondition` only
  compiles an ask after `observe_correction()` records a violation — the
  first instance of any rule is a structural free pass. q3 (the next round,
  separate `id_test` workspace) is where it mattered: `onsetter_frozen:
  true`, and the model's own `rationale` quotes the compiled ask verbatim
  from `CLAUDE.md`, then complies. The correction crossed the train→id_test
  boundary and worked.
- **A verification pass on an unrelated claim ("first violation ever")
  surfaced a real, older error in this same write-up: the original "first
  pass" pilot wasn't actually clean.** Checking a routine talking point
  before restating it (rather than trusting the running tally) turned up
  three real `pref_pass: false` rounds in `r1r2_pilot_v2` — all
  `palisade_grid` q4/id_test, all three conditions — that the original
  write-up missed by reading the run's aggregate summary instead of
  querying `records.jsonl` directly. Root cause, per the losing round's own
  `rationale`: the identical train/id_test workspace-boundary bug later
  documented as "a third bug, found live" in the redesign pilot — except it
  was already real here, one full pilot earlier, just never investigated
  because a `hook_events: 0` read as "nothing to catch" instead of
  "onsetter couldn't see it." `CUSTOM_EVAL.md`'s "First pass" section now
  states this correctly. Total real spend across all five real R1/R2
  pilots to date: $4.58 + $4.44 + $1.53 + $4.84 + $4.63 = **$20.01**, not
  the $13.86 this write-up stated a few entries above before the check.
- **Spot-checking successes, not just failures, found a real checker gap:
  `R2`'s "does the doc match the config" check is trivially satisfied by
  not touching the file at all.** Reading every `task_pass: false` round's
  actual checker output in `r1r2_pilot_v5_clean` (instead of pattern-
  matching on round names against prior pilots) split six misses into two
  real mechanisms. `riverstone_analytics` q4 failing all three conditions
  is the familiar report-content miss. `palisade_grid` q4/q2 failing for
  `no_memory`/`prompt_always` is new: the checker output showed the config
  value never changed, and the transcripts show why — both conditions
  refused the edit outright, flagged as redefining compliance to hide an
  under-voltage reading, or a temperature ceiling raised with no bulletin
  citation. `R2` reads that as a pass, since nothing drifted from nothing
  changing. Three of the run's 24 `pref_pass: true` results are vacuous, no
  genuine hold behind them. `palisade_grid` q4, the adversarial round
  built for the `untouched:` gate, only got a real test from one of three
  conditions this run. This refusal pattern appears in zero rounds across
  the other four real pilots, which ran identical task text under the
  leaky payload and complied without objection — the `scenario_description`
  fix changed more than the leak it targeted. `CUSTOM_EVAL.md` now states
  the real mechanism instead of the earlier untested "maybe lost context"
  guess.
- **A sixth real pilot, scoped to just `riverstone_analytics` (12 calls,
  $2.39) after mining the free evidence first, confirmed `removed:` is
  0-for-36 real rounds across every pilot tried — not more bad luck, a real
  result.** Before spending on a full re-run, checked riverstone's actual
  round text for the same refusal-trigger pattern found in `palisade_grid`
  (none present — no "hide a compliance violation" framing) and grepped all
  432 rounds of the published 36-scenario pack for the `scenario_description`
  leak's citation pattern (found in 3%, and unlike R1/R2 it doesn't
  correlate with avoided violations there — real evidence against spending
  $70-100 re-running that pack blind). The riverstone-only run's own `q4`
  transcript shows a genuine, verified attempt (re-derived totals from the
  raw CSV, not a refusal or vacuous pass) that still kept the guard —
  real model-behavior evidence, not mechanism evidence, since
  `hook_events` stayed `0` for the same reason as every prior round: zero
  violations means `OnsetterNativeCcCondition` never had an ask to compile,
  so the actual `removed:` question (survives a `Write`, or only an `Edit`)
  has never been reached. Total real spend across all six R1/R2 pilots:
  **$22.40**. Full result and the design implication (seed an already-
  compromised guard next time, rather than hoping a violation emerges from
  an extension task) in `CUSTOM_EVAL.md`.

## v0.10.3 — 2026-09-10

- **Closed the paper trail on `CUSTOM_EVAL.md`'s two real misses.**
  `block: true` (shipped in `v0.10.0`, commit `0e427f2`) targets exactly
  the failure shape both misses hit — a violation embedded in the same
  `Write` call that creates the file, with no later tool call for an
  advisory-only hook to land a correction on — but the eval write-up
  still described the gap as unaddressed. Confirmed directly, at zero API
  cost: driving `onsetter hook` with a `Write` payload that creates a
  brand-new file embedding `DEBUG = True`, against the shared eval
  fixture's `added:`/`block: true` ask, returns `permissionDecision:
  "deny"`. `CUSTOM_EVAL.md` now records this; a new local eval case,
  `evals/block-denies-write-embed/`, reproduces it alongside the existing
  `Edit`-only `block-forces-retry/` case; `ask/headers.md`'s `block:`
  section now cites the misses as the concrete case that motivated it.
  The external `trace_exp` harness itself has not been re-run with
  `block: true` wired onto the P4 ask, so this is a mechanical proof the
  gate closes the gap, not a re-measured score.

## v0.10.2 — 2026-09-09

- **Two real bugs left the plugin non-functional for every install since
  v0.10.0, found by actually running `/plugin marketplace add` +
  `/plugin install` for the first time, not just testing the raw binary.**
  `.mcp.json`'s `${CLAUDE_PLUGIN_ROOT}` in the command field hit a
  documented Claude Code platform bug: template substitution is confirmed
  to work on a session's first connect and to break on reconnect, spawning
  a literal unexpanded placeholder (tracked upstream at
  anthropics/claude-code#65747 and #67483, both closed without a fix). A
  fresh plugin install hit it immediately, not just a later reconnect.
  Fixed by spawning through `bash -c` and reading CLAUDE_PLUGIN_ROOT and
  CLAUDE_PLUGIN_DATA from the real process environment instead of Claude
  Code's own JSON-string substitution — confirmed those env vars are
  genuinely present by dumping a real spawned process's own environment,
  not assumed from the docs. Separately, `scripts/checksums-pin.txt` was
  never updated past v0.9.1, so `fetch.sh` correctly refused every
  v0.10.0/v0.10.1 install as an unpinned release rather than trusting an
  unverified binary — real security behavior working exactly as designed,
  just never fed the input it needed. `scripts/pin-checksums.sh` run for
  both versions.

- **`.mcp.json`'s command now uses an absolute `/bin/bash` instead of bare
  `bash`**, hardening against a PATH-related spawn failure Claude Code
  might apply to plugin-provided servers. Also worth recording: verifying
  the plugin connection from inside onsetter's own repo checkout is
  structurally unreliable, independent of anything fixed here — this
  directory's own `.mcp.json` auto-loads as a project-scoped MCP server,
  and a project-scoped server can never receive `CLAUDE_PLUGIN_ROOT` or
  `CLAUDE_PLUGIN_DATA`, which are plugin-only variables, so it fails by
  design every time. Both servers share the name `onsetter`, and Claude
  Code shows only one entry for it, so the always-failing project-scoped
  connection silently masked a working plugin-scoped one for most of this
  investigation. Confirmed the plugin genuinely connects
  (`plugin:onsetter:onsetter` — connected, 1 tool) only by checking `/mcp`
  from a different directory.

  **Update, 2026-09-10: the same root cause now surfaces louder.** What
  used to read as silent masking now shows a `PreToolUse:Bash hook error:
  MCP server 'plugin:onsetter:onsetter' not connected` banner on every
  Bash/Write/Edit/Read call inside this checkout — Claude Code's own
  reporting got more visible, not the underlying mechanism. Confirmed
  directly: the project-scoped failure and the plugin-scoped connection
  are both unchanged, `dispatch()` behaves identically through either the
  CLI (`onsetter hook`) or the MCP `hook` tool, and the hook still fails
  open. Considered and rejected dropping the MCP server for a
  `command`-type hook to eliminate the name collision entirely — MCP
  exists for a real, measured ~20x per-call win over a fresh process spawn
  (~0.6-0.9ms vs. ~15-17ms, `go test -bench` against a 150-ask fixture —
  see the entry below introducing `onsetter serve`), and dropping it would
  trade that win, for every user of the plugin, to quiet a banner that
  only appears in this repo's own dev loop. Left as-is: cosmetic, not
  worth the regression.

## v0.10.1 — 2026-09-09

- **`evokes:` was not firing in the live hook path on anything but the
  fastest machines.** `internal/embed.DefaultBudget` (200ms) was an
  unvalidated placeholder. Measured for real: a warm embed call on a
  loaded local host (a dozen concurrent Claude sessions competing for a
  fixed memory budget) ranged 120-700ms, and a forced-cold call measured
  10.3s — both far past 200ms, so the flagship semantic-match feature was
  silently failing on every real call in testing, not a rare slow one. A
  second measurement on a clean, single-tenant CPU-only host held warm
  calls at 80-90ms and a forced-cold call at 0.35s, confirming the wide
  range comes from host contention rather than anything `evokes:` itself
  does. `DefaultBudget` is now 1500ms: clears the loaded host's warm-case
  max with real margin and the clean host's numbers by 5-15x, while
  staying far short of either host's cold-start cost — a cold model
  still degrades to "does not fire" exactly as designed. Verified live:
  10/10 hook calls against this repo's own `evokes:` ask fired correctly
  after the fix, where every call failed before it.

## v0.10.0 — 2026-09-09

- **Tightened every file and directory onsetter writes to owner-only,
  self-healing on the next write rather than only at creation.**
  `os.MkdirAll`/`os.WriteFile` only apply their mode bit when they create a
  path; `gosec` found 12 production call sites writing group/world-readable
  state under `~/.cache/onsetter` and `~/.claude/skills`, and the naive fix
  — just tightening the literal — would have silently done nothing for
  every install that already ran once, this machine included:
  `~/.cache/onsetter/{asks,sessions}` and
  `~/.cache/onsetter/{asks.json,embeddings.json}` were still sitting at
  `0755`/`0644` when checked. New `internal/secfile` package narrows a
  path's mode toward the target by intersecting bits rather than
  overwriting them, proven by a regression test that a deliberately
  restricted directory never gets widened back. `gosec` is now part of the
  lint gate (`G304`/`G703` excluded as low-value for this tool's own trust
  boundary, two `G115` hits on `unix.Flock` silenced with scoped `//nolint`
  comments rather than a blanket exclude), every GitHub Action in
  `ci.yml`/`release.yml` is pinned to a commit SHA instead of a moving tag,
  and `ci.yml`'s Go version now tracks `go.mod` instead of `stable`.

- **README's opening example swapped for a real ask from a different,
  private project, not onsetter's own — plus a full pass for public
  readability.** The prior example (reachability from `onsetter hook`'s own
  entrypoint) was a solved static-analysis problem, undercutting the
  argument it was meant to demonstrate; the replacement is a `when:`-gated
  dead-state-write check whose resolution genuinely isn't mechanizable — the
  read side is a string-keyed template lookup rather than a Go call site,
  invisible to anything a compiler can trace — and its injected-context
  block is captured from a
  real `onsetter hook` run, not hand-written. A cold read surfaced three
  more gaps, all fixed: the table of contents was missing 5 of 13 real
  sections, including the one the opening paragraph explicitly links to;
  the repeat marker (`asked 2× already this session`) was described in
  prose but never shown as real captured output; and the closing vignette
  still used the personified-note narrative device the rest of the rewrite
  had deliberately dropped. Also swept every touched `.md` file through
  `cope-gate` and fixed ten real instances of the not-A-but-B flip
  construction, leaving the deliberate parallel structures (SECURITY.md's
  "What it reads"/"writes"/"sends"/"emits") and one verbatim third-party
  quote untouched. States plainly that onsetter is Linux/macOS only and
  that `evokes:` is the one feature needing anything beyond onsetter
  itself.

- **Authored a real `claude plugin eval` suite under `evals/` — five cases
  exercising onsetter's own mechanism through Claude Code's actual plugin
  wiring, not the Go binary in isolation — but it has not run.** `claude
  plugin eval` is early access as of Claude Code 2.1 (September 2026),
  confirmed directly: `--help` works and shows a real CLI, but even `claude
  plugin eval init --bare <name>` — the scaffold command that would have
  produced a confirmed-correct template — exits 1 with "plugin eval is
  currently in early access," and there's no public docs page for the
  feature yet either (checked code.claude.com's docs index directly). The
  file format the suite is written against is therefore a recalled
  early-access reference, not something fetched from a primary source or
  checked against a real scaffold — `evals/README.md` names the specific
  structural risks this leaves (`case.yaml`+`prompt.md` coexistence chief
  among them) so whoever runs this first knows exactly what to check if a
  case's shape is wrong.

  What *is* verified, for real, against onsetter's own CLI, not assumed:
  every grader's regex matches onsetter's own literal injected text,
  captured from a real `onsetter hook` call fed the actual nested Claude
  Code payload shape (`tool_input.file_path`/`content`, not a flat one — an
  assumption that failed silently, empty stdout, until traced to
  `cmd/onsetter/hook.go`'s `payload` struct). The `evokes:` case's fixture
  sentence was calibrated with `onsetter calib` against a real 5-positive/
  3-negative corpus (scores 0.498–0.586 against the 0.48 default threshold)
  and confirmed live that a markdown heading above the same sentence drops
  it below threshold — headers.md's documented dilution caveat, reproduced,
  not just cited — which is why that case's prompt appends a bare sentence
  to an existing file rather than writing a fresh one.

- **New `fires-on:`/`silent-on:` headers, germline's own feature request,
  traced to the root cause their `CHANGELOG.md` names directly: "A gate has
  two independent ways to be dark. Fixing one leaves the measurement looking
  identical."** `onsetter replay`'s rate reads the same — low, or zero —
  whether a corpus is genuinely clean or a gate is dead: germline hit this
  twice in one day, once from a broken anchoring regex (`when: ^TODO`
  anchors to the start of the whole file without `(?m)`, never a line three
  deep) and once from a glob narrower than the corpus it meant to cover.
  Both headers point at a real file whose current content is a known
  example; `onsetter lint` builds the same on-disk synthetic edit `list` and
  `replay` already construct and asserts `Match` agrees — every `fires-on:`
  file must fire, every `silent-on:` file must not, a mismatch is a lint
  failure naming the exact fixture rather than a rate to eyeball. Neither
  header gates Match, so neither is part of `ID()` — the same reasoning that
  already excludes `name:`. `removed:` has no fixture-testable form: a
  single file's on-disk content has no diff to remove a line from, so an ask
  carrying `removed:` is skipped by the check with a note printed, never a
  silent false pass or fail. Eighteenth header, same four-place checklist as
  `block:`.

  **Found and fixed a real regression while building this**, not a new-code
  bug: `internal/discover/cache.go`'s on-disk shard cache (`askSnapshot`)
  never gained `Block`, `FiresOn`, or `SilentOn` fields when `block:` shipped
  — every other field round-trips through a cache hit, those three silently
  reset to their zero value. `onsetter lint` walks every source file through
  `discover.ParseSource` twice in a row (once from `discover.Sources` probing
  for asks, once from its own loop), so the second call was always a cache
  hit serving a snapshot missing the new fields — the first real test written
  against `fires-on:` failed with `FiresOn` silently empty, traced to this,
  not a parser bug. The same gap meant a `block: true` ask could silently
  stop denying anything the moment its parse got cache-restored — the
  persistent `onsetter serve` process, `internal/discover.Warm`'s own
  fallback path on a memory-miss. `TestCacheRoundTripsAllHeaderKinds` now
  asserts `Block`, `FiresOn`, and `SilentOn` explicitly, so a future header
  added to `Ask` without a matching `askSnapshot` field fails a test instead
  of failing silently on a warm cache.

- **New `block: true` header, closing the gap `CUSTOM_EVAL.md` and
  `IDEAS.md` named directly: a `PreToolUse` hook can't correct the same
  `Write` or `Edit` call that trips it, only a later one, because the model
  has already committed to the tool call's exact arguments by the time the
  hook runs.** Scoped to `added:`/`removed:` on purpose and enforced at
  parse time — `block: true` with no `added:`/`removed:` on the same block
  now fails to parse, where it used to sit there silently inert — because
  those are the two gates that hand back the edit's own literal text rather
  than a path glob or a fuzzy phrase, so a denial always points at something
  concrete.
  A matched, uncued firing sets `hookSpecificOutput.permissionDecision:
  "deny"` with the ask's prose as `permissionDecisionReason`;
  `additionalContext` still carries every matched ask's prose in the same
  batch, blocking and advisory together, nothing dropped. A firing reached
  only through `cues:` never denies — `Cascade` never checks a cued ask's
  own gate, so it never has a quote to justify a denial with, the same
  reasoning `has:`-only and reminder asks already fall under. `ID()` now
  hashes `Block` the same way it already hashes `Always`. This is the
  sixteenth header, following the four-place checklist an earlier ask in
  this repo's own `CLAUDE.md` names for adding one: the `ask/ask.go` switch,
  the `Headers` slice, a section plus table row in `ask/headers.md`, and the
  header list in `ask/skill.md`'s frontmatter — plus a pass through
  `README.md` and `SECURITY.md`, both of which had stated plainly, more
  than once, that onsetter never blocks. That statement was true until this
  change and is now an overclaim everywhere it appeared unqualified; fixed
  in place rather than left to rot, since the whole design leans on that
  claim being exactly true, not approximately true.

- **First real `onsetter calib` run, against onsetter's own first `evokes:`
  ask.** Onsetter's own `CLAUDE.md` had zero `evokes:` asks until now —
  both existing blocks are `when:`-gated — so there was nothing for
  `calib` to measure and no evidence behind `DefaultThreshold = 0.48`
  beyond the two hand-picked pairs its own doc comment already admits to.
  Wrote a real one (`CLAUDE.md:51`, gated on `in: **/*.md`, catching a
  shipped-without-verification claim) and calibrated it three times against
  hand-built 5-positive/5-negative corpora, rewording the `evokes:` phrase
  each time per `calib`'s own stated remedy for an overlap. All three
  overlapped at the 0.48 default — no threshold separated every example in
  any of the three runs — and each failure had a specific, diagnosable
  cause: the first phrase asked for a citation-*presence* distinction (closer to a
  `when:`-shaped lexical feature than a topic), the second asked for a
  citation-*source-identity* distinction (which entity is being cited,
  world-knowledge no local embedding model encodes), and the third — a
  concrete action, phrased the way the README's own worked example is —
  still landed `unrelated-instruction.md` above 3 of 5 genuine positives.
  Real, repeated finding: `nomic-embed-text`'s document-level embedding has
  thin resolution for fine-grained epistemic/behavioral distinctions *within*
  a narrow, uniform-register domain (short technical software-engineering
  markdown), even when it separates broad topics fine — the false-fire rate
  at threshold 0.48 was 5/5 negatives in two of the three runs. Shipped the
  third phrasing anyway: onsetter's own stated design ("Why a question and
  not a check") treats a wrong `evokes:` firing as costing one sentence
  rather than breaking a build, so an imperfectly-calibrated reminder sits
  inside the tolerance the whole project is built around — and the real
  numbers are recorded here rather than a first-draft threshold quietly
  standing in for evidence that was never actually gathered.
  `IDEAS.md`'s "evokes:/warm/calib subsystem is onsetter's weakest,
  least-calibrated part" entry is retired: it asked for exactly this
  investigation pass, and this is what it found.

- **The plugin now runs the hook as a persistent MCP server, `onsetter
  serve`, instead of a fresh process per call.** Built an on-disk ask-parse
  cache first, measured it, and found it didn't touch the real cost: a bare
  `onsetter hook` invocation has a ~5-8ms process-spawn floor underneath the
  ~2.6ms parse cost of even a 150-ask `CLAUDE.md`, and no on-disk cache
  removes a cost that exists before the cache file can be opened. Real
  numbers, `go test -bench` against that same 150-ask fixture, real
  subprocess client either way: ~15-17ms per call spawning fresh each time
  vs. ~0.6-0.9ms per call over one persistent connection — roughly 20x.
  `hooks/hooks.json`'s plugin-path `PreToolUse` entry moves from `type:
  "command"` to `type: "mcp_tool"`, wiring `Read` unconditionally in the
  same change (the cost that made it opt-in doesn't apply to a warm
  connection). `cmd/onsetter/hook.go`'s dispatch logic split into a
  transport-agnostic `dispatch()` shared by the CLI path (still `os.Exit(0)`
  on a panic, unchanged) and the new MCP tool handler (`mcp-go`'s
  `WithRecovery`, confirmed live to matter: without it, a panicking call
  doesn't crash the process, it hangs forever with no response ever
  written — worse than a clean exit, not just different from one). The
  on-disk shard cache stays — real, sharding fixed a genuine bug where the
  first design decoded every cached repo's data on every single lookup —
  as the warm-across-processes fallback under `internal/discover.Warm`'s
  new in-memory layer, and as what every one-shot CLI subcommand still
  benefits from. The manual, non-plugin install (`onsetter install`) is
  unchanged: still `type: "command"`, still `--read` opt-in, since that
  path is still genuinely one process per call.

- **Adds `EVAL.md`**, the first real attempt to test onsetter's own untested
  claim — that delivery at the tool call beats delivery at prompt time — by
  extending TRACE's own public ClawArena harness with a sixth condition
  rather than building a parallel eval nobody could compare against their
  table. The condition itself works: verified through `onsetter lint`, a
  direct wrapper-firing test, a full zero-cost dry run, and 16 real
  subscription-auth calls with zero errors. What it found instead: only 12
  of the paper's 62 scenarios are public, only ~7% of their rounds carry a
  violation onsetter can even address, and the paper's own published
  numbers come from a mechanism that blocks completion — which isn't
  onsetter's mechanism and isn't even what the public harness code ships as
  "TRACE." Not a null result; the instrument was the wrong one to test the
  claim against. README's TRACE citation now points to it.

- **Adds `CUSTOM_EVAL.md`**, the follow-up that ran the comparison `EVAL.md`
  couldn't: a purpose-built, 36-scenario pack where every round carries a
  real, onsetter-addressable check, plus a third condition (`prompt_always`)
  that holds rule content identical to onsetter's own and varies only the
  channel — always in context versus delivered at the tool call. 432 real
  calls, $73.32. On 72 held-out paired rounds: onsetter 97.2% vs.
  `prompt_always` 76.4% vs. no-memory 0%; onsetter vs. `prompt_always`
  (the actual novel comparison) at 19 discordant pairs, p ≈ 0.00073. Also
  fixes a real confound found before any data was collected — onsetter's
  condition was leaking its own rule text into Claude Code's native
  `CLAUDE.md` auto-load, a channel the harness's own prompt payload never
  touched or accounted for — and names two genuine `onsetter_native_cc`
  misses plainly: a `PreToolUse` hook can't correct the same `Write` call
  that trips it, only a later one, so a violation buried inside a one-shot
  report generation structurally can't self-correct within that round.

- **`added:`/`removed:` no longer diff an unbounded edit.** `ask/ask.go`'s
  `diffLines` called `myers.ComputeEdits` on the full `old`/`new` text with
  no size check — found by a fresh research pass asking "does this actually
  work," which measured cost rather than assuming it: a 4,000-line edit cost
  1.1GB RSS, a 20,000-line edit got OOM-killed at 8.7GB before its own 120s
  timeout fired. Neither a panic `recover()` catches, since a Go OOM is a
  runtime throw rather than one — the one path that could defeat the hook's own
  "fail open" invariant. `maxDiffBytes` (100,000, an order of magnitude
  under the first measured danger point) now gates the diff; past it, the
  ask degrades to "does not fire," the same convention `evokes:` already
  uses when Ollama's unreachable. New test proves the cap fires and stays
  quiet at both sides of the boundary without needing an edit anywhere near
  the size that made it necessary.
- **`added:`/`removed:` now anchor per line, not per block.** The diff's
  matched lines are joined into one string before the pattern runs; without
  `(?m)`, a bare `^`/`$` anchored to the start and end of that whole join,
  not each line — a real user reported one ask that fired only when its
  match happened to land on the first added line, and a sibling
  (`removed: ^func (Check|Law|Prop|Test)`) that could never fire at all,
  since a removed function is essentially never the first deleted line of a
  hunk. Both headers now compile with `(?m)` unconditionally; writing it
  yourself compiles fine too, redundant flags being harmless in Go's regexp
  syntax. `ask/headers.md` names the anchor behavior directly instead of
  leaving it to be discovered.
- **README prior-art section names Anthropic's own `security-guidance`
  plugin**, which ships enabled by default in Claude Code and already gates
  `PostToolUse` on path and pending content with advisory
  `additionalContext` — same shape, `PostToolUse` instead of `PreToolUse`.
  Verified from its actual source this session, not assumed. Also adds a
  short table of contents after the opening story, so a reader deciding
  whether to install isn't required to scroll past the full header
  reference to reach `Commands`/`Design`/`Where it sits`.

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
  `git add -A` from landing in a commit: conversation-trace prose (a
  first-person aside naming who it was written for), a competitor's star
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
  recurs. An occurrence is the quote together with the edit around it:
  `"you nod"` in `betty.md` and `"you nod"` in
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
  a rule about a read rather than a write. `on:` is a strict four-way partition
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
  rewording the phrases rather than changing the number. Built after a cross-project
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
  hand-edit does not survive one. Adding a header now touches four places, up
  from three — the fourth is the header list in the skill's frontmatter, which is
  the entire retrieval surface, so a header missing there is a header whose
  questions never reach the guide. The test names the exact list.

- `replay` marks any row whose rate does not measure the gate the author cares
  about. It builds synthetic edits from files on disk, so there is no old text:
  `added:` degrades to `when:` and `removed:` never fires. A real
  `added: "aliases"` printed 100.0% while gating on nothing replay can see, and
  the number was silently about the ask's `in:` instead. The row now says so and
  points at `onsetter hook`, which is the honest check.

  This doesn't build `replay --since <rev>` to fix it properly: across every
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
  unchanged. A content gate means it is asking about a specific string, and
  *is this narrator overreach* means something different for `you nod` than
  for `you find yourself`. The rule is derived rather than configured, and
  there is no new header.

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

The first release with a changelog: `v0.1.0` was cut fifteen commits earlier,
before most of the header set existed, and has no entry of its own —
everything below covers the tool as it stands.

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
