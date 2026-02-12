package cli

import (
	"strings"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/config"
)

func buildSubagentRuntime(cfg *config.Config) (agentruntime.SubagentRuntime, string) {
	subagentCfg := cfg.Agents.Defaults.Subagents
	roleLimits := map[string]int{}
	defaultMaxConcurrent := 8
	if subagentCfg != nil {
		if subagentCfg.MaxConcurrent > 0 {
			defaultMaxConcurrent = subagentCfg.MaxConcurrent
		}
		for role, limit := range subagentCfg.RoleMaxConcurrent {
			if limit <= 0 {
				continue
			}
			roleLimits[role] = limit
		}
	}
	rolePool := agentruntime.NewSimpleRolePool(defaultMaxConcurrent, roleLimits)

	subagentModel := "claude-sonnet-4-5"
	if subagentCfg != nil {
		if strings.TrimSpace(subagentCfg.Model) != "" {
			subagentModel = strings.TrimSpace(subagentCfg.Model)
		}
	}

	maxTokens := cfg.Agents.Defaults.MaxTokens
	temperature := cfg.Agents.Defaults.Temperature

	return agentruntime.NewAgentsdkRuntime(agentruntime.AgentsdkRuntimeOptions{
		Pool:             rolePool,
		AnthropicAPIKey:  strings.TrimSpace(cfg.Providers.Anthropic.APIKey),
		AnthropicBaseURL: strings.TrimSpace(cfg.Providers.Anthropic.BaseURL),
		ModelName:        subagentModel,
		MaxTokens:        maxTokens,
		Temperature:      temperature,
		MaxIterations:    cfg.Agents.Defaults.MaxIterations,
	}), "agentsdk"
}
