package main

import (
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
