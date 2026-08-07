package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An install that writes the hook and not the skill leaves the authoring side
// undiscoverable while reporting success, which is the failure the install
// command exists to prevent in the first place.
func TestInstallWritesHookAndSkill(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.local.json")

	if err := cmdInstall([]string{settings}); err != nil {
		t.Fatalf("install: %v", err)
	}

	b, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("settings not written: %v", err)
	}
	if !strings.Contains(string(b), "onsetter") || !strings.Contains(string(b), "Write|Edit") {
		t.Errorf("settings has no onsetter PreToolUse entry:\n%s", b)
	}

	skill, err := os.ReadFile(filepath.Join(dir, "skills", "onsetter", "SKILL.md"))
	if err != nil {
		t.Fatalf("skill not written beside the settings: %v", err)
	}
	if !strings.HasPrefix(string(skill), "---\nname: onsetter\n") {
		t.Errorf("skill is missing its frontmatter:\n%.60s", skill)
	}
}

// Re-running is how the tool is upgraded, so it has to converge: one hook
// entry, and a skill regenerated from the binary rather than appended to.
func TestInstallConvergesOnRerun(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.local.json")
	skillPath := filepath.Join(dir, "skills", "onsetter", "SKILL.md")

	for range 3 {
		if err := cmdInstall([]string{settings}); err != nil {
			t.Fatalf("install: %v", err)
		}
	}

	b, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string
				Hooks   []struct{ Command string }
			}
		}
	}
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("install wrote invalid JSON: %v", err)
	}
	n := 0
	for _, e := range root.Hooks.PreToolUse {
		for _, h := range e.Hooks {
			if strings.Contains(h.Command, "onsetter") {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("three installs left %d onsetter hook entries, want 1", n)
	}

	// A hand-edit here is a second source of truth for a format the parser
	// defines, so install overwrites rather than merging.
	if err := os.WriteFile(skillPath, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdInstall([]string{settings}); err != nil {
		t.Fatalf("install: %v", err)
	}
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(skill), "edited by hand") {
		t.Error("install left a hand-edited skill in place instead of regenerating it")
	}
}
