package agent

import (
	"fmt"
	"os"
	"sort"
	"strings"

	sdkconfig "github.com/cexll/agentsdk-go/pkg/config"
	"github.com/smallnest/goclaw/extensions"
)

// buildAgentSDKSettingsOverrides loads workspace MCP config and converts it to
// agentsdk-go SettingsOverrides. Returns nil if there is no enabled MCP server.
func buildAgentSDKSettingsOverrides(workspaceDir string) (*sdkconfig.Settings, []string) {
	// Prefer .agents/config.toml; fallback to legacy .goclaw/mcp.json.
	cfgPath := extensions.AgentsConfigPath(workspaceDir)
	cfg, err := extensions.LoadAgentsConfig(cfgPath)
	if err != nil {
		return nil, []string{fmt.Sprintf("load agents config %s: %v", cfgPath, err)}
	}
	if cfg == nil || len(cfg.MCPServers) == 0 {
		legacyPath := extensions.MCPConfigPath(workspaceDir)
		legacy, legacyErr := extensions.LoadMCPConfig(legacyPath)
		if legacyErr != nil {
			return nil, nil
		}
		if legacy == nil || len(legacy.Servers) == 0 {
			return nil, nil
		}
		cfg = convertLegacyMCPToAgentsConfig(legacy)
	}

	enabled := make(map[string]sdkconfig.MCPServerConfig)
	var warnings []string

	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		server := cfg.MCPServers[name]

		ok := true
		if server.Enabled != nil && !*server.Enabled {
			ok = false
		}
		if !ok {
			continue
		}

		n := strings.TrimSpace(name)
		if n == "" {
			continue
		}

		typ := strings.ToLower(strings.TrimSpace(server.Type))
		if typ == "" {
			if strings.TrimSpace(server.Command) != "" {
				typ = "stdio"
			} else if strings.TrimSpace(server.URL) != "" {
				typ = "http"
			}
		}

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

		headers := cloneStringMap(server.HTTPHeaders)
		if strings.TrimSpace(server.BearerTokenEnvVar) != "" {
			token := strings.TrimSpace(os.Getenv(strings.TrimSpace(server.BearerTokenEnvVar)))
			if token == "" {
				warnings = append(warnings, fmt.Sprintf("mcp server %s: bearer token env var %q is empty", n, server.BearerTokenEnvVar))
			} else {
				if headers == nil {
					headers = map[string]string{}
				}
				if _, exists := headers["Authorization"]; !exists {
					headers["Authorization"] = "Bearer " + token
				}
			}
		}

		timeoutSeconds := 0
		if server.StartupTimeoutSec != nil && *server.StartupTimeoutSec > 0 {
			timeoutSeconds = *server.StartupTimeoutSec
		}
		toolTimeoutSeconds := 0
		if server.ToolTimeoutSec != nil && *server.ToolTimeoutSec > 0 {
			toolTimeoutSeconds = *server.ToolTimeoutSec
		}

		enabled[n] = sdkconfig.MCPServerConfig{
			Type:               typ,
			Command:            strings.TrimSpace(server.Command),
			Args:               append([]string(nil), server.Args...),
			URL:                strings.TrimSpace(server.URL),
			Env:                cloneStringMap(server.Env),
			Headers:            headers,
			TimeoutSeconds:     timeoutSeconds,
			EnabledTools:       append([]string(nil), server.EnabledTools...),
			DisabledTools:      append([]string(nil), server.DisabledTools...),
			ToolTimeoutSeconds: toolTimeoutSeconds,
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

func convertLegacyMCPToAgentsConfig(cfg *extensions.MCPConfig) *extensions.AgentsConfig {
	out := &extensions.AgentsConfig{MCPServers: map[string]extensions.MCPServerConfig{}}
	if cfg == nil || len(cfg.Servers) == 0 {
		return out
	}
	for name, srv := range cfg.Servers {
		if strings.TrimSpace(name) == "" {
			continue
		}
		enabled := srv.Enabled
		timeout := srv.TimeoutSeconds
		timeoutPtr := (*int)(nil)
		if timeout > 0 {
			v := timeout
			timeoutPtr = &v
		}
		out.MCPServers[name] = extensions.MCPServerConfig{
			Enabled:           &enabled,
			Type:              strings.TrimSpace(srv.Type),
			Command:           strings.TrimSpace(srv.Command),
			Args:              append([]string(nil), srv.Args...),
			URL:               strings.TrimSpace(srv.URL),
			Env:               cloneStringMap(srv.Env),
			HTTPHeaders:       cloneStringMap(srv.Headers),
			StartupTimeoutSec: timeoutPtr,
		}
	}
	return out
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
