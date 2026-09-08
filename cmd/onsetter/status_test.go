package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runStatus runs bin with args, isolated to an explicit settings path and
// cache dir so a test can never read or touch the real machine's
// ~/.claude/settings.local.json or evokes: cache. HOME is pointed at the same
// temp dir as cache — statusPlugin globs $HOME/.claude/plugins, and without
// this a test run on a machine that has actually installed the onsetter
// plugin would pick up the real one.
func runStatus(t *testing.T, bin, dir, settings, cache string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_SETTINGS="+settings, "XDG_CACHE_HOME="+cache, "HOME="+cache)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestStatusReportsNotWiredWithoutSettings(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	settings := filepath.Join(t.TempDir(), "settings.local.json") // never written

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: nothing is wired")
	}
	if !strings.Contains(out, "not found") || !strings.Contains(out, "onsetter install") {
		t.Errorf("wiring section does not say what to do about it:\n%s", out)
	}
}

func TestStatusReportsWiredAfterInstall(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, t.TempDir(), "install", settings); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err != nil {
		t.Fatalf("status after install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "wired") {
		t.Errorf("wiring section did not report wired after install:\n%s", out)
	}
	if !strings.Contains(out, "command: ") {
		t.Errorf("wiring section did not echo the registered command:\n%s", out)
	}
}

func TestStatusReportsParseErrors(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: (unclosed\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: the only block on disk fails to parse")
	}
	if !strings.Contains(out, "parse error") {
		t.Errorf("discovery section does not mention the parse error:\n%s", out)
	}
}

func TestStatusReportsMissingRequiresBinary(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nrequires: definitely-not-a-real-binary-xyz\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: the named binary does not exist")
	}
	if !strings.Contains(out, "MISSING definitely-not-a-real-binary-xyz") {
		t.Errorf("requires: section does not name the missing binary:\n%s", out)
	}
}

func TestStatusReportsFoundRequiresBinary(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nrequires: git\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	// install was never run, so overall status still fails on wiring alone —
	// assert requires: specifically rather than the exit code.
	out, _ := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if !strings.Contains(out, "found   git") {
		t.Errorf("requires: section did not report git as found:\n%s", out)
	}
}

func TestStatusReportsNoEvokesPhrasesCleanly(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: TODO\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	// install was never run, so overall status still fails on wiring alone —
	// assert the evokes: section specifically rather than the exit code.
	out, _ := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if !strings.Contains(out, "no evokes: phrases") {
		t.Errorf("evokes: section did not report cleanly on an ask with none:\n%s", out)
	}
}

// Where(base) needs base to be absolute to compute a clean relative path —
// dir stays the literal "." most callers type, so status must resolve it
// itself before handing it to Where, or every location prints as an ugly
// absolute path instead of "CLAUDE.md:1".
func TestStatusPrintsACleanLocationForTheDefaultDir(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nrequires: definitely-not-a-real-binary-xyz\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, _ := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if !strings.Contains(out, "MISSING definitely-not-a-real-binary-xyz (CLAUDE.md:1)") {
		t.Errorf("location did not resolve to a clean relative path against \".\":\n%s", out)
	}
	if strings.Contains(out, repo) {
		t.Errorf("output leaked the absolute temp dir path instead of resolving it:\n%s", out)
	}
}

func TestStatusReportsMissingEvokesPhraseWithoutTouchingOllama(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nevokes: committing without asking first\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: the phrase was never warmed")
	}
	if !strings.Contains(out, `MISSING "committing without asking first"`) ||
		!strings.Contains(out, "onsetter warm") {
		t.Errorf("evokes: section does not name the missing phrase or the fix:\n%s", out)
	}
}

// The one test in this file that needs a real embed call: a phrase that has
// actually been warmed should read as warmed, not just as present.
func TestStatusReportsWarmedEvokesPhrase(t *testing.T) {
	bin := buildBinary(t)
	cache := t.TempDir()
	skipUnlessOllama(t, bin, cache)

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nevokes: committing without asking first\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, cache, "warm", "."); err != nil {
		t.Fatalf("warm: %v\n%s", err, out)
	}

	// install was never run, so overall status still fails on wiring alone —
	// assert the evokes: section specifically rather than the exit code.
	out, _ := runStatus(t, bin, repo, settings, cache, "status", ".")
	if !strings.Contains(out, "1 phrase(s), 1 warmed, 0 missing") {
		t.Errorf("evokes: section did not report the phrase as warmed:\n%s", out)
	}
}

func TestStatusReportsDanglingCue(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\ncues: nowhere-at-all\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: cues: names no ask anywhere")
	}
	if !strings.Contains(out, `MISSING cues: "nowhere-at-all"`) {
		t.Errorf("cues: section does not name the dangling value:\n%s", out)
	}
}

