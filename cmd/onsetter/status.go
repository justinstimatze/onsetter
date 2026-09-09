package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/discover"
	"github.com/justinstimatze/onsetter/internal/embed"
)

// cmdStatus answers one question: if a file were written right now that
// should trip an ask, would anything actually happen. Six ways that can
// silently be false — the hook not wired, a block that fails to parse, an
// evokes: phrase never warmed, a requires: binary not on this machine, a
// cues: value that resolves to nothing (or to something out of scope), an
// on: read ask with no Read wiring to ever reach it — each fail quietly on
// their own: a rejected write, a dead hook, a block nobody notices was
// never checked. status is read-only, so running it changes nothing it
// reports on.
func cmdStatus(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	// Where(base) computes filepath.Rel(base, r.Source), and r.Source is
	// always absolute (ParseFileScoped resolves it). Rel between a relative
	// base and an absolute target does not resolve the way it looks like it
	// should — Where quietly falls back to the ugly absolute form on the
	// default "." every caller actually types. Resolve once, here.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	bad := 0

	fmt.Println("wiring")
	wiringOK, readWired := statusWiring()
	if !wiringOK {
		bad++
	}

	fmt.Println("\nplugin install")
	pluginBad, mcpConfigured := statusPlugin(wiringOK)
	bad += pluginBad

	fmt.Println("\ndiscovery")
	asks, problems := statusDiscovery(dir)
	bad += problems

	fmt.Println("\non: read wiring")
	// The plugin's mcp_tool transport wires Read unconditionally — no
	// --read flag to check for, unlike the manual settings.local.json path
	// readWired already covers. A plugin-only install with no manual entry
	// at all must not misreport every on: read ask as unreachable.
	bad += statusReadWiring(asks, readWired || mcpConfigured, dir)

	fmt.Println("\nevokes: cache")
	bad += statusEvokes(asks, dir)

	fmt.Println("\nrequires:")
	bad += statusRequires(asks, dir)

	fmt.Println("\ncues:")
	bad += statusCues(asks, dir)

	if bad > 0 {
		return fmt.Errorf("%d problem(s)", bad)
	}
	return nil
}

// wiredEntry is one hooks.PreToolUse[] entry install.go wrote, or would
// have dropped and replaced on the next run.
type wiredEntry struct{ Matcher, Command string }

// statusWiring checks the one settings file install.go would have written
// to by default. A custom CLAUDE_SETTINGS or a project-local settings.json
// is invisible to this check — see the CHANGELOG for why that scope was
// chosen over scanning every location Claude Code merges. Returns whether
// onsetter is wired at all, and separately whether Read is among the
// matchers wired — the fact statusReadWiring needs, captured here before
// discovery runs rather than re-parsed a second time.
func statusWiring() (ok, readWired bool) {
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.local.json")
	if v := os.Getenv("CLAUDE_SETTINGS"); v != "" {
		settings = v
	}

	b, err := os.ReadFile(settings)
	if err != nil {
		fmt.Printf("  %s — not found\n", shortOne(settings))
		fmt.Println("  → run `onsetter install`")
		return false, false
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		fmt.Printf("  %s — not valid JSON\n", shortOne(settings))
		return false, false
	}
	entries := findOnsetterEntries(root)
	if len(entries) == 0 {
		fmt.Printf("  %s — no onsetter entry in PreToolUse\n", shortOne(settings))
		fmt.Println("  → run `onsetter install`")
		return false, false
	}
	fmt.Printf("  %s — wired\n", shortOne(settings))
	for _, e := range entries {
		fmt.Printf("  matcher: %-16s command: %s\n", e.Matcher, e.Command)
		if matcherHasToken(e.Matcher, "Read") {
			readWired = true
		}
	}
	return true, readWired
}

// findOnsetterEntries walks the same hooks.PreToolUse[].hooks[] shape
// install.go writes, returning every entry install.go itself would drop
// and replace on the next run. The two must agree on what counts as
// "already wired" — one loose end here and status could report wired when
// install would silently duplicate an entry, or the reverse.
func findOnsetterEntries(root map[string]any) []wiredEntry {
	var found []wiredEntry
	hooks, _ := root["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	for _, e := range pre {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if isOnsetterHookCommand(cmd) {
				found = append(found, wiredEntry{Matcher: matcher, Command: cmd})
			}
		}
	}
	return found
}

