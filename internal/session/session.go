// Package session remembers which asks have already fired this session.
//
// A question you have already answered is noise the second time, and noise is
// how you teach someone to scroll past the block without reading it. Firing
// once per ask per session is the whole retirement mechanism: there is no
// decay model here on purpose, because nothing measurable distinguishes an ask
// that changed an edit from one that was skimmed and ignored.
package session

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store is a per-session set of ask IDs, kept as one file of newline-joined
// IDs. Small enough that read-modify-write beats anything cleverer.
//
// It also keeps the set of paths written this session, in a sibling file, so
// an ask can gate on "you changed this and not that". That set is only what
// onsetter saw go past: a file rewritten by a shell command never reaches a
// PreToolUse hook on Write or Edit, so it counts as untouched.
type Store struct {
	path    string
	seen    map[string]bool
	touched map[string]bool
}

// Open loads the fired-set for a session. A blank or unusable id yields a
// store that never persists, which degrades to firing every time rather than
// to failing.
func Open(id string) *Store {
	s := &Store{seen: map[string]bool{}, touched: map[string]bool{}}
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
			s.seen[line] = true
		}
	}
	return s
}

// Fired reports whether this ask has already been shown.
func (s *Store) Fired(id string) bool { return s.seen[id] }

// Record marks asks as shown. Errors are dropped: failing to persist means a
// ask repeats, which is a nuisance, not a fault worth surfacing mid-edit.
func (s *Store) Record(ids ...string) {
	if s.path == "" {
		return
	}
	changed := false
	for _, id := range ids {
		if !s.seen[id] {
			s.seen[id] = true
			changed = true
		}
	}
	if !changed {
		return
	}
	keys := make([]string, 0, len(s.seen))
	for k := range s.seen {
		keys = append(keys, k)
	}
	_ = os.WriteFile(s.path, []byte(strings.Join(keys, "\n")+"\n"), 0o644)
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
