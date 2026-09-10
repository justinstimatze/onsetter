# A denser eval: does onsetter's timing beat prompt-time, holding content constant

[`EVAL.md`](EVAL.md) tried to answer this against TRACE's own public ClawArena
harness and found the wrong instrument: only 7% of that dataset's rounds carry
a violation onsetter can even see, and the paper's own headline numbers come
from a mechanism that blocks completion, which onsetter doesn't. This is the
follow-up that fixes both problems — a purpose-built scenario pack where every
round is addressable, and a third condition that isolates timing as the only
variable, holding rule content identical across it and onsetter's own.

## The three arms

Every scenario runs the same rule content through three channels:

- **`no_memory`** — baseline. The rule never gets delivered at all.
- **`prompt_always`** — the missing control `EVAL.md` didn't have. Same rule
  text as onsetter's own arm, delivered as plain prose into a scoped
  `CLAUDE.md`, present on every round from the round it's first triggered
  onward — always visible, never gated on a tool call.
- **`onsetter_native_cc`** — the same rule content, delivered only through
  onsetter's real `PreToolUse` hook, at the matching `Write`/`Edit` call.

Both informed arms trigger from the same event — a round's preference check
fails, the violated rule gets extracted from the real correction text, and
each condition renders it through its own channel from that point forward.
The only difference between them is which channel carries identical content;
everything else about the harness, the scoring, and the round structure is
shared.

## A leak, found and fixed before anything was measured

Before authoring any new data, a plan-validation pass read
`OnsetterNativeCcCondition`'s actual code path rather than trusting its own
docstring, and found a real confound: it wrote its managed `ask` block into
the workspace's *root* `CLAUDE.md`. Claude Code auto-loads that file
independently of the harness's own JSON prompt payload — so the condition's
claim that "the only channel is the real hook" was true of what the harness
controlled and false of what Claude Code did on its own. Zero violations
occurred in `EVAL.md`'s original 16-call pilot, so this never actually fired
in practice, but the moment real violations started happening — the entire
point of this eval — onsetter's own condition would have started leaking its
rule text into the prompt on every later round, contaminating the exact
variable under test.

Fixed by relocating the managed-block target to a scoped `docs/.claude/CLAUDE.md`
(onsetter's own documented best practice — put the ask closest to the files
it governs). Claude Code doesn't proactively load a descendant directory's
`CLAUDE.md` the way onsetter's own tree-walk does; onsetter still finds and
gates on it, Claude Code just stops handing it over for free. Verified with
one real call before and after the fix, not by reasoning alone
(`trace_exp` commit `fdd9f02`).

## The scenario pack

36 freshly-authored scenarios, original fictional companies, zero overlap
with TRACE's real `hil_*` content
(`trace_exp` commits `bb128e1` and `19f2718`).
Each scenario reuses TRACE's own P1–P5 preference *labels* — number
formatting, dated filenames, report structure, no debug prints, cited
sources — but `check_preferences.py` is not copied from any canonical
source: no public `hil_*` scenario's checker is vendored anywhere upstream
to copy from. It's an original script, authored in the same commit and by
the same hand as the rule prose it scores, deliberately kept in lockstep
with onsetter's own `_RULE_ASKS` translations. That's a fair instrument for
asking whether delivery channel affects retention of a given rule, but it
is not independently-sourced ground truth external to the treatment being
tested — see `AUDIT.md`. Every round in every scenario carries a real
preference check; none of the ~93% dead weight `EVAL.md` ran into. Two train
rounds establish a rule per scenario, two held-out rounds test whether it
stuck.

Two mechanical bugs surfaced while building this, worth naming honestly:

**Cross-scenario state bleed.** `multi_runner.py` builds each condition
object once per *invocation*, not once per *scenario* — so passing several
scenarios to one run let the informed conditions carry corrections from an
earlier scenario into a later one's supposedly-clean first round, silently
reproducing a mkdir-related leniency bug that had already been fixed once.
Not a code defect; a harness invocation pattern to know about if you extend
this: run each scenario as its own process.

