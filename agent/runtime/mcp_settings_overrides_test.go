package runtime

import (
	"path/filepath"
	"testing"

	"github.com/smallnest/goclaw/extensions"
)

func TestBuildSubagentSDKSettingsOverrides_UsesGoClawDirWhenRoleDirInvalid(t *testing.T) {
	goclawDir := t.TempDir()
	repoDir := filepath.Join(goclawDir, "repo")

	// Base layer (goclawdir) config.
	cfgPath := extensions.AgentsConfigPath(goclawDir)
	if err := extensions.SaveAgentsConfig(cfgPath, &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {
				Enabled:           boolPtr(true),
				Type:              "stdio",
				Command:           "uvx",
				Args:              []string{"mcp-server-time"},
				StartupTimeoutSec: intPtr(10),
			},
			"bad": {
				Enabled: boolPtr(true),
				Type:    "unknown",
			},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		GoClawDir: goclawDir,
		RoleDir:   filepath.Join(goclawDir, "missing-role-pack"),
		RepoDir:   repoDir,
	})
	if settings == nil {
		t.Fatalf("settings overrides is nil")
	}
	if settings.CleanupPeriodDays == nil || *settings.CleanupPeriodDays != 0 {
		t.Fatalf("CleanupPeriodDays=%v, want 0", settings.CleanupPeriodDays)
	}
	if settings.MCP == nil || len(settings.MCP.Servers) != 1 {
		t.Fatalf("mcp servers=%v, want 1 (warnings=%v)", settings.MCP, warnings)
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

func TestBuildSubagentSDKSettingsOverrides_RoleDirIsolatesGoClawDir(t *testing.T) {
	goclawDir := t.TempDir()
	roleDir := filepath.Join(t.TempDir(), "frontend-pack")

	// GoClawDir has "time" but should be ignored once RoleDir is valid.
	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(goclawDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {Enabled: boolPtr(true), Type: "stdio", Command: "uvx"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig goclaw: %v", err)
	}

	// RoleDir has "search" and is valid (contains .agents/config.toml).
	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(roleDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"search": {Enabled: boolPtr(true), Type: "http", URL: "http://localhost:9000/mcp"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig role: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		GoClawDir: goclawDir,
		RoleDir:   roleDir,
		RepoDir:   "",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if _, ok := settings.MCP.Servers["time"]; ok {
		t.Fatalf("goclaw server unexpectedly present when role dir is valid")
	}
	if _, ok := settings.MCP.Servers["search"]; !ok {
		t.Fatalf("role server missing")
	}
}

func TestBuildSubagentSDKSettingsOverrides_RepoDirOverlaysBase(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := t.TempDir()

	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(baseDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {Enabled: boolPtr(true), Type: "stdio", Command: "uvx"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig base: %v", err)
	}
	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(repoDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"search": {Enabled: boolPtr(true), Type: "http", URL: "http://localhost:9000/mcp"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig repo: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		GoClawDir: baseDir,
		RepoDir:   repoDir,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if _, ok := settings.MCP.Servers["time"]; !ok {
		t.Fatalf("base server missing")
	}
	if _, ok := settings.MCP.Servers["search"]; !ok {
		t.Fatalf("repo overlay server missing")
	}
}

func TestBuildSubagentSDKSettingsOverrides_RepoDirCanDisableBaseServer(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := t.TempDir()

	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(baseDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {Enabled: boolPtr(true), Type: "stdio", Command: "uvx"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig base: %v", err)
	}
	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(repoDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {Enabled: boolPtr(false)},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig repo: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		GoClawDir: baseDir,
		RepoDir:   repoDir,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil {
		t.Fatalf("settings overrides is nil")
	}
	if settings.MCP != nil {
		t.Fatalf("settings.MCP=%v, want nil (repo disabled base server)", settings.MCP)
	}
}

func TestBuildSubagentSDKSettingsOverrides_UsesExplicitConfigPath(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := t.TempDir()
	explicitDir := t.TempDir()

	// base has "time"; explicit has "search". Explicit path should win.
	if err := extensions.SaveAgentsConfig(extensions.AgentsConfigPath(baseDir), &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"time": {Enabled: boolPtr(true), Type: "stdio", Command: "uvx"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig base: %v", err)
	}

	explicitPath := filepath.Join(explicitDir, "config.toml")
	if err := extensions.SaveAgentsConfig(explicitPath, &extensions.AgentsConfig{
		MCPServers: map[string]extensions.MCPServerConfig{
			"search": {Enabled: boolPtr(true), Type: "http", URL: "http://localhost:9000/mcp"},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig explicit: %v", err)
	}

	settings, warnings := buildSubagentSDKSettingsOverrides(SubagentRunRequest{
		GoClawDir:     baseDir,
		RepoDir:       repoDir,
		MCPConfigPath: explicitPath,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if settings == nil || settings.MCP == nil {
		t.Fatalf("settings overrides is nil")
	}
	if _, ok := settings.MCP.Servers["time"]; ok {
		t.Fatalf("base server unexpectedly present when explicit config path is set")
	}
	if _, ok := settings.MCP.Servers["search"]; !ok {
		t.Fatalf("explicit server missing")
	}
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }
