package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"

	sdkconfig "github.com/cexll/agentsdk-go/pkg/config"
	"github.com/smallnest/goclaw/extensions"
)

func buildSubagentSDKSettingsOverrides(req SubagentRunRequest, pluginCfg *extensions.AgentsConfig) (*sdkconfig.Settings, []string) {
	// Always disable agentsdk-go on-disk history for subagent runs. Subagents
	// often run inside real repos (repodir), so we avoid writing .claude/history.
	cleanupPeriodDays := 0
	overrides := &sdkconfig.Settings{
		CleanupPeriodDays: &cleanupPeriodDays,
	}

	// Precedence:
	// 1) Explicit override via request (MCPConfigPath) [single file path]
	// 2) Layered merge: baseRoot + repodir overlay (from .agents/config.toml)
	explicit := strings.TrimSpace(req.MCPConfigPath)
	if explicit != "" {
		mcp, warnings := buildSDKMCPOverridesFromExplicitPath(explicit)
		overrides.MCP = mcp
		return overrides, warnings
	}

	baseRoot := resolveSubagentBaseRoot(req)
	repoRoot := strings.TrimSpace(req.RepoDir)
	if strings.TrimSpace(baseRoot) == "" && strings.TrimSpace(repoRoot) == "" {
		return overrides, []string{"both base root and repo root are empty; cannot resolve agents config"}
	}

	baseCfg, baseWarnings := loadAgentsConfigFromRoot(baseRoot)
	repoCfg, repoWarnings := loadAgentsConfigFromRoot(repoRoot)
	merged := extensions.MergeAgentsConfig(pluginCfg, baseCfg)
	merged = extensions.MergeAgentsConfig(merged, repoCfg)
	mcp, mcpWarnings := buildSDKMCPOverridesFromAgentsConfig(merged)
	overrides.MCP = mcp

	warnings := make([]string, 0, len(baseWarnings)+len(repoWarnings)+len(mcpWarnings))
	warnings = append(warnings, baseWarnings...)
	warnings = append(warnings, repoWarnings...)
	warnings = append(warnings, mcpWarnings...)
	return overrides, warnings
}

func buildSDKMCPOverridesFromExplicitPath(cfgPath string) (*sdkconfig.MCPConfig, []string) {
	cfgPath = strings.TrimSpace(cfgPath)
	if cfgPath == "" {
		return nil, []string{"explicit config path is empty"}
	}

	stat, err := os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []string{fmt.Sprintf("explicit mcp config path does not exist: %s", cfgPath)}
		}
		return nil, []string{fmt.Sprintf("stat explicit mcp config path %s: %v", cfgPath, err)}
	}
	if stat.IsDir() {
		return nil, []string{fmt.Sprintf("explicit mcp config path is a directory: %s", cfgPath)}
	}

	// Prefer TOML (.agents/config.toml). Fallback to legacy JSON mcp.json.
	cfg, err := extensions.LoadAgentsConfig(cfgPath)
	if err == nil && cfg != nil {
		mcp, warnings := buildSDKMCPOverridesFromAgentsConfig(cfg)
		return mcp, warnings
	}

	legacy, legacyErr := extensions.LoadMCPConfig(cfgPath)
	if legacyErr != nil {
		if err != nil {
			return nil, []string{fmt.Sprintf("load agents config %s: %v", cfgPath, err)}
		}
		return nil, []string{fmt.Sprintf("load legacy mcp config %s: %v", cfgPath, legacyErr)}
	}
	if legacy == nil {
		return nil, nil
	}
	converted := convertLegacyMCPToAgentsConfig(legacy)
	mcp, warnings := buildSDKMCPOverridesFromAgentsConfig(converted)
	return mcp, warnings
}

func loadAgentsConfigFromRoot(root string) (*extensions.AgentsConfig, []string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return &extensions.AgentsConfig{MCPServers: map[string]extensions.MCPServerConfig{}}, nil
	}

	agentsPath := extensions.AgentsConfigPath(root)
	if fileExists(agentsPath) {
		cfg, err := extensions.LoadAgentsConfig(agentsPath)
		if err != nil {
			return &extensions.AgentsConfig{MCPServers: map[string]extensions.MCPServerConfig{}}, []string{fmt.Sprintf("load agents config %s: %v", agentsPath, err)}
		}
		return cfg, nil
	}

	// Legacy fallback: <root>/.goclaw/mcp.json
	legacyPath := extensions.MCPConfigPath(root)
	if fileExists(legacyPath) {
		legacy, err := extensions.LoadMCPConfig(legacyPath)
		if err != nil {
			return &extensions.AgentsConfig{MCPServers: map[string]extensions.MCPServerConfig{}}, []string{fmt.Sprintf("load legacy mcp config %s: %v", legacyPath, err)}
		}
		return convertLegacyMCPToAgentsConfig(legacy), nil
	}

	return &extensions.AgentsConfig{MCPServers: map[string]extensions.MCPServerConfig{}}, nil
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

func buildSDKMCPOverridesFromAgentsConfig(cfg *extensions.AgentsConfig) (*sdkconfig.MCPConfig, []string) {
	if cfg == nil || len(cfg.MCPServers) == 0 {
		return nil, nil
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
				// Avoid injecting a duplicate Authorization header when a case-insensitive
				// equivalent already exists (e.g. "authorization").
				if !hasHeaderKeyCI(headers, "authorization") {
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
			EnabledTools:       normalizeToolFilters(server.EnabledTools),
			DisabledTools:      normalizeToolFilters(server.DisabledTools),
			ToolTimeoutSeconds: toolTimeoutSeconds,
		}
	}

	if len(enabled) == 0 {
		return nil, warnings
	}

	return &sdkconfig.MCPConfig{Servers: enabled}, warnings
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

func hasHeaderKeyCI(headers map[string]string, want string) bool {
	if len(headers) == 0 || strings.TrimSpace(want) == "" {
		return false
	}
	for k := range headers {
		if strings.EqualFold(k, want) {
			return true
		}
	}
	return false
}

func normalizeToolFilters(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, entry := range in {
		tool := strings.TrimSpace(entry)
		if tool == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
