package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
// printCalibReport is pure computation wrapped in fmt.Print calls, and this
// is what lets the wording and the arithmetic be tested together without a
// real embed call anywhere near it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestCalibReportPosFloorAndNegCeiling(t *testing.T) {
	r := calibReport{
		positive:  []scored{{"a", 0.6}, {"b", 0.5}, {"c", 0.7}},
		negative:  []scored{{"x", 0.3}, {"y", 0.45}, {"z", 0.2}},
		threshold: 0.48,
	}
	if got := r.posFloor(); got.path != "b" || got.score != 0.5 {
		t.Errorf("posFloor = %+v, want the weakest positive (b, 0.5)", got)
	}
	if got := r.negCeiling(); got.path != "y" || got.score != 0.45 {
		t.Errorf("negCeiling = %+v, want the strongest negative (y, 0.45)", got)
	}
}

func TestCalibReportRecallAndFalseFires(t *testing.T) {
	r := calibReport{
		positive:  []scored{{"a", 0.6}, {"b", 0.4}},  // one clears 0.5, one doesn't
		negative:  []scored{{"x", 0.55}, {"y", 0.2}}, // one false-fires, one doesn't
		threshold: 0.5,
	}
	if fired, total := r.recall(); fired != 1 || total != 2 {
		t.Errorf("recall = %d/%d, want 1/2", fired, total)
	}
	if fired, total := r.falseFires(); fired != 1 || total != 2 {
		t.Errorf("falseFires = %d/%d, want 1/2", fired, total)
	}
}

func TestPrintCalibReportNamesAGapWhenSeparated(t *testing.T) {
	out := captureStdout(t, func() {
		printCalibReport(calibReport{
			positive:  []scored{{"pos.md", 0.6}},
			negative:  []scored{{"neg.md", 0.3}},
			threshold: 0.48,
		})
	})
	if !strings.Contains(out, "gap +0.300") {
		t.Errorf("did not report the gap between a clean floor and ceiling:\n%s", out)
	}
	if strings.Contains(out, "overlap") {
		t.Errorf("reported overlap when the examples are cleanly separated:\n%s", out)
	}
}

func TestPrintCalibReportNamesOverlapWhenNotSeparable(t *testing.T) {
	out := captureStdout(t, func() {
		printCalibReport(calibReport{
			positive:  []scored{{"weak_pos.md", 0.4}},
			negative:  []scored{{"strong_neg.md", 0.45}},
			threshold: 0.48,
		})
	})
	if !strings.Contains(out, "overlap") {
		t.Errorf("did not flag that these examples cannot be cleanly separated:\n%s", out)
	}
	if !strings.Contains(out, "not just a number") {
		t.Errorf("overlap case did not point at the actual fix:\n%s", out)
	}
}

func TestPrintCalibReportMarksMissesAndFalsePositives(t *testing.T) {
	out := captureStdout(t, func() {
		printCalibReport(calibReport{
			positive:  []scored{{"missed.md", 0.3}},
			negative:  []scored{{"leaked.md", 0.9}},
			threshold: 0.48,
		})
	})
	if !strings.Contains(out, "missed.md") || !strings.Contains(out, "below threshold") {
		t.Errorf("did not mark the positive example that missed:\n%s", out)
	}
	if !strings.Contains(out, "leaked.md") || !strings.Contains(out, "false positive") {
		t.Errorf("did not mark the negative example that false-fired:\n%s", out)
	}
}

func TestResolveCalibTargetRequiresLineWhenAmbiguous(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	write(t, claudeMD,
		"```ask\nevokes: first thing\n\nAsk one.\n```\n\n"+
			"```ask\nevokes: second thing\n\nAsk two.\n```\n")

	if _, err := resolveCalibTarget(claudeMD); err == nil {
		t.Fatal("want an error: two evokes: asks in the file, no line given")
	} else if !strings.Contains(err.Error(), "2 evokes: asks") {
		t.Errorf("error does not say how many were found: %v", err)
	}
}

