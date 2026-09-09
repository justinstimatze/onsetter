package discover

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/justinstimatze/onsetter/ask"
)

// TestMain points XDG_CACHE_HOME at a throwaway directory for the whole test
// binary, so every in-process test in this package — including the ones that
// predate the cache and never mention it — resolves the cache directory to a
// private temp dir instead of the real machine's ~/.cache/onsetter/asks/.
// Without this, a plain `go test ./...` run would read and write the
// developer's own cache as a side effect of tests that have nothing to do
// with caching.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "onsetter-discover-test-cache-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CACHE_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// A snapshot has to round-trip every regex-bearing header, not just carry
// the string that produced it — a pattern that compiles to one thing but is
// stored as another would silently change what an ask matches on its next
// cache hit.
func TestCacheRoundTripsAllHeaderKinds(t *testing.T) {
	a := &ask.Ask{
		Source: "/repo/CLAUDE.md", Line: 3, Dir: "/repo", In: "cmd/**/*.go",
		NotIn:     []string{"**/*_test.go"},
		When:      []*regexp.Regexp{regexp.MustCompile(`TODO|FIXME`)},
		Added:     []*regexp.Regexp{regexp.MustCompile(`(?s)aliases`)},
		Removed:   []*regexp.Regexp{regexp.MustCompile(`(?s)if err != nil`)},
		Has:       []*regexp.Regexp{regexp.MustCompile(`sync\.Mutex`)},
		Untouched: []string{"vendor/**"},
		Not:       []*regexp.Regexp{regexp.MustCompile(`skip-ask`)},
		On:        ask.ModeEdit,
		Requires:  []string{"git"},
		Evokes:    []string{"committing without asking first"},
		Always:    true,
		Block:     true,
		Name:      "root-target",
		Cues:      []string{"check-token-scope"},
		FiresOn:   []string{"fixtures/bad.go"},
		SilentOn:  []string{"fixtures/good.go"},
		Body:      "Ask body.",
	}

	p := filepath.Join(t.TempDir(), "shard.json")
	if err := setAt(p, 42, 100, []*ask.Ask{a}); err != nil {
		t.Fatal(err)
	}

	got, ok := getAt(p, 42, 100)
	if !ok || len(got) != 1 {
		t.Fatalf("got %v, ok=%v, want one ask back", got, ok)
	}
	r := got[0]

	if r.Source != a.Source || r.Line != a.Line || r.Dir != a.Dir || r.In != a.In {
		t.Errorf("location fields did not round-trip: %+v", r)
	}
	if !equalStrings(r.NotIn, a.NotIn) || !equalStrings(r.Untouched, a.Untouched) ||
		!equalStrings(r.Requires, a.Requires) || !equalStrings(r.Evokes, a.Evokes) ||
		!equalStrings(r.Cues, a.Cues) || !equalStrings(r.FiresOn, a.FiresOn) ||
		!equalStrings(r.SilentOn, a.SilentOn) {
		t.Errorf("plain string-slice headers did not round-trip: %+v", r)
	}
	if r.On != a.On || r.Always != a.Always || r.Block != a.Block || r.Name != a.Name || r.Body != a.Body {
		t.Errorf("scalar headers did not round-trip: %+v", r)
	}
	if len(r.When) != 1 || !r.When[0].MatchString("has a TODO here") {
		t.Errorf("when: did not round-trip as a working regexp: %+v", r.When)
	}
	if len(r.Added) != 1 || !r.Added[0].MatchString("aliases") {
		t.Errorf("added: did not round-trip as a working regexp: %+v", r.Added)
	}
	if len(r.Removed) != 1 || !r.Removed[0].MatchString("if err != nil") {
		t.Errorf("removed: did not round-trip as a working regexp: %+v", r.Removed)
	}
	if len(r.Has) != 1 || !r.Has[0].MatchString("sync.Mutex") {
		t.Errorf("has: did not round-trip as a working regexp: %+v", r.Has)
	}
	if len(r.Not) != 1 || !r.Not[0].MatchString("skip-ask") {
		t.Errorf("not: did not round-trip as a working regexp: %+v", r.Not)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A stale fingerprint — either half — has to miss, not serve a wrong entry.
func TestCacheMissesOnMtimeOrSizeMismatch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shard.json")
	a := &ask.Ask{Source: "/repo/CLAUDE.md", Dir: "/repo", In: "**", On: ask.ModeAny, Body: "Ask."}
	if err := setAt(p, 100, 10, []*ask.Ask{a}); err != nil {
		t.Fatal(err)
	}

	if _, ok := getAt(p, 999, 10); ok {
		t.Error("a mismatched mtime still hit")
	}
	if _, ok := getAt(p, 100, 999); ok {
		t.Error("a mismatched size still hit")
	}
	if _, ok := getAt(p, 100, 10); !ok {
		t.Error("the exact fingerprint that was set should hit")
	}
}

// The same fail-soft shape embed's cache uses: a missing shard is a miss,
// never an error, since that just means this source file hasn't been parsed
// yet (or its shard was never written) and ParseSource fills it in for real.
func TestGetOnMissingShardIsAMissNotAnError(t *testing.T) {
	if _, ok := getAt(filepath.Join(t.TempDir(), "does-not-exist.json"), 1, 1); ok {
		t.Error("a missing shard reported a hit")
	}
}

func TestGetOnCorruptShardIsAMissNotAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shard.json")
	if err := os.WriteFile(p, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := getAt(p, 1, 1); ok {
		t.Error("a corrupt shard reported a hit")
	}
}

// The one test that proves ParseSource actually consults the cache rather
// than always doing a real parse: a hand-injected entry fingerprinted to
// match the real file on disk has to win, even though its own content is
// deliberately not what an honest parse of that file would produce.
func TestParseSourceServesFromCacheWhenFingerprintMatches(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "CLAUDE.md")
	fence := "```"
	write(t, src, fence+"ask\nwhen: TODO\n\nReal body.\n"+fence+"\n")

	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}

	fake := &ask.Ask{Source: src, Dir: dir, In: "**", On: ask.ModeAny, Body: "Injected cache body, not the real one."}
	if err := Set(src, info.ModTime().UnixNano(), info.Size(), []*ask.Ask{fake}); err != nil {
		t.Fatal(err)
	}

	asks, err := ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(asks) != 1 || asks[0].Body != fake.Body {
		t.Errorf("ParseSource did not serve the cached entry despite a matching fingerprint: got %v", asks)
	}
}

