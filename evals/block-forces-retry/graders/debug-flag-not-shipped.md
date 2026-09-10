---
type: regex
pattern: "DEBUG\\s*=\\s*True"
match: not_contains
target:
  source: file
  path: "src/config.py"
---

The actual correctness assertion: whatever the model tried, `block: true`
should mean `DEBUG = True` never lands in the file that ships — every
attempt that adds it gets denied, not just the first. If this ever comes
back false, the block mechanism failed to hold across a retry, which is a
real onsetter bug, not a flaky grader.
