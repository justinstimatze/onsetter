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

// block: true with no added:/removed: is a parse error, not a silently-inert
// header — cmdLint surfaces a parse error the same way it surfaces any other
// bad block: printed to stderr, counted toward bad, non-zero exit.
func TestLintFailsOnBlockWithoutAddedOrRemoved(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: TODO\nblock: true\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: block: true with no added:/removed: does not parse")
	}
	if !strings.Contains(got, "block: true needs an added: or removed:") {
		t.Errorf("lint does not surface the parse error's own reason:\n%s", got)
	}
}

// A valid block: true ask — added: present, value spelled "true" — lints
// exactly as clean as any other ask with a content gate.
func TestLintCleanOnAValidBlockAsk(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nadded: \\bprint\\(\nblock: true\n\nNo print( in this repo.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("a valid block: true ask should lint clean: %v\n%s", err, got)
	}
}

// block: true still requires added:/removed: to parse at all, so it falls
// under the same on: read dead-combo check added: already trips — nothing
// new to teach cmdLint, just confirmed end to end rather than assumed.
func TestLintFailsOnReadCombinedWithBlock(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\non: read\nadded: TODO\nblock: true\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: on: read + added: can never fire, block: true or not")
	}
	if !strings.Contains(got, "added:") {
		t.Errorf("lint does not name the dead gate:\n%s", got)
	}
}

// A clean fires-on:/silent-on: pair — the ask actually catches its own
// positive fixture and stays quiet on its own negative one — lints clean.
func TestLintCleanOnFiresOnAndSilentOnWhenTheGateWorks(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: **/*.go\nwhen: (?m)^\\s*//\\s*TODO\nfires-on: fixtures/bad.go\nsilent-on: fixtures/good.go\n\nNo bare TODOs.\n```\n")
	mkdir(t, filepath.Join(repo, "fixtures"))
	write(t, filepath.Join(repo, "fixtures", "bad.go"), "package a\n\n// TODO: fix this\n")
	write(t, filepath.Join(repo, "fixtures", "good.go"), "package a\n\n// tracked in TICKET-1\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("a working fires-on:/silent-on: pair should lint clean: %v\n%s", err, got)
	}
}

// germline's first real bug: a broken anchoring regex. when: ^TODO anchors
// to the start of the whole file, not to each line, so a TODO on line 3
// never matches — and replay's rate alone reads identical to a genuinely
// clean corpus. fires-on: catches it directly.
func TestLintFailsOnFiresOnWhenTheAnchoringIsBroken(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: **/*.go\nwhen: ^TODO\nfires-on: fixtures/bad.go\n\nNo bare TODOs.\n```\n")
	mkdir(t, filepath.Join(repo, "fixtures"))
	write(t, filepath.Join(repo, "fixtures", "bad.go"), "package a\n\n// TODO: fix this\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: ^TODO cannot match a TODO on line 3")
	}
	if !strings.Contains(got, "fires-on:") || !strings.Contains(got, "does not fire") {
		t.Errorf("lint does not name the fires-on: mismatch:\n%s", got)
	}
}

// germline's second real bug: a glob narrower than the corpus it means to
// cover. in: src/** never reaches a fixture that lives outside src/, so it
// turns away at in: — indistinguishable from a healthy ask by rate alone,
// caught here because fires-on: names a real file the ask should reach.
func TestLintFailsOnFiresOnWhenTheGlobIsTooNarrow(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: src/**\nwhen: TODO\nfires-on: lib/bad.go\n\nNo bare TODOs.\n```\n")
	mkdir(t, filepath.Join(repo, "lib"))
	write(t, filepath.Join(repo, "lib", "bad.go"), "package a\n\n// TODO: fix this\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: in: src/** never reaches lib/bad.go")
	}
	if !strings.Contains(got, "fires-on:") || !strings.Contains(got, "does not fire") {
		t.Errorf("lint does not name the fires-on: mismatch:\n%s", got)
	}
}

// A silent-on: fixture the ask actually fires on — the author mislabeled a
// real positive as a negative example — is exactly as real a bug as a
// fires-on: mismatch, and gets the same treatment.
func TestLintFailsOnSilentOnWhenTheFixtureActuallyFires(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: **/*.go\nwhen: TODO\nsilent-on: fixtures/not_actually_good.go\n\nNo bare TODOs.\n```\n")
	mkdir(t, filepath.Join(repo, "fixtures"))
	write(t, filepath.Join(repo, "fixtures", "not_actually_good.go"), "package a\n\n// TODO: still here\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: the silent-on: fixture actually contains a TODO")
	}
	if !strings.Contains(got, "silent-on:") || !strings.Contains(got, "fires") {
		t.Errorf("lint does not name the silent-on: mismatch:\n%s", got)
	}
}

// A fires-on:/silent-on: glob matching no file at all is its own bug — a
// typo'd path, or a fixture that was never committed — distinct from a gate
// that fails to fire on a real file.
func TestLintFailsOnFiresOnGlobMatchingNoFile(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nwhen: TODO\nfires-on: fixtures/does_not_exist.go\n\nAsk.\n```\n")

	got, err := lintIn(t, bin, repo)
	if err == nil {
		t.Fatal("want a non-zero exit: fires-on: names a file that does not exist")
	}
	if !strings.Contains(got, "matches no file") {
		t.Errorf("lint does not explain why:\n%s", got)
	}
}

// An on: read ask's fires-on: fixture has to be checked against the same
// Read-shaped synthetic edit cmdList and cmdReplay already build for it —
// New="" — or a has: gate would see the wrong shape. This fixture has no
// has:, so the only way it fires at all is if the Read-shaped edit (not the
// write-shaped one, whose New would be the fixture's own content) is what's
// actually being tested.
func TestLintFixtureCheckHandlesOnReadAsks(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: **/*.go\non: read\nfires-on: fixtures/anything.go\n\nA Read of any Go file is worth noting.\n```\n")
	mkdir(t, filepath.Join(repo, "fixtures"))
	write(t, filepath.Join(repo, "fixtures", "anything.go"), "package a\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("an on: read ask with no content gate should fire on its own fires-on: fixture: %v\n%s", err, got)
	}
}

// removed: can never pass on the synthetic single-file edit fires-on:/
// silent-on: build — there is no old text to remove a line from — so an
// ask carrying removed: is skipped rather than given a result that would
// always be wrong in one direction or the other.
func TestLintSkipsFiresOnForARemovedGatedAsk(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"),
		"```ask\nin: **/*.go\nremoved: if err != nil\nfires-on: fixtures/anything.go\n\nDeleting a check.\n```\n")
	mkdir(t, filepath.Join(repo, "fixtures"))
	write(t, filepath.Join(repo, "fixtures", "anything.go"), "package a\n")

	got, err := lintIn(t, bin, repo)
	if err != nil {
		t.Fatalf("a removed: gate's fires-on: should be skipped, not fail: %v\n%s", err, got)
	}
	if !strings.Contains(got, "not checked") {
		t.Errorf("lint does not explain that removed: was skipped:\n%s", got)
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
