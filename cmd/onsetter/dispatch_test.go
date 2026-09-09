package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/justinstimatze/onsetter/ask"
)

// dispatch's own subprocess-driven tests (hook_test.go) already exercise
// this function's full behavior through cmdHook, end to end. What only a
// direct in-process call can prove is that findAsks is a real seam serve
// actually needs — not a parameter dispatch silently ignores in favor of
// calling discover.Asks itself. Proven by injecting a fake ask an honest
// discover.Asks lookup could never produce (the target path has no
// governing CLAUDE.md on disk at all) and confirming dispatch fires on it.
func TestDispatchUsesInjectedFindAsks(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // dispatch reaches session.Open; never touch the real machine's session store
	dir := t.TempDir()
	target := dir + "/a.md"
	fake := &ask.Ask{Dir: dir, In: "**", On: ask.ModeAny, Body: "Injected, not discovered."}
	findAsks := func(string) ([]*ask.Ask, []error) { return []*ask.Ask{fake}, nil }

	p := payload{SessionID: "s", ToolName: "Write"}
	p.ToolInput.FilePath = target
	p.ToolInput.Content = "anything"

	o, err := dispatch(p, findAsks)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if o == nil {
		t.Fatal("want a result: the injected findAsks returned a reminder-shaped ask that should fire")
	}
	if !strings.Contains(o.HookSpecificOutput.AdditionalContext, "Injected, not discovered.") {
		t.Errorf("result did not come from the injected ask:\n%s", o.HookSpecificOutput.AdditionalContext)
	}
}

// A Bash call is recorded and returns before findAsks is ever consulted —
// asserted here by a findAsks that fails the test outright if called, not
// just by checking the returned result.
func TestDispatchBashCallNeverCallsFindAsks(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // recordBashTouches reaches session.Open too
	called := false
	findAsks := func(string) ([]*ask.Ask, []error) {
		called = true
		return nil, nil
	}

	p := payload{SessionID: "s", ToolName: "Bash"}
	p.ToolInput.Command = "echo hi"

	if _, err := dispatch(p, findAsks); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if called {
		t.Error("dispatch called findAsks for a Bash call, which should short-circuit before discovery")
	}
}

func TestDispatchEmptyFilePathReturnsNothing(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // dispatch returns before session.Open here, but isolate defensively anyway
	findAsks := func(string) ([]*ask.Ask, []error) {
		t.Fatal("findAsks should never be called when there is no path to look up")
		return nil, nil
	}
	p := payload{SessionID: "s", ToolName: "Write"}

	o, err := dispatch(p, findAsks)
	if err != nil || o != nil {
		t.Errorf("want (nil, nil) for an empty file_path, got (%v, %v)", o, err)
	}
}

// Closes the gap IDEAS.md named directly: a block: true ask's matched
// Write is denied in the same call that tripped it, not merely informed
// about in additionalContext for some later call. Constructed directly, the
// same way TestDispatchUsesInjectedFindAsks is, since Block only needs to be
// true here — it doesn't need to have come through parseBlock's own
// added:/removed: check to exercise dispatch's own behavior.
func TestDispatchDeniesOnAMatchedBlockAsk(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	target := dir + "/a.go"
	blocking := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		Added: []*regexp.Regexp{regexp.MustCompile(`\bprint\(`)},
		Block: true, Body: "No print( in this repo.",
	}
	findAsks := func(string) ([]*ask.Ask, []error) { return []*ask.Ask{blocking}, nil }

	p := payload{SessionID: "s", ToolName: "Write"}
	p.ToolInput.FilePath = target
	p.ToolInput.Content = "print(\"debug\")\n"

	o, err := dispatch(p, findAsks)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if o == nil {
		t.Fatal("want a result: the blocking ask's added: gate matched")
	}
	if o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("want permissionDecision %q, got %q", "deny", o.HookSpecificOutput.PermissionDecision)
	}
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "No print( in this repo.") {
		t.Errorf("permissionDecisionReason does not quote the blocking ask's own prose:\n%s", o.HookSpecificOutput.PermissionDecisionReason)
	}
	if !strings.Contains(o.HookSpecificOutput.AdditionalContext, "· blocks") {
		t.Errorf("additionalContext does not mark the blocking hit:\n%s", o.HookSpecificOutput.AdditionalContext)
	}
}

