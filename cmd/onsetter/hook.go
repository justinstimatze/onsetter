package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/discover"
	"github.com/justinstimatze/onsetter/internal/embed"
	"github.com/justinstimatze/onsetter/internal/session"
)

// payload is the PreToolUse call as Claude Code presents it on stdin.
type payload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`    // Write
		NewString string `json:"new_string"` // Edit
		OldString string `json:"old_string"` // Edit
		Command   string `json:"command"`    // Bash
	} `json:"tool_input"`
}

// out is the only response shape onsetter ever emits. PreToolUse is the one
// event that accepts additionalContext. A sibling project shipped a PreCompact
// hook using this field, where it is not supported, and the block silently
// failed schema validation on the first real compaction. Emitting from exactly
// one place means that class of mistake has one place to be wrong.
type out struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// hit is one ask reached for this edit, plus how many times its occurrence
// key has already fired this session — 0 for a first firing, a reminder, or
// an always: ask, which never gets a marker at all.
type hit struct {
	ask.CascadeHit
	priorCount int
}

// cmdHook is the dispatcher. Every failure path here exits 0 with no output:
// one hook standing in front of every Write and Edit is a single point of
// failure, and the only acceptable failure is the silent, harmless kind.
func cmdHook() error {
	defer func() {
		if r := recover(); r != nil {
			os.Exit(0)
		}
	}()

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}

	if p.ToolName == "Bash" {
		recordBashTouches(p)
		return nil
	}

	isRead := p.ToolName == "Read"
	content := p.ToolInput.Content
	if content == "" {
		content = p.ToolInput.NewString
	}
	path := p.ToolInput.FilePath
	if path == "" {
		return nil
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil

	asks, _ := discover.Asks(path) // parse errors are lint's job, not the edit's
	if len(asks) == 0 {
		return nil
	}

	// Test-only fault injection for the recover() above (hook_test.go's
	// TestHookRecoversFromARealPanic). Every other test that reaches this
	// point exercises the real dispatch fully but never panics, which proved
	// nothing about recover() itself — this env var doesn't exist in any
	// documented interface and a real session will never set it; it's read
	// once, after real parsing and discovery already ran, so the test proves
	// recover() catches a fault mid-dispatch, not just at the entry.
	if os.Getenv("ONSETTER_TEST_PANIC") != "" {
		panic("onsetter: deliberate test panic")
	}

	store := session.Open(p.SessionID)
	ev := ask.Edit{
		Path: path, New: content, Old: p.ToolInput.OldString,
		Touched: store.Touched(), Exists: exists, IsRead: isRead,
	}
	// The file itself is only read when some ask actually gates on it, so the
	// common path stays one stat and no read.
	for _, r := range asks {
		if len(r.Has) > 0 {
			b, err := os.ReadFile(path)
			if err == nil {
				ev.Disk = string(b)
			}
			break
		}
	}
	// Same principle for the one live embed call: only paid when some ask
	// actually has an evokes: list, and paid once regardless of how many do —
	// they all score against the same edit content. Skipped entirely on a
	// Read: content is always "" there, and lint already rejects on: read +
	// evokes: as a dead combination — this is defense in depth for an
	// unlinted CLAUDE.md, not the primary enforcement.
	for _, r := range asks {
		if isRead {
			break
		}
		if len(r.Evokes) > 0 {
			ev.Evokes = embed.BuildPredicate(content, embed.DefaultBudget)
			break
		}
	}

	// Every ask whose own gate matches seeds the cascade, regardless of
	// whether this exact firing has already been shown this session — that
	// filter runs once below, over the combined direct+cued list, so a
	// citer already asked-and-answered can still cue a target that hasn't
	// been. What Cascade walks is "did the gate match", not "is this new".
	var direct []ask.CascadeHit
	for _, r := range asks {
		res := r.Match(ev)
		if res.OK {
			direct = append(direct, ask.CascadeHit{Ask: r, Matched: res.Matched})
		}
	}
	combined := ask.Cascade(asks, direct)

	// A matched hit (Matched != "") always fires — it's a specific quote in
	// a specific edit, and a repeat of it is a genuinely new occurrence
	// worth a fresh look, not the same already-answered question. It's
	// marked with a running count instead. A reminder / has:-only / cued
	// hit (Matched == "") has nothing to distinguish a repeat from the
	// first firing — the literal same sentence — so it keeps the original
	// suppress-after-first behavior. always: skips tracking for either
	// bucket: for a reminder that's unchanged from before (bypass
	// suppression); for a matched ask it now only means "don't clutter this
	// one with a count," since firing every time is already the default.
	var hits []hit
	var ids []string
	for _, h := range combined {
		if h.Ask.Always {
			hits = append(hits, hit{h, 0})
			continue
		}
		if h.Matched == "" {
			key := session.Key(h.Ask.ID(), "", "")
			if store.Fired(key) {
				continue
			}
			hits = append(hits, hit{h, 0})
			ids = append(ids, key)
			continue
		}
		key := session.Key(h.Ask.ID(), h.Matched, content)
		hits = append(hits, hit{h, store.Count(key)})
		ids = append(ids, key)
	}
	// After matching, so an edit never counts as having already satisfied an
	// `untouched:` gate about itself. Never for a Read — reading a file is
	// not writing it, and marking it touched would corrupt untouched:'s
	// semantics for every other ask watching that path.
	if !isRead {
		store.Touch(path)
	}
	if len(hits) == 0 {
		return nil
	}
	store.Record(ids...)

	base, _ := os.Getwd()
	var b strings.Builder
	// Not "check": a check is the thing that blocks, and this never blocks.
	noun := "asks"
	if len(hits) == 1 {
		noun = "ask"
	}
	trigger := "edit"
	if isRead {
		trigger = "read"
	}
	fmt.Fprintf(&b, "onsetter — %d %s for this %s. A repeat is marked, not hidden.\n", len(hits), noun, trigger)
	for _, h := range hits {
		gate := "no content gate"
		switch {
		case h.Matched != "":
			gate = fmt.Sprintf("matched %q", h.Matched)
		case h.Via != nil:
			gate = fmt.Sprintf("cued by %s", h.Via.Where(base))
		}
		if h.priorCount > 0 {
			gate += fmt.Sprintf(" (asked %d× already this session)", h.priorCount)
		}
		if h.Ask.Always {
			gate += " · always"
		}
		fmt.Fprintf(&b, "\n▸ %s · %s\n%s\n", h.Ask.Where(base), gate, h.Ask.Body)
	}
	b.WriteString("\nTo retire one, delete its block from the file named above.")

	var o out
	o.HookSpecificOutput.HookEventName = "PreToolUse"
	o.HookSpecificOutput.AdditionalContext = b.String()
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(o)
	return nil
}
