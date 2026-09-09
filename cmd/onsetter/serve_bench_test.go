package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// buildBenchBinary is buildBinary for *testing.B — a benchmark can't share
// hook_test.go's *testing.T-typed helper, and the duplication here is small
// enough not to be worth a shared interface for one build step.
func buildBenchBinary(b *testing.B) string {
	b.Helper()
	bin := filepath.Join(b.TempDir(), "onsetter")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// buildBigRepo writes a synthetic n-ask CLAUDE.md, the same shape used to
// find the process-spawn-floor result documented in the plan: 150 asks each
// with a when:/not: regex pair is realistic for a large monorepo's
// accumulated CLAUDE.md, and big enough that a genuine parse-cost difference
// would show up if one existed.
func buildBigRepo(b *testing.B, n int) (repo, target string) {
	b.Helper()
	repo = b.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		b.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		sb.WriteString("```ask\nin: src/**/*.go\nwhen: pattern-" + s + "-(alpha|beta|gamma)[0-9]+\nnot: skip-" + s + "\n\n")
		sb.WriteString("This is ask number " + s + ", a moderately real-looking regex gate with prose.\n```\n\n")
	}
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	target = filepath.Join(repo, "src", "a.go")
	if err := os.WriteFile(target, []byte("package a\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	return repo, target
}

// The baseline this phase exists to beat: today's transport, one fresh
// process per call.
func BenchmarkHookSubprocessPerCall(b *testing.B) {
	bin := buildBenchBinary(b)
	_, target := buildBigRepo(b, 150)
	cache := b.TempDir()
	in, err := json.Marshal(map[string]any{
		"session_id": "bench", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": target, "content": "package a\n"},
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin, "hook")
		cmd.Stdin = strings.NewReader(string(in))
		cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
		if _, err := cmd.Output(); err != nil {
			b.Fatalf("hook: %v", err)
		}
	}
}

// The fix: one persistent server, N calls over the same live connection.
func BenchmarkServeOverOneConnection(b *testing.B) {
	bin := buildBenchBinary(b)
	_, target := buildBigRepo(b, 150)
	cache := b.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cache)

	c, err := client.NewStdioMCPClient(bin, env, "serve")
	if err != nil {
		b.Fatalf("NewStdioMCPClient: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "onsetter-bench", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		b.Fatalf("Initialize: %v", err)
	}

	var req mcp.CallToolRequest
	req.Params.Name = "hook"
	req.Params.Arguments = map[string]any{
		"session_id": "bench", "tool_name": "Write",
		"file_path": target, "content": "package a\n",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.CallTool(ctx, req); err != nil {
			b.Fatalf("CallTool: %v", err)
		}
	}
}