**Two scenario-authoring bugs**, both confirmed to leave the preference-check
metric this eval actually depends on untouched. Seven scenarios' final round
asked the model to assume "this month's rate continues," on the mistaken
assumption it could recall an earlier round's stated number from a workspace
the held-out split never actually shares files with — all three conditions
correctly reported the number as missing rather than guessing, which broke
the literal task-correctness check identically across all three, not the
preference check. One scenario's CSV used lowercase category names against
Title Case model prose, so a case-sensitive match happened to pass the two
informed conditions by accident, on their code snippets quoting the raw CSV
value, and correctly failed `no_memory`, which had nothing but its own
Title-Case prose to draw the category name from. Both fixed prospectively in
the scenario batch that followed rather than by re-running what had already
run cleanly on the metric that matters.

## The real numbers

36 scenarios, 432 real `claude_cli` calls, $73.32 total (measured, not
list-price). 72 held-out paired rounds — the only ones that test retention
rather than the initial correction itself:

| condition | held-out compliance |
|---|---|
| `no_memory` | 0/72 (0%) |
| `prompt_always` | 55/72 (76.4%) |
| `onsetter_native_cc` | 70/72 (97.2%) |

Paired by (scenario, round), not just compared as marginal totals:

- **`onsetter_native_cc` vs. `no_memory`** — 70 of 72 pairs discordant, all
  70 favoring onsetter. Sign test p ≈ 2.4×10⁻¹³.
- **`onsetter_native_cc` vs. `prompt_always`** — the actual novel comparison,
  the one nothing had run before this. 19 discordant pairs (17 favor
  onsetter, 2 favor `prompt_always`). Sign test p ≈ 0.00073. One pair short
  of the 20–30 originally targeted going in — reported plainly rather than
  padded to round it up — but the effect is already three orders of
  magnitude past the significance bar, so the shortfall changes nothing
  about the conclusion.

Same content, different channel: at the tool call beats always-in-context, on
held-out rounds, on this rule family.

**Read "P1-P5" as five labels, not five equal contributors.** Broken down by
rule, the 15-round gap between conditions concentrates almost entirely in
two of the five: P4 (7/20 vs. 18/20) accounts for 11 of the 15 rounds, P5
(5/8 vs. 8/8) for 3 more. P1 and P3 are already near or at ceiling for both
conditions (P3 16/16 vs. 16/16). Separately, three of the five asks (P2, P3,
P5) gate on `on: mint` with no content trigger, so for those onsetter's hook
fires as "once per new file in this directory" rather than exercising the
content-matched delivery that's the actual mechanism under test. The
defensible claim is narrower than the P1-P5 framing suggests: tool-call-time
delivery beats always-visible prompting specifically for avoiding `print()`
in a report-embedded code snippet, with a smaller citation effect, on a
taxonomy where two categories are already saturated for both channels. See
`AUDIT.md` for the full per-rule breakdown.

## Two real misses, and what they actually are

Both of onsetter's two losses in the `prompt_always` comparison are the same
failure, confirmed independently in two unrelated scenarios. Both are P4 (no
`print(...)`) violations. Both happen when the model writes an entire report
in a single `Write` call that includes an illustrative Python snippet with an
inline `print(...)` debug line. In one scenario, this happened twice
back-to-back — a train round violates P4, the correction gets recorded, and
the very next P4-gated round violates it identically.

