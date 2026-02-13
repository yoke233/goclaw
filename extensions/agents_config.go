package extensions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// AgentsConfig is the on-disk configuration stored in .agents/config.toml.
//
// This is intentionally minimal and designed for safe CRUD from LLM tools.
// Over time we can extend it with additional agent-related configuration.
type AgentsConfig struct {
	MCPServers map[string]MCPServerConfig `toml:"mcp_servers"`
}

// MCPServerConfig describes how to reach an MCP server (TOML form).
//
// Note: many fields use pointers so we can distinguish "unset" vs "set to zero",
// enabling clean base+overlay merges across layers.
type MCPServerConfig struct {
	Enabled           *bool             `toml:"enabled,omitempty"`
	Type              string            `toml:"type,omitempty"` // stdio/http/sse (optional when inferrable)
	Command           string            `toml:"command,omitempty"`
	Args              []string          `toml:"args,omitempty"`
	URL               string            `toml:"url,omitempty"`
	Env               map[string]string `toml:"env,omitempty"`
	HTTPHeaders       map[string]string `toml:"http_headers,omitempty"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var,omitempty"`
	EnabledTools      []string          `toml:"enabled_tools,omitempty"`
	DisabledTools     []string          `toml:"disabled_tools,omitempty"`
	StartupTimeoutSec *int              `toml:"startup_timeout_sec,omitempty"`
	ToolTimeoutSec    *int              `toml:"tool_timeout_sec,omitempty"`
}

func (c *AgentsConfig) normalize() *AgentsConfig {
	if c == nil {
		return &AgentsConfig{MCPServers: map[string]MCPServerConfig{}}
	}
	if c.MCPServers == nil {
		c.MCPServers = map[string]MCPServerConfig{}
	}
	return c
}

// LoadAgentsConfig loads .agents/config.toml from disk. Missing files return an empty config.
func LoadAgentsConfig(path string) (*AgentsConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("agents config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return (&AgentsConfig{}).normalize(), nil
		}
		return nil, fmt.Errorf("read agents config: %w", err)
	}
	if len(data) == 0 {
		return (&AgentsConfig{}).normalize(), nil
	}

	var cfg AgentsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode agents config toml: %w", err)
	}
	return (&cfg).normalize(), nil
}

// SaveAgentsConfig writes .agents/config.toml to disk, creating parent directories as needed.
func SaveAgentsConfig(path string, cfg *AgentsConfig) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("agents config path is empty")
	}
	if cfg == nil {
		return fmt.Errorf("agents config is nil")
	}
	cfg = cfg.normalize()

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir agents config dir: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode agents config toml: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write agents config tmp: %w", err)
	}

	// Best-effort atomic replace (works on Windows as long as the target isn't locked).
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename agents config tmp: %w", err)
	}
	return nil
}

// MergeAgentsConfig merges a lower-priority config with a higher-priority config.
// Unset fields in the higher config do not override lower values.
func MergeAgentsConfig(lower, higher *AgentsConfig) *AgentsConfig {
	if lower == nil && higher == nil {
		return (&AgentsConfig{}).normalize()
	}
	if lower == nil {
		return (&AgentsConfig{MCPServers: cloneMCPServers(higher.MCPServers)}).normalize()
	}
	if higher == nil {
		return (&AgentsConfig{MCPServers: cloneMCPServers(lower.MCPServers)}).normalize()
	}

	out := &AgentsConfig{
		MCPServers: cloneMCPServers(lower.MCPServers),
	}
	if out.MCPServers == nil {
		out.MCPServers = map[string]MCPServerConfig{}
	}

	for name, hi := range higher.MCPServers {
		lo, exists := out.MCPServers[name]
		if !exists {
			out.MCPServers[name] = cloneMCPServerConfig(hi)
			continue
		}
		out.MCPServers[name] = mergeMCPServerConfig(lo, hi)
	}

	return out.normalize()
}

func cloneMCPServers(in map[string]MCPServerConfig) map[string]MCPServerConfig {
	if len(in) == 0 {
		return map[string]MCPServerConfig{}
	}
	out := make(map[string]MCPServerConfig, len(in))
	for k, v := range in {
		out[k] = cloneMCPServerConfig(v)
	}
	return out
}

func cloneMCPServerConfig(in MCPServerConfig) MCPServerConfig {
	out := in
	out.Args = append([]string(nil), in.Args...)
	out.EnabledTools = append([]string(nil), in.EnabledTools...)
	out.DisabledTools = append([]string(nil), in.DisabledTools...)
	if len(in.Env) > 0 {
		out.Env = make(map[string]string, len(in.Env))
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
	if len(in.HTTPHeaders) > 0 {
		out.HTTPHeaders = make(map[string]string, len(in.HTTPHeaders))
		for k, v := range in.HTTPHeaders {
			out.HTTPHeaders[k] = v
		}
	}
	if in.StartupTimeoutSec != nil {
		v := *in.StartupTimeoutSec
		out.StartupTimeoutSec = &v
	}
	if in.ToolTimeoutSec != nil {
		v := *in.ToolTimeoutSec
		out.ToolTimeoutSec = &v
	}
	if in.Enabled != nil {
		v := *in.Enabled
		out.Enabled = &v
	}
	return out
}

func mergeMCPServerConfig(lower, higher MCPServerConfig) MCPServerConfig {
	out := cloneMCPServerConfig(lower)

	if higher.Enabled != nil {
		v := *higher.Enabled
		out.Enabled = &v
	}
	if strings.TrimSpace(higher.Type) != "" {
		out.Type = strings.TrimSpace(higher.Type)
	}
	if strings.TrimSpace(higher.Command) != "" {
		out.Command = strings.TrimSpace(higher.Command)
	}
	if higher.Args != nil {
		out.Args = append([]string(nil), higher.Args...)
	}
	if strings.TrimSpace(higher.URL) != "" {
		out.URL = strings.TrimSpace(higher.URL)
	}
	if higher.Env != nil {
		if out.Env == nil {
			out.Env = map[string]string{}
		}
		for k, v := range higher.Env {
			out.Env[k] = v
		}
	}
	if higher.HTTPHeaders != nil {
		if out.HTTPHeaders == nil {
			out.HTTPHeaders = map[string]string{}
		}
		for k, v := range higher.HTTPHeaders {
			out.HTTPHeaders[k] = v
		}
	}
	if strings.TrimSpace(higher.BearerTokenEnvVar) != "" {
		out.BearerTokenEnvVar = strings.TrimSpace(higher.BearerTokenEnvVar)
	}
	if higher.EnabledTools != nil {
		out.EnabledTools = append([]string(nil), higher.EnabledTools...)
	}
	if higher.DisabledTools != nil {
		out.DisabledTools = append([]string(nil), higher.DisabledTools...)
	}
	if higher.StartupTimeoutSec != nil {
		v := *higher.StartupTimeoutSec
		out.StartupTimeoutSec = &v
	}
	if higher.ToolTimeoutSec != nil {
		v := *higher.ToolTimeoutSec
		out.ToolTimeoutSec = &v
	}

	return out
}
