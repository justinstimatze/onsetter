package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justinstimatze/onsetter/ask"
)

// cmdInstall writes the one settings entry onsetter will ever need.
//
// The wiring belongs in the tool because settings.local.json is the one piece
// git cannot carry: it is per-machine, and often gitignored on top of that. A
// sibling project kept the symlink step in a Makefile target and the settings
// edit in its README as a manual step; the symlinks got made, the wiring did
// not, and its whole hook set sat dark for months while the install target
// exited 0. An install that does not write the settings is not an install.
func cmdInstall(args []string) error {
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.local.json")
	if v := os.Getenv("CLAUDE_SETTINGS"); v != "" {
		settings = v
	}
	if len(args) > 0 {
		settings = args[0]
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the onsetter binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	command := self + " hook"

	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		return err
	}
	root := map[string]any{}
	if b, err := os.ReadFile(settings); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &root); err != nil {
			return fmt.Errorf("%s is not valid JSON — refusing to touch it: %w", settings, err)
		}
		backup := fmt.Sprintf("%s.bak-%s", settings, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, b, 0o600); err != nil {
			return err
		}
		defer fmt.Printf("  backup: %s\n", shortOne(backup))
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	// Drop any previous onsetter entry so re-running converges instead of
	// stacking, and leave every other project's hooks exactly where they are.
	kept := make([]any, 0, len(pre))
	for _, e := range pre {
		entry, ok := e.(map[string]any)
		if !ok {
			kept = append(kept, e)
			continue
		}
		inner, _ := entry["hooks"].([]any)
		survivors := make([]any, 0, len(inner))
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				survivors = append(survivors, h)
				continue
			}
			if cmd, _ := hm["command"].(string); isOnsetterHookCommand(cmd) {
				continue
			}
			survivors = append(survivors, h)
		}
		if len(survivors) == 0 {
			continue
		}
		entry["hooks"] = survivors
		kept = append(kept, entry)
	}

	kept = append(kept, map[string]any{
		"matcher": "Write|Edit",
		"hooks":   []any{map[string]any{"type": "command", "command": command}},
	})
	hooks["PreToolUse"] = kept
	root["hooks"] = hooks

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	tmp := settings + ".onsetter-tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, settings); err != nil {
		return err
	}

	skill, err := writeSkill(filepath.Dir(settings))
	if err != nil {
		return err
	}

	fmt.Printf("  wired %s\n", shortOne(settings))
	fmt.Printf("  command: %s\n", command)
	fmt.Printf("  skill:   %s\n", shortOne(skill))
	fmt.Printf("\n  Open /hooks once in Claude Code to reload the settings watcher.\n")
	fmt.Printf("  Hook settings are read per session — a session already running\n")
	fmt.Printf("  will not pick this up.\n")
	return nil
}

// isOnsetterHookCommand reports whether cmd is an onsetter PreToolUse entry.
// Shared with cmdStatus so wiring "already there" and wiring "reported
// present" cannot drift apart.
func isOnsetterHookCommand(cmd string) bool {
	return strings.HasSuffix(strings.TrimSpace(cmd), " hook") && strings.Contains(cmd, "onsetter")
}

// writeSkill installs the authoring guide as a Claude Code skill next to the
// settings file being wired.
//
// The hook needs no advertisement — it fires on the tool call whether or not
// anything knows it exists. Writing an ask is the opposite: there is no tool
// named onsetter in an agent's list and no way to discover the format except
// prose somebody left lying around. A skill is that prose, indexed by its
// description, so it loads when an ask is being written rather than sitting in
// the always-on file costing tokens on every unrelated turn.
//
// Regenerated on every install, because the guide belongs to the binary and a
// hand-edit here would be a second source of truth for a format the parser
// beside it defines.
func writeSkill(claudeDir string) (string, error) {
	dir := filepath.Join(claudeDir, "skills", "onsetter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SKILL.md")
	tmp := path + ".onsetter-tmp"
	if err := os.WriteFile(tmp, []byte(ask.Skill()), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}