func TestResolveCalibTargetPicksTheLineGiven(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	write(t, claudeMD,
		"```ask\nevokes: first thing\n\nAsk one.\n```\n\n"+
			"```ask\nevokes: second thing\n\nAsk two.\n```\n")

	a, err := resolveCalibTarget(claudeMD + ":7")
	if err != nil {
		t.Fatalf("resolveCalibTarget: %v", err)
	}
	if len(a.Evokes) != 1 || a.Evokes[0] != "second thing" {
		t.Errorf("resolved to the wrong ask: %+v", a.Evokes)
	}
}

func TestResolveCalibTargetSkipsDisambiguationWithOneAsk(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	write(t, claudeMD, "```ask\nevokes: the only phrase\n\nAsk.\n```\n")

	a, err := resolveCalibTarget(claudeMD)
	if err != nil {
		t.Fatalf("resolveCalibTarget: %v", err)
	}
	if len(a.Evokes) != 1 || a.Evokes[0] != "the only phrase" {
		t.Errorf("resolved to the wrong ask: %+v", a.Evokes)
	}
}

func TestResolveCalibTargetErrorsWithNoEvokesAsk(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	write(t, claudeMD, "```ask\nwhen: TODO\n\nAsk.\n```\n")

	if _, err := resolveCalibTarget(claudeMD); err == nil {
		t.Fatal("want an error: no evokes: ask in this file")
	}
}

// calib's precondition check — every phrase must be warmed first — needs no
// network at all: it fails before any embed call, against an empty cache.
func TestCalibRefusesUnwarmedPhrasesWithoutTouchingOllama(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nevokes: never warmed\n\nAsk.\n```\n")
	write(t, filepath.Join(repo, "pos.md"), "content")
	write(t, filepath.Join(repo, "neg.md"), "content")

	out, err := runIn(t, bin, repo, t.TempDir(), "calib", "CLAUDE.md", "pos.md", "neg.md")
	if err == nil {
		t.Fatalf("want an error: no warm has run\n%s", out)
	}
	if !strings.Contains(out, "not warmed") {
		t.Errorf("error does not say the phrase needs warming:\n%s", out)
	}
}

// The whole tool, real embeddings end to end: warm, then calib against real
// labeled files, and check the report actually separates them the way the
// standalone paraphrase test in warm_test.go already proved this exact pair
// of examples should.
func TestCalibEndToEnd(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	skipUnlessOllama(t, bin, cache)

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "fires"))
	mkdir(t, filepath.Join(repo, "not"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nevokes: committing without asking the user first\nevokes: pushing straight to the main branch\n\n"+
			"Never commit or push without explicit confirmation.\n```\n")
	write(t, filepath.Join(repo, "fires", "paraphrase.md"),
		"Went ahead and shipped the deploy straight to prod, figured it was a small\n"+
			"enough change that nobody needed to sign off on it first.\n")
	write(t, filepath.Join(repo, "not", "unrelated.md"),
		"The recipe calls for two cups of flour and a pinch of salt, mixed slowly.\n")

	if out, err := runIn(t, bin, repo, cache, "warm", "."); err != nil {
		t.Fatalf("warm: %v\n%s", out, err)
	}
	out, err := runIn(t, bin, repo, cache, "calib", "CLAUDE.md", "fires/**", "not/**")
	if err != nil {
		t.Fatalf("calib: %v\n%s", err, out)
	}
	if !strings.Contains(out, "1 positive example(s), 1 negative example(s)") {
		t.Errorf("did not count the examples correctly:\n%s", out)
	}
	if !strings.Contains(out, "1/1 positive(s) fire, 0/1 negative(s) false-fire") {
		t.Errorf("report did not confirm the known-good pair separates:\n%s", out)
	}
	if !strings.Contains(out, "POS floor") || !strings.Contains(out, "NEG ceiling") {
		t.Errorf("report is missing the floor/ceiling summary:\n%s", out)
	}
}
