package main

import (
	"os"
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
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+goCoverDir(t))
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

// on: read combined with any content gate can never fire — a Read call has
// no incoming text for when:/added:/removed:/not:/evokes: to match.
func TestLintFailsOnReadCombinedWithContentGates(t *testing.T) {
	for _, tc := range []struct {
		name, header, value string
	}{
		{"when", "when", "TODO"},
		{"added", "added", "TODO"},
		{"removed", "removed", "TODO"},
		{"not", "not", "TODO"},
		{"evokes", "evokes", "committing without asking first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := buildBinary(t)
			repo := t.TempDir()
			mkdir(t, filepath.Join(repo, ".git"))
			write(t, filepath.Join(repo, "CLAUDE.md"),
				"```ask\non: read\n"+tc.header+": "+tc.value+"\n\nAsk.\n```\n")

			got, err := lintIn(t, bin, repo)
			if err == nil {
				t.Fatalf("want a non-zero exit: on: read + %s: can never fire", tc.header)
			}
			if !strings.Contains(got, tc.header+":") {
				t.Errorf("lint does not name the dead gate:\n%s", got)
			}
		})
	}
}

// on: read alone, or paired with in:/has:, is not a dead-gate combination —
// only content gates trip it.
func TestLintDoesNotFlagOnReadWithoutContentGates(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nin: corpus/**\non: read\nnot-in: **\n\nCue-only, on read.\n```\n\n"+
		"```ask\non: read\nhas: sync\\.Mutex\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("on: read with only in:/has: should lint clean: %v\n%s", err, got)
	}
}

// revisit: true no longer changes anything for a matched ask — every
// matched ask always widens the session key now — so lint flags it as
// redundant rather than staying quiet about a header that does nothing.
func TestLintFlagsRedundantRevisit(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: TODO\nrevisit: true\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: revisit: true does nothing for a matched ask now")
	}
	if !strings.Contains(got, "revisit: true is redundant") {
		t.Errorf("lint does not explain why revisit: is flagged:\n%s", got)
	}
}

// revisit: true on a reminder (no content gate) was already a no-op before
// this change — has: alone contributes no quote either — so lint's new
// check, scoped to Matched-capable gates, should not fire on it.
func TestLintDoesNotFlagRevisitOnAReminder(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: corpus/**\nrevisit: true\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("revisit: true on a reminder should lint clean: %v\n%s", err, got)
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