// matcherHasToken reports whether a "|"-separated matcher string
// (install.go's own format, e.g. "Write|Edit|Bash") names token exactly —
// a split-and-compare rather than strings.Contains, so a hypothetical
// future tool name that happens to contain "Read" as a substring can never
// false-positive.
func matcherHasToken(matcher, token string) bool {
	for _, part := range strings.Split(matcher, "|") {
		if part == token {
			return true
		}
	}
	return false
}

// statusReadWiring is the sixth silent-failure mode cmdStatus's own doc
// comment names: an on: read ask defined with no Read wiring to ever reach
// it fails exactly like a requires: binary that's missing or a cues: value
// that resolves to nothing — silently, discovered only by a firing that
// never happens, unless something checks proactively. asks is already in
// hand from statusDiscovery; this is a look at data already collected, not
// a new walk.
func statusReadWiring(asks []*ask.Ask, readWired bool, dir string) int {
	var readAsks []*ask.Ask
	for _, a := range asks {
		if a.On == ask.ModeRead {
			readAsks = append(readAsks, a)
		}
	}
	if len(readAsks) == 0 {
		fmt.Println("  no on: read asks")
		return 0
	}
	if readWired {
		fmt.Printf("  %d on: read ask(s), Read is wired\n", len(readAsks))
		return 0
	}
	for _, a := range readAsks {
		fmt.Printf("  UNREACHABLE %s — on: read, but Read is not wired\n", a.Where(dir))
	}
	fmt.Println("  → run `onsetter install --read`")
	return len(readAsks)
}

// statusPlugin looks for a marketplace-installed onsetter, the second of the
// two independent ways onsetter's hook gets wired. A plugin's hooks.json is
// never written into settings.local.json, so before this check existed a
// plugin-only install read as "not found, run onsetter install" above —
// wrong advice for someone who never needed to run it. Detection is a
// filesystem glob rather than reading Claude Code's own settings merge,
// since enabledPlugins can be unset and still mean "enabled" (it falls back
// to the plugin's defaultEnabled), so absence there proves nothing; a
// populated cache or data directory is the more honest signal to check.
//
// Returns mcpConfigured — whether an installed cached copy carries the
// mcp_tool wiring (.mcp.json alongside it) — separately from bad, since a
// cache hit without it is not a fault, only an older, still-working
// version: the "configured" state this reports is static (does the file
// exist and name the right shape), never a live connection claim, since a
// one-shot status process has no channel into a running session's actual
// MCP handshake.
func statusPlugin(manualWired bool) (bad int, mcpConfigured bool) {
	home := os.Getenv("HOME")
	cacheHits, _ := filepath.Glob(filepath.Join(home, ".claude", "plugins", "cache", "*", "onsetter"))
	dataHits, _ := filepath.Glob(filepath.Join(home, ".claude", "plugins", "data", "onsetter*"))

	if len(cacheHits) == 0 && len(dataHits) == 0 {
		fmt.Println("  no marketplace-installed onsetter found under ~/.claude/plugins")
		return 0, false
	}

	for _, hit := range cacheHits {
		fmt.Printf("  plugin cache: %s\n", shortOne(hit))
		mcp := filepath.Join(hit, ".mcp.json")
		if _, err := os.Stat(mcp); err == nil {
			fmt.Println("  MCP server wiring: configured (mcp_tool) — connection state is not checked here, only that the file names it correctly")
			mcpConfigured = true
		} else {
			fmt.Println("  MCP server wiring: not present in this cached copy — /plugin update to pick up onsetter serve")
		}
	}
	for _, hit := range dataHits {
		bin := filepath.Join(hit, "bin", "onsetter")
		if info, err := os.Stat(bin); err == nil && info.Mode()&0o111 != 0 {
			fmt.Printf("  fetched binary: %s\n", shortOne(bin))
			continue
		}
		fmt.Printf("  data dir present, no fetched binary yet: %s\n", shortOne(hit))
		fmt.Println("  → start a new session so the SessionStart hook can fetch one")
	}

	if !manualWired {
		return 0, mcpConfigured
	}
	fmt.Println("  WARNING a manual install is also wired in settings.local.json — every Write/Edit/Bash call runs onsetter hook twice")
	fmt.Println("  → keep one: uninstall the plugin, or remove the entry `onsetter install` wrote")
	return 1, mcpConfigured
}

