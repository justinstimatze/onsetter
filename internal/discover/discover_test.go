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
