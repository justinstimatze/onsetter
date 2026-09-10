# onsetter's `claude plugin eval` suite

Tests the plugin the way Claude Code actually loads and runs it — the
`.mcp.json`/`hooks.json` wiring, a real PreToolUse call, a real transcript —
not `cmd/onsetter`'s Go unit tests, which exercise the binary in isolation.

## Status: authored, not run-verified

`claude plugin eval` is early access as of Claude Code 2.1 (September 2026).
Confirmed directly: `claude plugin eval --help` works and shows a real,
current CLI; `claude plugin eval init --bare <name>`, the scaffold command
that would have produced a confirmed-correct template, exits 1 with `plugin
eval is currently in early access`. There is also no public docs page for
this feature yet (checked code.claude.com's docs index directly — no
dedicated page for the eval harness, grader types, or case file format).

The file format below comes from Claude Code's own early-access internal
reference material — a recalled specification, unconfirmed against a live
doc or a real scaffold. It's still the most complete schema available. Every
case here parses as
plausible `prompt.md` + `graders/*.md` + `case.yaml` content under that
schema. Whether it's exactly right won't be known until `claude plugin eval`
runs for real. **The first thing to do once access opens is run this suite,
read the actual error if one file's shape is wrong, and fix that file** —
not to re-derive the schema from scratch.

Named risks, most likely to bite first:

1. **`prompt.md` frontmatter and a sibling `case.yaml` coexisting.** Every
   case here has both — `prompt.md` for the body/tags/`allowed_tools`,
   `case.yaml` for `context.scaffold_script` only (nothing else, no
   duplicate `name`/`plugins`/`runs`). The reference material describes
   `case.yaml` as an *alternative* to `prompt.md` frontmatter, not
   explicitly as a supplement alongside it. If the real tool treats
   `case.yaml`'s presence as replacing `prompt.md` entirely, every case
   here needs its scaffold folded into one file or the other.
2. **`scaffold_script` timing vs. plugin bootstrap.** Every fixture below
   needs the `onsetter` binary on `$PATH` to run `onsetter warm` (the
   `evokes:` case genuinely needs this — see its own directory). Whether
   `context.scaffold_script` runs before or after the plugin's own
   `SessionStart` hook (`fetch.sh`, which downloads that binary) is
   unconfirmed. Every scaffold script guards the `onsetter warm` call with
   `command -v onsetter` and no-ops rather than aborting if it's missing —
   but a missing warm means the `evokes:` case will read as a false
   negative, not a real one. Check this first if that case fails.
3. **Grader `target: trace` matching exact injected prose.** The regex
   graders below match onsetter's own literal output strings (verified for
   real — see below), not a guess at how the harness serializes a
   `hookSpecificOutput` block into the trace. If the trace format wraps or
   escapes that text differently than plain JSON, the patterns may need
   loosening.

## What is verified, for real, right now

The fixture `CLAUDE.md` embedded in every case's `scaffold_script`, and the
exact injected text each grader matches against, were checked directly
against onsetter's own CLI:

- `onsetter lint` parses the fixture clean (3 asks, 1 file, no errors).
- `onsetter hook`, fed the real nested Claude Code PreToolUse payload shape
  (`tool_input.file_path`/`content`/`new_string`/`old_string`, not a flat
  one — confirmed by reading `cmd/onsetter/hook.go`'s `payload` struct)
  fires the bare-`except:` ask on a matching `Write` and stays silent on a
  `Write` using `except ValueError:`.
- The same drive fires the `block: true`/`added:` ask on an `Edit` adding
  `DEBUG = True`, producing `permissionDecision: "deny"` and the ask's own
  prose as `permissionDecisionReason`.
- The `evokes:` ask fires on a bare, heading-free paraphrase sentence with
  **zero shared keywords** with the trigger phrase, after `onsetter warm`.
  Real measured scores via `onsetter calib` on a 5-positive/3-negative
  corpus: positives ranged 0.498–0.586 against the 0.48 default threshold.
  **A markdown heading above the claim dilutes the embedding enough to miss
  the threshold** — confirmed directly (headers.md's own documented
  caveat, reproduced live: the identical sentence scored above threshold
  standalone and failed to fire with `# Perf notes\n\n` prepended). Every
  case whose prompt could elicit a heading is designed around an `Edit`
  that appends a sentence to an existing file, not a fresh `Write`, so the
  embedded `new_string` is the sentence alone.

The exact literal text onsetter injects (used verbatim in the regex
graders below, not reconstructed from memory):

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"onsetter — 1 ask for this edit. A repeat is marked, not hidden.\n\n▸ CLAUDE.md:3 · matched \"    except:\"\n<ask prose>\n\nTo retire one, delete its block from the file named above."}}
```

and, for a `block: true` match:

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"...▸ CLAUDE.md:21 · matched \"DEBUG = True\" · blocks\n<ask prose>...","permissionDecision":"deny","permissionDecisionReason":"<ask prose>"}}
```

## Layout

Five cases, each exercising one thing the plugin wiring is supposed to do:

- `fires-on-matching-write/` — a `when:`-gated ask fires on the matching edit.
- `silent-on-unrelated-write/` — the same ask stays dark on an unrelated one.
- `evokes-paraphrase-fires/` — an `evokes:` ask fires on a paraphrase sharing
  no keywords with the trigger phrase.
- `block-forces-retry/` — a `block: true` ask denies the first attempt and
  the model retries with a corrected edit.
- `sustained-session-stays-fast/` — many sequential edits in one session all
  land; the actual latency comparison against the persistent-server
  benchmark in `CHANGELOG.md` has to be read from the run's own `--json`
  output afterward, not from a grader — no documented grader type asserts
  wall-clock.

Every case's `scaffold_script` writes the same fixture `CLAUDE.md` (three
asks: a `when:`-gated bare-except check, an `evokes:`-gated
unmeasured-claim check, and a `block: true`/`added:`-gated debug-flag
check) into the sandbox workspace before the prompt runs, and `git init`s
it if no `.git` exists — onsetter's discovery walk needs a boundary.

## Running it, once access opens

```
claude plugin eval --ablation with-without .
```

from the plugin root. Read the delta per case — `--ablation` isolates
whether the *plugin* is what causes the behavior, separate from the
behavior simply occurring on its own.
