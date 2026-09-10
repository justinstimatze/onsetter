---
type: regex
pattern: "swallows every error"
match: contains
target: trace
---

The fixture's `when:`-gated ask fires only when its injected prose reaches
the transcript. Verified directly against onsetter's own CLI (not guessed):
feeding `onsetter hook` the same shape of `Write` — a bare `except:` under
`src/**/*.py` — produces `additionalContext` containing this exact phrase,
drawn verbatim from the ask's own body in the fixture `CLAUDE.md`. Under
`--ablation with-without`, this should score present in the `with` arm and
absent in the `without` arm — the delta is the actual thing being tested,
not just whether the phrase shows up once.
