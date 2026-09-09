package main

import (
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
