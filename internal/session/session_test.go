package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeOldFormatSessionFile writes a session file the way this package did
// before Record started appending a count — bare lines, no trailing
// ":count" — to exercise the degrade-to-fresh path on the next Open.
func writeOldFormatSessionFile(t *testing.T, id string, keys []string) {
	t.Helper()
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "onsetter", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, []byte(strings.Join(keys, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A reminder has nothing to quote, so its key is the bare ask ID and it
// suppresses on the second sighting however different the edit was.
func TestKeyWithoutAMatchIsTheAskItself(t *testing.T) {
	if got := Key("abc123", "", "anything"); got != "abc123" {
		t.Errorf("got %q, want the bare id", got)
	}
}

// An inspection keys on the quote, so a different quote is a different
// question.
func TestKeyVariesWithTheQuote(t *testing.T) {
	a := Key("abc123", "you nod", "and you nod slowly")
	b := Key("abc123", "you realize", "and you realize slowly")
	if a == b {
		t.Error("two different quotes produced the same key")
	}
	if a == Key("abc123", "", "") {
		t.Error("a quoted firing collided with the unquoted one")
	}
}

// The whole fix: two edits quoting identical text are two different
// occurrences once the edit around the quote differs — the collision this
// package used to have when the key was the match alone.
func TestKeyVariesWithTheEditNotJustTheQuote(t *testing.T) {
	a := Key("abc123", "you nod", "betty.md: and you nod slowly")
	b := Key("abc123", "you nod", "art_callahan.md: and you nod once")
	if a == b {
		t.Error("the same quote in two different edits produced the same key")
	}
}

// The same edit asked about twice is still the same occurrence — Record is
// what decides whether that occurrence gets counted again, not Key.
func TestKeyIsStableForTheSameEdit(t *testing.T) {
	a := Key("abc123", "TODO", "TODO: fix this")
	b := Key("abc123", "TODO", "TODO: fix this")
	if a != b {
		t.Error("the same quote and edit produced two different keys")
	}
}

// Two asks that happen to match the same string stay separate questions.
func TestKeySeparatesAsksSharingAQuote(t *testing.T) {
	if Key("abc123", "TODO", "x") == Key("def456", "TODO", "x") {
		t.Error("different asks with the same quote produced the same key")
	}
}

// The store is one key per line, and a match can be up to 80 bytes of whatever
// the file contained — including newlines, which clip does not strip. Hashing
// is what keeps one firing from writing two lines and poisoning the set.
func TestKeySurvivesAMatchContainingNewlines(t *testing.T) {
	k := Key("abc123", "a parenthetical\nthat spans\nthree lines", "edit")
	if strings.ContainsAny(k, "\n\r") {
		t.Errorf("key contains a line break: %q", k)
	}
}

// Round-trip through the file: what Record writes, Count reads back — and a
// repeat of the same key increments rather than staying at one, since a
// matched ask's whole point now is that a repeat still counts.
func TestRecordAndCountRoundTripThroughDisk(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	nod := Key("abc123", "you nod", "and you nod")
	realize := Key("abc123", "you realize", "and you realize")

	s := Open("sess-1")
	if s.Count(nod) != 0 {
		t.Fatal("a fresh session already has a count for the key")
	}
	s.Record(nod)
	s.Record(nod) // a genuine repeat: still counts, doesn't collapse

	reopened := Open("sess-1")
	if got := reopened.Count(nod); got != 2 {
		t.Errorf("Count = %d after two Records, want 2", got)
	}
	if reopened.Count(realize) != 0 {
		t.Error("recording one quote gave a count to a different one")
	}
	if !reopened.Fired(nod) {
		t.Error("a key with count > 0 should report Fired")
	}
	if reopened.Fired(realize) {
		t.Error("a never-recorded key reported Fired")
	}
}

// A session file written before counts existed (a bare key or key:hash with
// no trailing count) fails to parse as the new format and is dropped —
// self-healing, that occurrence just starts counting fresh in this session,
// rather than erroring or corrupting every other line.
func TestPreUpgradeFormatDegradesToFreshRatherThanErroring(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	nod := Key("abc123", "you nod", "and you nod")
	writeOldFormatSessionFile(t, "sess-old", []string{"abc123", nod})

	reopened := Open("sess-old")
	if got := reopened.Count(nod); got != 0 {
		t.Errorf("an old-format line should not parse into a count, got %d", got)
	}
	reopened.Record(nod)
	if got := Open("sess-old").Count(nod); got != 1 {
		t.Errorf("after recording once against a degraded store, want count 1, got %d", got)
	}
}

// A blank session id yields a store that never persists, which degrades to
// asking every time rather than to failing.
func TestBlankSessionIDNeverSuppresses(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	k := Key("abc123", "you nod", "and you nod")
	s := Open("")
	s.Record(k)
	if Open("").Count(k) != 0 {
		t.Error("a store with no session id persisted something")
	}
}
