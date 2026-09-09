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

// Invalid JSON already sitting at the settings path is refused outright
// rather than overwritten — a hand-broken or mid-write file from some other
// tool should never silently lose its contents to an install run.
func TestInstallRefusesInvalidSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(settings, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cmdInstall([]string{settings})
	if err == nil {
		t.Fatal("want an error: settings.local.json is not valid JSON")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error does not say the file is invalid JSON: %v", err)
	}
	b, rerr := os.ReadFile(settings)
	if rerr != nil || string(b) != "{ not json" {
		t.Errorf("refused install still touched the file: %v, %q", rerr, b)
	}
}

// writeSkill's own MkdirAll fails when a path component it needs to create is
// already a plain file rather than a directory — the shape any install hits
// if something else already occupies "skills" under the target .claude dir.
func TestWriteSkillFailsWhenSkillsPathIsAFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "skills"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := writeSkill(dir); err == nil {
		t.Fatal("want an error: skills/ cannot be created where a file of that name already exists")
	}
}

// writeSkill's WriteFile fails when the target directory exists but is not
// writable — MkdirAll is a no-op on an already-existing directory, so this
// is the one failure MkdirAll succeeding can still be followed by. Rename's
// own failure mode needs a cross-device or permission change mid-call that
// cannot be forced portably from a unit test — same accepted-gap shape as
// install.go's os.Executable() branch.
func TestWriteSkillFailsWhenDirNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory write permission bits")
	}
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "onsetter")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(skillDir, 0o755) // let t.TempDir() clean up afterward

	if _, err := writeSkill(dir); err == nil {
		t.Fatal("want an error: the skill directory is not writable")
	}
}

// --read is opt-in and reversible: it adds a second PreToolUse entry, and a
// plain re-install without the flag removes it — the flags a given run
// passes are the whole desired state, the same convergence rule the base
// entry already follows.
func TestInstallReadIsOptInAndReversible(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.local.json")

	onsetterHookCount := func(t *testing.T) int {
		t.Helper()
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
		return n
	}

	if err := cmdInstall([]string{"--read", settings}); err != nil {
		t.Fatalf("install --read: %v", err)
	}
	if n := onsetterHookCount(t); n != 2 {
		t.Fatalf("install --read left %d onsetter hook entries, want 2", n)
	}

	if err := cmdInstall([]string{settings}); err != nil {
		t.Fatalf("plain re-install: %v", err)
	}
	if n := onsetterHookCount(t); n != 1 {
		t.Errorf("a plain re-install left %d onsetter hook entries, want 1 (Read should be dropped)", n)
	}
}
