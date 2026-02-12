package extensions

import (
	"path/filepath"
	"testing"
)

func TestLoadMCPConfigMissingFileReturnsEmpty(t *testing.T) {
	workspace := t.TempDir()
	path := MCPConfigPath(workspace)

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected cfg, got nil")
	}
	if cfg.Version != MCPConfigVersion {
		t.Fatalf("Version=%d, want %d", cfg.Version, MCPConfigVersion)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("Servers=%v, want empty", cfg.Servers)
	}

	// Ensure it points under workspace.
	if got, want := filepath.Dir(path), filepath.Join(workspace, ".goclaw"); got != want {
		t.Fatalf("dir=%s, want %s", got, want)
	}
}

func TestSaveMCPConfigRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	path := MCPConfigPath(workspace)

	in := &MCPConfig{
		Version: MCPConfigVersion,
		Servers: map[string]MCPServer{
			"time": {
				Enabled:        true,
				Type:           "stdio",
				Command:        "uvx",
				Args:           []string{"mcp-server-time"},
				TimeoutSeconds: 10,
			},
			"disabled": {
				Enabled: false,
				Type:    "http",
				URL:     "https://example.invalid/mcp",
			},
		},
	}

	if err := SaveMCPConfig(path, in); err != nil {
		t.Fatalf("SaveMCPConfig: %v", err)
	}

	out, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if out == nil {
		t.Fatal("expected cfg, got nil")
	}
	if out.Version != MCPConfigVersion {
		t.Fatalf("Version=%d, want %d", out.Version, MCPConfigVersion)
	}
	if len(out.Servers) != 2 {
		t.Fatalf("Servers size=%d, want 2", len(out.Servers))
	}

	if got := out.Servers["time"]; got.Command != "uvx" || got.Type != "stdio" || !got.Enabled {
		t.Fatalf("time server mismatch: %+v", got)
	}
	if got := out.Servers["disabled"]; got.Enabled {
		t.Fatalf("expected disabled server to remain disabled, got %+v", got)
	}
}

