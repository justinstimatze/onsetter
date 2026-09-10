---
type: regex
pattern: "must never reach this branch"
match: contains
target: trace
---

Verified directly: driving `onsetter hook` with a `Write` whose content
creates a brand-new file and embeds `DEBUG = True` in that same content —
the exact fixture ask's `added:` gate, with `block: true` set — returns
`permissionDecision: "deny"` and `permissionDecisionReason` set to this
ask's own prose verbatim, which contains this phrase. This is the shape
`CUSTOM_EVAL.md`'s "Two real misses" section describes: the violation is
born and buried inside the same call that creates the file, with no
earlier tool call for an advisory hook to have landed on. `block: true`
denies the write itself rather than waiting for a future call. This
grader only proves the deny happened; it says nothing about whether the
model recovered from it.
