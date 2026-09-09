package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// replayIn runs `onsetter replay` from dir and returns its output.
func replayIn(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"replay"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+goCoverDir(t))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("replay: %v\n%s", err, out)
	}
	return string(out)
}

// Replay builds a synthetic edit from a file on disk, so there is no old text
// and no diff. The rate it prints for an `added:` or `removed:` ask is about
// that ask's other gates, and saying nothing is how a reader concludes the
// number means what the other rows mean. A real `added: "aliases"` printed
// 100.0% while gating on nothing that replay can see.
func TestReplaySaysWhenItCannotMeasureADiffGate(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\nadded: aliases\n\nAdding an alias.\n```\n\n"+
			"```ask\nin: corpus/**\nremoved: if err != nil\n\nDeleting a check.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.json"), `{"aliases": ["take"]}`)

	got := replayIn(t, bin, repo, "corpus/a.json")
	if !strings.Contains(got, "does not measure added:") {
		t.Errorf("no warning for the added: ask:\n%s", got)
	}
	if !strings.Contains(got, "does not measure removed:") {
		t.Errorf("no warning for the removed: ask:\n%s", got)
	}
	if !strings.Contains(got, "onsetter hook") {
		t.Errorf("warning does not name the honest check:\n%s", got)
	}
}

// The warning has to be rare enough to mean something, so an ask replay can
// actually measure must not carry it.
func TestReplayIsQuietForAsksItCanMeasure(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\nwhen: aliases\n\nAn alias.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.json"), `{"aliases": ["take"]}`)

	if got := replayIn(t, bin, repo, "corpus/a.json"); strings.Contains(got, "does not measure") {
		t.Errorf("warned about a when: ask, which replay measures fine:\n%s", got)
	}
}

// requires: is checked before every other gate, so an ask that never fires
// because its tool is missing should have the funnel say so by name — not
// "in: ×1", which would read as a path bug that does not exist.
func TestReplayFunnelsRequires(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\nrequires: definitely-not-a-real-binary-onsetter-test\nwhen: aliases\n\nAn alias.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.json"), `{"aliases": ["take"]}`)

	got := replayIn(t, bin, repo, "corpus/a.json")
	if !strings.Contains(got, "requires: definitely-not-a-real-binary-onsetter-test ×1") {
		t.Errorf("funnel does not name requires: as what turned the file away:\n%s", got)
	}
}

// A cue-only ask (not-in: ** — never gate-matches by design) still has to
// show a real rate, not 0%, or replay misreports exactly the ask this
// feature exists to measure honestly.
func TestReplayCountsCuedFires(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nin: corpus/**\nwhen: aliases\ncues: check-token-scope\n\nCiting question.\n```\n\n"+
		"```ask\nin: corpus/**\nname: check-token-scope\nnot-in: **\n\nCued question.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.json"), `{"aliases": ["take"]}`)

	got := replayIn(t, bin, repo, "corpus/a.json")
	if strings.Count(got, "100.0%") != 2 {
		t.Errorf("both the citer and the cued ask should read 100%%, not 0%%:\n%s", got)
	}
	if !strings.Contains(got, "were cued, not gate-matched") {
		t.Errorf("the cued fire should be called out as cued, not gate-matched:\n%s", got)
	}
	if !strings.Contains(got, "(cued by") {
		t.Errorf("the sample line should name the citer:\n%s", got)
	}
}

// An on: read ask needs the same Read-shaped edit cmdList's fix gives it —
// left write-shaped, replay would report 0% turned away at on: read for
// every file, which is the ask always being dead on the ground it's
// actually meant to cover.
func TestReplayReportsOnReadAsksCorrectly(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "corpus"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\non: read\n\nNever read the corpus directly.\n```\n")
	write(t, filepath.Join(repo, "corpus", "a.json"), `{"aliases": ["take"]}`)

	got := replayIn(t, bin, repo, "corpus/a.json")
	if !strings.Contains(got, "100.0%") {
		t.Errorf("on: read should read 100%%, not 0%% turned away at on: read:\n%s", got)
	}
	if strings.Contains(got, "turned away at  on: read") {
		t.Errorf("on: read should not report itself as its own rejection:\n%s", got)
	}
}