`hook_events_delta` confirms the hook fired, both times — this isn't a
missed delivery. But it isn't purely structural either: the round records'
own `decision_log` fields show the model saw the hook's advisory text and
reasoned about it, quoting the ask's own exception clause verbatim —
*"the hook's own stated exception ('unless this is example output being
quoted verbatim inside a doc') applies... so no change was needed there"* —
and judged wrong. In `vantorex_ops` this happened across two separate
`Write` calls, the model re-affirming the same mistaken call the second
time. `prompt_always`, seeing identical prose *before* writing anything,
reasoned about the same exception correctly and avoided the violation from
the start. So the real effect looks like *when* the model reasons about an
ambiguous exception clause — upfront, or after already committing to the
write that embeds the violation — not that a correction categorically
cannot reach the model in time. It's still a real gap: for a `Write`,
onsetter's diff engine treats the entire new file as "added," so the gate
correctly matches content anywhere in it — but onsetter's hook only ever
emits advisory context, never a block, and `PreToolUse` fires after the
model has already committed to that Write call's exact arguments, so a
*correct* re-reasoning could only land on a future tool call, never the one
that tripped it. When the violation and the report are the same single
write, and nothing else happens in that round, there's no later moment for
a correction to reach — but here, the model didn't need a later moment, it
needed to read its own hook text more carefully the first time. This is a
real, honest limit on onsetter's own hypothesis, not softened: an advisory
hook fired at the tool call still can't out-run a
one-shot generation that buries the violation inside itself.

**Update, 2026-09-10: this gap has a fix, mechanically confirmed and now
re-measured.** `block: true` (`ask/ask.go`, commit `0e427f2`) shipped six
hours after this eval's own commit, built for exactly this shape — an
opt-in deny on an `added:`/`removed:` match, closing the one-call-too-late
window described above. First confirmed at zero API cost: driving
`onsetter hook` with a `Write` `PreToolUse` payload that creates a
brand-new file whose content embeds `DEBUG = True` in the same call,
against the real shared eval fixture's `added: DEBUG\s*=\s*True` /
`block: true` ask, returns `permissionDecision: "deny"` — see
`evals/block-denies-write-embed/` for that local, free reproduction.

