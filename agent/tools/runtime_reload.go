package tools

import (
	"context"
	"encoding/json"
	"strings"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
)

type runtimeReloadResult struct {
	Success  bool   `json:"success"`
	AgentID  string `json:"agent_id"`
	Reloaded bool   `json:"reloaded"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// NewRuntimeReloadTool creates a lightweight tool to invalidate/reload the main runtime.
// The actual reload happens after the current turn finishes.
func NewRuntimeReloadTool(invalidate RuntimeInvalidator) *BaseTool {
	return NewBaseTool(
		"runtime_reload",
		"Request a main-agent runtime reload (safe: applied after the current turn). Useful after changing skills/MCP config on disk.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional agent id to reload. Defaults to current agent from context.",
				},
			},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			agentID := strings.TrimSpace(asString(params["agent_id"]))
			if agentID == "" {
				agentID = strings.TrimSpace(asString(ctx.Value(agentruntime.CtxAgentID)))
			}
			if agentID == "" {
				agentID = "default"
			}

			if invalidate == nil {
				out, _ := json.Marshal(runtimeReloadResult{
					Success:  false,
					AgentID:  agentID,
					Reloaded: false,
					Error:    "runtime invalidator not configured",
				})
				return string(out), nil
			}

			if err := invalidate(ctx, agentID); err != nil {
				out, _ := json.Marshal(runtimeReloadResult{
					Success:  false,
					AgentID:  agentID,
					Reloaded: false,
					Error:    err.Error(),
				})
				return string(out), nil
			}

			out, _ := json.Marshal(runtimeReloadResult{
				Success:  true,
				AgentID:  agentID,
				Reloaded: true,
				Message:  "runtime reload requested",
			})
			return string(out), nil
		},
	)
}

