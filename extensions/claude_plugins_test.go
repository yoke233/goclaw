package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadClaudePlugins_Basic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	pluginRoot := filepath.Join(home, ".claude", "plugins", "demo-plugin")
	manifestDir := filepath.Join(pluginRoot, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}

	manifest := `{
  "name": "demo-plugin",
  "version": "0.1.0",
  "components": {
    "skills": "./skills",
    "hooks": "./hooks",
    "mcpServers": "./.mcp.json"
  }
}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	skillDir := filepath.Join(pluginRoot, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillBody := "---\nname: demo\ndescription: demo skill\n---\nhello"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	hooksDir := filepath.Join(pluginRoot, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	hooksJSON := `{
  "PreToolUse": [
    { "command": "echo pre", "matcher": "Bash" }
  ]
}`
	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(hooksJSON), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}

	mcpJSON := `{
  "demo": {
    "command": "python",
    "args": ["${CLAUDE_PLUGIN_ROOT}/server.py"]
  }
}`
	if err := os.WriteFile(filepath.Join(pluginRoot, ".mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	result := LoadClaudePlugins(workspace)
	if len(result.Skills) != 1 {
		t.Fatalf("skills=%d, want 1", len(result.Skills))
	}
	if result.Skills[0].Definition.Name != "demo" {
		t.Fatalf("skill name=%q, want %q", result.Skills[0].Definition.Name, "demo")
	}
	if len(result.Hooks) == 0 {
		t.Fatalf("hooks should not be empty")
	}
	env := result.Hooks[0].Env
	if env["CLAUDE_PLUGIN_ROOT"] != pluginRoot {
		t.Fatalf("hook env plugin root=%q, want %q", env["CLAUDE_PLUGIN_ROOT"], pluginRoot)
	}
	if env["CLAUDE_PROJECT_DIR"] != workspace {
		t.Fatalf("hook env project dir=%q, want %q", env["CLAUDE_PROJECT_DIR"], workspace)
	}
	if result.MCP == nil || len(result.MCP.MCPServers) != 1 {
		t.Fatalf("mcp servers=%v, want 1", result.MCP)
	}
	server, ok := result.MCP.MCPServers["demo"]
	if !ok {
		t.Fatalf("mcp server demo missing")
	}
	if len(server.Args) != 1 {
		t.Fatalf("mcp args=%v, want 1", server.Args)
	}
	arg := filepath.Clean(strings.ReplaceAll(server.Args[0], "/", string(filepath.Separator)))
	wantArg := filepath.Join(pluginRoot, "server.py")
	if arg != wantArg {
		t.Fatalf("mcp arg=%q, want %q", arg, wantArg)
	}
}
