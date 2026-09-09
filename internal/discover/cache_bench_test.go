package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/justinstimatze/onsetter/ask"
)

// buildBigClaudeMD writes a synthetic n-ask CLAUDE.md — the fixture that
// found the disk cache's real cost. 150 asks each with a when:/not: regex
// pair is realistic for a large monorepo's accumulated CLAUDE.md, not a
// contrived worst case.
func buildBigClaudeMD(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		f.WriteString("```ask\nin: src/**/*.go\nwhen: pattern-" + s + "-(alpha|beta|gamma)[0-9]+\nnot: skip-" + s + "\n\n")
		f.WriteString("This is ask number " + s + ", a moderately real-looking regex gate with some prose body text.\n```\n\n")
	}
	return path
}

// The baseline: a real text-scan-and-compile, no cache at all.
func BenchmarkParseFileScopedCold(b *testing.B) {
	src := buildBigClaudeMD(b, 150)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ask.ParseFileScoped(src, ""); err != nil {
			b.Fatal(err)
		}
	}
}

// The sharded on-disk cache's own hit path — real and correct (see
// cache_test.go), but roughly even with the cold baseline above at this
// scale, since a hit still recompiles every regex from its stored source
// string. This is what proved the disk cache alone doesn't remove the
// dominant cost; internal/discover.Warm (warm_test.go) is what actually
// does, by skipping recompilation too.
func BenchmarkParseSourceWarmCache(b *testing.B) {
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	src := buildBigClaudeMD(b, 150)
	if _, err := ParseSource(src); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseSource(src); err != nil {
			b.Fatal(err)
		}
	}
}

// The bug this shape found, isolated: decoding one source path's worth of
// entries out of a single aggregate JSON blob keyed by every path this
// machine has ever parsed. With only one entry in it, this already cost
// more than the cold parse it was meant to replace — the aggregate design
// never shipped past this benchmark.
func BenchmarkJSONDecodeOfOneAggregateEntry(b *testing.B) {
	src := buildBigClaudeMD(b, 150)
	asks, err := ask.ParseFileScoped(src, "")
	if err != nil {
		b.Fatal(err)
	}
	snaps := make([]askSnapshot, len(asks))
	for i, a := range asks {
		snaps[i] = snapshot(a)
	}
	blob := map[string]fileEntry{src: {ModTime: 1, Size: 1, Asks: snaps}}
	data, err := json.Marshal(blob)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m map[string]fileEntry
		if err := json.Unmarshal(data, &m); err != nil {
			b.Fatal(err)
		}
	}
}

// The fix, measured directly: a Warm hit skips both the JSON decode and the
// regex recompile the on-disk cache above still pays.
func BenchmarkWarmAsksHit(b *testing.B) {
	dir := b.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		b.Fatal(err)
	}
	claude := buildBigClaudeMD(b, 150)
	if err := os.Rename(claude, filepath.Join(dir, "CLAUDE.md")); err != nil {
		b.Fatal(err)
	}
	target := filepath.Join(dir, "src", "a.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package a\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	w := NewWarm()
	if _, errs := w.Asks(target); len(errs) != 0 {
		b.Fatal(errs)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, errs := w.Asks(target); len(errs) != 0 {
			b.Fatal(errs)
		}
	}
}
