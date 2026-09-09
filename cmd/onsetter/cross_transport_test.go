package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/session"
)

// A double-wired session — a manual command install alongside a plugin
// mcp_tool install, both live at once — is the real-world shape the plan's
// own design points 4 and 6 name directly. internal/session.Store's own
// tests (internal/session/session_test.go's TestConcurrentStoresDoNotLoseIncrements)
// already prove N racing Stores never lose an increment; what they don't
// prove is that both real transports actually funnel into that same store
// correctly — a CLI hook subprocess and a serve MCP tool call, firing on the
// identical matched occurrence for the identical session id, concurrently.
// This is that proof, at the level where a real regression would show up.
func TestCrossTransportFiringsBothCount(t *testing.T) {
	bin := buildBinary(t)
	repo := t.TempDir()
	cache := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	claudeMD := filepath.Join(repo, "CLAUDE.md")
	write(t, claudeMD, "```ask\nwhen: alpha\n\nAsk.\n```\n")
	target := filepath.Join(repo, "a.md")
	write(t, target, "x")

	asks, err := ask.ParseFile(claudeMD)
	if err != nil || len(asks) != 1 {
		t.Fatalf("fixture parse: %d ask(s), %v", len(asks), err)
	}
	sessionID := "cross-transport-race"
	key := session.Key(asks[0].ID(), "alpha", "alpha")

	env := append(os.Environ(), "XDG_CACHE_HOME="+cache, "GOCOVERDIR="+goCoverDir(t))

	var wg sync.WaitGroup
	wg.Add(2)

	// The CLI transport: a real onsetter hook subprocess.
	go func() {
		defer wg.Done()
		in, err := json.Marshal(map[string]any{
			"session_id": sessionID, "tool_name": "Write",
			"tool_input": map[string]any{"file_path": target, "content": "alpha"},
		})
		if err != nil {
			t.Error(err)
			return
		}
		cmd := exec.Command(bin, "hook")
		cmd.Stdin = strings.NewReader(string(in))
		cmd.Env = env
		if _, err := cmd.Output(); err != nil {
			t.Errorf("CLI transport: %v", err)
		}
	}()

	// The MCP transport: a real onsetter serve subprocess, one call.
	go func() {
		defer wg.Done()
		c, err := client.NewStdioMCPClient(bin, env, "serve")
		if err != nil {
			t.Errorf("NewStdioMCPClient: %v", err)
			return
		}
		defer c.Close()

		ctx := context.Background()
		var initReq mcp.InitializeRequest
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "onsetter-test", Version: "1.0.0"}
		if _, err := c.Initialize(ctx, initReq); err != nil {
			t.Errorf("Initialize: %v", err)
			return
		}

		var req mcp.CallToolRequest
		req.Params.Name = "hook"
		req.Params.Arguments = map[string]any{
			"session_id": sessionID, "tool_name": "Write",
			"file_path": target, "content": "alpha",
		}
		if _, err := c.CallTool(ctx, req); err != nil {
			t.Errorf("MCP transport: %v", err)
		}
	}()

	wg.Wait()

	t.Setenv("XDG_CACHE_HOME", cache)
	if got := session.Open(sessionID).Count(key); got != 2 {
		t.Errorf("Count = %d after one CLI firing and one MCP firing on the same occurrence, want 2 — an increment was lost across transports", got)
	}
}
