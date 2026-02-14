package tools

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/config"
)

type mockSubagentRegistry struct {
	called bool
	params *SubagentRunParams
}

func (m *mockSubagentRegistry) RegisterRun(params *SubagentRunParams) error {
	m.called = true
	m.params = params
	return nil
}

func TestSubagentSpawnToolExecuteReadsRequesterContext(t *testing.T) {
	registry := &mockSubagentRegistry{}
	tool := NewSubagentSpawnTool(registry)
	tool.SetDefaultConfigGetter(func() *config.AgentDefaults {
		return &config.AgentDefaults{
			Subagents: &config.SubagentsConfig{
				ArchiveAfterMinutes: 77,
				TimeoutSeconds:      321,
			},
		}
	})

	ctx := context.Background()
	ctx = context.WithValue(ctx, agentruntime.CtxSessionKey, "telegram:bot1:chat42")
	ctx = context.WithValue(ctx, agentruntime.CtxAgentID, "assistant-main")
	ctx = context.WithValue(ctx, agentruntime.CtxChannel, "telegram")
	ctx = context.WithValue(ctx, agentruntime.CtxAccountID, "bot1")
	ctx = context.WithValue(ctx, agentruntime.CtxChatID, "chat42")

	result, err := tool.Execute(ctx, map[string]interface{}{
		"task":    "implement backend API",
		"task_id": "task-42",
	})
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.Contains(result, "Subagent spawned successfully.") {
		t.Fatalf("unexpected result text: %s", result)
	}
	if !registry.called || registry.params == nil {
		t.Fatalf("registry.RegisterRun was not called")
	}

	if registry.params.RequesterSessionKey != "telegram:bot1:chat42" {
		t.Fatalf("RequesterSessionKey = %q, want %q", registry.params.RequesterSessionKey, "telegram:bot1:chat42")
	}
	if registry.params.RequesterDisplayKey != "telegram:bot1:chat42" {
		t.Fatalf("RequesterDisplayKey = %q, want %q", registry.params.RequesterDisplayKey, "telegram:bot1:chat42")
	}
	if registry.params.RequesterOrigin == nil {
		t.Fatalf("RequesterOrigin should not be nil")
	}
	if registry.params.RequesterOrigin.Channel != "telegram" {
		t.Fatalf("RequesterOrigin.Channel = %q, want %q", registry.params.RequesterOrigin.Channel, "telegram")
	}
	if registry.params.RequesterOrigin.AccountID != "bot1" {
		t.Fatalf("RequesterOrigin.AccountID = %q, want %q", registry.params.RequesterOrigin.AccountID, "bot1")
	}
	if registry.params.RequesterOrigin.To != "chat42" {
		t.Fatalf("RequesterOrigin.To = %q, want %q", registry.params.RequesterOrigin.To, "chat42")
	}
	if registry.params.TimeoutSeconds != 321 {
		t.Fatalf("TimeoutSeconds = %d, want %d", registry.params.TimeoutSeconds, 321)
	}
	if registry.params.TaskID != "task-42" {
		t.Fatalf("TaskID = %q, want %q", registry.params.TaskID, "task-42")
	}
	if registry.params.ArchiveAfterMinutes != 77 {
		t.Fatalf("ArchiveAfterMinutes = %d, want %d", registry.params.ArchiveAfterMinutes, 77)
	}
}

func TestSubagentSpawnToolExecuteTimeoutOverride(t *testing.T) {
	registry := &mockSubagentRegistry{}
	tool := NewSubagentSpawnTool(registry)
	tool.SetDefaultConfigGetter(func() *config.AgentDefaults {
		return &config.AgentDefaults{
			Subagents: &config.SubagentsConfig{
				TimeoutSeconds: 300,
			},
		}
	})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"task":                "implement frontend page",
		"run_timeout_seconds": float64(45),
	})
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.Contains(result, "Subagent spawned successfully.") {
		t.Fatalf("unexpected result text: %s", result)
	}
	if registry.params == nil {
		t.Fatalf("registry params should not be nil")
	}
	if registry.params.TimeoutSeconds != 45 {
		t.Fatalf("TimeoutSeconds = %d, want %d", registry.params.TimeoutSeconds, 45)
	}
}

func TestSubagentSpawnToolExecuteRejectsNegativeTimeout(t *testing.T) {
	registry := &mockSubagentRegistry{}
	tool := NewSubagentSpawnTool(registry)
	tool.SetDefaultConfigGetter(func() *config.AgentDefaults { return nil })

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"task":                "run diagnostics",
		"run_timeout_seconds": -10,
	})
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.HasPrefix(result, "Error:") {
		t.Fatalf("expected negative timeout to be rejected, got result: %s", result)
	}
	if registry.called {
		t.Fatalf("registry.RegisterRun should not be called for invalid timeout")
	}
}

func TestSubagentSpawnToolExecuteNormalizesCleanupPolicy(t *testing.T) {
	registry := &mockSubagentRegistry{}
	tool := NewSubagentSpawnTool(registry)
	tool.SetDefaultConfigGetter(func() *config.AgentDefaults { return nil })

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"task":    "refactor parser",
		"cleanup": "  DELETE  ",
	})
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.Contains(result, "Subagent spawned successfully.") {
		t.Fatalf("unexpected result text: %s", result)
	}
	if registry.params == nil {
		t.Fatalf("registry params should not be nil")
	}
	if registry.params.Cleanup != "delete" {
		t.Fatalf("Cleanup = %q, want %q", registry.params.Cleanup, "delete")
	}
}

func TestSubagentSpawnToolExecuteTrimsAgentIDBeforePermissionCheck(t *testing.T) {
	registry := &mockSubagentRegistry{}
	tool := NewSubagentSpawnTool(registry)
	tool.SetDefaultConfigGetter(func() *config.AgentDefaults { return nil })

	ctx := context.WithValue(context.Background(), agentruntime.CtxAgentID, "assistant-main")
	result, err := tool.Execute(ctx, map[string]interface{}{
		"task":     "collect metrics",
		"agent_id": "  assistant-main  ",
	})
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.Contains(result, "Subagent spawned successfully.") {
		t.Fatalf("expected same agent id with whitespace to be accepted, got: %s", result)
	}
	if !registry.called {
		t.Fatalf("registry.RegisterRun should be called for same-agent spawn")
	}
}
