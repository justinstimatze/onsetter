// Command onsetter delivers asks: short epistemic sniff checks that
// live in an ordinary CLAUDE.md and arrive at the tool call where the mistake
// would happen, instead of being loaded once and hopefully attended to.
//
// One hook is registered, ever: PreToolUse on Write|Edit. Adding an ask is
// writing prose in the file you were already writing prose in — nothing to
// install, nothing to wire, no second source of truth.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/justinstimatze/onsetter/ask"
)

// version is overridden at release via
// -ldflags "-X main.version=$(git describe --tags --always --dirty)".
// There is no version constant to hand-edit; the git tag is the only source.
var version = "dev"

func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 12 {
				rev = s.Value[:12]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return rev + dirty
	}
	return version
}

const usage = `onsetter — asks that arrive at the edit, not at session start.

  onsetter hook              PreToolUse dispatcher; reads the pending call on
                             stdin. This is the only thing settings.json runs.
  onsetter install           Wire the hook into ~/.claude/settings.local.json
                             and write the authoring skill. Re-running converges.
  onsetter list [path]       Asks governing a path, and whether each would
                             fire against the file as it stands.
  onsetter replay <glob>...  Fire rate of every ask against a corpus. Run this
                             before wiring an ask: every first draft over-fires.
  onsetter lint [dir]        Parse every ask under dir.
  onsetter status [dir]      Whether onsetter is actually working right now:
                             hook wired, asks parsing clean, evokes: cache
                             warm, requires: binaries present. Read-only.
  onsetter warm [dir]        Embed every evokes: phrase under dir into the
                             local cache. Run after adding or editing one —
                             hook never fills a cache miss itself.
  onsetter calib <ask> <fires-glob> <not-glob>
                             Measure one evokes: ask against labeled examples
                             you supply — the equivalent of replay for a gate
                             that has no ground truth of its own.
  onsetter headers           Full reference for writing one. Read this before
                             drafting an ask; it is the only complete list.
  onsetter --version

An ask is a fenced block in any CLAUDE.md:

    ` + "```" + `ask
    in: corpus/{locations,chars}/**
    when: you (nod|realize|decide|turn away)

    The narrator describes the world; the player decides what they do and what
    it means. Rewrite to observable world-state — unless the match is genuinely
    sensory, in which case continue.
    ` + "```" + `

Headers, one blank line, then the prose. Every header is optional, and they
apply in this order — which is the order ` + "`replay`" + ` reports a funnel in,
except the last three: ` + "`revisit`" + `, ` + "`name`" + ` and ` + "`cues`" + `
never gate, so none of the three ever appears there.

  requires   a binary resolving on $PATH — a fact about the machine, not the file
  in         glob, relative to this CLAUDE.md's directory  (default: all below)
  not-in     glob excluding a path that in would match
  on         any | mint | edit — mint means the file does not exist yet
  not        regex against the incoming text; suppresses
  has        regex against the file as it stands on disk
  untouched  glob no file written this session may match; suppresses
  added      regex against the lines this edit introduces
  removed    regex against the lines this edit deletes
  when       regex against the incoming text
  evokes     a fuzzy trigger phrase, not a regex — fires on any one, not all
  revisit    true — widens the session key to the whole edit, not just the quote
  name       a handle another ask's cues: can point at
  cues       fires a second, named ask in the same injection, gate unchecked

Repeat a regex header for an AND (` + "`not`" + ` is an OR of suppressors, and
` + "`evokes`" + ` — not a regex at all — is an OR too); RE2 has no lookahead, so
repeating is the only way to require two things. Use not-in, never not, to
exclude a path: not is a content regex, so ` + "`not: _test\\.go`" + ` matches
nothing and the ask fires on the tests anyway.

` + "`onsetter headers`" + ` explains each one, with the gotchas.
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch args[0] {
	case "hook":
		err = cmdHook()
	case "install":
		err = cmdInstall(args[1:])
	case "list":
		err = cmdList(args[1:])
	case "replay":
		err = cmdReplay(args[1:])
	case "lint":
		err = cmdLint(args[1:])
	case "status":
		err = cmdStatus(args[1:])
	case "warm":
		err = cmdWarm(args[1:])
	case "calib":
		err = cmdCalib(args[1:])
	case "headers":
		fmt.Print(ask.Reference())
	case "--version", "-v", "version":
		fmt.Println(buildVersion())
	case "--help", "-h", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "onsetter: unknown command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "onsetter: %v\n", err)
		os.Exit(1)
	}
}