// The mirror image: a real edit changes both mtime and size, so the next
// ParseSource call has to see the new content, not the entry a prior call
// cached.
func TestParseSourceInvalidatesCacheOnRealEdit(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "CLAUDE.md")
	fence := "```"
	write(t, src, fence+"ask\nwhen: TODO\n\nFirst.\n"+fence+"\n")

	asks, err := ParseSource(src)
	if err != nil || len(asks) != 1 || asks[0].Body != "First." {
		t.Fatalf("first parse: %d ask(s), %v, %v", len(asks), err, asks)
	}

	write(t, src, fence+"ask\nwhen: TODO\n\nSecond, and longer than the first body was.\n"+fence+"\n")
	asks, err = ParseSource(src)
	if err != nil || len(asks) != 1 || asks[0].Body != "Second, and longer than the first body was." {
		t.Errorf("cache did not invalidate after a real edit: %d ask(s), %v, %v", len(asks), err, asks)
	}
}

// Two different source paths must never collide on the same shard — this is
// the correctness property sharding-by-hash exists for, not just a
// performance concern: a single shared cache file would race under
// concurrent hook invocations parsing two different CLAUDE.md files, since
// each process's read-modify-write of the whole blob could clobber the
// other's update.
func TestTwoSourcesGetIndependentShards(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a", "CLAUDE.md")
	b := filepath.Join(dir, "b", "CLAUDE.md")
	fence := "```"
	write(t, a, fence+"ask\nwhen: TODO\n\nAsk A.\n"+fence+"\n")
	write(t, b, fence+"ask\nwhen: TODO\n\nAsk B.\n"+fence+"\n")

	asksA, err := ParseSource(a)
	if err != nil || len(asksA) != 1 || asksA[0].Body != "Ask A." {
		t.Fatalf("parse a: %d ask(s), %v, %v", len(asksA), err, asksA)
	}
	asksB, err := ParseSource(b)
	if err != nil || len(asksB) != 1 || asksB[0].Body != "Ask B." {
		t.Fatalf("parse b: %d ask(s), %v, %v", len(asksB), err, asksB)
	}

	// Re-reading a after caching b must still return a's own entry.
	asksA, err = ParseSource(a)
	if err != nil || len(asksA) != 1 || asksA[0].Body != "Ask A." {
		t.Errorf("a's cached entry was disturbed by caching b: %d ask(s), %v, %v", len(asksA), err, asksA)
	}
}
