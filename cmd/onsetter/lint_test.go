package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lintIn runs `onsetter lint` from dir and returns its output and exit error.
func lintIn(t *testing.T, bin, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"lint"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestLintFailsOnDuplicateName(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nname: dup\n\nFirst.\n```\n\n"+
		"```ask\nname: dup\n\nSecond.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: two asks declare the same name:")
	}
	if !strings.Contains(got, `name: "dup"`) {
		t.Errorf("lint does not name the duplicated value:\n%s", got)
	}
}

func TestLintFailsOnDanglingCue(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\ncues: nowhere-at-all\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: cues: names no ask anywhere")
	}
	if !strings.Contains(got, `cues: "nowhere-at-all"`) {
		t.Errorf("lint does not name the dangling value:\n%s", got)
	}
}

// The idiom for a cue-only ask, not-in: **, must not itself trip the banner
// check — it can never fire on its own, which is the opposite problem from
// firing on everything.
func TestLintDoesNotBannerACueOnlyAsk(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\ncues: check-token-scope\nwhen: TODO\n\nCiting question.\n```\n\n"+
		"```ask\nname: check-token-scope\nnot-in: **\n\nCued question.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("a valid cue-only ask should lint clean: %v\n%s", err, got)
	}
	if strings.Contains(got, "banner") {
		t.Errorf("not-in: ** should not read as a banner:\n%s", got)
	}
}

// A not-in: that excludes something other than everything is still a
// banner everywhere else — the not-in: ** exemption must not be so loose
// it swallows this case too.
func TestLintStillBannersAPartialNotIn(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nnot-in: vendor/**\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: this still fires on everything outside vendor/")
	}
	if !strings.Contains(got, "banner") {
		t.Errorf("a partial not-in: should still read as a banner:\n%s", got)
	}
}
