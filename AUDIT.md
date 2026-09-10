# Independent audit of CUSTOM_EVAL.md's benchmark

`CUSTOM_EVAL.md` reports 70/72 held-out compliance for onsetter's real hook
against 55/72 for a prompt-time control — a benchmark where the same effort
built the feature, wrote the scenarios, and ran the numbers. This is the
independent pressure-test that benchmark never got: two adversarial audits,
dispatched together so neither could see the other's prompt or findings,
against data already collected in the sibling `trace_exp` repo. No new
`claude_cli`/API calls. Every claim below traces to a file one of the two
audits actually read, not a summarized impression.

## What held up

- **The headline numbers are exact.** Independently re-parsed all 216
  held-out round records under
  `experiments/clawarena/state/runtime/{pilot3_real,pilot8_real,pilot25_real}/`
  — `no_memory` 0/72, `prompt_always` 55/72, `onsetter_native_cc` 70/72,
  matching `CUSTOM_EVAL.md` exactly. Call count (432) and cost ($73.32,
  summed from `total_cost_usd` in every round record) also reproduce exactly.
- **The onsetter-vs-`prompt_always` significance figure reproduces exactly.**
  19 discordant pairs, 17 favoring onsetter, sign test p ≈ 0.00073 —
  matches, and this is the comparison `CUSTOM_EVAL.md` itself calls "the
  actual novel comparison."
- **Both named P4 misses are real**, and the hook genuinely fired in both
  rounds (`hook_events_delta` confirms it) — not a missed-delivery artifact.
  Rule text is byte-identical between the two informed conditions for both
  scenarios, so this isn't a content-asymmetry bug either.
- **The intended variable is genuinely isolated in code.**
  `OnsetterNativeCcCondition` (`conditions.py:862`) and `PromptAlwaysCondition`
  (`conditions.py:1155`) share identical rule prose, the identical extraction
  pipeline, and identical round-boundary timing — `setup_workspace()` runs
  fresh at the top of every round for both (`multi_runner.py:392-398`), so
  round N reflects only corrections observed through round N-1 for both
  alike.
- **No train/held-out leakage, no cross-scenario state bleed in the real
  run.** Spot-checked `train` vs. `id_test` workspace `docs/` — disjoint file
  sets. Every real scenario has a distinct `run_hash`, consistent with
  one-process-per-scenario; the `multi_runner.py:185` bug (condition object
  built once per invocation, outside the per-scenario loop) is real and
  reproducible but did not contaminate the actual 432-call run.
- **Both self-disclosed scenario-authoring bugs check out** — confirmed to
  leave `pref_pass` untouched in every affected round, exactly as
  `CUSTOM_EVAL.md` claims (mechanism correct; see count correction below).
- **The pre-run mkdir leniency / `CLAUDE.md` prompt-leak bugs found during
  construction are real, fixed, and don't touch the real run** — the leak
  fix (`fdd9f02`) predates the pilot; the mkdir leniency bug's vacuous-pass
  pattern was grepped for across all 432 real round records and found zero
  times.

## What didn't

**1. The `check_preferences.py` "byte-identical from the canonical source"
claim is false, and materially so.** `CUSTOM_EVAL.md:60-61` says the script
was "copied byte-identical from the canonical source... so translation drift
has nowhere to hide." No copy of the true public upstream
(`github.com/YujunZhou/TRACE_exp`) exists anywhere in the `trace_exp` fork —
the public repo has no `check_preferences.py` at all; its own
`experiments/clawarena/data/README.md` says real scoring scripts aren't
redistributed, only fetched separately via `CLAWARENA_ROOT`. The script's own
docstring (byte-identical across all 36 copies) says outright: "this doesn't
copy any real `hil_*` scenario's `check_preferences.py`: it authors one
canonical version... Only `P2` recurs with the same meaning across every
scenario checked. This script instead implements exactly the five
conventions **onsetter's own `_RULE_ASKS` already translates**." Same author,
same commit, same private fork wrote both the rule prose and the checker that
scores compliance with it, deliberately kept in lockstep — the opposite of an
independent external ground truth. This doesn't bias `onsetter_native_cc`
against `prompt_always` (both score against the identical checker and
identical rule text), but "translation drift has nowhere to hide" overstates
the independence of the scoring instrument from the treatment being tested.

