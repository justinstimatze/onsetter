package embed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justinstimatze/onsetter/internal/secfile"
)

// cacheEntry pairs a vector with the model that produced it. Without the
// model tag, switching DefaultModel would silently compare vectors from two
// different embedding spaces, which cosine similarity has no way to detect —
// the number it returns would just be wrong, not absent.
type cacheEntry struct {
	Model  string    `json:"model"`
	Vector []float32 `json:"vector"`
}

// Cache is a warm, on-disk phrase -> vector store, global across every
// project rather than per-repo: a phrase's meaning does not depend on which
// CLAUDE.md wrote it, so two asks in two different repos sharing an
// evokes: phrase pay for the embed call once between them, not once each.
//
// Match time only ever reads it. Nothing in this file calls Ollama; that is
// `onsetter warm`'s job, run offline before an evokes: ask is trusted.
type Cache struct {
	path    string
	entries map[string]cacheEntry
}

// cachePath is $XDG cache dir/onsetter/embeddings.json, the same
// os.UserCacheDir()/onsetter/... convention internal/session already uses
// for per-machine state that should not live in the repo.
func cachePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "onsetter", "embeddings.json"), nil
}

// OpenCache loads the warm cache. A missing or unreadable file degrades to an
// empty in-memory cache rather than an error — the same fail-soft shape
// every other piece of this feature uses, since an empty cache just means
// every evokes: phrase currently rejects until `onsetter warm` runs.
func OpenCache() *Cache {
	p, err := cachePath()
	if err != nil {
		return &Cache{entries: map[string]cacheEntry{}}
	}
	return openCacheAt(p)
}

// openCacheAt loads the cache from an explicit path, sidestepping
// os.UserCacheDir() — the seam a test uses to round-trip through a temp file
// instead of the real per-machine cache.
func openCacheAt(p string) *Cache {
	c := &Cache{path: p, entries: map[string]cacheEntry{}}
	b, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c.entries) // a corrupt cache degrades to empty, not a crash
	return c
}

// Vector returns phrase's cached vector, if one exists under model. A phrase
// added since the cache was last built, or cached under a different model,
// reports ok=false — the caller's Predicate treats that as "not evoked",
// never as an error.
func (c *Cache) Vector(phrase, model string) ([]float32, bool) {
	e, ok := c.entries[phrase]
	if !ok || e.Model != model {
		return nil, false
	}
	return e.Vector, true
}

// Set stores phrase's vector in memory. Callers that mutate a cache are
// expected to Save it; Set alone never touches disk.
func (c *Cache) Set(phrase, model string, vector []float32) {
	c.entries[phrase] = cacheEntry{Model: model, Vector: vector}
}

// Save writes the cache to the path it was opened from, creating its
// directory if needed. A Cache from OpenCache always has one; cachePath()
// only fails when os.UserCacheDir() does, and OpenCache already degraded to
// a pathless Cache in that case, so Save on it reports that same error again
// rather than silently writing nowhere.
func (c *Cache) Save() error {
	if c.path == "" {
		return fmt.Errorf("no cache path available (os.UserCacheDir() failed)")
	}
	if err := secfile.EnsureDir(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(c.entries)
	if err != nil {
		return err
	}
	return secfile.WriteFile(c.path, b, 0o600)
}

// Warm reports whether phrase already has a usable cached vector — the exact
// tag Predicate reads, so a caller deciding whether to re-embed never needs
// to know what that tag actually is.
func (c *Cache) Warm(phrase string) bool {
	_, ok := c.Vector(phrase, cacheModelTag)
	return ok
}

// WarmQuery embeds phrase as a query (see EmbedQuery) and stores it under
// this package's cache tag. `onsetter warm` is the only caller — it never
// needs to know the tagging convention, only that a phrase it warms here is
// a phrase BuildPredicate can later score.
func (c *Cache) WarmQuery(phrase string, budget time.Duration) error {
	v, err := EmbedQuery(phrase, budget)
	if err != nil {
		return err
	}
	c.Set(phrase, cacheModelTag, v)
	return nil
}

// Score returns phrase's raw cosine similarity against editVector — the
// number Predicate collapses into a boolean. `onsetter calib` needs the
// number: a threshold decision is exactly what it exists to inform, so it
// cannot be the thing doing the collapsing. ok=false means phrase has no
// cached vector under model, the same "not warmed" signal Vector gives.
func (c *Cache) Score(phrase string, editVector []float32, model string) (score float32, ok bool) {
	v, ok := c.Vector(phrase, model)
	if !ok {
		return 0, false
	}
	return Cosine(v, editVector), true
}

// ScoreQuery scores phrase against editVector, looked up under this
// package's cache tag — the same one Warm/WarmQuery/Predicate use, so
// `onsetter calib` gets a raw number to report without ever needing to know
// what that tag actually is.
func (c *Cache) ScoreQuery(phrase string, editVector []float32) (score float32, ok bool) {
	return c.Score(phrase, editVector, cacheModelTag)
}

// Predicate builds an ask.Edit.Evokes-shaped function from an edit's own
// vector: each phrase's cached vector is looked up (never computed here) and
// scored by cosine similarity against threshold. A cache miss scores false,
// the same fail-soft shape a missing requires: binary uses.
func (c *Cache) Predicate(editVector []float32, model string, threshold float32) func(phrase string) bool {
	return func(phrase string) bool {
		score, ok := c.Score(phrase, editVector, model)
		return ok && score >= threshold
	}
}
