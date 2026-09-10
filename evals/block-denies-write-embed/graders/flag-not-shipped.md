---
type: regex
pattern: "DEBUG\\s*=\\s*True"
match: not_contains
target:
  source: file
  path: "src/settings.py"
---

The actual correctness assertion: whatever the model tried, `block: true`
should mean `DEBUG = True` never lands in the file that ships, even though
the violation was born inside the same call that would have created the
file in the first place — every attempt that embeds it gets denied, not
just the first. If this ever comes back false, the block mechanism failed
to hold against a same-call embed, which is a real onsetter bug, not a
flaky grader.
