package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	coreevents "github.com/cexll/agentsdk-go/pkg/core/events"
	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

type approvalDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func (m *AgentManager) configureSubagentApprovals() {
	if m == nil {
		return
	}
	setter, ok := m.subagentRuntime.(agentruntime.PermissionDeciderSetter)
	if !ok || setter == nil {
		return
	}

	setter.SetPermissionDecider(m.decideSubagentPermissionByMainAgent)
	logger.Info("Subagent approvals delegated to main agent")
}

func (m *AgentManager) decideSubagentPermissionByMainAgent(ctx context.Context, run agentruntime.SubagentRunRequest, req sdkapi.PermissionRequest) (coreevents.PermissionDecisionType, error) {
	// Fail-safe: if we cannot evaluate safely, deny.
	if m == nil || m.mainRuntime == nil {
		return coreevents.PermissionDeny, nil
	}

	record, _ := m.subagentRegistry.GetRun(strings.TrimSpace(run.RunID))
	approverAgent, approverAgentID := m.resolveApproverAgent(record)
	if approverAgent == nil {
		return coreevents.PermissionDeny, nil
	}
	if strings.TrimSpace(approverAgentID) == "" {
		approverAgentID = "default"
	}

	requesterSessionKey := "approvals"
	if record != nil && strings.TrimSpace(record.RequesterSessionKey) != "" {
		requesterSessionKey = strings.TrimSpace(record.RequesterSessionKey)
	}
	approvalSessionKey := fmt.Sprintf("%s:approvals:%s", requesterSessionKey, strings.TrimSpace(run.RunID))

	prompt := buildSubagentApprovalPrompt(run, req)
	resp, err := m.mainRuntime.Run(ctx, MainRunRequest{
		AgentID:       strings.TrimSpace(approverAgentID),
		SessionKey:    approvalSessionKey,
		Prompt:        prompt,
		SystemPrompt:  approverAgent.GetState().SystemPrompt,
		Workspace:     approverAgent.GetWorkspace(),
		ToolWhitelist: []string{"__no_tools__"},
		Metadata: map[string]any{
			"source":          "subagent_permission_approval",
			"subagent_run_id": strings.TrimSpace(run.RunID),
			"subagent_role":   strings.TrimSpace(run.Role),
			"tool_name":       strings.TrimSpace(req.ToolName),
		},
	})
	if err != nil {
		logger.Warn("Subagent approval failed; denying by default",
			zap.String("run_id", strings.TrimSpace(run.RunID)),
			zap.String("tool", strings.TrimSpace(req.ToolName)),
			zap.Error(err))
		return coreevents.PermissionDeny, nil
	}

	output := ""
	if resp != nil {
		output = strings.TrimSpace(resp.Output)
	}
	decision, reason := parseApprovalDecision(output)

	switch decision {
	case coreevents.PermissionAllow:
		logger.Info("Subagent tool approved by main agent",
			zap.String("run_id", strings.TrimSpace(run.RunID)),
			zap.String("tool", strings.TrimSpace(req.ToolName)),
			zap.String("target", strings.TrimSpace(req.Target)),
			zap.String("rule", strings.TrimSpace(req.Rule)),
			zap.String("reason", reason))
		return coreevents.PermissionAllow, nil
	default:
		logger.Info("Subagent tool denied by main agent",
			zap.String("run_id", strings.TrimSpace(run.RunID)),
			zap.String("tool", strings.TrimSpace(req.ToolName)),
			zap.String("target", strings.TrimSpace(req.Target)),
			zap.String("rule", strings.TrimSpace(req.Rule)),
			zap.String("reason", reason))
		return coreevents.PermissionDeny, nil
	}
}

func (m *AgentManager) resolveApproverAgent(record *SubagentRunRecord) (*Agent, string) {
	if m == nil {
		return nil, ""
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if record != nil && record.RequesterOrigin != nil {
		bindingKey := fmt.Sprintf("%s:%s",
			strings.TrimSpace(record.RequesterOrigin.Channel),
			strings.TrimSpace(record.RequesterOrigin.AccountID),
		)
		if entry, ok := m.bindings[bindingKey]; ok && entry != nil && entry.Agent != nil {
			return entry.Agent, strings.TrimSpace(entry.AgentID)
		}
	}

	if m.defaultAgent != nil {
		agentID := ""
		for id, agent := range m.agents {
			if agent == m.defaultAgent {
				agentID = id
				break
			}
		}
		if agentID == "" {
			agentID = "default"
		}
		return m.defaultAgent, agentID
	}

	for id, agent := range m.agents {
		if agent != nil {
			return agent, id
		}
	}
	return nil, ""
}

func buildSubagentApprovalPrompt(run agentruntime.SubagentRunRequest, req sdkapi.PermissionRequest) string {
	paramsJSON := ""
	if len(req.ToolParams) > 0 {
		if payload, err := json.Marshal(req.ToolParams); err == nil {
			paramsJSON = string(payload)
		}
	}

	lines := []string{
		"你是主Agent的安全审批器。",
		"一个 subagent 的工具调用触发了沙盒 ask 规则，需要你决定是否允许执行。",
		"",
		"只输出一段 JSON，不要输出 Markdown，不要解释。",
		"输出格式：",
		"- {\"decision\":\"allow\"}",
		"- {\"decision\":\"deny\",\"reason\":\"...\"}",
		"",
		"如果不确定或风险偏高：deny。",
		"",
		"Subagent Context:",
		fmt.Sprintf("- run_id: %s", strings.TrimSpace(run.RunID)),
		fmt.Sprintf("- role: %s", strings.TrimSpace(run.Role)),
		fmt.Sprintf("- task: %s", strings.TrimSpace(run.Task)),
		fmt.Sprintf("- repo_dir: %s", strings.TrimSpace(run.RepoDir)),
		"",
		"Tool Request:",
		fmt.Sprintf("- tool: %s", strings.TrimSpace(req.ToolName)),
		fmt.Sprintf("- rule: %s", strings.TrimSpace(req.Rule)),
		fmt.Sprintf("- target: %s", strings.TrimSpace(req.Target)),
		fmt.Sprintf("- reason: %s", strings.TrimSpace(req.Reason)),
	}
	if paramsJSON != "" {
		lines = append(lines, fmt.Sprintf("- params_json: %s", paramsJSON))
	}
	return strings.Join(lines, "\n")
}

func parseApprovalDecision(output string) (coreevents.PermissionDecisionType, string) {
	text := strings.TrimSpace(output)
	if text == "" {
		return coreevents.PermissionDeny, "empty approval response"
	}
	text = stripCodeFences(text)

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(text[start : end+1])
		var dec approvalDecision
		if err := json.Unmarshal([]byte(candidate), &dec); err == nil {
			decision := strings.ToLower(strings.TrimSpace(dec.Decision))
			reason := strings.TrimSpace(dec.Reason)
			switch decision {
			case "allow", "approved", "yes", "true":
				return coreevents.PermissionAllow, reason
			case "deny", "denied", "no", "false":
				if reason == "" {
					reason = "denied"
				}
				return coreevents.PermissionDeny, reason
			}
		}
	}

	lower := strings.ToLower(text)
	if strings.Contains(lower, "allow") && !strings.Contains(lower, "deny") {
		return coreevents.PermissionAllow, ""
	}

	return coreevents.PermissionDeny, "unparseable approval response"
}

func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) < 2 {
		return ""
	}
	body := parts[1]
	if idx := strings.LastIndex(body, "```"); idx >= 0 {
		body = body[:idx]
	}
	return strings.TrimSpace(body)
}
