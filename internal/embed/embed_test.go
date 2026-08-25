package embed

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCosineIdenticalVectorsScoreOne(t *testing.T) {
	v := []float32{1, 2, 3}
	if got := Cosine(v, v); got < 0.999 || got > 1.001 {
		t.Errorf("Cosine(v, v) = %v, want ~1", got)
	}
}

func TestCosineOrthogonalVectorsScoreZero(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	if got := Cosine(a, b); got != 0 {
		t.Errorf("Cosine(orthogonal) = %v, want 0", got)
	}
}

func TestCosineOppositeVectorsScoreNegativeOne(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{-1, 0}
	if got := Cosine(a, b); got < -1.001 || got > -0.999 {
		t.Errorf("Cosine(opposite) = %v, want ~-1", got)
	}
}

// A zero vector has no direction, so its similarity to anything is
// undefined — Cosine reports 0 rather than dividing by zero into NaN, which
// would make Predicate silently treat a corrupt cache entry as a real score
// instead of the missing data it is.
func TestCosineZeroVectorScoresZeroNotNaN(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	if got := Cosine(a, b); got != 0 {
		t.Errorf("Cosine(zero, b) = %v, want 0", got)
	}
}

func TestCosineMismatchedLengthsScoreZero(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	if got := Cosine(a, b); got != 0 {
		t.Errorf("Cosine(mismatched lengths) = %v, want 0", got)
	}
}

func TestCacheRoundTripsThroughDisk(t *testing.T) {
	p := filepath.Join(t.TempDir(), "embeddings.json")
	c := openCacheAt(p)
	c.Set("commit without asking", "test-model", []float32{1, 2, 3})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded := openCacheAt(p)
	v, ok := reloaded.Vector("commit without asking", "test-model")
	if !ok {
		t.Fatal("phrase not found after reload")
	}
	if len(v) != 3 || v[0] != 1 || v[1] != 2 || v[2] != 3 {
		t.Errorf("vector = %v, want [1 2 3]", v)
	}
}

// A phrase cached under one model must not answer a lookup under another —
// switching DefaultModel would otherwise silently compare two incompatible
// vector spaces and Predicate would return a real-looking, meaningless score.
func TestCacheMissesOnModelMismatch(t *testing.T) {
	c := openCacheAt(filepath.Join(t.TempDir(), "embeddings.json"))
	c.Set("some phrase", "model-a", []float32{1, 0})
	if _, ok := c.Vector("some phrase", "model-b"); ok {
		t.Error("Vector found an entry cached under a different model")
	}
}

func TestOpenCacheOnMissingFileIsEmptyNotAnError(t *testing.T) {
	c := openCacheAt(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if _, ok := c.Vector("anything", DefaultModel); ok {
		t.Error("a fresh cache over a missing file should have no entries")
	}
}

func TestPredicateMissesFailSoft(t *testing.T) {
	c := openCacheAt(filepath.Join(t.TempDir(), "embeddings.json"))
	pred := c.Predicate([]float32{1, 0}, DefaultModel, 0.5)
	if pred("a phrase never warmed") {
		t.Error("predicate matched a phrase with no cached vector")
	}
}

// TestEmbedAgainstRealOllama is the one test in this package that touches the
// network. Every other test here proves the plumbing with a fake vector;
// this proves the model on the other end actually separates related text
// from unrelated text, which no amount of injected fixtures can stand in
// for. Skips rather than fails when Ollama or the model is not there — CI
// and a teammate's machine should not fail this suite over a dependency the
// feature is already built to degrade without.
func TestEmbedAgainstRealOllama(t *testing.T) {
	near, err := Embed("commit without asking me first", DefaultModel, 5*time.Second)
	if err != nil {
		t.Skipf("Ollama with %s not reachable, skipping: %v", DefaultModel, err)
	}
	related, err := Embed("push to git without confirmation", DefaultModel, 5*time.Second)
	if err != nil {
		t.Fatalf("second Embed call failed after the first succeeded: %v", err)
	}
	unrelated, err := Embed("the weather in Denver this weekend", DefaultModel, 5*time.Second)
	if err != nil {
		t.Fatalf("third Embed call failed after the first two succeeded: %v", err)
	}

	relatedScore := Cosine(near, related)
	unrelatedScore := Cosine(near, unrelated)
	if relatedScore <= unrelatedScore {
		t.Errorf("related score %v did not beat unrelated score %v — the model is not separating these",
			relatedScore, unrelatedScore)
	}
}

func TestPredicateScoresAboveAndBelowThreshold(t *testing.T) {
	c := openCacheAt(filepath.Join(t.TempDir(), "embeddings.json"))
	c.Set("close", DefaultModel, []float32{1, 0})
	c.Set("far", DefaultModel, []float32{0, 1})
	pred := c.Predicate([]float32{1, 0}, DefaultModel, 0.5)
	if !pred("close") {
		t.Error("identical-direction phrase scored below threshold")
	}
	if pred("far") {
		t.Error("orthogonal phrase scored above threshold")
	}
}
