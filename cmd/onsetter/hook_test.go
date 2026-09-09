package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The only response shape onsetter emits must be the one PreToolUse accepts.
// A sibling project shipped a PreCompact hook using additionalContext, where
// the field is not supported, and it silently failed schema validation on the
// first real compaction. This pins the shape so that cannot happen quietly.
func TestOutputShapeIsLegalForPreToolUse(t *testing.T) {
	var o out
	o.HookSpecificOutput.HookEventName = "PreToolUse"
	o.HookSpecificOutput.AdditionalContext = "x"
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"x"}}`
	if string(b) != want {
		t.Errorf("shape drifted:\n got %s\nwant %s", b, want)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "onsetter")
	// -cover instruments the binary so a subprocess run of it (which is how
	// every test in this package exercises it — JSON over stdin, exactly the
	// way Claude Code does) contributes real coverage data instead of being
	// invisible to `go test -cover`, which only ever sees the test binary's
	// own in-process calls. Confirmed separately that an instrumented binary
	// still flushes counters on a bare os.Exit(0) path (this package's own
	// dispatch shape) as long as GOCOVERDIR is set — see goCoverDir below.
	cmd := exec.Command("go", "build", "-cover", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// goCoverDir resolves where a -cover-built subprocess should write its
// coverage counters. `go test` reserves the GOCOVERDIR name for its own
// in-process instrumentation — it rewrites whatever the shell exported into
// an internal go-build scratch path before the test binary ever sees it
// (confirmed directly: printing os.Getenv("GOCOVERDIR") from inside a test
// shows .../go-build.../b001/gocoverdir, never the shell's own value) — so
// `make coverage` passes the shared directory through a distinct name,
// ONSETTER_TEST_GOCOVERDIR, that go test has no reason to touch. Falling
// back to the test's own TempDir when it isn't set (the plain `go test ./...`
// path) matters for a reason beyond just "coverage gets dropped": an
// instrumented binary run with GOCOVERDIR unset prints "warning: GOCOVERDIR
// not set" to stderr on every invocation, which corrupts any test asserting
// on cmd.CombinedOutput() (runIn/runStatus, below) — always setting it, even
// to a directory nobody reads afterward, is what keeps every existing
// output-content assertion in this package correct regardless of how the
// suite is invoked.
func goCoverDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("ONSETTER_TEST_GOCOVERDIR"); d != "" {
		return d
	}
	return t.TempDir()
}

// run invokes the built hook the way Claude Code does: JSON on stdin.
func run(t *testing.T, bin, cache string, p map[string]any) string {
	t.Helper()
	in, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "hook")
	cmd.Stdin = strings.NewReader(string(in))
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	return string(out)
}

func TestHookEndToEnd(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus", "chars"))
	write(t, filepath.Join(repo, "corpus", "CLAUDE.md"),
		"# corpus\n\n```ask\nin: chars/**\nwhen: you (nod|realize)\n\nThe narrator describes the world.\n```\n")
	target := filepath.Join(repo, "corpus", "chars", "betty.md")
	write(t, target, "old text")

	sid := "sess-abc"
	got := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is lying"},
	})
	if !strings.Contains(got, "The narrator describes the world.") {
		t.Fatalf("ask body missing from output:\n%s", got)
	}
	if !strings.Contains(got, `matched \"you realize\"`) {
		t.Errorf("output does not quote the match back:\n%s", got)
	}
	if !strings.Contains(got, "CLAUDE.md:3") {
		t.Errorf("output does not name its source line:\n%s", got)
	}
	if strings.Contains(got, "already this session") {
		t.Errorf("a first firing should carry no repeat marker:\n%s", got)
	}

	// Same session, same quote, a different edit around it: fires again,
	// still unmarked — the key includes the edit, not just the quote, so
	// this is a genuinely new occurrence, not a repeat of the first one.
	// This is exactly the collision fix: "you realize" in one edit no
	// longer shares an identity with "you realize" in a different one.
	again := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is stalling"},
	})
	if !strings.Contains(again, `matched \"you realize\"`) {
		t.Errorf("the same quote in a different edit should still fire:\n%s", again)
	}
	if strings.Contains(again, "already this session") {
		t.Errorf("a different edit is a new occurrence and should carry no marker:\n%s", again)
	}

	// A genuine repeat — the identical edit, byte for byte, as the one
	// above — is the same occurrence, and it still fires, now marked. There
	// is no floor left where an exact repeat stays silent.
	exact := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is stalling"},
	})
	if !strings.Contains(exact, "asked 1× already this session") {
		t.Errorf("a byte-identical repeat of the prior edit should fire, marked as a repeat:\n%s", exact)
	}

	// Same session, same ask, different quote: fires, no marker — this is
	// its first time.
	other := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "you nod again"},
	})
	if !strings.Contains(other, `matched \"you nod\"`) {
		t.Errorf("a new quote did not fire the ask:\n%s", other)
	}
	if strings.Contains(other, "already this session") {
		t.Errorf("a different occurrence's first firing should carry no marker:\n%s", other)
	}

	// New session: no marker, count starts over.
	fresh := run(t, bin, cache, map[string]any{
		"session_id": "sess-xyz", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "you nod again"},
	})
	if fresh == "" {
		t.Error("ask did not fire in a new session")
	}
	if strings.Contains(fresh, "already this session") {
		t.Errorf("a fresh session should not carry a marker from a different session:\n%s", fresh)
	}
}

// A plain matched ask already fires on every occurrence now (see
// TestHookEndToEnd) — what always: still uniquely buys is skipping the
// repeat marker. Without it, this exact scenario would print "asked 1×
// already" / "asked 2× already" on passes two and three; always: keeps
// every firing looking identical, which is the point for a near-check ask
// where every matching edit is already expected to be suspect.
func TestAlwaysNeverSuppresses(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: you (nod|realize)\nalways: true\n\nEvery matching edit is suspect.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "old text")

	sid := "sess-always"
	for i := 0; i < 3; i++ {
		got := run(t, bin, cache, map[string]any{
			"session_id": sid, "tool_name": "Edit",
			"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is lying"},
		})
		if !strings.Contains(got, "Every matching edit is suspect.") {
			t.Fatalf("pass %d: always: true stayed quiet on an identical repeat:\n%s", i, got)
		}
		if !strings.Contains(got, "· always") {
			t.Errorf("pass %d: injection did not mark the ask as always:\n%s", i, got)
		}
		if strings.Contains(got, "already this session") {
			t.Errorf("pass %d: always: true should never carry a repeat-count marker:\n%s", i, got)
		}
	}
}

// A no-content-gate always: true ask fires on every matching file, not just
// the first — the reminder case still isn't suppressed once per session.
func TestAlwaysPathOnlyFiresOnEveryFile(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "corpus", "CLAUDE.md"),
		"```ask\nalways: true\n\nDoes this make sense for the world?\n```\n")

	sid := "sess-always-reminder"
	one := filepath.Join(repo, "corpus", "one.md")
	two := filepath.Join(repo, "corpus", "two.md")
	write(t, one, "old")
	write(t, two, "old")

	if got := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": one, "new_string": "x"},
	}); !strings.Contains(got, "Does this make sense for the world?") {
		t.Fatalf("did not fire on the first file:\n%s", got)
	}
	if got := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": two, "new_string": "x"},
	}); !strings.Contains(got, "Does this make sense for the world?") {
		t.Fatalf("always: true suppressed a no-content-gate ask on a second file:\n%s", got)
	}
}

// A path-only ask is a reminder: the reader needs to know a standard exists,
// and once they know it, repeating it is noise. It quotes nothing, so there is
// nothing to key on but the ask itself, and it fires once per session however
// many files it governs. This is what every ask did before the key carried a
// match, and the case that would regress if the key ever included the path.
func TestPathOnlyAskFiresOncePerSession(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus", "chars"))
	write(t, filepath.Join(repo, "corpus", "CLAUDE.md"),
		"# corpus\n\n```ask\nin: chars/**\n\nDoes this make sense for the world?\n```\n")

	sid := "sess-reminder"
	one := filepath.Join(repo, "corpus", "chars", "one.md")
	two := filepath.Join(repo, "corpus", "chars", "two.md")
	write(t, one, "old")
	write(t, two, "old")

	got := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": one, "new_string": "anything at all"},
	})
	if !strings.Contains(got, "Does this make sense for the world?") {
		t.Fatalf("reminder did not fire on the first edit:\n%s", got)
	}

	if again := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": two, "new_string": "something else entirely"},
	}); again != "" {
		t.Errorf("reminder fired again, on a different file with different text:\n%s", again)
	}
}

func TestHookSilentOnNoMatch(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: NEVERMATCHES\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	if got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "ordinary text"},
	}); got != "" {
		t.Errorf("want silence, got:\n%s", got)
	}
}

// One hook in front of every Write and Edit is a single point of failure. The
// only acceptable failure is the silent, harmless kind.
func TestHookFailsOpen(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	for name, stdin := range map[string]string{
		"garbage":      "not json at all",
		"empty":        "",
		"no file_path": `{"session_id":"s","tool_name":"Write","tool_input":{}}`,
		"null input":   `{"tool_input":null}`,
	} {
		cmd := exec.Command(bin, "hook")
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: exited non-zero (%v): %s", name, err, out)
		}
		if len(out) != 0 {
			t.Errorf("%s: emitted output: %s", name, out)
		}
	}
}

// Every input TestHookFailsOpen feeds is absorbed by an earlier explicit
// return nil (bad JSON, empty input) — none of them ever reaches the
// deferred recover() at all, so that test proves ordinary error handling
// works, not that a genuine panic gets caught. This one forces a real panic
// partway through the real dispatch path (after stdin parsing and ask
// discovery have already run, via ONSETTER_TEST_PANIC — see hook.go) and
// asserts the same contract: exit 0, no output.
func TestHookRecoversFromARealPanic(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: alpha\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	in, err := json.Marshal(map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "hook")
	cmd.Stdin = strings.NewReader(string(in))
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t), "ONSETTER_TEST_PANIC=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("a real panic mid-dispatch should still exit 0, got %v:\n%s", err, out)
	}
	if len(out) != 0 {
		t.Errorf("a recovered panic should still emit nothing, got:\n%s", out)
	}
}

func TestHookCoalescesMultipleRules(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: alpha\n\nRoot question.\n```\n")
	write(t, filepath.Join(repo, "corpus", "CLAUDE.md"), "```ask\nwhen: alpha\n\nLocal question.\n```\n")
	target := filepath.Join(repo, "corpus", "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Root question.") || !strings.Contains(got, "Local question.") {
		t.Errorf("both asks should arrive in one block:\n%s", got)
	}
	if strings.Count(got, "hookSpecificOutput") != 1 {
		t.Errorf("want a single coalesced block:\n%s", got)
	}
	if !strings.Contains(got, "2 asks") {
		t.Errorf("want the count in the header:\n%s", got)
	}
}

// The walk stops at the repo root. An ask two repos up has no business
// asking about a file it has never seen.
func TestDiscoveryStopsAtRepoRoot(t *testing.T) {
	bin := buildBinary(t)
	outer := t.TempDir()
	cache := t.TempDir()
	write(t, filepath.Join(outer, "CLAUDE.md"), "```ask\nwhen: alpha\n\nOuter question.\n```\n")
	repo := filepath.Join(outer, "inner")
	mkdir(t, filepath.Join(repo, ".git"))
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	if got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	}); got != "" {
		t.Errorf("an ask above the repo root fired:\n%s", got)
	}
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Claude Code loads <root>/.claude/CLAUDE.md as the project's file. Asks from
// it must scope to the project directory: scoping them to .claude/ would make
// every `in:` glob match nothing, and the file would look wired while being
// dead. A repo keeps its conventions there precisely because `.claude/*` is
// gitignored and must not ship with the tree it publishes.
func TestDotClaudeScopesToTheProjectNotToDotClaude(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, ".claude"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, ".claude", "CLAUDE.md"),
		"```ask\nin: corpus/*.yaml\nwhen: alpha\n\nScoped to the project root.\n```\n")
	target := filepath.Join(repo, "corpus", "a.yaml")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Scoped to the project root.") {
		t.Fatalf("an ask in .claude/CLAUDE.md did not reach corpus/:\n%s", got)
	}

	// And the scope is real, not a wildcard: a sibling directory the glob does
	// not name stays untouched.
	mkdir(t, filepath.Join(repo, "docs"))
	other := filepath.Join(repo, "docs", "a.yaml")
	write(t, other, "x")
	if got := run(t, bin, cache, map[string]any{
		"session_id": "s2", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": other, "content": "alpha"},
	}); got != "" {
		t.Errorf("in: corpus/*.yaml fired on docs/:\n%s", got)
	}
}

// `untouched:` is the paired-file gate: change the schema without going near a
// migration and it asks; do the migration first and it stays quiet. Both halves
// need two edits in one session, so this drives the real binary twice.
func TestUntouchedIsSatisfiedByAnEarlierEditInTheSameSession(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "migrations"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: schema.go\nuntouched: migrations/*.sql\n\nSchema changed, no migration touched.\n```\n")
	schema := filepath.Join(repo, "schema.go")
	migration := filepath.Join(repo, "migrations", "003_add_col.sql")
	write(t, schema, "package main")
	write(t, migration, "-- up")

	edit := func(cache, target, text string) string {
		return run(t, bin, cache, map[string]any{
			"session_id": "s", "tool_name": "Edit",
			"tool_input": map[string]any{"file_path": target, "new_string": text},
		})
	}

	// Schema first, migration never touched: it asks.
	cold := t.TempDir()
	if got := edit(cold, schema, "type User struct{ Email string }"); !strings.Contains(got, "no migration touched") {
		t.Errorf("want the ask on a lone schema edit, got:\n%s", got)
	}

	// Migration first, then schema, same session: nothing to ask about.
	warm := t.TempDir()
	edit(warm, migration, "ALTER TABLE users ADD COLUMN email text;")
	if got := edit(warm, schema, "type User struct{ Email string }"); got != "" {
		t.Errorf("migration was touched first; want silence, got:\n%s", got)
	}
}

// on: read fires on a real Read tool call, and quotes nothing back — has:
// is the content-shaped gate that still works against a Read, since it
// reads disk content rather than incoming text.
func TestOnReadFiresOnAReadCall(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\non: read\n\nNever read the corpus directly before generating.\n```\n")
	mkdir(t, filepath.Join(repo, "corpus"))
	target := filepath.Join(repo, "corpus", "a.md")
	write(t, target, "some corpus text")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Read",
		"tool_input": map[string]any{"file_path": target},
	})
	if !strings.Contains(got, "Never read the corpus directly before generating.") {
		t.Fatalf("on: read did not fire on a Read call:\n%s", got)
	}
	if !strings.Contains(got, "for this read") {
		t.Errorf("the banner should say \"read\", not \"edit\":\n%s", got)
	}
}

// The mismatch in the other direction: an on: read ask must never fire on
// an ordinary Write or Edit.
func TestReadNeverTriggersAWriteShapedAsk(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\non: read\n\nNever read the corpus directly.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "y"},
	})
	if got != "" {
		t.Errorf("on: read fired on an Edit call:\n%s", got)
	}
}

// A Read must never satisfy an untouched: gate — reading a file is not
// writing it, and confusing the two would corrupt untouched: for every
// other ask watching that path.
func TestReadDoesNotSatisfyUntouched(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "migrations"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: schema.go\nuntouched: migrations/*.sql\n\nSchema changed, no migration touched.\n```\n")
	schema := filepath.Join(repo, "schema.go")
	migration := filepath.Join(repo, "migrations", "003_add_col.sql")
	write(t, schema, "package main")
	write(t, migration, "-- up")

	// Read the migration — this must not count as touching it.
	run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Read",
		"tool_input": map[string]any{"file_path": migration},
	})
	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": schema, "new_string": "type User struct{ Email string }"},
	})
	if got == "" {
		t.Error("a Read of the migration satisfied untouched: — it should still ask")
	}
}

// A Bash call that writes the paired file (via a real shell redirect, not
// Write/Edit) satisfies untouched: the same way an Edit would — the whole
// point of the observer.
func TestBashObserverSatisfiesUntouched(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "migrations"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: schema.go\nuntouched: migrations/*.sql\n\nSchema changed, no migration touched.\n```\n")
	schema := filepath.Join(repo, "schema.go")
	migration := filepath.Join(repo, "migrations", "003_add_col.sql")
	write(t, schema, "package main")
	write(t, migration, "-- up")

	cache := t.TempDir()
	// A shell redirect writes the migration first, then the schema edit.
	run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Bash", "cwd": repo,
		"tool_input": map[string]any{"command": "echo 'ALTER TABLE users ADD COLUMN email text;' >> migrations/003_add_col.sql"},
	})
	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": schema, "new_string": "type User struct{ Email string }"},
	})
	if got != "" {
		t.Errorf("a Bash call writing the migration should satisfy untouched:, got:\n%s", got)
	}
}

// A Bash call never triggers ask-matching or injection on its own, even
// against a CLAUDE.md whose in: would otherwise reach the extracted path —
// it is bookkeeping only.
func TestBashCallNeverInjects(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: out.txt\n\nDoes this make sense?\n```\n")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Bash", "cwd": repo,
		"tool_input": map[string]any{"command": "echo hi > out.txt"},
	})
	if got != "" {
		t.Errorf("a Bash call should never itself trigger an ask, got:\n%s", got)
	}
}

// The edit in hand must not satisfy an `untouched:` gate about its own path,
// which is the whole reason the touch is recorded after matching.
func TestUntouchedIsNotSatisfiedByTheEditInHand(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: a.go\nuntouched: *.go\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.go")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "y"},
	})
	if got == "" {
		t.Error("the edit in hand satisfied an untouched: gate about itself")
	}
}

// A cued target's own gate — here in: nothing, an impossible glob — never
// gets checked at all. It fires because it was named, with no matched text
// of its own, cited back to the ask that cued it.
func TestCueInjectsTheTargetAsksProseWithoutCheckingItsOwnGate(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: alpha\ncues: check-token-scope\n\nCiting question.\n```\n\n"+
		"```ask\nname: check-token-scope\nin: nothing/**\n\nCued question.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Citing question.") || !strings.Contains(got, "Cued question.") {
		t.Errorf("both the citer and the cued ask should appear:\n%s", got)
	}
	if !strings.Contains(got, "cued by") {
		t.Errorf("the cued ask's line should say how it got there:\n%s", got)
	}
	if !strings.Contains(got, "2 asks") {
		t.Errorf("want the count in the header:\n%s", got)
	}
}

// A cycle (A cues B, B cues A) must not loop, and must fire each ask
// exactly once.
func TestCueCycleFiresEachAskOnce(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: alpha\nname: a\ncues: b\n\nQuestion A.\n```\n\n"+
		"```ask\nname: b\nin: nothing/**\ncues: a\n\nQuestion B.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Question A.") || !strings.Contains(got, "Question B.") {
		t.Errorf("both asks in the cycle should fire once:\n%s", got)
	}
	if !strings.Contains(got, "2 asks") {
		t.Errorf("a cycle must not loop — want exactly 2 asks in the header:\n%s", got)
	}
}

// A diamond (A and B both cue C) must fire C exactly once, not twice.
func TestCueDiamondFiresTargetOnce(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: alpha\ncues: c\n\nQuestion A.\n```\n\n"+
		"```ask\nwhen: alpha\ncues: c\n\nQuestion B.\n```\n\n"+
		"```ask\nname: c\nin: nothing/**\n\nQuestion C.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if strings.Count(got, "Question C.") != 1 {
		t.Errorf("C is cued by both A and B — want it injected exactly once:\n%s", got)
	}
	if !strings.Contains(got, "3 asks") {
		t.Errorf("want A, B and C once each — 3 asks in the header:\n%s", got)
	}
}

// A cues: value naming nothing anywhere stays silent — the same fail-soft
// shape as an unwarmed evokes: phrase or a missing requires: binary — never
// an error at hook time, which the hook's only acceptable failure mode
// forbids.
func TestDanglingCueStaysSilentNotAnError(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: alpha\ncues: nowhere-at-all\n\nQuestion A.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Question A.") {
		t.Errorf("the citer should still fire on its own merits:\n%s", got)
	}
	if !strings.Contains(got, "1 ask") || strings.Contains(got, "2 asks") {
		t.Errorf("a dangling cue must contribute nothing:\n%s", got)
	}
}

// requires: is the one gate a cued firing does not bypass — a cued ask
// naming an uninstalled tool still shouldn't inject.
func TestCuedTargetStillHonorsRequires(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: alpha\ncues: needs-tool\n\nQuestion A.\n```\n\n"+
		"```ask\nname: needs-tool\nrequires: definitely-not-a-real-binary-xyz\n\nQuestion B.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	got := run(t, bin, cache, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "alpha"},
	})
	if !strings.Contains(got, "Question A.") {
		t.Errorf("the citer should still fire:\n%s", got)
	}
	if strings.Contains(got, "Question B.") {
		t.Errorf("the cued target names a missing binary and should not inject:\n%s", got)
	}
}
