package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A file whose only block is broken has to reach lint. Sources used to require
// at least one ask that parsed, so an unclosed regex made `onsetter lint`
// print "0 ask(s) in 0 file(s)" and exit 0 — a green light on a block the
// author believes is watching, which is the one thing lint is for.
func TestSourcesKeepsFilesWhoseOnlyBlockIsBroken(t *testing.T) {
	dir := t.TempDir()
	fence := "```"
	write(t, filepath.Join(dir, "CLAUDE.md"), fence+"ask\nwhen: (unclosed\n\nAsk.\n"+fence+"\n")

	sources, err := Sources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d source(s), want the file with the broken block", len(sources))
	}
	if _, perr := ParseSource(sources[0]); perr == nil {
		t.Error("the file is listed but parses cleanly, so lint would still say nothing")
	}
}

// A CLAUDE.md with no ask blocks at all is not lint's business.
func TestSourcesSkipsFilesWithNoBlocks(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "CLAUDE.md"), "Just ordinary project notes.\n")

	sources, err := Sources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Errorf("got %v, want nothing", sources)
	}
}

// Sources walks to `.claude/CLAUDE.md` by base name, so it has to place the
// globs the same way Asks does. Parsing it unscoped puts every `in:` one
// directory too deep, and the banner check then names `.claude` as the
// directory an ask governs.
func TestSourcesScopesDotClaudeLikeAsksDoes(t *testing.T) {
	dir := t.TempDir()
	fence := "```"
	write(t, filepath.Join(dir, ".claude", "CLAUDE.md"), fence+"ask\nin: internal/**/*.go\n\nAsk.\n"+fence+"\n")

	sources, err := Sources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d source(s), want the .claude one", len(sources))
	}
	asks, err := ParseSource(sources[0])
	if err != nil || len(asks) != 1 {
		t.Fatalf("parse: %d ask(s), %v", len(asks), err)
	}
	if got := asks[0].Dir; got != dir {
		t.Errorf("globs scoped to %q, want the project root %q", got, dir)
	}
}

// $HOME/.claude/CLAUDE.md collides with the project-local .claude/CLAUDE.md
// rule by directory-name coincidence — scoping it to $HOME the same way meant
// `in: feedback_*.md` never reached anything under
// $HOME/.claude/projects/**/memory/, confirmed live against the real global
// file before this was fixed.
func TestScopeOfGlobalClaudeMdScopesToItself(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	fence := "```"
	write(t, filepath.Join(claudeDir, "CLAUDE.md"), fence+"ask\nin: projects/**/memory/feedback_*.md\non: mint\n\nAsk.\n"+fence+"\n")

	asks, err := ParseSource(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil || len(asks) != 1 {
		t.Fatalf("parse: %d ask(s), %v", len(asks), err)
	}
	if got := asks[0].Dir; got != claudeDir {
		t.Errorf("globs scoped to %q, want the global file's own directory %q", got, claudeDir)
	}
}

// The rule this collides with still has to work: a real project root, not
// $HOME, one level above its .claude/CLAUDE.md.
func TestScopeOfProjectDotClaudeStillScopesToParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "some-project")
	fence := "```"
	write(t, filepath.Join(root, ".claude", "CLAUDE.md"), fence+"ask\nin: internal/**/*.go\n\nAsk.\n"+fence+"\n")

	asks, err := ParseSource(filepath.Join(root, ".claude", "CLAUDE.md"))
	if err != nil || len(asks) != 1 {
		t.Fatalf("parse: %d ask(s), %v", len(asks), err)
	}
	if got := asks[0].Dir; got != root {
		t.Errorf("globs scoped to %q, want the project root %q", got, root)
	}
}

// Roots has no direct test anywhere — every existing check exercises it only
// behaviorally, through cmd/onsetter's subprocess tests, and even those only
// ever cover the ".git present" case. This is the other half: a target with
// no .git anywhere between it and $HOME, which is the deliberate fallback
// this package's own doc comment names as what bounds an ask's blast radius
// when there's no repo root to stop at. Confirms the walk actually climbs
// all the way to $HOME and stops there — picking up a CLAUDE.md placed at
// $HOME itself — rather than silently walking past it to "/" or missing it.
func TestRootsStopsAtHomeWhenNoGitExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fence := "```"
	write(t, filepath.Join(home, "CLAUDE.md"), fence+"ask\nwhen: alpha\n\nAsk.\n"+fence+"\n")

	// A fresh, not-yet-git-init'd project directly under $HOME — no .git
	// anywhere in its ancestor chain.
	target := filepath.Join(home, "fresh-project", "a.md")
	write(t, target, "x")

	roots := Roots(target)
	if len(roots) != 1 {
		t.Fatalf("got %v, want exactly the $HOME/CLAUDE.md fallback", roots)
	}
	if roots[0] != filepath.Join(home, "CLAUDE.md") {
		t.Errorf("got %q, want the $HOME CLAUDE.md", roots[0])
	}
}

// The same boundary, confirmed from the other side: a CLAUDE.md sitting
// above $HOME (in $HOME's own parent) must never govern a file under a
// git-root-less project inside $HOME — the walk has to actually stop at
// $HOME, not just happen to find the right file first.
func TestRootsNeverReachesAboveHomeWithoutGit(t *testing.T) {
	above := t.TempDir()
	home := filepath.Join(above, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	fence := "```"
	write(t, filepath.Join(above, "CLAUDE.md"), fence+"ask\nwhen: alpha\n\nShould never be reached.\n"+fence+"\n")

	target := filepath.Join(home, "fresh-project", "a.md")
	write(t, target, "x")

	roots := Roots(target)
	if len(roots) != 0 {
		t.Errorf("got %v, want nothing — the walk must stop at $HOME, not climb past it", roots)
	}
}

// The headerless block is the format's sharpest edge: written the obvious way
// it does not parse, and the error has to say why rather than complain that a
// sentence is not `key: value`.
func TestHeaderlessBlockErrorSaysWhatToDo(t *testing.T) {
	dir := t.TempDir()
	fence := "```"
	write(t, filepath.Join(dir, "CLAUDE.md"), fence+"ask\nJust prose, no headers.\n"+fence+"\n")

	_, err := ParseSource(filepath.Join(dir, "CLAUDE.md"))
	if err == nil {
		t.Fatal("a block whose first line is prose parsed cleanly")
	}
	if !strings.Contains(err.Error(), "blank line") {
		t.Errorf("error does not mention the blank line: %v", err)
	}
}
