# Does tool-call-time delivery actually beat prompt-time delivery?

This is the one variable onsetter's whole pitch rests on and the one thing
nobody had measured. The README cites TRACE (Zhou et al. 2026,
[arXiv:2606.13174](https://arxiv.org/abs/2606.13174)) for the
access-compliance gap — a rule sitting in context gets followed a bit better
than half the time — but the paper itself names what it never varied:
*when* the rule arrives. What follows is the attempt to close that gap,
what it found instead, and why the honest stopping point is earlier than a
bigger run would have been.

## What got built

TRACE's own experiment code is public
([`github.com/YujunZhou/TRACE_exp`](https://github.com/YujunZhou/TRACE_exp)),
including the exact ClawArena harness that produced the paper's §5 numbers.
Rather than build a parallel eval from scratch, this added a sixth
condition — `onsetter_native_cc` — to that harness directly, so any result
would sit next to the paper's own published table instead of a fresh number
nobody could compare.

`OnsetterNativeCcCondition` wires onsetter's real, unmodified binary into
each isolated round workspace through a hand-written
`.claude/settings.json` `PreToolUse` hook — the only scope
`claude -p --setting-sources project,local` ever loads; onsetter's own
`.claude-plugin/` install path isn't reachable from this harness at all.
`memory_notes()`/`enforcement_rules()` stay `[]` by construction, so
nothing from this condition can leak into the prompt the way it does for
the harness's other conditions — the hook is the only channel. A
correction's `--rules P1`..`P5` token gets regex-extracted from the real
`correction_text` the harness produces and translated into a real onsetter
`ask` block, appended to the scenario's own existing `CLAUDE.md` rather
than replacing it. `O1`-`O4` (see below) are left unmapped on purpose.

Verified four separate ways before spending anything real: `onsetter lint`
passes clean on the generated `CLAUDE.md`; a direct wrapper-firing test
confirmed the right file trips the right ask and an unrelated file stays
silent; a zero-cost `oracle_dry_run` completed across all 12 currently-public
scenarios with zero errors; then a real 16-call pilot (below) ran clean.

## The real pilot

One scenario (`hil_c7`), 8 rounds (5 train + 3 held-out), two conditions
(`no_memory`, `onsetter_native_cc`), model `sonnet`, real `claude_cli`
subscription-auth calls — 16 calls total, zero `runtime_error`, ~16 minutes
wall time, ~$3.60 notional cost (list-price equivalent — actual billing ran
through subscription usage; TRACE's own harness deliberately strips
`ANTHROPIC_API_KEY` before every call to force that).

The result: zero addressable violations occurred in either condition. Both
real file-based preference checks in range (a filename-convention check at
round 5, a code-quality check at round 8) passed cleanly, in both
conditions, without any correction ever being needed. `task_pass` matched
round-for-round between the two conditions — which confirms the isolation
is clean (nothing leaked between them) but means the pilot produced no
signal either way about the actual hypothesis.

## Why scaling the pilot wouldn't have fixed this

Only 12 of the paper's full 62 `hil_*` scenarios are public (checked both
current `main` and the `v1.0.0` tag directly — `v1.0.0` predates this data
entirely, and nothing's been added since `main`'s last push in July). Across
all 12, only 24 of 337 total rounds — about 7% — carry a real file-based
preference check at all. Most of the rest are plain task rounds; a good
share also carry the answer-format check described below.

A paired comparison over a binary outcome needs the discordant pairs to
clear a real bar: with only 24 checks in the entire public dataset, getting
even marginal significance (p<0.05, one-sided) requires 5–6 of them to land
as onsetter-favoring — which means a genuine baseline violation rate of at
least 21–25%, ideally closer to 40% for a believable effect. The pilot's own
observed baseline was 0 of 2, which is uninformative on its own but is
consistent with file-based violations being rare here in general. Running
every public scenario in full — 674 calls, roughly 11 hours, ~$150
notional — would still cap out at those same 24 checks and that same p≈0.03
ceiling in the best case. That's not a good trade for the information it
buys.

## The real finding: the instrument doesn't measure what onsetter does

Every ClawArena round carries two separate checks, not one: the rare
scenario-specific `pref` command above, and a universal check the paper's
own text calls a **preference overlay** (the exact phrase — matching
`preference_overlay.py`'s own name and scope in the harness), applied to
*every* round regardless of type: does the final JSON answer include
`answer`/`rationale`/`decision_log`/`unresolved_questions`, do `rationale`'s
"Evidence:"/"Decision:" labels appear, does `decision_log` state an explicit
`updates:` status, is `unresolved_questions` a real list. In the real pilot,
every single observed "preference_violation" — 16 for 16 rounds, both
conditions — came from this overlay, never from a file-based check.

Onsetter's entire mechanism is a `PreToolUse` hook: it only ever sees a
pending `Write`, `Edit`, or `Bash` call. A model's final JSON answer isn't a
tool call, so there's nothing for a `PreToolUse` hook to gate on. This isn't
a translation gap in how the `onsetter_native_cc` condition was built — it's
a structural fact about what onsetter is.

And it goes further than that. The paper's published ClawArena numbers
(No Memory 100.0% violation → TRACE 37.6% ID / 2.0% OOD) almost certainly
draw most of their signal from that same overlay — it applies to literally
every round, while the file-based checks apply to about 7% of them. But more
importantly: **the paper's own description of TRACE itself is a blocking
mechanism** — it forces a retry rather than advising in advance. Figure 3's
own caption states it plainly —
*"TRACE enforces before completion"* — and its own text: *"matching rules
are loaded as runtime gates: completion is allowed only after the
corresponding constraint is satisfied."* That's a real gate, appearing three
separate times in the paper's own language. The public harness code's
`trace_native_cc` condition — the one this eval's `onsetter_native_cc`
condition sits alongside — strips that gate out entirely, specifically
because leaving it in caused Claude Code sessions to end with
`stop_hook_prevented` and an empty, unparseable answer (documented directly
in the harness's own code comments). So the comparison this eval ran was
never really "onsetter vs. TRACE" — it was onsetter vs. a narrower,
already-non-blocking ablation of TRACE that the public release ships to
keep the harness working at all, and that comparison never got the chance
to run on real, addressable data.

Given both of these, extending onsetter itself to reach the overlay's
answer-shape checks was considered and set aside. The only Claude Code hook
that sees a model's final answer text is `Stop`. By the time it fires, the
answer already exists, so any effect from it means forcing a retry — a
different mechanism than onsetter has ever been. Worse, the overlay applies
to every round with no selectivity at all, which is exactly the case
onsetter's own design already treats as not worth a gate: a check with
nothing to select against is already just plain always-on prose, and the
`ask` fence adds nothing to it. Building a blocking surface to chase a
number that the paper's own harness had to strip blocking out of to make
usable would trade onsetter's actual identity for a comparison that was
never apples-to-apples to begin with.

## What's real here

`OnsetterNativeCcCondition` is a working, verified adapter — lint-clean,
fire/silence-tested, dry-run-clean across every public scenario, and it ran
16 real calls with zero errors and confirmed clean isolation. It's sitting
in a private fork of `TRACE_exp` (`justinstimatze/trace_exp`,
commit `144470f`), ready to point at denser data: a fuller ClawArena
release (only 12 of 62 scenarios are public; the other 50 would need to
come from the authors), or a purpose-built eval where the preferences are
genuinely tool-call-shaped — file placement, naming, format-on-write — the
way this benchmark's public slice mostly isn't. That kind of eval would
push addressable rounds from ~7% toward 100% and make thirty rounds of it
worth more than the 337 run here.

None of this is a null result on onsetter's timing hypothesis. It was
never tested at a scale that could show anything either way — the public
data is too sparse in the violations onsetter can address, and the paper's
own headline number came from a mechanism that blocks, which isn't
onsetter's and isn't even what the public harness code ships as "TRACE."
What's real: the mechanism works, the isolation is clean, and this
dataset is the wrong instrument to test it against.