// statusDiscovery is lint's own walk, kept in sync deliberately: same
// discover.Sources call, same per-file ParseSource, same parse-error
// counting. The asks it returns feed the two sections below, so a file
// status can't see is a file evokes: and requires: can't see either.
func statusDiscovery(dir string) ([]*ask.Ask, int) {
	sources, walkErr := discover.Sources(dir)
	if walkErr != nil {
		fmt.Printf("  %v\n", walkErr)
		return nil, 1
	}
	var all []*ask.Ask
	bad := 0
	for _, src := range sources {
		asks, err := discover.ParseSource(src)
		if err != nil {
			fmt.Printf("  %v\n", err)
			bad++
		}
		all = append(all, asks...)
	}
	fmt.Printf("  %d ask(s) across %d file(s) below %s\n", len(all), len(sources), shortOne(dir))
	if bad > 0 {
		fmt.Printf("  %d parse error(s)\n", bad)
	}
	return all, bad
}

// statusEvokes checks every distinct evokes: phrase against the warm cache
// with Cache.Warm — a pure lookup, no Ollama call, so this runs even when
// nothing is running locally to serve one. Dedup is by phrase text, matching
// the cache's own key: two asks sharing a phrase pay for one warm between
// them, so they should read as one line here too.
func statusEvokes(asks []*ask.Ask, dir string) int {
	type site struct{ phrase, where string }
	seen := map[string]bool{}
	var sites []site
	for _, a := range asks {
		for _, p := range a.Evokes {
			if seen[p] {
				continue
			}
			seen[p] = true
			sites = append(sites, site{p, a.Where(dir)})
		}
	}
	if len(sites) == 0 {
		fmt.Println("  no evokes: phrases")
		return 0
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].phrase < sites[j].phrase })

	cache := embed.OpenCache()
	missing := 0
	for _, s := range sites {
		if !cache.Warm(s.phrase) {
			missing++
			fmt.Printf("  MISSING %q (%s)\n", s.phrase, s.where)
		}
	}
	fmt.Printf("  %d phrase(s), %d warmed, %d missing\n", len(sites), len(sites)-missing, missing)
	if missing > 0 {
		fmt.Println("  → run `onsetter warm`")
	}
	return missing
}

// statusRequires checks every distinct requires: binary the same way Match
// does — exec.LookPath, nothing executed — but proactively instead of only
// ever discovered through a silent rejection.
func statusRequires(asks []*ask.Ask, dir string) int {
	type site struct{ bin, where string }
	seen := map[string]bool{}
	var sites []site
	for _, a := range asks {
		for _, b := range a.Requires {
			if seen[b] {
				continue
			}
			seen[b] = true
			sites = append(sites, site{b, a.Where(dir)})
		}
	}
	if len(sites) == 0 {
		fmt.Println("  no requires: binaries named")
		return 0
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].bin < sites[j].bin })

	missing := 0
	for _, s := range sites {
		if _, err := exec.LookPath(s.bin); err != nil {
			missing++
			fmt.Printf("  MISSING %s (%s)\n", s.bin, s.where)
		} else {
			fmt.Printf("  found   %s\n", s.bin)
		}
	}
	return missing
}

// statusCues is lint's own name:/cues: check, over the same asks
// statusDiscovery already walked — same duplication-on-purpose as that
// function staying in sync with lint's own walk: a cues: that lint would
// reject should read as broken here too, before match time rather than
// only ever discovered through a firing that silently never happens.
func statusCues(asks []*ask.Ask, dir string) int {
	problems := ask.ValidateCues(asks)
	if len(problems) == 0 {
		fmt.Println("  no problems")
		return 0
	}
	for _, p := range problems {
		switch p.Kind {
		case "duplicate-name":
			fmt.Printf("  DUPLICATE name: %q (%s)\n", p.Value, p.Ask.Where(dir))
		case "dangling-cue":
			fmt.Printf("  MISSING cues: %q (%s)\n", p.Value, p.Ask.Where(dir))
		case "cue-out-of-scope":
			fmt.Printf("  OUT-OF-SCOPE cues: %q (%s)\n", p.Value, p.Ask.Where(dir))
		}
	}
	return len(problems)
}
