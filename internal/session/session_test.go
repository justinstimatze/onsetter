package session

import (
	"strings"
	"testing"
)

// A reminder has nothing to quote, so its key is the bare ask ID and it
// suppresses on the second sighting however different the edit was.
func TestKeyWithoutAMatchIsTheAskItself(t *testing.T) {
	if got := Key("abc123", ""); got != "abc123" {
		t.Errorf("got %q, want the bare id", got)
	}
}

// An inspection keys on the quote, so a different quote is a different
// question and the same quote is one already answered.
func TestKeyVariesWithTheQuote(t *testing.T) {
	a := Key("abc123", "you nod")
	b := Key("abc123", "you realize")
	if a == b {
		t.Error("two different quotes produced the same key")
	}
	if a != Key("abc123", "you nod") {
		t.Error("the same quote produced two different keys")
	}
	if a == Key("abc123", "") {
		t.Error("a quoted firing collided with the unquoted one")
	}
}

// Two asks that happen to match the same string stay separate questions.
func TestKeySeparatesAsksSharingAQuote(t *testing.T) {
	if Key("abc123", "TODO") == Key("def456", "TODO") {
		t.Error("different asks with the same quote produced the same key")
	}
}

// The store is one key per line, and a match can be up to 80 bytes of whatever
// the file contained — including newlines, which clip does not strip. Hashing
// is what keeps one firing from writing two lines and poisoning the set.
func TestKeySurvivesAMatchContainingNewlines(t *testing.T) {
	k := Key("abc123", "a parenthetical\nthat spans\nthree lines")
	if strings.ContainsAny(k, "\n\r") {
		t.Errorf("key contains a line break: %q", k)
	}
}

// Round-trip through the file: what Record writes, Fired reads back.
func TestRecordAndFiredRoundTripThroughDisk(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	nod := Key("abc123", "you nod")
	realize := Key("abc123", "you realize")

	s := Open("sess-1")
	if s.Fired(nod) {
		t.Fatal("a fresh session already has the key")
	}
	s.Record(nod)

	reopened := Open("sess-1")
	if !reopened.Fired(nod) {
		t.Error("the recorded key did not survive a reopen")
	}
	if reopened.Fired(realize) {
		t.Error("recording one quote suppressed a different one")
	}
}

// A blank session id yields a store that never persists, which degrades to
// asking every time rather than to failing.
func TestBlankSessionIDNeverSuppresses(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	k := Key("abc123", "you nod")
	s := Open("")
	s.Record(k)
	if Open("").Fired(k) {
		t.Error("a store with no session id persisted something")
	}
}
