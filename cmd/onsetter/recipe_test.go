package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recipeBlock extracts the ```ask fenced block from README.md that sets
// requires: stull — the one worked example in this repo meant to be copied
// verbatim rather than adapted, from the Recipes section. There is exactly
// one; a second requires: block anywhere in the README would make which one
// this test means ambiguous, so it fails loud rather than picking one.
func recipeBlock(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const fence = "```ask\n"
	var found []string
	for _, chunk := range strings.Split(string(readme), fence) {
		end := strings.Index(chunk, "```")
		if end < 0 {
			continue
		}
		block := chunk[:end]
		if strings.Contains(block, "requires: stull") {
			found = append(found, block)
		}
	}
	if len(found) != 1 {
		t.Fatalf("README.md has %d ask block(s) setting requires: stull, want exactly 1", len(found))
	}
	return found[0]
}

// headers.md's own worked example is protected for free: it is embedded and
// TestReferenceDocumentsEveryHeader parses the whole reference at the end of
// that test, so a broken example there fails CI. README.md is not embedded
// or parsed by anything, so the stull recipe — the one block in this repo
// meant to be pasted whole into someone else's CLAUDE.md — had no such
// protection until this test. It shipped once already with a scope bug
// (in: CLAUDE.md instead of in: **/CLAUDE.md, matching only the ask's own
// directory) that a parse-only check would not have caught, which is why
// this exercises firing behavior against a nested file, not just parsing.
func TestStullRecipeInReadme(t *testing.T) {
	block := recipeBlock(t)
	if !strings.Contains(block, "in: **/CLAUDE.md") {
		t.Fatalf("recipe no longer scopes to in: **/CLAUDE.md, the fix for the same-directory-only bug:\n%s", block)
	}

	bin := buildBinary(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, filepath.Join(repo, "sub"))

	// requires: go, not requires: stull — the assertion above already pins
	// the shipped text; firing behavior has to be provable without stull
	// installed in whatever environment runs this test. go is guaranteed
	// present, since it is the binary running this test.
	deterministic := strings.Replace(block, "requires: stull", "requires: go", 1)
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\n"+deterministic+"```\n")
	write(t, filepath.Join(repo, "sub", "CLAUDE.md"), "Never commit without asking me first.\n")

	got := listIn(t, bin, repo, filepath.Join("sub", "CLAUDE.md"))
	if !strings.Contains(got, `would fire, matching "Never"`) {
		t.Errorf("recipe did not fire on a nested CLAUDE.md containing \"Never\":\n%s", got)
	}

	absent := strings.Replace(block, "requires: stull", "requires: definitely-not-a-real-binary-onsetter-test", 1)
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\n"+absent+"```\n")
	got = listIn(t, bin, repo, filepath.Join("sub", "CLAUDE.md"))
	if !strings.Contains(got, "turned away at requires:") {
		t.Errorf("recipe did not reject when its required tool is absent:\n%s", got)
	}
}
