package agent

import (
	"fmt"
	"sort"
	"strings"

	sdkconfig "github.com/cexll/agentsdk-go/pkg/config"
	"github.com/smallnest/goclaw/extensions"
)

// buildAgentSDKSettingsOverrides loads workspace MCP config and converts it to
// agentsdk-go SettingsOverrides. Returns nil if there is no enabled MCP server.
func buildAgentSDKSettingsOverrides(workspaceDir string) (*sdkconfig.Settings, []string) {
	cfgPath := extensions.MCPConfigPath(workspaceDir)
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