**2. The P4 causal narrative is overstated.** `CUSTOM_EVAL.md:130-140`
attributes both P4 losses purely to timing structure — "no later moment for
the correction to reach... an advisory hook fired at the tool call still
can't out-run a one-shot generation." The round records' own `decision_log`
fields contradict this. In both losses the model explicitly saw and reasoned
about the hook's exception clause, quoting it verbatim: *"the hook's own
stated exception ('unless this is example output being quoted verbatim
inside a doc') applies... so no change was needed there."* For
`vantorex_ops` this happened across two separate `Write` calls, with the
model re-affirming the same judgment the second time. `prompt_always`, seeing
identical rule text before writing anything, reasoned correctly and avoided
the violation from the start. The real effect looks like *when* the model
reasons about an ambiguous exception clause — upfront vs. post-hoc
rationalization after already committing to the write — not that a
correction structurally cannot reach the model in time. `block: true`
(already shipped) closes this regardless of which framing is right, but the
framing itself should be corrected. Confirmed for real, not just
mechanically: wired onto the eval's own P4 ask and re-run against both
affected scenarios (8 calls, $1.73) — both previously-losing held-out
rounds now pass `P4: PASSED (no print(...) calls)`. See `CUSTOM_EVAL.md`'s
2026-09-10 update for the full result.

**3. The model is told which experimental arm it's in, on every call, and
nothing addresses it — and a second field in the same payload turned out to
be worse.** `prompts.py:27` embeds `condition.name` — literally
`"no_memory"`, `"prompt_always"`, or `"onsetter_native_cc"` — into the JSON
payload sent to the model every round. Onsetter is a real, publicly
documented tool, and Sonnet 5's training cutoff (January 2026) postdates
onsetter's public existence — so the model could in principle be recognizing
the string `"onsetter_native_cc"` from pretraining exposure to onsetter's own
docs, not purely reacting to hook-delivered content. Not proven, but a real,
easily-avoided confound (condition names could have been anonymized) that
`CUSTOM_EVAL.md` never discusses.

The same line (`prompts.py:29`) also sends `scenario.description` —
`questions.json`'s own `desc` field, written as documentation for a human
reading the scenario, not for the model under test. Confirmed causally
responsible, not just theoretically possible: `CUSTOM_EVAL.md`'s R1/R2 pilot
(`r1r2_pilot_v4`) came back with zero violations across 24 real calls, all
three conditions, both scenarios, both adversarial rounds, and reading the
round records' `decision_log` fields directly showed the model quoting the
leaked `desc` text nearly verbatim to justify not violating the rule under
test — e.g. *"the harness freezes learned state at the train->eval boundary
so no new rule should be introduced,"* language pulled from the scenario's
own design rationale, not the task. `vantorex_ops/questions.json`'s `desc` —
part of the already-published 36-scenario pack — carries the identical
shape (*"never introduce a new rule after the train split, since the harness
freezes a condition's learned state at the train->eval boundary"*), so this
is not scoped to the new scenarios. It didn't null the 36-scenario pack's
result the way it nulled R1/R2 (P4 still shows real differential
violations, item 5 below) — plausibly because P4's violation mechanism has
no natural connection to "don't add a new rule," while R1/R2's does almost
exactly — but that's a read of why the damage differs, not proof the
36-scenario headline is unaffected. Fixed in `prompts.py` (both fields
dropped from the payload) as part of the R1/R2 write-up; the already-published
36-scenario numbers were measured under the leaky payload and have not been
re-run under the fixed one.

**4. The 36-scenario pack is 3 templates × 12 cosmetic draws, not 36
independent designs.** Every scenario reuses one of three question
skeletons, distinguished only by which rule pair it tests (P1+P4 in 20 of
36, P1+P3 in 8, P3+P5 in 8), over a reused CSV schema with swapped domain
vocabulary. Reasonable for mass-producing scenarios, and never claimed
otherwise — but the pack's statistical power reads as roughly 3 designs ×
12 draws, not 36 independent trials, which the headline table doesn't
signal.

**5. The entire measured effect concentrates in one rule type, and three of
the five nominal categories barely exercise onsetter's real mechanism.**
Breaking the 19 held-out failures down by rule: P1 27/28 vs 28/28, P3 16/16
vs 16/16 (ceiling, both conditions), P4 7/20 vs 18/20, P5 5/8 vs 8/8. The
P4 gap alone is 11 of the 15-round headline gap (73%); P5 contributes 3 more.
P2 (dated filenames) is tested in every round and never fails for either
informed condition. Separately: P2, P3, and P5's asks use `on: mint` with no
content trigger (`conditions.py:788-827`, docstring admits it at `:742-749`)
— for those three of five categories, onsetter's hook fires as "once per new
file in this directory," not the semantic diff-matching that's the actual
differentiator being tested. The defensible claim is narrower than "P1-P5":
tool-call-time delivery beats always-visible prompting specifically for
avoiding `print()` in a report-embedded code snippet — a violation the
question wording (`"...with a short Python snippet you'd use to compute
it"`) is written to provoke — with a smaller citation effect, on a taxonomy
where two of five categories are already at ceiling for both channels.

**6. Three small factual slips, none touching the headline numbers:**
- The case-sensitivity scenario-authoring bug is misattributed.
  `CUSTOM_EVAL.md` says it "correctly failed the plain-prose condition"
  (`prompt_always`); the raw `corvid_media` `q3` records and `trace_exp`
  commit `19f2718`'s own message show it was `no_memory` that failed
  (Title-Case model prose vs. the case-sensitive checker's lowercase CSV
  values), with `prompt_always` and `onsetter_native_cc` both passing by
  accident.
- "Six scenarios" hit the recall-unavailable q4 bug; the raw data shows
  **seven** — `basaltridge_mining`, `cinderbrook_utilities`, `fenwick_labs`,
  `marrowpeak_foods`, `quillfire_support`, `solstice_freight`, plus
  `haldor_maintenance` (an earlier batch, same failure shape). `pref_pass` is
  untouched in all seven, as claimed — only the count is off by one.
- The sign-test p-value for onsetter-vs-`no_memory` (2.4×10⁻¹³) does not
  independently reproduce by exact binomial sign test (≈1.69×10⁻²¹) or
  McNemar's test (≈1.6–5.9×10⁻¹⁶) on the paired data. Every method tried
  lands *more* extreme than the published figure, so this doesn't inflate
  the claim — the underlying effect is real and, if anything, understated —
  but the specific number as printed isn't reproducible by any method
  identified here.

**7. One uncatalogued task-level asymmetry, noted for completeness.**
`copperfield_mining` `q4`, `onsetter_native_cc` only, fails task correctness
on a real model arithmetic slip ($144,984 vs. expected $145,044). Not a
scoring bug, doesn't touch `pref_pass`, not one of the two named
scenario-authoring bugs — just model noise, but it's a third uncatalogued
task-level asymmetry alongside the two `CUSTOM_EVAL.md` already names.

## Corrections owed to CUSTOM_EVAL.md

- Line 60-61: drop "copied byte-identical from the canonical source... so
  translation drift has nowhere to hide" — replace with an honest statement
  that the checker is an original script, authored to match onsetter's own
  rule prose, with no independently-sourced ground truth for this pack.
- The P4 miss section (118-140): reframe from "no later moment to react" to
  what the decision logs show — the model reasoned about the hook's own
  exception clause and chose wrong, post-hoc, where `prompt_always` reasoned
  about identical prose upfront and chose right.
- Name the P4/P5 concentration and the P2/P3/P5 no-content-trigger gap
  explicitly, rather than letting "P1-P5" imply five categories contributing
  roughly evenly.
- Note the in-band `condition.name` and `scenario.description` leaks
  (`prompts.py:27,29`) — fixed in code as of the R1/R2 write-up, but the
  published 36-scenario numbers were measured before the fix and haven't
  been re-run since.
- Fix the case-sensitivity attribution (`no_memory`, not `prompt_always`)
  and the scenario count (seven, not six).

The `condition.name`/`P4`/`P5` items above don't change the qualitative
conclusion — tool-call-time delivery beats always-visible prompting, on
held-out rounds, on this rule family. The `scenario.description` leak is a
different case: it's now confirmed to null a real pilot outright when a
rule's violation mechanism happens to align with the leaked design language,
and `vantorex_ops`'s own `desc` field carries that same language. Whether it
did the same to any part of the 36-scenario headline (in full or in part,
for some rule types and not others) is genuinely unknown until that pack is
re-run under the fixed payload — the 97.2%/76.4% numbers should carry that
caveat, not just the smaller ones above, if published or cited anywhere past
this repo.

## The fuller TRACE dataset — never requested

Searched both `onsetter` and `trace_exp` for any record of asking the TRACE
paper's authors for the other 50 of 62 `hil_*` scenarios. `EVAL.md:139` only
notes the possibility ("would need to come from the authors"); neither repo
has a ticket, draft, or commit referencing outreach. Confirmed directly with
the user: never attempted, not a declined or unanswered request — a known,
accepted limitation up to this point.

## Method

Two `general-purpose` agents, dispatched in one batch with no visibility into
each other's prompt or findings:

- **Audit 1** — data and scoring integrity. Re-derived pass/fail from raw
  round JSON, diffed `check_preferences.py` against public upstream.
- **Audit 2** — construction and taxonomy-coverage bias. Read the full
  condition code, all 36 scenario definitions, and broke the raw pass/fail
  data down by rule type.

Both independently converged on the same two problems — the checker's
false provenance claim, and the effect's concentration in P4 — from
different starting points (a byte-diff vs. a per-rule breakdown), which is
what makes them worth trusting over either audit alone.