func TestStatusExitsZeroWithAValidCue(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), ""+
		"```ask\nwhen: TODO\ncues: check-token-scope\n\nCiting question.\n```\n\n"+
		"```ask\nname: check-token-scope\nnot-in: **\n\nCued question.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, t.TempDir(), "install", settings); err != nil {
		t.Fatalf("install: %v\n%s", out, err)
	}

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err != nil {
		t.Fatalf("want exit 0 with a valid name:/cues: pair:\n%v\n%s", err, out)
	}
}

// The sixth silent-failure mode: an on: read ask with no Read wiring to
// ever reach it. install (no --read) leaves the ask unreachable, and
// status should say so rather than exit 0 over a dead ask.
func TestStatusReportsOnReadAskWithReadUnwired(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\non: read\n\nNever read the corpus directly.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, t.TempDir(), "install", settings); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: on: read has no Read wiring to ever reach it")
	}
	if !strings.Contains(out, "UNREACHABLE") || !strings.Contains(out, "on: read") ||
		!strings.Contains(out, "install --read") {
		t.Errorf("status did not report the unreachable on: read ask:\n%s", out)
	}
}

// Wiring Read with --read clears the same report.
func TestStatusClearsOnReadReportWhenReadIsWired(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\non: read\n\nNever read the corpus directly.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, t.TempDir(), "install", "--read", settings); err != nil {
		t.Fatalf("install --read: %v\n%s", err, out)
	}

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err != nil {
		t.Fatalf("want exit 0 once Read is wired:\n%v\n%s", err, out)
	}
	if !strings.Contains(out, "Read is wired") {
		t.Errorf("status did not confirm Read is wired:\n%s", out)
	}
}

func TestStatusReportsNoPluginInstallCleanly(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	out, _ := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if !strings.Contains(out, "no marketplace-installed onsetter found") {
		t.Errorf("plugin section did not report cleanly with nothing installed:\n%s", out)
	}
}

func TestStatusReportsFetchedPluginBinary(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	settings := filepath.Join(t.TempDir(), "settings.local.json") // never written — no manual install

	home := t.TempDir()
	dataDir := filepath.Join(home, ".claude", "plugins", "data", "onsetter-community")
	mkdir(t, filepath.Join(dataDir, "bin"))
	pluginBin := filepath.Join(dataDir, "bin", "onsetter")
	write(t, pluginBin, "#!/bin/sh\n")
	if err := os.Chmod(pluginBin, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runStatus(t, bin, repo, settings, home, "status", ".")
	if !strings.Contains(out, "fetched binary") {
		t.Errorf("plugin section did not report the fetched binary:\n%s", out)
	}
	if strings.Contains(out, "WARNING") {
		t.Errorf("a plugin-only install should not warn about double-wiring:\n%s", out)
	}
	_ = err // other sections may still fail this bare repo; only the plugin section is under test
}

func TestStatusWarnsOnDoubleWiring(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	home := t.TempDir()
	dataDir := filepath.Join(home, ".claude", "plugins", "data", "onsetter-community")
	mkdir(t, filepath.Join(dataDir, "bin"))
	pluginBin := filepath.Join(dataDir, "bin", "onsetter")
	write(t, pluginBin, "#!/bin/sh\n")
	if err := os.Chmod(pluginBin, 0o755); err != nil {
		t.Fatal(err)
	}

	// runStatus always points HOME at its cache arg, so the manual install
	// below and the plugin data dir above must share one temp dir (home) for
	// both to be visible to the same status run.
	if out, err := runStatus(t, bin, repo, settings, home, "install", settings); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	out, err := runStatus(t, bin, repo, settings, home, "status", ".")
	if err == nil {
		t.Fatal("want a non-zero exit: double-wiring is a real problem")
	}
	if !strings.Contains(out, "WARNING") || !strings.Contains(out, "runs onsetter hook twice") {
		t.Errorf("status did not warn about double-wiring:\n%s", out)
	}
}

func TestStatusExitsZeroWhenEverythingIsFine(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nrequires: git\nwhen: TODO\n\nAsk.\n```\n")
	settings := filepath.Join(t.TempDir(), "settings.local.json")

	if out, err := runStatus(t, bin, repo, settings, t.TempDir(), "install", settings); err != nil {
		t.Fatalf("install: %v\n%s", out, err)
	}

	out, err := runStatus(t, bin, repo, settings, t.TempDir(), "status", ".")
	if err != nil {
		t.Fatalf("want exit 0 when wired, parsing clean, requires: satisfied, and no evokes: phrases:\n%v\n%s", err, out)
	}
}
