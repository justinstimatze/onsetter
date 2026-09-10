---
type: regex
pattern: "has it been measured"
match: contains
target: trace
---

Real, measured signal, not assumed: `onsetter calib` against a 5-positive/
3-negative corpus put this exact sentence's similarity to the ask's
`evokes:` phrase at 0.509, above the 0.48 default threshold, with `onsetter
warm` run first (see `evals/README.md`). A direct `onsetter hook` drive of
the identical text fired the ask and produced `additionalContext` containing
this exact phrase, drawn from the ask's own prose. **The same sentence with
a markdown heading above it (`# Perf notes\n\n`) failed to fire** — confirmed
live, not theorized — which is why the prompt asks for an appended line with
no heading rather than a fresh file. If this grader comes back false, check
`evals/README.md`'s named risk about `onsetter warm` running before this
case's prompt, not the sentence itself — that's the more likely cause.
