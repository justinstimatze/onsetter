// Package embed is the fuzzy-matching backend for evokes:, kept entirely
// outside the ask package on purpose — Match stays pure and testable with a
// plain function value, and every network call, every cache file, and the
// threshold decision live here instead.
//
// The shape is lexicon's own embedgate, reused rather than reinvented: warm
// only (a phrase's vector is built offline, by `onsetter warm`, never inline),
// one hard-budgeted local Ollama call at match time, fail soft on anything
// that goes wrong. A same-turn LLM judge pass was ruled out for onsetter
// specifically — this package's score is the whole decision, not a filter
// ahead of one — because every onsetter ask is already built to be cheap to
// dismiss, the same way the regex gates already accept some false positives
// in exchange for asking at all.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// DefaultModel is the embedding model onsetter asks Ollama for.
const DefaultModel = "nomic-embed-text"

// cacheModelTag is what the cache actually keys entries on — DefaultModel
// plus a convention version, not DefaultModel alone. The model name is not
// the whole story: this package changed how it prompts nomic-embed-text
// (adding the search_document:/search_query: task prefix, see EmbedDocument/
// EmbedQuery) without changing which model it calls, and a cache built
// before that change would have silently compared prefixed query vectors
// against unprefixed document vectors under the same model name — wrong
// numbers, no error. Bump the suffix whenever the prompting convention
// changes; that invalidates the old cache the same way a model swap does.
const cacheModelTag = DefaultModel + "+prefixed-v1"

// DefaultBudget is the per-edit ceiling for the one live embed call a hook
// invocation makes. 200ms was a placeholder, calibrated against a ~30ms
// warm call on one quiet machine, and does not hold under real host
// contention. Two real measurements: a quiet, single-tenant CPU-only host
// held warm calls at 80-90ms and a forced-cold call at 0.35s; a
// contended host (a dozen processes competing for a fixed memory budget)
// pushed warm calls to 120-700ms and a forced-cold call to 10.3s. 1500ms
// clears the contended host's warm-case max with real margin and the
// quiet host's numbers by 5-15x, while staying far short of either
// host's cold-start cost — a cold model still degrades to "does not
// fire" exactly as designed, the same case nomic-embed-text's own
// ~5-minute Ollama keep-alive already makes real for anyone editing in
// bursts. Still not a number with a `replay`-style corpus behind it —
// two machines, two load conditions — and the tradeoff is real: a
// worst-case warm miss now costs up to 1.5s of real edit latency instead
// of capping at 200ms, the price of the feature actually firing rather
// than almost never doing so.
const DefaultBudget = 1500 * time.Millisecond

// DefaultThreshold is the cosine-similarity cutoff a score must clear to
// count as evoked. Measured, not borrowed — lexicon's own POS/NEG overlap
// band (0.581-0.608) is a number from a different embedding domain and does
// not transfer to nomic-embed-text's scale. A same-machine measurement using
// EmbedDocument/EmbedQuery: a true paraphrase sharing no vocabulary with its
// phrase ("shipped the deploy straight to prod, nobody needed to sign off"
// vs. "committing without asking the user first") scored 0.577, against an
// unrelated sentence's 0.372 — a real gap. The same paraphrase unprefixed
// scored 0.486, indistinguishable from noise, which is what caught the
// missing task prefix in the first place. Whole code files also dilute the
// signal toward that same noise floor regardless of prefixing: evokes: has
// real signal on prose-shaped content and close to none on syntax-heavy
// code. 0.48 sits just under the one measured positive, with margin above
// the one measured negative. Still a first draft — one machine, one model,
// a handful of data points — and needs the same replay-based tuning every
// ask gets before it should be trusted at scale.
const DefaultThreshold float32 = 0.48

// endpoint is Ollama's local embeddings API. Never anything but localhost —
// evokes: has no business leaving the machine it runs on.
const endpoint = "http://localhost:11434/api/embeddings"

// EmbedDocument and EmbedQuery are the two entry points anything comparing an
// edit against an evokes: phrase should use — never bare Embed. nomic-embed-text
// is trained on asymmetric retrieval with a task prefix on each side, and on
// evokes:'s own short-phrase case, skipping it was not a stylistic omission:
// measured directly, a true paraphrase with no shared vocabulary scored
// 0.486 unprefixed, indistinguishable from noise, and 0.577 prefixed, a real
// gap above an unrelated case's 0.372. The edit's content is the thing being
// searched (the document); an evokes: phrase is what searches it (the
// query) — that asymmetry is why this is two functions and not one with a
// bool.
//
// This does NOT generalize the way it looks like it should. lexicon tested
// the same prefix against its own held-out calibration corpus (32 positive,
// 7 negative, the set that sets its live threshold) and got the opposite
// result: prefixing made separation AND recall worse, not better. Their read,
// and it holds up: nomic's asymmetric prefix is trained on literal
// document/query retrieval, which is what a short evokes: phrase is close
// to — but lexicon's atoms are deliberately abstracted away from a query's
// surface wording (an atom about the decorator pattern is supposed to share
// little vocabulary with "add logging around an HTTP handler"), a different
// regime the prefix does not transfer to. The finding below is real for
// evokes:'s own case, measured on one machine against a handful of
// hand-tested pairs — not a general claim about the model, and worth
// re-checking as the evokes: corpus this runs against actually grows past
// that handful.
func EmbedDocument(text string, budget time.Duration) ([]float32, error) {
	return Embed("search_document: "+text, DefaultModel, budget)
}

func EmbedQuery(text string, budget time.Duration) ([]float32, error) {
	return Embed("search_query: "+text, DefaultModel, budget)
}

// Embed asks Ollama for text's embedding under model, hard-budgeted to
// budget. Any failure — Ollama not running, the model not pulled, the
// budget exceeded — is returned as an error; every caller in this project
// treats that as "skip the fuzzy check", never as a reason to fail the edit.
//
// This is the raw primitive: no task prefix is added. Every caller comparing
// an edit against an evokes: phrase wants EmbedDocument/EmbedQuery instead —
// see those for why. Exported for direct use only by tests that want to
// measure the model's raw behavior.
func Embed(text, model string, budget time.Duration) ([]float32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	body, err := json.Marshal(struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}{Model: model, Prompt: text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama embeddings: %s: %s", resp.Status, b)
	}

	var out struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embeddings: empty vector for model %q", model)
	}
	return out.Embedding, nil
}

// Cosine returns the cosine similarity of a and b, in [-1, 1]. 0 if either is
// the zero vector, which a real embedding never is, but a caller comparing
// against a corrupt or truncated cache entry should not divide by zero.
func Cosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// BuildPredicate does the one live embed call a caller needs for content, and
// wraps it with the warm cache into an ask.Edit.Evokes-shaped function. Errors
// — Ollama unreachable, budget exceeded, anything — collapse to a nil
// function: Match's existing fail-soft handling of a nil Evokes already does
// the right thing, so callers never branch on this failing.
func BuildPredicate(content string, budget time.Duration) func(phrase string) bool {
	v, err := EmbedDocument(content, budget)
	if err != nil {
		return nil
	}
	c := OpenCache()
	return c.Predicate(v, cacheModelTag, DefaultThreshold)
}
