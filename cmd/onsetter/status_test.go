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
// ~/.claude/settings.local.json or evokes: cache.
func runStatus(t *testing.T, bin, dir, settings, cache string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_SETTINGS="+settings, "XDG_CACHE_HOME="+cache)
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
