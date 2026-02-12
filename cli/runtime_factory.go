package cli

import (
	"strings"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/providers"
)

func buildSubagentRuntime(cfg *config.Config, provider providers.Provider) (agentruntime.SubagentRuntime, string) {
	subagentCfg := cfg.Agents.Defaults.Subagents
	roleLimits := map[string]int{
		agentruntime.RoleFrontend: 5,
		agentruntime.RoleBackend:  4,
	}
	defaultMaxConcurrent := 8
	if subagentCfg != nil {
		if subagentCfg.MaxConcurrent > 0 {
			defaultMaxConcurrent = subagentCfg.MaxConcurrent
		}
		if subagentCfg.FrontendMaxConcurrent > 0 {
			roleLimits[agentruntime.RoleFrontend] = subagentCfg.FrontendMaxConcurrent
		}
		if subagentCfg.BackendMaxConcurrent > 0 {
			roleLimits[agentruntime.RoleBackend] = subagentCfg.BackendMaxConcurrent
		}
	}
	rolePool := agentruntime.NewSimpleRolePool(defaultMaxConcurrent, roleLimits)

	subagentModel := "claude-sonnet-4-5"
	timeoutSeconds := 900
	if subagentCfg != nil {
		if strings.TrimSpace(subagentCfg.Model) != "" {
			subagentModel = strings.TrimSpace(subagentCfg.Model)
		}
		if subagentCfg.TimeoutSeconds > 0 {
			timeoutSeconds = subagentCfg.TimeoutSeconds
		}
	}

	maxTokens := cfg.Agents.Defaults.MaxTokens
	temperature := cfg.Agents.Defaults.Temperature

	runtimeMode := "agentsdk"
	if subagentCfg != nil && strings.TrimSpace(subagentCfg.Runtime) != "" {
		runtimeMode = strings.ToLower(strings.TrimSpace(subagentCfg.Runtime))
	}

	switch runtimeMode {
	case "goclaw":
		return agentruntime.NewProviderRuntime(agentruntime.ProviderRuntimeOptions{
			Pool:            rolePool,
			Provider:        provider,
			ModelName:       subagentModel,
			Temperature:     temperature,
			MaxTokens:       maxTokens,
			DefaultTimeoutS: timeoutSeconds,
		}), "goclaw"
	default:
		return agentruntime.NewAgentsdkRuntime(agentruntime.AgentsdkRuntimeOptions{
			Pool:             rolePool,
			AnthropicAPIKey:  strings.TrimSpace(cfg.Providers.Anthropic.APIKey),
			AnthropicBaseURL: strings.TrimSpace(cfg.Providers.Anthropic.BaseURL),
			ModelName:        subagentModel,
			MaxTokens:        maxTokens,
			Temperature:      temperature,
			MaxIterations:    cfg.Agents.Defaults.MaxIterations,
			FallbackProvider: provider,
		}), "agentsdk"
	}
}
