package ask_test

// Everything in this file lives in package ask_test, not package ask — it
// exercises only the exported surface, the way a second Go module (the
// motivating case: winze-agent's capture-guard) actually would. Every other
// test in this package is white-box (package ask) and could pass against a
// surface no external caller could reach; this file is what proves the
// public API is the API.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/justinstimatze/onsetter/ask"
)

// Example_pathless mirrors README.md's "Using ask as a library" snippet —
// ParseFile, then Match against an Edit with no Path, the shape a caller
// with no file (an MCP tool argument, not a Write/Edit) actually has. If
// this stops compiling, or the output changes, that section of the README
// needs the same edit; the two are meant to drift together, not silently.
func Example_pathless() {
	dir, err := os.MkdirTemp("", "ask-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("```ask\nwhen: TODO\n\nFound a TODO.\n```\n"), 0o644); err != nil {
		panic(err)
	}

	asks, err := ask.ParseFile(claudeMD)
	if err != nil {
		panic(err)
	}
	note := "TODO: fix this"
	for _, a := range asks {
		res := a.Match(ask.Edit{New: note}) // no Path — this call has none
		if res.OK {
			fmt.Println(res.Matched)
		}
	}
	// Output: TODO
}

// TestExternalPackageCanRoundTrip is the black-box counterpart to
// TestMatchWithoutAPath (which is white-box, package ask): parse a real
// file, match a real Edit, read every exported field of Result — entirely
// through names package ask_test has no special access to.
func TestExternalPackageCanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	src := "```ask\nrequires: go\nin: **/*.md\nwhen: (?i)\\bTODO\\b\n\nFound a TODO.\n```\n"
	if err := os.WriteFile(claudeMD, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	asks, err := ask.ParseFile(claudeMD)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(asks) != 1 {
		t.Fatalf("got %d asks, want 1", len(asks))
	}

	target := filepath.Join(dir, "notes.md")
	res := asks[0].Match(ask.Edit{Path: target, New: "TODO: ship this"})
	if !res.OK {
		t.Fatalf("want a fire, got rejection: %s", res.Why())
	}
	if res.Matched != "TODO" {
		t.Errorf("Matched = %q, want %q", res.Matched, "TODO")
	}

	res = asks[0].Match(ask.Edit{New: "TODO: ship this"}) // no Path this time
	if res.OK || res.Gate != "in" {
		t.Errorf("path-less call against an ask with a non-default in: should reject at in:, got %+v", res)
	}
}
