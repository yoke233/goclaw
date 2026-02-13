package runtime

import (
	"path/filepath"
	"testing"

	"github.com/smallnest/goclaw/extensions"
)

func TestBuildSubagentSDKSettingsOverrides_UsesWorkspaceDir(t *testing.T) {
	workspace := t.TempDir()

	cfgPath := extensions.MCPConfigPath(workspace)
	if err := extensions.SaveMCPConfig(cfgPath, &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"time": {
				Enabled:        true,
				Type:           "stdio",
				Command:        "uvx",
				Args:           []string{"mcp-server-time"},
				TimeoutSeconds: 10,
			},
			"off": {
				Enabled: true,
				Type:    "unknown",
			},
			"disabled": {
				Enabled: true,
				Type:    "stdio",
				Command: "",
			},
			"skip": {
				Enabled: false,
				Type:    "stdio",
				Command: "ignored",
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		WorkspaceDir: workspace,
		WorkDir:      filepath.Join(workspace, "subagent-workdir"),
	})
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if len(settings.MCP.Servers) != 1 {
		t.Fatalf("server count=%d, want 1 (warnings=%v)", len(settings.MCP.Servers), warnings)
	}
	server, ok := settings.MCP.Servers["time"]
	if !ok {
		t.Fatalf("server time missing (warnings=%v)", warnings)
	}
	if server.Type != "stdio" {
		t.Fatalf("server type=%q, want %q", server.Type, "stdio")
	}
	if server.Command != "uvx" {
		t.Fatalf("server command=%q, want %q", server.Command, "uvx")
	}
	if server.TimeoutSeconds != 10 {
		t.Fatalf("server timeout=%d, want %d", server.TimeoutSeconds, 10)
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings should include invalid server entries")
	}
}

func TestBuildSubagentSDKSettingsOverrides_WorkDirOverridesWorkspace(t *testing.T) {
	workspace := t.TempDir()
	workDir := filepath.Join(workspace, "subagent-workdir")

	// Workspace has "time".
	if err := extensions.SaveMCPConfig(extensions.MCPConfigPath(workspace), &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"time": {
				Enabled: true,
				Type:    "stdio",
				Command: "uvx",
				Args:    []string{"mcp-server-time"},
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig workspace: %v", err)
	}

	// WorkDir has "search". WorkDir config should win when it exists.
	if err := extensions.SaveMCPConfig(extensions.MCPConfigPath(workDir), &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"search": {
				Enabled: true,
				Type:    "http",
				URL:     "http://localhost:9000/mcp",
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig workdir: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		WorkspaceDir: workspace,
		WorkDir:      workDir,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if _, ok := settings.MCP.Servers["time"]; ok {
		t.Fatalf("workspace server unexpectedly present when workdir config exists")
	}
	if _, ok := settings.MCP.Servers["search"]; !ok {
		t.Fatalf("workdir server missing")
	}
}

func TestBuildSubagentSDKSettingsOverrides_WorkDirEmptyDisablesWorkspace(t *testing.T) {
	workspace := t.TempDir()
	workDir := filepath.Join(workspace, "subagent-workdir")

	// Workspace has "time".
	if err := extensions.SaveMCPConfig(extensions.MCPConfigPath(workspace), &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"time": {
				Enabled: true,
				Type:    "stdio",
				Command: "uvx",
				Args:    []string{"mcp-server-time"},
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig workspace: %v", err)
	}

	// WorkDir config file exists but disables everything.
	if err := extensions.SaveMCPConfig(extensions.MCPConfigPath(workDir), &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"time": {
				Enabled: false,
				Type:    "stdio",
				Command: "uvx",
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig workdir: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		WorkspaceDir: workspace,
		WorkDir:      workDir,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings != nil {
		t.Fatalf("settings=%v, want nil (workdir config should disable workspace MCP)", settings)
	}
}

func TestBuildSubagentSDKSettingsOverrides_UsesExplicitConfigPath(t *testing.T) {
	workspace := t.TempDir()
	alt := t.TempDir()

	// Workspace config has "time"; alt config has "search". Explicit path should win.
	if err := extensions.SaveMCPConfig(extensions.MCPConfigPath(workspace), &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"time": {
				Enabled: true,
				Type:    "stdio",
				Command: "uvx",
				Args:    []string{"mcp-server-time"},
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig workspace: %v", err)
	}

	altPath := filepath.Join(alt, "mcp.json")
	if err := extensions.SaveMCPConfig(altPath, &extensions.MCPConfig{
		Version: extensions.MCPConfigVersion,
		Servers: map[string]extensions.MCPServer{
			"search": {
				Enabled: true,
				Type:    "http",
				URL:     "http://localhost:9000/mcp",
			},
		},
	}); err != nil {
		t.Fatalf("SaveMCPConfig alt: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		WorkspaceDir:   workspace,
		MCPConfigPath:  altPath,
		WorkDir:        filepath.Join(workspace, "subagent-workdir"),
		TimeoutSeconds: 1,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if _, ok := settings.MCP.Servers["time"]; ok {
		t.Fatalf("workspace server unexpectedly present when explicit config path is set")
	}
	if _, ok := settings.MCP.Servers["search"]; !ok {
		t.Fatalf("explicit config server missing")
	}
}
