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
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
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
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
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

	// Same session, same quote: silence. That question has been answered, and
	// the answer does not change because it is a different file.
	if again := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is stalling"},
	}); again != "" {
		t.Errorf("same quote asked twice in one session:\n%s", again)
	}

	// Same session, same ask, different quote: fires. "Is this narrator
	// overreach" is a different question about `you nod` than about
	// `you realize`, and the answers can differ.
	other := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "you nod again"},
	})
	if !strings.Contains(other, `matched \"you nod\"`) {
		t.Errorf("a new quote did not re-arm the ask:\n%s", other)
	}

	// New session: it fires again.
	if fresh := run(t, bin, cache, map[string]any{
		"session_id": "sess-xyz", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "you nod again"},
	}); fresh == "" {
		t.Error("ask did not fire in a new session")
	}
}

// revisit: true is the inverted case of TestHookEndToEnd's "same quote,
// silence": the same regex match ("you realize") appears in two different
// edits, and this ask should ask about both, because the file changing again
// with the same issue still present is itself the thing worth a second look.
func TestRevisitReArmsWhenTheEditDiffersButTheQuoteDoesNot(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: you (nod|realize)\nrevisit: true\n\nStill narrator overreach.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "old text")

	sid := "sess-revisit"
	first := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is lying"},
	})
	if !strings.Contains(first, "Still narrator overreach.") {
		t.Fatalf("did not fire on the first edit:\n%s", first)
	}

	second := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is stalling"},
	})
	if !strings.Contains(second, "Still narrator overreach.") {
		t.Errorf("revisit: true stayed quiet on a different edit quoting the same text:\n%s", second)
	}

	// A genuine repeat — the identical edit, byte for byte — is still the
	// same question asked the same way, so it stays quiet even with
	// revisit: true. Only the file changing again is what re-arms it.
	third := run(t, bin, cache, map[string]any{
		"session_id": sid, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": target, "new_string": "and you realize she is stalling"},
	})
	if third != "" {
		t.Errorf("an exact repeat of the same edit fired again:\n%s", third)
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
		cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: exited non-zero (%v): %s", name, err, out)
		}
		if len(out) != 0 {
			t.Errorf("%s: emitted output: %s", name, out)
		}
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
