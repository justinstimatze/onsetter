---
type: tool_used
tool: Write
input_match: "notes\\.txt"
min: 1
scored: false
---

Sanity check that the scenario actually ran — the agent wrote `notes.txt`
at all. Unscored: without this, a run where the agent wrote nothing would
also pass `ask-silent.md` vacuously (no write, no ask, no onsetter), which
proves nothing about the gate actually staying quiet on a real edit.
