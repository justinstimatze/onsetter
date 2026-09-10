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
Each scenario reuses TRACE's own P1–P5 preference taxonomy — number
formatting, dated filenames, report structure, no debug prints, cited
sources — with `check_preferences.py` copied byte-identical from the
canonical source and only the `--target` argument edited, so translation
drift has nowhere to hide. Every round in every scenario carries a real
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
metric this eval actually depends on untouched. Six scenarios' final round
asked the model to assume "this month's rate continues," on the mistaken
assumption it could recall an earlier round's stated number from a workspace
the held-out split never actually shares files with — all three conditions
correctly reported the number as missing rather than guessing, which broke
the literal task-correctness check identically across all three, not the
preference check. One scenario's CSV used lowercase category names against
Title Case model prose, so a case-sensitive match happened to pass two
conditions by accident, on their code snippets quoting the raw CSV value, and
correctly failed the plain-prose condition. Both fixed prospectively in the
scenario batch that followed rather than by re-running what had already run
cleanly on the metric that matters.

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

## Two real misses, and what they actually are

Both of onsetter's two losses in the `prompt_always` comparison are the same
failure, confirmed independently in two unrelated scenarios. Both are P4 (no
`print(...)`) violations. Both happen when the model writes an entire report
in a single `Write` call that includes an illustrative Python snippet with an
inline `print(...)` debug line. In one scenario, this happened twice
back-to-back — a train round violates P4, the correction gets recorded, and
the very next P4-gated round violates it identically.

`hook_events_delta` confirms the hook fired, both times — this isn't a
missed delivery, and it isn't ask-wording ambiguity either: `prompt_always`
complied on the exact same round with the identical prose. It's structural.
For a `Write`, onsetter's diff engine treats the entire new file as "added,"
so the gate correctly matches content anywhere in it — but onsetter's hook
only ever emits advisory context, never a block. `PreToolUse` fires after the
model has already committed to that Write call's exact arguments, so the
correction can only land on a *future* tool call, never the one that tripped
it. When the violation and the report are the same single write, and nothing
else happens in that round, there's no later moment for the correction to
reach. This is a real, honest limit on onsetter's own hypothesis, not
softened: an advisory hook fired at the tool call still can't out-run a
one-shot generation that buries the violation inside itself.

**Update, 2026-09-10: this gap has a fix, mechanically confirmed, not yet
re-measured.** `block: true` (`ask/ask.go`, commit `0e427f2`) shipped six
hours after this eval's own commit, built for exactly this shape — an
opt-in deny on an `added:`/`removed:` match, closing the one-call-too-late
window described above. Confirmed directly, at zero API cost: driving
`onsetter hook` with a `Write` `PreToolUse` payload that creates a
brand-new file whose content embeds `DEBUG = True` in the same call,
against the real shared eval fixture's `added: DEBUG\s*=\s*True` /
`block: true` ask, returns `permissionDecision: "deny"` — the exact
failure shape both P4 misses above hit, blocked. What's still open: the
`trace_exp` harness that produced the 97.2% number has not been re-run
with `block: true` wired onto the P4 ask, so there is no re-measured
score backing a claim of 100% — only a mechanical proof that the gate
denies the shape that caused both losses. See
`evals/block-denies-write-embed/` for the local, free reproduction of
this exact check.

## What this deliberately does not cover

Reuses TRACE's own P1–P5 taxonomy rather than exercising onsetter's
`removed:`, `untouched:`, or `has:`+`when:` gate shapes — real, separate
feature-coverage work, named as a limit rather than dropped silently. No
cross-vendor model panel: onsetter's mechanism is a Claude Code `PreToolUse`
hook by construction, so there's no equivalent surface on other agent tooling
to even ask the question against. Onsetter's own source was never touched —
this is external eval infrastructure and a results write-up, same as
`EVAL.md`.

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
