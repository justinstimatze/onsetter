// Package session counts how many times each ask has already fired this
// session.
//
// It used to decide whether to show a question again at all. It doesn't
// anymore, for a matched ask: nothing measurable distinguishes an ask that
// changed an edit from one that was skimmed and ignored, so guessing "this
// one's been handled" from a persisted bit is exactly the kind of decay
// model this package has always refused to build. What it does instead is
// count, so the injection can say "asked N times already" and let the one
// thing actually good at "seen this, still the same, continuing" — the
// model reading it — do the triage. What counts as the same occurrence is
// decided by Key: the ask, plus the text it quoted back and the edit that
// produced it. A reminder (nothing quoted) still suppresses after the
// first sighting, because a second firing of a bare reminder is the
// literal same sentence, not a new occurrence with anything new to say.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Store is a per-session count of ask-occurrence keys, kept as one file of
// newline-joined `key:count` lines. Small enough that read-modify-write
// beats anything cleverer.
//
// It also keeps the set of paths written this session, in a sibling file, so
// an ask can gate on "you changed this and not that". That set is only what
// onsetter saw go past: a file rewritten by a shell command never reaches a
// PreToolUse hook on Write or Edit, so it counts as untouched.
type Store struct {
	path    string
	seen    map[string]int
	touched map[string]bool
}

// Open loads the fired-set for a session. A blank or unusable id yields a
// store that never persists, which degrades to firing every time rather than
// to failing.
func Open(id string) *Store {
	s := &Store{seen: map[string]int{}, touched: map[string]bool{}}
	if id == "" || strings.ContainsAny(id, "/\\") {
		return s
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return s
	}
	dir := filepath.Join(base, "onsetter", "sessions")
	s.path = filepath.Join(dir, id)

	if tb, err := os.ReadFile(s.path + ".paths"); err == nil {
		for _, line := range strings.Split(string(tb), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				s.touched[line] = true
			}
		}
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		// First sighting of this session; a good moment to sweep old ones,
		// since it happens once per session rather than once per edit.
		_ = os.MkdirAll(dir, 0o755)
		prune(dir, 14*24*time.Hour)
		return s
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			// key:count, split on the last colon since a matched key already
			// contains one of its own (see keyed). A line from before this
			// format existed — bare, or key:hash with no count — fails to
			// parse as a count and is dropped: that occurrence starts
			// counting fresh in this session rather than erroring, the same
			// fail-soft shape the rest of this package already uses.
			i := strings.LastIndex(line, ":")
			if i < 0 {
				continue
			}
			n, err := strconv.Atoi(line[i+1:])
			if err != nil || n <= 0 {
				continue
			}
			s.seen[line[:i]] = n
		}
	}
	return s
}

// Key is the identity of one occurrence: the ask, the text it quoted back,
// and the edit that produced it.
//
// Two kinds of ask live in the same format and want opposite treatment. An
// ask that gates only on a path is a reminder — the reader needs to know a
// standard exists, and once they know it, saying it again is noise, since a
// second firing is the literal same sentence with nothing new in it. It has
// no match, so its key is the bare ask ID, edit ignored, and it still
// suppresses after the first sighting.
//
// An ask that gates on content is an inspection: it is asking about a
// specific string in a specific place, and "is this narrator overreach" is a
// different question about `you nod` in one file than the same three words
// in another. Keying on the match alone would collapse those into one
// question — the collision this package used to have, and the reason `edit`
// is part of the key now, not an opt-in widening. Two edits producing
// identical matched text are still two different occurrences once the edit
// around the quote differs, and the exact same edit recurring is still
// worth a fresh count rather than a permanent silence.
//
// The match is hashed rather than appended, because it can be up to 80 bytes
// of arbitrary text including newlines, and the store is one key per line.
func Key(id, matched, edit string) string {
	if matched == "" {
		return id
	}
	return keyed(id, matched+"\x00"+edit)
}

func keyed(id, material string) string {
	sum := sha256.Sum256([]byte(material))
	return id + ":" + hex.EncodeToString(sum[:])[:8]
}

// Fired reports whether this key has already been shown at least once.
// Callers pass a Key, not a bare ask ID. This is what the reminder bucket
// still suppresses on; a matched ask reads Count instead.
func (s *Store) Fired(id string) bool { return s.seen[id] > 0 }

// Count reports how many times this key has already fired, 0 if never. A
// matched ask's injection uses this to mark a repeat rather than hide it.
func (s *Store) Count(id string) int { return s.seen[id] }

// Record marks occurrences as shown, incrementing each one's count by one —
// including a key that was already present, since "fired again" is the
// whole point for a matched ask now, not a fact to collapse away. Errors are
// dropped: failing to persist means a count resets, a nuisance, not a fault
// worth surfacing mid-edit.
func (s *Store) Record(ids ...string) {
	if s.path == "" {
		return
	}
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		s.seen[id]++
	}
	lines := make([]string, 0, len(s.seen))
	for k, n := range s.seen {
		lines = append(lines, k+":"+strconv.Itoa(n))
	}
	_ = os.WriteFile(s.path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// Touched returns every path written this session, in no particular order.
func (s *Store) Touched() []string {
	out := make([]string, 0, len(s.touched))
	for p := range s.touched {
		out = append(out, p)
	}
	return out
}

// Touch records a path as written. Call it after matching, so that the edit in
// hand does not count as having already satisfied an `untouched:` gate about
// itself.
func (s *Store) Touch(path string) {
	if s.path == "" || s.touched[path] {
		return
	}
	s.touched[path] = true
	f, err := os.OpenFile(s.path+".paths", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(path + "\n")
}

func prune(dir string, age time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-age)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
		_ = os.Remove(filepath.Join(dir, e.Name()) + ".paths")
	}
}