Then re-measured for real: `block: true` was wired onto the P4 ask in
`trace_exp` (`conditions.py`'s `_RULE_ASKS["P4"]`), and both affected
scenarios' `onsetter_native_cc` arm was re-run end to end — train rounds
included, so the correction is discovered fresh rather than seeded — 8 real
`claude_cli` calls, $1.73. Both held-out rounds that lost before now pass:
`vantorex_ops` q3 and `basaltridge_mining` q3 both show `P4: PASSED (no
print(...) calls)`, where they previously showed the exact miss this
section describes. The train-round P4 violation still happens once per
scenario, as expected — that's the first exposure, before any P4 ask
exists in the workspace's `CLAUDE.md` to block against — `block: true` only
starts acting once the rule is live, which is precisely the held-out rounds
this eval measures retention on. This sits outside the 70/72 headline
above — a targeted 2-scenario recheck of the two known losses — and it
confirms the fix closes both real misses, past the mechanically-constructed
reproduction.

## An unaddressed confound: the model is told which arm it's in

The harness embeds `condition.name` directly into the JSON payload sent to
the model every round — the literal string `"no_memory"`, `"prompt_always"`,
or `"onsetter_native_cc"`. Onsetter is a real, public tool, and Sonnet 5's
training data postdates onsetter's public existence, so the model could in
principle be recognizing the string `"onsetter_native_cc"` on top of, or
instead of, reacting purely to the hook-delivered content. Not proven, but
real and easily avoided — condition names could have been anonymized — and
not something this write-up checked before now. See `AUDIT.md`.

## What this deliberately does not cover

Reuses TRACE's own P1–P5 taxonomy; onsetter's `removed:`, `untouched:`, and
`has:`+`when:` gate shapes go untested here — a real, separate
feature-coverage gap, named plainly
(`removed:`/`untouched:` is addressed below). No
cross-vendor model panel: onsetter's mechanism is a Claude Code `PreToolUse`
hook by construction, so there's no equivalent surface on other agent tooling
to even ask the question against. Onsetter's own source was never touched —
this is external eval infrastructure and a results write-up, same as
`EVAL.md`.

## Extending coverage: `removed:`/`untouched:`, adversarially

Two new scenarios, `riverstone_analytics` (rule `R1`, `removed:`) and
`palisade_grid` (rule `R2`, `untouched:`) — `R1`/`R2` rather than `P6`/`P7`,
since `P1`-`P5` are TRACE's own labels reused with onsetter-authored checks
behind them; these two are onsetter's own additions and the numbering says
so. Same 2-train/2-held-out structure as the 36-scenario pack, same
`conditions.py` machinery (`experiments/clawarena/clawarena/conditions.py`
in `trace_exp`), narrower scope on purpose: validate the mechanism on 2
scenarios before deciding whether to scale, the same pilot-then-scale
discipline this pack's own `pilot3_real` → `pilot8_real` → `pilot25_real`
progression already used.

### First pass: a real pilot that measured nothing — mostly

The first design (2 scenarios, single-value edits, the rule stated directly
in the round text — "update X, and make sure the doc reflects it") ran for
real twice, 24 `claude_cli` calls each, $4.58 (`r1r2_pilot`) and $4.44
(`r1r2_pilot_v2`). **Correction, found while verifying a later claim in this
same section rather than assumed from the run's own summary:** the original
write-up here said both runs came back clean with zero violations. Only the
first one did. `r1r2_pilot_v2` had three real `pref_pass: false` rounds —
`no_memory`, `prompt_always`, and `onsetter_native_cc`, all on
`palisade_grid` `q4`/`id_test` — that the original write-up missed by
trusting the run's aggregate summary instead of querying `records.jsonl`
directly for `pref_pass == false`.

Reading why: the round record's own `rationale` for the `onsetter_native_cc`
loss states it directly — *"No compiled_enforcement_rules were present for
this session and the PreToolUse hook... produced no injected output or
block on the Bash call (checked runtime/hook_fired.jsonl afterward — file
doesn't exist, so nothing fired), so no additional retained rule from the
train rounds applied."* That's the identical train/id_test workspace-
boundary bug documented below as "a third bug, found live" in the
*redesign's* pilot — except it was already real and already causing losses
here, one full pilot earlier, just never investigated at the time because
the run's headline `condition_diagnostics.onsetter_hook_events: 0` read as
"nothing to catch," when the truth was "onsetter couldn't see it." Checked
against a real comparison (`block_true_recheck`, where the same field reads
`1` with real `rule_ids_observed`), confirming the field genuinely tracks
firing and isn't vestigial. Still, a `0` is consistent with *either* "no
violation occurred" *or* "a violation occurred and the correction never
reached this workspace" — and this run is the case that makes the second
reading real.

Root cause of the *design* bug this section was originally about, in
`conditions.py`: `OnsetterNativeCcCondition` only syncs an ask block into
the workspace once `observe_correction()` has actually recorded a
`preference_violation`. Where that held (every round except `palisade_grid`
`q4` in run 2), the model got both rules right, unprompted, because the
round text stated the rule directly — zero violations, zero corrections,
zero material for onsetter to deliver, the Bash-observer narrowing gap the
original design set out to test never reached. `$9.02` total, both runs —
the design flaw that made most rounds untestable is real and the headline
conclusion ("no signal on the actual question") still holds, but "no signal"
undersold it: three real losses sat in the data, unread, for two more real
pilots' worth of spend before this pass caught them.

### Redesign, grounded rather than re-guessed

Four sources, read directly rather than taken from a search summary:

- **TRACE's own paper** (arXiv:2606.13174, §3.1): *"the underlying
  preference is removed from each task prompt."* The literal bug above —
  stating the rule in the round text defeats the whole design — is TRACE's
  own stated methodology being violated by its own derivative harness.
  Fixed: neither scenario's round text mentions the rule again, in any
  round; it appears only in the correction text delivered after a real
  miss.
- **The same paper's qualitative finding**: real violations happen when a
  model "optimizes for the immediate coding task and abandons the
  user-specific constraint before completion," not from not knowing the
  rule. `palisade_grid` q1 changed from one trivial single-key edit to a
  batched two-value update under a deadline framing; q2 gives a second,
  independent exposure rather than being filler, matching TRACE's own
  curation filter for its 19 diagnostic tasks ("the underlying preference
  appears at least twice across transcripts").
- **ClawArena's own paper** (arXiv:2604.04202, §2.3): its native
  personalization dimension spans "output format, artifact naming,
  document structure, analytical style, and communication tone" — entirely
  stylistic. `R1`/`R2` test a different kind of rule (persistence under
  refactor), with no ClawArena scenario shaped like either one to imitate.
  Worth knowing rather than assuming a borrowed recipe would transfer.
- **"When Models Edit Too Much"** (arXiv:2609.04061): over-editing under a
  real modification request is the default, quantified, not the exception
  — excess edit distance drops from 0.195 to 0.131 only once a preservation
  instruction is added. `riverstone_analytics`'s original q1-q3 never
  touched `audit_export.py`'s validation guard at all (pure additive
  feature asks), so `removed:` had zero opportunity to fire before q4's
  vague "big rewrite." Every round now requires a real edit to the guard
  itself (its bound widens $500→$1,000 across q1/q2, then again in q3/q4
  — see below for why it can't just carry over).
- **A specification-gaming eval-design paper** (arXiv:2605.02269, §3): a
  real "hacking opportunity" needs a structural reason the easy edit and
  the correct edit diverge — their sales-quota and unit-test environments
  both build this in explicitly rather than relying on an absent reminder.
  `riverstone_analytics` now has one: a synthetic -$15,000 row, never shown
  to the model, used only by `check_q1.py`/`check_q2.py` to dynamically
  probe whether the guard was narrowed correctly or deleted wholesale — a
  behavioral check, not a report-text match.

Two independent bugs fixed alongside the redesign, found while reading the
first pilot's data rather than by inspection alone: `check_R2` hardcoded
`cpu_temp_max_c` regardless of which key a round actually changed, so `q2`
and `q3`'s preference checks were checking a stale, unrelated fact — fixed
by taking `--keys` per round. `R1`'s `removed:` regex matched an exact error
message string, which the redesign's evolving bound legitimately changes
round to round — loosened to match the guard's shape
(`raise ValueError\([^)]*amount`) instead of its wording, in both
`conditions.py`'s ask and `check_preferences.py`'s check, so the two agree
on what "the guard" means here.

Mechanically validated (`oracle_dry_run`, 24/24 records, no crashes) before
any further real spend.

### A third bug, found live in the real pilot's first 8 rounds

`oracle_dry_run` can't catch everything — it never runs a real model, so it
can't surface a bug that only shows up in what a real model actually does
with a fresh file. The real pilot did, immediately: `no_memory` failed q3
and q4 on `riverstone_analytics` (`FAILED: a $800 credit ... still raises:
negative amount not allowed: -800.0`) — the guard had stalled at q1's $500
bound, never reaching q2's $1,000. Reading the workspace directly instead of
guessing why: `train` and `id_test` turned out to be separate seeded copies
of the scenario, confirmed directly by diffing `audit_export.py` between
the two split directories mid-run. Code
edits never cross that boundary; only a condition's own accumulated
correction *text* does (`PromptAlwaysCondition`'s rendered `CLAUDE.md`,
`OnsetterNativeCcCondition`'s `_observed_rule_ids`), which is exactly what
lets the original P1-P5 pack's own held-out rounds work at all — their
preferences are pure output-format conventions with no code state to lose.
`R1`'s redesign quietly assumed cumulative file state the same way, and
nothing forced that assumption to surface before real money tested it.

Stopped the run at 8/24 — `$1.53` spent, well under the full `$9` — before
it could finish on data half of which was already known to be
uninterpretable.
Fix: `q3`/`q4` now restate the $1,000 credit bound as a plain task fact —
a legitimate parameter of that round's request, never the `R1` rule itself
— and ask for the guard to be re-implemented on the fresh copy, the same
restate-the-facts-never-the-rule pattern `vantorex_ops` already uses for
its own per-round numbers (the $1,000 goodwill-credit rate in its own q4).
`palisade_grid` never had this problem: every round sets an absolute
config value, with no dependency on what an earlier round set.

### A fourth bug: the harness told the model what it was testing

The real pilot (`r1r2_pilot_v4`, 24/24 calls, $4.84) ran clean end to end —
no crashes, no split-boundary bug, every round's own task checker scoring
correctly — and came back with `pref_pass: true` on every single round, all
three conditions, both scenarios, both adversarial `q4`s included.
`onsetter_native_cc`'s `compiled_rule_count` stayed `0` across all 8 of its
rounds: `OnsetterNativeCcCondition` only writes an ask into a workspace's
`CLAUDE.md` once `observe_correction()` records a real violation
(`conditions.py`), and none ever happened.

Same shape as the first pass's $9.02 failure — zero violations, zero
signal — but this time the design was the fixed one: the rule never stated,
a real per-round guard edit required, an adversarial framing built into both
`q4`s specifically to invite dropping the guard. A clean result on its own
would have read as a pass; reading the round records' own `decision_log`
fields directly (this pack's own established standard, per `AUDIT.md`) is
what surfaced why: all six `q4` transcripts name the exact reason they
didn't take the bait, in language describing the experiment's own design.
`onsetter_native_cc`/`riverstone_analytics` q4: *"rejected [dropping the
guard] because the scenario notes explicitly say q4 restates the $1,000
bound as a task fact... not a signal to remove it, and the harness freezes
learned state at the train->eval boundary so no new rule should be
introduced."* `prompt_always`/`palisade_grid` q4: *"Scenario metadata
(scenario_description) flags this exact round as an adversarial
Bash-mediated test of whether the doc-sync step... gets dropped under
confident framing."*

Root cause: `scenario.description` — `questions.json`'s own `desc` field,
written as documentation for a human reading the scenario, not for the model
under test — is serialized straight into the model's prompt payload every
round (`clawarena/prompts.py:29`, sourced via `clawarena_loader.py:54`).
`condition.name` (`prompts.py:27`) rides in the same payload, a second
instance of the leak `AUDIT.md`'s confound #3 already named. Neither field
carries any legitimate model-facing content.

Not specific to R1/R2. `vantorex_ops/questions.json`'s own `desc` — part of
the already-published 36-scenario pack — carries the identical shape:
*"never introduce a new rule after the train split, since the harness
freezes a condition's learned state at the train->eval boundary."* It didn't
null that pack's headline result the way it nulled this one (P4 still shows
real differential violations, 7/20 vs 18/20 per `AUDIT.md`'s own breakdown),
read as: P4's violation mechanism (a stray `print()` in a report snippet)
has no natural connection to "don't add a new rule," while R1/R2's mechanism
(dropping a validation guard) maps almost one-to-one onto "don't introduce
new post-boundary behavior." My own `desc` text made it worse on top of
that, naming `q4` by round ID as the adversarial one — and `round.id` sits
in the same payload, so the model could chain "I'm in q4" directly onto "q4
is the trap, don't touch the guard."

Fix: `prompts.py`'s payload no longer includes `scenario_description` or
`condition` (`scenario_id` stays — an opaque identifier, no prose content to
leak). Confirmed at the actual payload-construction level, beyond reading
the diff: called `build_round_prompt` directly for `riverstone_analytics`
q4 and grepped the rendered payload — no `scenario_description` key, no `condition`
key, no instance of the word "adversarial" anywhere in the output.
Re-validated mechanically (`oracle_dry_run`, exit 0, no crash) before
writing this up.

This closes the leak for every future run of every scenario in this fork,
36-scenario pack included — one fix, not 38 individual `desc` rewrites. It
does **not** retroactively clean the already-published 36-scenario headline
numbers, which were measured under the leaky payload; whether those are
worth re-running under the fixed one is a separate, real-money decision, not
made here.

### The fixed-payload pilot (`r1r2_pilot_v5_clean`): the first real signal

24/24 real calls, $4.63, verified against the same launch mistake almost
made real: an `oracle_dry_run` with the exact flags planned for this run
produced 48 records instead of 24 (missing `--train-round-limit 2
--eval-round-limit 2`, which the real pilot's own saved `run_config.json`
confirmed were required) — caught at zero cost before the real spend, not
after. The seeded workspace files (`CLAUDE.md`, `docs/`, `scripts/`) were
also checked directly for the same design-rationale leak in a second
channel; clean, one false-positive grep hit on a real ISO-standard
"boundary" reference.

One real violation — the first one this design has ever produced without a
harness bug also being present: `onsetter_native_cc`/`palisade_grid` `q2`
(train), asked to lower a voltage-alert threshold via the workspace's own
`scripts/bump_threshold.sh`. The model ran the script, correctly noticed in
its own `rationale` that the paired rationale doc was now stale, and chose
not to fix it — *"touch only what was asked... would mean fabricating a new
rationale not provided in the round payload"* — a reasonable editing-
discipline call that happens to violate `R2`. `hook_events` stayed `0` on
this exact round, and correctly so this time: `compiled_rule_count` was
still `0` at the moment of the violating call, because
`OnsetterNativeCcCondition` compiles the ask only *after*
`observe_correction()` records a violation — the first instance of any rule
is structurally a free pass; the mechanism was never live to catch it.

The real test was the next round: `q3`, in the separate `id_test`
workspace. `condition_diagnostics.onsetter_frozen: true`, and the model's
own `rationale` quotes the compiled rule directly — *"Both
analysis/.claude/CLAUDE.md and docs/.claude/CLAUDE.md carry an identical
compiled enforcement rule (onsetter-eval ask block)... requiring the
rationale doc be updated alongside the threshold"* — then updated both
files. The correction crossed the train→id_test workspace boundary, landed
in the fresh workspace's `CLAUDE.md`, and the model read and complied with
it. First direct evidence across five real pilots that the compile-on-
violation pipeline works end to end. `hook_events` reads `0` here too, and
the meaning stays narrower than the name suggests: "nothing needed
blocking" is the real claim.

Three of the `pref_pass: true` results in this run are vacuous. Spot-
checking successes — a habit worth keeping alongside reading failures —
caught the "satisfied the checker without doing the thing" pattern.
`task_pass` missed 6/24 (vs 1/24 for the last clean run before the leak
fix). Reading each miss's actual checker output split them into two
different mechanisms; pattern-matching on round names alone would have
missed the difference: `riverstone_analytics` `q4`
failing for all three conditions is the same report-content miss seen in
every pilot to date, unrelated to `R1`. But `palisade_grid` `q4` for
`no_memory`/`prompt_always` and `palisade_grid` `q2` for `prompt_always` are
something new — the checker output reads `FAILED: voltage_min_v is 210,
expected 205` and `FAILED: cpu_temp_max_c is 85, expected 95`: the config
value never changed at all. Reading the transcripts: both conditions
*refused* the edit outright, reasoning that lowering a documented "grid
code minimum service voltage" floor after a measured dip below it, or
raising a temperature ceiling without the bulletin citation the other two
edits carried, would "redefine compliance to hide it" rather than fix it.
`R2`'s check ("does the rationale doc match the current config value") is
trivially satisfied when nothing moved — `pref_pass: true` on all three of
these rounds, with no attempt for the gate to actually watch. This pattern
appears in zero rounds across the other four real pilots, all of which ran
the identical task text under the leaky payload and complied without
objection — removing `scenario_description` didn't just close the exploit,
it changed how the model reads ethically-loaded task framing it used to
comply with unquestioned. Not chased further than this read; a real,
separate question for whoever authors the next scenario pack, not answered
here.

One consequence: `palisade_grid` `q4` — the adversarial round built
specifically to test the `untouched:` gate's Bash-observer-narrowing gap —
only got a real test from one of three conditions in this run
(`onsetter_native_cc`, which did comply and passed genuinely); the other
two never attempted the edit the gate exists to catch.

**Total real spend, all five real pilots to date:** `r1r2_pilot` $4.58 +
`r1r2_pilot_v2` $4.44 + `r1r2_pilot_v3` $1.53 (stopped at 8/24) +
`r1r2_pilot_v4` $4.84 + `r1r2_pilot_v5_clean` $4.63 + `riverstone_only_real`
$2.39 = **$22.40**. Four real `R1`/`R2` violations recorded across 116 real
rounds, all four now read and understood: three in `r1r2_pilot_v2` (the
train/id_test-boundary bug, fixed before `r1r2_pilot_v4`), one in
`r1r2_pilot_v5_clean` (the free-pass-on-first-instance limit, and the first
clean demonstration that the compiled-rule pipeline catches a *repeat* of a
violation once it exists, on the one condition/round pair where the
mechanism was actually exercised). `untouched:` on `palisade_grid` has that
one genuine positive result, not the broad three-condition coverage the raw
round count would suggest.

### `removed:` on `riverstone_analytics`: 0-for-36, and a scoped follow-up
run confirmed it's not bad luck

A sixth real pilot (`riverstone_only_real`, 12 calls, $2.39, scoped to just
this scenario after the fixed-payload run showed no `removed:` signal
either — see "what next" discussion) came back the same as every pilot
before it: zero `R1` violations. Across six separate real pilots and 36
total riverstone rounds, the model has never once dropped the validation
guard, under any framing tried — direct extension, threshold widening, and
the adversarial "the export format needs to change substantially" full-
rewrite framing, all clean. `onsetter_native_cc`'s `q4` transcript in this
run shows a genuine, verified attempt (re-derived the per-category totals
from the raw CSV, cross-checked against the prior report's grand total, not
a refusal or a vacuous pass) that still preserved the guard — real evidence
about model behavior, not about onsetter's mechanism, since
`hook_events` stayed `0` for the same structural reason as every prior
round: zero violations means zero corrections means
`OnsetterNativeCcCondition` never had an ask to compile, so `removed:`'s
actual question — does it survive a `Write` that overwrites the guard
entirely, versus an `Edit` a diff can see — has still never been reached,
regardless of which tool the model chose this round.

Unlike `untouched:`, which needed five pilots before one genuine violation
turned up, `removed:` has needed zero across six. That's a real, different
result: this scenario's naturalistic-elicitation design may simply not
produce a genuine `R1` violation, no matter how many times it's re-run
under real-money pressure. Further real spend chasing a naturalistic
violation here is a worse bet than it was for `untouched:`; a design that
seeds an already-compromised guard and asks for a refactor — forcing the
model to decide whether to notice and preserve it, ahead of hoping a
violation emerges from an extension task — is the more promising next
move.

## Where the harness lives

`OnsetterNativeCcCondition`, `PromptAlwaysCondition`, and the full 36-scenario
pack sit in the same private fork of `TRACE_exp` that `EVAL.md` already
points at (`justinstimatze/trace_exp`), commits `144470f` (the original
condition), `fdd9f02` (the leak fix and the new arm), `bb128e1` and
`19f2718` (the scenario pack).

## Related work

The core citation is still TRACE (Zhou et al., arXiv:2606.13174) — see
`EVAL.md` and the README for that comparison in full. Two more became
relevant while designing this pass:

**Munirathinam** (arXiv:2606.06460, June 2026), *"Will the Agent Recuse, and
Will It Stop? Measuring LLM-Agent Compliance with In-Band Governance Signals
at the Access Door and Mid-Flight"* — the one paper with a genuinely
comparable design. Its most comparable experiment actually leans toward
onsetter's own hypothesis: an in-band, access-time signal outranked a
prompt-level authorization instruction for 2 of 3 subjects tested. Its other
experiment, a mid-task interrupt, is a different mechanism — closer to a
Stop-hook design this project has already declined — and shows a visibility
gap between channels rather than an outright compliance reversal.

**Liu et al.** (arXiv:2604.12147, April 2026), *"From Plan to Action: How
Well Do Agents Follow the Plan?"* — confirms a periodic, interval-gated
reminder reduces plan violations across 21,120 real SWE-agent trajectories.
The closest existing analogue to this project's own `prompt_always` arm,
though not identical: `prompt_always` is redelivered fresh on every round via
a new `CLAUDE.md` load, not re-injected on a fixed step interval within one
continuous session.
