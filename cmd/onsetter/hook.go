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
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`    // Write
		NewString string `json:"new_string"` // Edit
		OldString string `json:"old_string"` // Edit
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

	store := session.Open(p.SessionID)
	ev := ask.Edit{
		Path: path, New: content, Old: p.ToolInput.OldString,
		Touched: store.Touched(), Exists: exists,
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
	// they all score against the same edit content.
	for _, r := range asks {
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

	var hits []ask.CascadeHit
	var ids []string
	for _, h := range combined {
		key := session.Key(h.Ask.ID(), h.Matched)
		if h.Ask.Revisit && h.Via == nil {
			key = session.KeyRevisit(h.Ask.ID(), h.Matched, content)
		}
		if store.Fired(key) {
			continue
		}
		hits = append(hits, h)
		ids = append(ids, key)
	}
	// After matching, so an edit never counts as having already satisfied an
	// `untouched:` gate about itself.
	store.Touch(path)
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
	fmt.Fprintf(&b, "onsetter — %d %s for this edit. Each question is asked once per session.\n", len(hits), noun)
	for _, h := range hits {
		gate := "no content gate"
		switch {
		case h.Matched != "":
			gate = fmt.Sprintf("matched %q", h.Matched)
		case h.Via != nil:
			gate = fmt.Sprintf("cued by %s", h.Via.Where(base))
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
