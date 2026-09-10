---
type: regex
pattern: "onsetter — \\d+ ask"
match: not_contains
target: trace
---

`notes.txt` matches none of the fixture's three asks — not `src/**/*.py`
(the bare-except and `block:` asks), not `**/*.md` (the `evokes:` ask), and
its content shares no keyword or claim shape with any of them. Every
onsetter injection starts with the literal string `onsetter — N ask` (the
line right after the fenced example in `evals/README.md`, verified against
a real `onsetter hook` call, not guessed) — so this is a general "onsetter
said nothing at all" check, not just "this one ask stayed quiet." Under
`--ablation with-without`, this should read the same (empty) in both arms,
proving the gate doesn't over-fire rather than merely not testing it.
