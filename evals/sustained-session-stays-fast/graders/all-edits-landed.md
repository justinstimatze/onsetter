---
type: tool_used
tool: Write
input_match: "mod_\\d\\.py"
min: 8
max: 8
---

Correctness proxy for the real question this case exists to ask: every
`PreToolUse` call in the session routes through the same warm `onsetter
serve` process (`.mcp.json`/`hooks.json`'s `mcp_tool` wiring, landed in
Phase 2), and none of the eight edits should stall, time out, or get
dropped as the ask count and session state accumulate across the run. All
eight `Write` calls landing is necessary but not sufficient — it can't
distinguish "fast the whole way through" from "correct but degrading."

The actual latency signal has no grader here because no documented grader
type asserts wall-clock: read it from `--json`'s per-call timing in the
run output after a real pass, and compare it against the subprocess-per-call
baseline the roadmap's Phase 2 already measured with `hyperfine` (bare
`onsetter hook` invocations, cold vs. warm, ~6–8ms either way at this
repo's own two-ask scale — the persistent server's whole premise is that a
`serve`-backed session shouldn't pay that floor eight times over).
