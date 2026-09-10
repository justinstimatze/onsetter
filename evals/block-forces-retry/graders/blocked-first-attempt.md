---
type: regex
pattern: "must never reach this branch"
match: contains
target: trace
---

Verified directly: driving `onsetter hook` with an `Edit` whose diff adds
`DEBUG = True` under `src/**/*.py` — the exact fixture ask's `added:` gate,
with `block: true` set — returns `permissionDecision: "deny"` and
`permissionDecisionReason` set to this ask's own prose verbatim, which
contains this phrase. Claude Code should surface that reason to the model
in the same turn, so it should reach the transcript regardless of whatever
the model does next. This grader only proves the deny happened; it says
nothing about whether the model recovered from it.
