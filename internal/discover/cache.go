package discover

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/secfile"
)

// askSnapshot is a JSON-safe copy of a *ask.Ask: every []*regexp.Regexp field
// becomes []string via Regexp.String(), which Go guarantees returns exactly
// the source text Compile was given. Recompiling that string is therefore
// lossless, so restoring a snapshot never needs its own copy of parseBlock's
// per-header compilation logic (multiline()'s added:/removed: wrapping
// included) to stay correct — whatever that logic produced is already baked
// into the string being stored.
type askSnapshot struct {
	Source    string   `json:"source"`
	Line      int      `json:"line"`
	Dir       string   `json:"dir"`
	In        string   `json:"in"`
	NotIn     []string `json:"not_in,omitempty"`
	When      []string `json:"when,omitempty"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Has       []string `json:"has,omitempty"`
	Untouched []string `json:"untouched,omitempty"`
	Not       []string `json:"not,omitempty"`
	On        ask.Mode `json:"on"`
	Requires  []string `json:"requires,omitempty"`
	Evokes    []string `json:"evokes,omitempty"`
	Revisit   bool     `json:"revisit,omitempty"`
	Always    bool     `json:"always,omitempty"`
	Block     bool     `json:"block,omitempty"`
	Name      string   `json:"name,omitempty"`
	Cues      []string `json:"cues,omitempty"`
	FiresOn   []string `json:"fires_on,omitempty"`
	SilentOn  []string `json:"silent_on,omitempty"`
	Body      string   `json:"body"`
}

func snapshot(a *ask.Ask) askSnapshot {
	return askSnapshot{
		Source: a.Source, Line: a.Line, Dir: a.Dir, In: a.In, NotIn: a.NotIn,
		When: reStrings(a.When), Added: reStrings(a.Added), Removed: reStrings(a.Removed),
		Has: reStrings(a.Has), Untouched: a.Untouched, Not: reStrings(a.Not), On: a.On,
		Requires: a.Requires, Evokes: a.Evokes, Revisit: a.Revisit, Always: a.Always,
		Block: a.Block, Name: a.Name, Cues: a.Cues, FiresOn: a.FiresOn, SilentOn: a.SilentOn,
		Body: a.Body,
	}
}

// restore rebuilds a *ask.Ask from a snapshot, recompiling every pattern
// string back into a *regexp.Regexp. An error here means a cache entry has
// been hand-edited or corrupted into something Compile rejects — it should
// never happen for a snapshot this package wrote itself, since Regexp.String()
// round-trips, but the caller treats it as an ordinary cache miss rather than
// a crash, the same fail-soft shape a missing or unparseable cache file uses.
func (s askSnapshot) restore() (*ask.Ask, error) {
	when, err := compileAll(s.When)
	if err != nil {
		return nil, err
	}
	added, err := compileAll(s.Added)
	if err != nil {
		return nil, err
	}
	removed, err := compileAll(s.Removed)
	if err != nil {
		return nil, err
	}
	has, err := compileAll(s.Has)
	if err != nil {
		return nil, err
	}
	not, err := compileAll(s.Not)
	if err != nil {
		return nil, err
	}
	return &ask.Ask{
		Source: s.Source, Line: s.Line, Dir: s.Dir, In: s.In, NotIn: s.NotIn,
		When: when, Added: added, Removed: removed, Has: has, Untouched: s.Untouched,
		Not: not, On: s.On, Requires: s.Requires, Evokes: s.Evokes, Revisit: s.Revisit,
		Always: s.Always, Block: s.Block, Name: s.Name, Cues: s.Cues,
		FiresOn: s.FiresOn, SilentOn: s.SilentOn, Body: s.Body,
	}, nil
}

func reStrings(res []*regexp.Regexp) []string {
	if len(res) == 0 {
		return nil
	}
	out := make([]string, len(res))
	for i, re := range res {
		out[i] = re.String()
	}
	return out
}

func compileAll(pats []string) ([]*regexp.Regexp, error) {
	if len(pats) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		out[i] = re
	}
	return out, nil
}

// fileEntry is one source file's cached parse, fingerprinted by the mtime and
// size ParseSource observed when it wrote the entry. Either changing means
// the file has changed since, and the entry is stale — no separate
// invalidation step exists or is needed, since the fingerprint check runs on
// every lookup.
type fileEntry struct {
	ModTime int64         `json:"mtime"`
	Size    int64         `json:"size"`
	Asks    []askSnapshot `json:"asks"`
}

// The cache is sharded one file per source path rather than one shared blob
// keyed by path — measured, not assumed: a single aggregate JSON file meant
// every lookup paid to decode the *entire* cache (every repo this machine has
// ever run onsetter in), not just the one entry being checked. A benchmark
// against a 150-ask CLAUDE.md found decoding a one-entry aggregate file alone
// cost ~5.7ms, more than the ~2.6ms a cold text-scan-and-compile of that same
// file cost outright — the "optimization" was net negative before a second
// repo was ever added to the same cache. Sharding by a hash of the absolute
// path means a lookup only ever costs the JSON size of that one file's own
// asks, and a concurrent write to a different source path can never race with
// this one — no shared read-modify-write to lose an update on.

// cacheDir is $XDG cache dir/onsetter/asks/ — a directory of small per-file
// entries, the same os.UserCacheDir()/onsetter/... convention
// internal/embed/cache.go and internal/session already use for per-machine
// state that should not live in the repo.
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "onsetter", "asks"), nil
}

// entryPath is src's own shard: a hash of its absolute path rather than the
// path itself, since a real path can contain characters a filesystem treats
// specially (or can simply be too long once nested under cacheDir()).
func entryPath(src string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(src))
	return filepath.Join(dir, hex.EncodeToString(h[:])+".json"), nil
}

// Get returns src's cached asks if a shard exists for it and its mtime and
// size still match what the caller observed just now. Neither matching is
// not proof the file is unchanged — an edit that lands on the same size
// within the same nanosecond mtime would collide — but that is the same
// tradeoff every mtime-keyed cache makes (make, ccache) in exchange for never
// having to read a file's content just to decide whether to trust the entry
// that names it. A missing, corrupt, or stale shard is an ordinary miss, the
// same fail-soft shape every other cache in this codebase uses — never an
// error a caller has to handle.
func Get(src string, mtime, size int64) ([]*ask.Ask, bool) {
	p, err := entryPath(src)
	if err != nil {
		return nil, false
	}
	return getAt(p, mtime, size)
}

// getAt loads a shard from an explicit path, sidestepping os.UserCacheDir()
// and entryPath's hashing — the seam a test uses to target a known temp file
// instead of the real per-machine cache.
func getAt(p string, mtime, size int64) ([]*ask.Ask, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var e fileEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, false
	}
	if e.ModTime != mtime || e.Size != size {
		return nil, false
	}
	out := make([]*ask.Ask, len(e.Asks))
	for i, s := range e.Asks {
		a, err := s.restore()
		if err != nil {
			return nil, false
		}
		out[i] = a
	}
	return out, true
}

// Set writes src's parsed asks to its own shard, fingerprinted by mtime and
// size, creating the cache directory if needed.
func Set(src string, mtime, size int64, asks []*ask.Ask) error {
	p, err := entryPath(src)
	if err != nil {
		return err
	}
	return setAt(p, mtime, size, asks)
}

func setAt(p string, mtime, size int64, asks []*ask.Ask) error {
	snaps := make([]askSnapshot, len(asks))
	for i, a := range asks {
		snaps[i] = snapshot(a)
	}
	b, err := json.Marshal(fileEntry{ModTime: mtime, Size: size, Asks: snaps})
	if err != nil {
		return err
	}
	if err := secfile.EnsureDir(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return secfile.WriteFile(p, b, 0o600)
}
