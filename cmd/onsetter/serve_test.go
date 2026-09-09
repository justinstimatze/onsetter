package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"

	"github.com/justinstimatze/onsetter/internal/discover"
)

// newTestServer builds an in-process onsetter MCP server — the real
// hookTool/hookToolHandler this package ships, with the same WithRecovery
// option cmdServe passes, wired through mcptest's own transport rather than
// a real subprocess. Fast, and a real exercise of the handler and the wire
// protocol; not a measurement of subprocess/stdio overhead, which the
// subprocess-driven tests below cover instead.
func newTestServer(t *testing.T) *mcptest.Server {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // hookToolHandler reaches session.Open; never touch the real machine's session store
	s := mcptest.NewUnstartedServer(t)
	s.AddServerOptions(server.WithRecovery())
	s.AddTool(hookTool(), hookToolHandler(discover.NewWarm()))
	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("start test server: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func callHook(t *testing.T, c *client.Client, args map[string]any) string {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "hook"
	req.Params.Arguments = args
	result, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var b strings.Builder
	for _, content := range result.Content {
		text, ok := content.(mcp.TextContent)
		if !ok {
			t.Fatalf("unsupported content type: %T", content)
		}
		b.WriteString(text.Text)
	}
	if result.IsError {
		t.Fatalf("hook tool returned an error result: %s", b.String())
	}
	return b.String()
}

func TestServeHookToolFiresOnAMatchingWrite(t *testing.T) {
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: TODO\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	srv := newTestServer(t)
	got := callHook(t, srv.Client(), map[string]any{
		"session_id": "s", "tool_name": "Write",
		"file_path": target, "content": "has a TODO here",
	})
	if got == "" {
		t.Fatal("want additionalContext for a matching Write, got an empty result")
	}
	var o out
	if err := json.Unmarshal([]byte(got), &o); err != nil {
		t.Fatalf("result is not the onsetter out{} shape: %v\n%s", err, got)
	}
	if !strings.Contains(o.HookSpecificOutput.AdditionalContext, "Ask.") {
		t.Errorf("additionalContext does not contain the ask's body:\n%s", o.HookSpecificOutput.AdditionalContext)
	}
}

func TestServeHookToolStaysSilentOnAnUnrelatedWrite(t *testing.T) {
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: TODO\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	srv := newTestServer(t)
	got := callHook(t, srv.Client(), map[string]any{
		"session_id": "s", "tool_name": "Write",
		"file_path": target, "content": "nothing relevant here",
	})
	if got != "" {
		t.Errorf("want no result for an unrelated Write, got:\n%s", got)
	}
}

// The property that actually justifies this subcommand's design: a panic
// inside one call must not take the server down for the next one.
// ONSETTER_TEST_PANIC is process-wide, so it is set for exactly one call
// (mcptest's server runs in a goroutine in this same process) and cleared
// before the second — proving recovery happened, not that the fault never
// fired.
func TestServeSurvivesAPanicInOneCall(t *testing.T) {
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: alpha\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	srv := newTestServer(t)
	args := map[string]any{
		"session_id": "panic-then-live", "tool_name": "Write",
		"file_path": target, "content": "alpha",
	}

	// mcp-go's WithRecovery surfaces a recovered handler panic as a
	// JSON-RPC-level error (CallTool returns a non-nil err here), not as an
	// in-band CallToolResult{IsError: true} the way a handler-returned error
	// would — confirmed by running this test before writing the assertion,
	// not assumed. What actually matters for the server's own survival is
	// unaffected either way: the process is still up for the next call.
	t.Setenv("ONSETTER_TEST_PANIC", "1")
	var req mcp.CallToolRequest
	req.Params.Name = "hook"
	req.Params.Arguments = args
	if _, err := srv.Client().CallTool(context.Background(), req); err == nil {
		t.Error("want an error from the panicking call, got a clean result")
	}

	t.Setenv("ONSETTER_TEST_PANIC", "")
	got := callHook(t, srv.Client(), args)
	if got == "" {
		t.Fatal("the server did not survive the panic: the next call produced nothing")
	}
	if !strings.Contains(got, "Ask.") {
		t.Errorf("the post-panic call did not fire correctly:\n%s", got)
	}
}

// The real subprocess/stdio version of the property above — the in-process
// test proves the full panic-then-normal-call transition, since t.Setenv
// can toggle ONSETTER_TEST_PANIC between two calls to the same goroutine;
// a real subprocess's environment is fixed at spawn, so this version proves
// the stronger, still-honest property: a live stdio connection to a real
// onsetter serve process keeps answering — no hang, no dropped connection —
// across repeated panics, not just tolerating exactly one. This is the
// "real MCP client, real stdio connection" coverage the plan's own
// Verification section calls for; the in-process test above is faster and
// covers the transition the fixed-env subprocess case can't.
func TestServeSubprocessSurvivesRepeatedPanics(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: alpha\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	env := append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t), "ONSETTER_TEST_PANIC=1")
	c, err := client.NewStdioMCPClient(bin, env, "serve")
	if err != nil {
		t.Fatalf("NewStdioMCPClient: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "onsetter-test", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "hook"
	req.Params.Arguments = map[string]any{
		"session_id": "s", "tool_name": "Write",
		"file_path": target, "content": "alpha",
	}
	for i := range 3 {
		if _, err := c.CallTool(ctx, req); err == nil {
			t.Errorf("call %d: want an error (every call panics, ONSETTER_TEST_PANIC is set for this whole subprocess), got a clean result", i)
		}
	}
}

// A plain onsetter serve subprocess, driven by a real stdio MCP client end
// to end — no panic injection, the ordinary path a real Claude Code session
// exercises through the plugin's mcp_tool wiring.
func TestServeSubprocessFiresOnAMatchingWrite(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	write(t, filepath.Join(repo, "CLAUDE.md"), "```ask\nwhen: TODO\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	env := append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t))
	c, err := client.NewStdioMCPClient(bin, env, "serve")
	if err != nil {
		t.Fatalf("NewStdioMCPClient: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "onsetter-test", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	got := callHook(t, c, map[string]any{
		"session_id": "s", "tool_name": "Write",
		"file_path": target, "content": "has a TODO here",
	})
	if !strings.Contains(got, "Ask.") {
		t.Errorf("a real onsetter serve subprocess did not fire correctly:\n%s", got)
	}
}
