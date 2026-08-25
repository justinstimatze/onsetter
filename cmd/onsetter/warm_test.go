package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runIn runs bin with args from dir, isolated to its own cache dir — the
// embed cache and the session store both resolve through os.UserCacheDir(),
// which honors $XDG_CACHE_HOME, so this is what keeps these tests from
// reading or writing the real machine-wide evokes: cache.
func runIn(t *testing.T, bin, dir, cache string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// skipUnlessOllama runs `onsetter warm` once against a throwaway phrase and
// skips the calling test if it fails — every test in this file needs a real
// embed call, and a machine without Ollama (or without the model pulled)
// should skip these rather than fail the suite over an optional dependency
// the feature is already built to degrade without.
func skipUnlessOllama(t *testing.T, bin, cache string) {
	t.Helper()
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))
	write(t, filepath.Join(dir, "CLAUDE.md"), "```ask\nevokes: a throwaway phrase\n\nAsk.\n```\n")
	out, err := runIn(t, bin, dir, cache, "warm", ".")
	if err != nil || !strings.Contains(out, "1 built") {
		t.Skipf("Ollama with %s not reachable, skipping: %v\n%s", "the embed model", err, out)
	}
}

func TestWarmReportsNoPhrasesWithoutTouchingOllama(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: TODO\n\nAsk.\n```\n")

	out, err := runIn(t, bin, repo, t.TempDir(), "warm", ".")
	if err != nil {
		t.Fatalf("warm: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no evokes: phrases found") {
		t.Errorf("output does not say no phrases were found:\n%s", out)
	}
}

func TestWarmBuildsThenReusesTheCache(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	skipUnlessOllama(t, bin, cache)

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nevokes: committing without asking\nevokes: pushing to prod\n\nAsk.\n```\n")

	first, err := runIn(t, bin, repo, cache, "warm", ".")
	if err != nil {
		t.Fatalf("first warm: %v\n%s", err, first)
	}
	if !strings.Contains(first, "2 built, 0 already warm") {
		t.Errorf("first run did not build both phrases:\n%s", first)
	}

	second, err := runIn(t, bin, repo, cache, "warm", ".")
	if err != nil {
		t.Fatalf("second warm: %v\n%s", err, second)
	}
	if !strings.Contains(second, "0 built, 2 already warm") {
		t.Errorf("second run re-embedded instead of reusing the cache:\n%s", second)
	}
}

// The whole point of the feature, proven end to end: a true paraphrase with
// no shared vocabulary fires, and unrelated prose does not. Unit tests in
// internal/embed prove the plumbing with injected vectors; this is the one
// place the real model, the real cache, and Match's evokes: gate all run
// together.
func TestEvokesFiresOnAParaphraseNotOnUnrelatedProse(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	skipUnlessOllama(t, bin, cache)

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nevokes: committing without asking the user first\nevokes: pushing straight to the main branch\n\n"+
			"Never commit or push without explicit confirmation.\n```\n")

	if out, err := runIn(t, bin, repo, cache, "warm", "."); err != nil {
		t.Fatalf("warm: %v\n%s", err, out)
	}

	write(t, filepath.Join(repo, "NOTES.md"),
		"Went ahead and shipped the deploy straight to prod, figured it was a small\n"+
			"enough change that nobody needed to sign off on it first.\n")
	fires, err := runIn(t, bin, repo, cache, "list", "NOTES.md")
	if err != nil {
		t.Fatalf("list NOTES.md: %v\n%s", err, fires)
	}
	if !strings.Contains(fires, "would fire") {
		t.Errorf("paraphrase with no shared vocabulary did not fire:\n%s", fires)
	}

	write(t, filepath.Join(repo, "OTHER.md"),
		"The recipe calls for two cups of flour and a pinch of salt, mixed slowly.\n")
	quiet, err := runIn(t, bin, repo, cache, "list", "OTHER.md")
	if err != nil {
		t.Fatalf("list OTHER.md: %v\n%s", err, quiet)
	}
	if !strings.Contains(quiet, "turned away at evokes") {
		t.Errorf("unrelated prose fired:\n%s", quiet)
	}
}

// cmdReplay builds its own predicate independently of cmdList — a separate
// code path this test exercises directly, rather than trusting that list's
// coverage above implies replay's funnel line works too.
func TestReplayFunnelsEvokes(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	skipUnlessOllama(t, bin, cache)

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\nevokes: committing without asking the user first\n\nAsk.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.md"),
		"The recipe calls for two cups of flour and a pinch of salt, mixed slowly.\n")

	if out, err := runIn(t, bin, repo, cache, "warm", "."); err != nil {
		t.Fatalf("warm: %v\n%s", err, out)
	}
	out, err := runIn(t, bin, repo, cache, "replay", "corpus/a.md")
	if err != nil {
		t.Fatalf("replay: %v\n%s", err, out)
	}
	if !strings.Contains(out, "turned away at  evokes: committing without asking the user first ×1") {
		t.Errorf("funnel does not name evokes: as what turned the file away:\n%s", out)
	}
}
