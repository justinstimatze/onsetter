package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// listIn runs `onsetter list` from dir and returns its output.
func listIn(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"list"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	return string(out)
}

// list's whole job is showing every header a block set and marking whichever
// one rejected it. requires: was added to Match and to replay's funnel
// before it was added to this printer — a header invisible here defeats the
// command's own purpose, since "requires:" with no binary name forces a
// reader back into the raw CLAUDE.md to find out what is missing.
func TestListPrintsRequires(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nrequires: definitely-not-a-real-binary-onsetter-test\nwhen: TODO\n\nAsk.\n```\n")
	write(t, filepath.Join(repo, "a.md"), "TODO fix this")

	got := listIn(t, bin, repo, "a.md")
	if !strings.Contains(got, "requires:  definitely-not-a-real-binary-onsetter-test") {
		t.Errorf("list does not print the requires: header at all:\n%s", got)
	}
	if !strings.Contains(got, "turned away at requires:") {
		t.Errorf("list does not name requires: as the rejecting gate:\n%s", got)
	}
}

// A cue-only ask's own gate always rejects — that's the idiom — so list has
// to know it was cued in before it renders that ask's own turn, or it
// misreports "would not fire" on an ask the hook would actually inject.
func TestListReportsACuedAskAsWouldFire(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: TODO\ncues: check-token-scope\n\nCiting question.\n```\n\n"+
		"```ask\nname: check-token-scope\nnot-in: **\n\nCued question.\n```\n")
	write(t, filepath.Join(repo, "a.md"), "TODO fix this")

	got := listIn(t, bin, repo, "a.md")
	if !strings.Contains(got, "name:      check-token-scope") {
		t.Errorf("list does not print the name: header:\n%s", got)
	}
	if !strings.Contains(got, "cues:      check-token-scope") {
		t.Errorf("list does not print the cues: header:\n%s", got)
	}
	if !strings.Contains(got, "would fire, cued by") {
		t.Errorf("list should report the cue-only ask as would fire, cued by its citer:\n%s", got)
	}
	if strings.Contains(got, "would not fire") {
		t.Errorf("the cued ask should not read as would not fire:\n%s", got)
	}
}