// The same ask, an edit its own added: gate does not match: no denial, no
// permissionDecision at all — a block: true ask is exactly as narrow as its
// own gate, never a blanket deny for the file.
func TestDispatchDoesNotDenyOnAnUnrelatedWrite(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	target := dir + "/a.go"
	blocking := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		Added: []*regexp.Regexp{regexp.MustCompile(`\bprint\(`)},
		Block: true, Body: "No print( in this repo.",
	}
	findAsks := func(string) ([]*ask.Ask, []error) { return []*ask.Ask{blocking}, nil }

	p := payload{SessionID: "s", ToolName: "Write"}
	p.ToolInput.FilePath = target
	p.ToolInput.Content = "package a\n"

	o, err := dispatch(p, findAsks)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if o != nil {
		t.Errorf("want no result for an edit the blocking ask's added: gate did not match, got: %+v", o)
	}
}

// A block: true ask reached only through cues: never denies — Cascade never
// checks a cued ask's own gate (CascadeHit.Matched stays empty), so there is
// no quoted text in hand to justify a denial with. This is the scoping
// headers.md documents for `block:`, proven here rather than left as prose.
func TestDispatchNeverDeniesOnACuedBlockAsk(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	target := dir + "/a.go"
	citer := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		When: []*regexp.Regexp{regexp.MustCompile(`fetch\(`)},
		Cues: []string{"b"}, Body: "Citer prose.",
	}
	cued := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		Added: []*regexp.Regexp{regexp.MustCompile(`token`)},
		Block: true, Name: "b", Body: "Cued blocking prose.",
	}
	findAsks := func(string) ([]*ask.Ask, []error) { return []*ask.Ask{citer, cued}, nil }

	p := payload{SessionID: "s", ToolName: "Write"}
	p.ToolInput.FilePath = target
	p.ToolInput.Content = "fetch(url)\n" // trips citer's when:, never mentions "token"

	o, err := dispatch(p, findAsks)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if o == nil {
		t.Fatal("want a result: the citer's own when: gate matched")
	}
	if o.HookSpecificOutput.PermissionDecision != "" {
		t.Errorf("a cued block: true ask denied the edit even though its own gate never ran: %q", o.HookSpecificOutput.PermissionDecision)
	}
	if !strings.Contains(o.HookSpecificOutput.AdditionalContext, "Cued blocking prose.") {
		t.Errorf("additionalContext does not include the cued ask's prose:\n%s", o.HookSpecificOutput.AdditionalContext)
	}
}

// Multiple blocking asks matching the same edit join into one reason —
// nothing dropped, matching the batch design the plan calls for.
func TestDispatchJoinsMultipleBlockingReasons(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	target := dir + "/a.go"
	a := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		Added: []*regexp.Regexp{regexp.MustCompile(`\bprint\(`)},
		Block: true, Body: "No print(.",
	}
	b := &ask.Ask{
		Dir: dir, In: "**", On: ask.ModeAny,
		Added: []*regexp.Regexp{regexp.MustCompile(`fmt\.Println`)},
		Block: true, Body: "No fmt.Println.",
	}
	findAsks := func(string) ([]*ask.Ask, []error) { return []*ask.Ask{a, b}, nil }

	p := payload{SessionID: "s", ToolName: "Write"}
	p.ToolInput.FilePath = target
	p.ToolInput.Content = "print(\"x\")\nfmt.Println(\"y\")\n"

	o, err := dispatch(p, findAsks)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if o == nil {
		t.Fatal("want a result: both blocking asks' added: gates matched")
	}
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "No print(.") ||
		!strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "No fmt.Println.") {
		t.Errorf("permissionDecisionReason does not carry both blocking asks' prose:\n%s", o.HookSpecificOutput.PermissionDecisionReason)
	}
}
