package extensions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MCPConfigVersion is the current on-disk schema version.
	MCPConfigVersion = 1
)

// MCPConfig is goclaw-owned MCP server configuration persisted per-workspace.
// It is intentionally simple and designed for safe CRUD from LLM tools.
type MCPConfig struct {
	Version int                  `json:"version"`
	Servers map[string]MCPServer `json:"servers,omitempty"`
}

// MCPServer describes a single MCP server entry.
// It largely mirrors agentsdk-go's MCPServerConfig but adds an Enabled flag.
type MCPServer struct {
	Enabled        bool              `json:"enabled"`
	Type           string            `json:"type"` // stdio/http/sse
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	URL            string            `json:"url,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

// MCPConfigPath returns the default MCP config path under a workspace root.
func MCPConfigPath(workspaceDir string) string {
	return filepath.Join(workspaceDir, ".goclaw", "mcp.json")
}

// LoadMCPConfig loads MCP config from disk. Missing files return an empty config.
func LoadMCPConfig(path string) (*MCPConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("mcp config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return (&MCPConfig{Version: MCPConfigVersion}).normalize(), nil
		}
		return nil, fmt.Errorf("read mcp config: %w", err)
	}
	if len(data) == 0 {
		return (&MCPConfig{Version: MCPConfigVersion}).normalize(), nil
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode mcp config: %w", err)
	}
	return (&cfg).normalize(), nil
}

// SaveMCPConfig writes MCP config to disk, creating parent directories as needed.
func SaveMCPConfig(path string, cfg *MCPConfig) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("mcp config path is empty")
	}
	if cfg == nil {
		return fmt.Errorf("mcp config is nil")
	}

	cfg = cfg.normalize()

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir mcp config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mcp config: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write mcp config tmp: %w", err)
	}

	// Best-effort atomic replace (works on Windows as long as the target isn't locked).
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename mcp config tmp: %w", err)
	}
	return nil
}

func (c *MCPConfig) normalize() *MCPConfig {
	if c == nil {
		return &MCPConfig{Version: MCPConfigVersion, Servers: map[string]MCPServer{}}
	}
	if c.Version <= 0 {
		c.Version = MCPConfigVersion
	}
	if c.Servers == nil {
		c.Servers = map[string]MCPServer{}
	}
	return c
}

