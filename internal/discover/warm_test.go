package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// A Warm hit has to return the compiled asks from memory, not re-derive them
// — proven here by pointer identity on a regex field, which round-tripping
// through the on-disk shard cache (Get/Set in cache.go) could never produce,
// since that path always calls regexp.Compile fresh on every hit.
func TestWarmAsksReturnsSamePointerOnRepeatedCalls(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	fence := "```"
	write(t, filepath.Join(dir, "CLAUDE.md"), fence+"ask\nwhen: TODO\n\nAsk.\n"+fence+"\n")
	target := filepath.Join(dir, "a.md")
	write(t, target, "x")

	w := NewWarm()
	first, errs := w.Asks(target)
	if len(errs) != 0 || len(first) != 1 {
		t.Fatalf("first call: %d ask(s), %v", len(first), errs)
	}
	second, errs := w.Asks(target)
	if len(errs) != 0 || len(second) != 1 {
		t.Fatalf("second call: %d ask(s), %v", len(second), errs)
	}
	if first[0] != second[0] {
		t.Error("a Warm hit returned a different *ask.Ask, not the cached one")
	}
	if len(first[0].When) != 1 || first[0].When[0] != second[0].When[0] {
		t.Error("a Warm hit recompiled the when: regexp instead of reusing it")
	}
}

// A real edit changes both mtime and size, so the next call has to see the
// new content — the same invalidation property cache_test.go proves for the
// on-disk layer, now proven for the in-memory one sitting above it.
func TestWarmAsksInvalidatesOnRealEdit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	fence := "```"
	target := filepath.Join(dir, "a.md")
	write(t, target, "x")
	claude := filepath.Join(dir, "CLAUDE.md")
	write(t, claude, fence+"ask\nwhen: TODO\n\nFirst.\n"+fence+"\n")

	w := NewWarm()
	asks, errs := w.Asks(target)
	if len(errs) != 0 || len(asks) != 1 || asks[0].Body != "First." {
		t.Fatalf("first parse: %d ask(s), %v, %v", len(asks), errs, asks)
	}

	write(t, claude, fence+"ask\nwhen: TODO\n\nSecond, and longer than the first body was.\n"+fence+"\n")
	asks, errs = w.Asks(target)
	if len(errs) != 0 || len(asks) != 1 || asks[0].Body != "Second, and longer than the first body was." {
		t.Errorf("in-memory cache did not invalidate after a real edit: %d ask(s), %v, %v", len(asks), errs, asks)
	}
}

// Two different source files must never collide in the same Warm — each
// governs its own path, and a hit for one must never disturb the other's
// entry.
func TestWarmAsksKeepsTwoSourcesIndependent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	fence := "```"
	a := filepath.Join(dir, "a", "CLAUDE.md")
	b := filepath.Join(dir, "b", "CLAUDE.md")
	write(t, a, fence+"ask\nwhen: TODO\n\nAsk A.\n"+fence+"\n")
	write(t, b, fence+"ask\nwhen: TODO\n\nAsk B.\n"+fence+"\n")
	targetA := filepath.Join(dir, "a", "x.md")
	targetB := filepath.Join(dir, "b", "x.md")
	write(t, targetA, "x")
	write(t, targetB, "x")

	w := NewWarm()
	asksA, _ := w.Asks(targetA)
	asksB, _ := w.Asks(targetB)
	if len(asksA) != 1 || asksA[0].Body != "Ask A." {
		t.Fatalf("a: %v", asksA)
	}
	if len(asksB) != 1 || asksB[0].Body != "Ask B." {
		t.Fatalf("b: %v", asksB)
	}

	asksA, _ = w.Asks(targetA)
	if len(asksA) != 1 || asksA[0].Body != "Ask A." {
		t.Errorf("a's cached entry was disturbed by caching b: %v", asksA)
	}
}

// A file with no .git anywhere above it still resolves through Warm the
// same way it does through plain Asks — Roots itself is untouched by this
// layer, only ParseSource is.
func TestWarmAsksHandlesAMissingSourceFileTheSameAsPlainAsks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "fresh-project", "a.md")
	write(t, target, "x")

	w := NewWarm()
	asks, errs := w.Asks(target)
	if len(asks) != 0 || len(errs) != 0 {
		t.Errorf("want nothing for a target with no governing CLAUDE.md anywhere: %d ask(s), %v", len(asks), errs)
	}
}
