package main

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/justinstimatze/onsetter/internal/discover"
)

// cmdServe runs onsetter as a persistent stdio MCP server exposing one tool,
// hook, with the same input shape cmdHook already parses from stdin JSON —
// see cmd_hook.go's dispatch for why: this is the fix for the process-spawn
// floor a fresh CLI invocation pays on every single Write/Edit/Bash/Read,
// which no on-disk cache can touch, because a fresh process has to exist
// before it can open a cache file. Claude Code connects to this once per
// session (a plugin's mcp_tool hook, wired in hooks.json) and keeps it warm
// for the session's lifetime; the asks a governing CLAUDE.md tree holds get
// parsed once per file, not once per call.
func cmdServe() error {
	s := server.NewMCPServer("onsetter", buildVersion(), server.WithRecovery())
	warm := discover.NewWarm()
	s.AddTool(hookTool(), hookToolHandler(warm))
	return server.ServeStdio(s)
}

// hookTool declares the same fields payload already parses from stdin JSON —
// the MCP transport substitutes them from the hook's own call context
// (hooks/hooks.json's input map), so the tool's schema exists to describe
// that shape to any MCP client inspecting it, not to add a second format.
func hookTool() mcp.Tool {
	return mcp.NewTool("hook",
		mcp.WithDescription("onsetter's PreToolUse dispatcher. Injects prose from a CLAUDE.md at the Write, Edit, Bash, or Read call it's about."),
		mcp.WithString("session_id"),
		mcp.WithString("tool_name"),
		mcp.WithString("cwd"),
		mcp.WithString("file_path"),
		mcp.WithString("content"),
		mcp.WithString("new_string"),
		mcp.WithString("old_string"),
		mcp.WithString("command"),
	)
}

// hookToolHandler closes over one Warm cache, shared across every call this
// server ever handles for its whole lifetime — the actual fix this
// subcommand exists to ship. WithRecovery (passed to NewMCPServer above)
// already keeps a handler panic from taking the process down; this handler
// itself stays a thin adapter, translating MCP call args into the same
// payload shape cmdHook builds from stdin and the resulting *out back into
// a tool result.
func hookToolHandler(warm *discover.Warm) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var p payload
		p.SessionID = req.GetString("session_id", "")
		p.ToolName = req.GetString("tool_name", "")
		p.Cwd = req.GetString("cwd", "")
		p.ToolInput.FilePath = req.GetString("file_path", "")
		p.ToolInput.Content = req.GetString("content", "")
		p.ToolInput.NewString = req.GetString("new_string", "")
		p.ToolInput.OldString = req.GetString("old_string", "")
		p.ToolInput.Command = req.GetString("command", "")

		o, err := dispatch(p, warm.Asks)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if o == nil {
			// Nothing fired. cmdHook's equivalent is emitting no stdout at
			// all — the empty string is this transport's version of that,
			// read by Claude Code the same way an empty stdout is: no
			// additionalContext, no permissionDecision, nothing to act on.
			return mcp.NewToolResultText(""), nil
		}
		b, err := json.Marshal(o)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(b)), nil
	}
}
