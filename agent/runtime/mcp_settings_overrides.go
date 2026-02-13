package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"

	sdkconfig "github.com/cexll/agentsdk-go/pkg/config"
	"github.com/smallnest/goclaw/extensions"
)

func buildSubagentSDKSettingsOverrides(req SubagentRunRequest) (*sdkconfig.Settings, []string) {
	// Precedence:
	// 1) Explicit override via request (MCPConfigPath)
	// 2) Per-subagent override under WorkDir (WorkDir/.goclaw/mcp.json) if the file exists
	// 3) Inherited workspace config (WorkspaceDir/.goclaw/mcp.json)
	// This keeps "shared by default" while allowing a self-contained subagent home directory.
	explicit := strings.TrimSpace(req.MCPConfigPath)
	if explicit != "" {
		return buildSubagentSDKSettingsOverridesFromPath(explicit)
	}

	workDir := strings.TrimSpace(req.WorkDir)
	if workDir != "" {
		localPath := extensions.MCPConfigPath(workDir)
		if fileExists(localPath) {
			return buildSubagentSDKSettingsOverridesFromPath(localPath)
		}
	}

	workspaceDir := strings.TrimSpace(req.WorkspaceDir)
	if workspaceDir == "" {
		// Best-effort fallback so tests/callers that don't pass WorkspaceDir still work.
		workspaceDir = strings.TrimSpace(req.WorkDir)
	}
	if workspaceDir == "" {
		return nil, []string{"workspace dir is empty; cannot resolve mcp config path"}
	}

	return buildSubagentSDKSettingsOverridesFromPath(extensions.MCPConfigPath(workspaceDir))
}

func buildSubagentSDKSettingsOverridesFromPath(cfgPath string) (*sdkconfig.Settings, []string) {
	cfg, err := extensions.LoadMCPConfig(cfgPath)
	if err != nil {
		return nil, []string{fmt.Sprintf("load mcp config %s: %v", cfgPath, err)}
	}
	if cfg == nil || len(cfg.Servers) == 0 {
		return nil, nil
	}

	enabled := make(map[string]sdkconfig.MCPServerConfig)
	var warnings []string

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		server := cfg.Servers[name]
		if !server.Enabled {
			continue
		}
		n := strings.TrimSpace(name)
		if n == "" {
			continue
		}

		typ := strings.ToLower(strings.TrimSpace(server.Type))
		switch typ {
		case "stdio":
			if strings.TrimSpace(server.Command) == "" {
				warnings = append(warnings, fmt.Sprintf("mcp server %s: command is required for stdio", n))
				continue
			}
		case "http", "sse":
			if strings.TrimSpace(server.URL) == "" {
				warnings = append(warnings, fmt.Sprintf("mcp server %s: url is required for %s", n, typ))
				continue
			}
		default:
			warnings = append(warnings, fmt.Sprintf("mcp server %s: unsupported type %q", n, server.Type))
			continue
		}

		enabled[n] = sdkconfig.MCPServerConfig{
			Type:           typ,
			Command:        strings.TrimSpace(server.Command),
			Args:           append([]string(nil), server.Args...),
			URL:            strings.TrimSpace(server.URL),
			Env:            cloneStringMap(server.Env),
			Headers:        cloneStringMap(server.Headers),
			TimeoutSeconds: server.TimeoutSeconds,
		}
	}

	if len(enabled) == 0 {
		return nil, warnings
	}

	return &sdkconfig.Settings{
		MCP: &sdkconfig.MCPConfig{
			Servers: enabled,
		},
	}, warnings
}

func fileExists(path string) bool {
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat != nil && !stat.IsDir()
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
