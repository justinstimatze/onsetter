package discover

import (
	"os"
	"sync"

	"github.com/justinstimatze/onsetter/ask"
)

// Warm is an in-memory layer over Roots/ParseSource, for a caller that lives
// across many hook calls — onsetter serve, not any one-shot CLI subcommand.
// A hit returns the already-compiled *ask.Ask slice directly: no JSON decode,
// no regexp.Compile, the two costs the on-disk shard cache (cache.go) still
// pays on every hit. A miss falls through to ParseSource, so this process's
// first-ever sight of a given file — or its first call after a prior
// session's cache already warmed the shard — still benefits from that disk
// layer instead of a cold text-scan-and-compile.
//
// The zero value is not usable; construct with NewWarm.
type Warm struct {
	mu    sync.Mutex
	files map[string]warmEntry
}

type warmEntry struct {
	modTime int64
	size    int64
	asks    []*ask.Ask
}

// NewWarm returns an empty Warm cache, ready to use.
func NewWarm() *Warm {
	return &Warm{files: map[string]warmEntry{}}
}

// Asks mirrors Asks's own signature — a caller holding a *Warm can pass
// w.Asks directly wherever a plain discover.Asks would otherwise go.
func (w *Warm) Asks(path string) ([]*ask.Ask, []error) {
	var out []*ask.Ask
	var errs []error
	for _, src := range Roots(path) {
		asks, err := w.parseSource(src)
		if err != nil {
			errs = append(errs, err)
		}
		out = append(out, asks...)
	}
	return out, errs
}

// parseSource is ParseSource with an in-memory hit before the disk cache
// ever gets a chance to run. Two concurrent misses for the same
// never-before-seen path may each call ParseSource once, redundantly — the
// mutex guards map access, not the parse itself, an accepted, self-healing
// race matching how internal/session.Store already tolerates concurrent
// writers rather than serializing them.
func (w *Warm) parseSource(src string) ([]*ask.Ask, error) {
	info, statErr := os.Stat(src)
	if statErr != nil {
		return ParseSource(src) // let the usual path surface the real error
	}
	mtime, size := info.ModTime().UnixNano(), info.Size()

	w.mu.Lock()
	if e, ok := w.files[src]; ok && e.modTime == mtime && e.size == size {
		asks := e.asks
		w.mu.Unlock()
		return asks, nil
	}
	w.mu.Unlock()

	asks, err := ParseSource(src)
	if err == nil {
		w.mu.Lock()
		w.files[src] = warmEntry{modTime: mtime, size: size, asks: asks}
		w.mu.Unlock()
	}
	return asks, err
}
