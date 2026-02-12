package cli

import (
	"fmt"
	"testing"

	"github.com/smallnest/goclaw/config"
)

func TestBuildSubagentRuntimeDefaultAgentsdk(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model:         "openrouter:anthropic/claude-opus-4-5",
				MaxIterations: 15,
				MaxTokens:     4096,
				Temperature:   0.7,
				Subagents:     &config.SubagentsConfig{},
			},
		},
	}

	runtime, mode := buildSubagentRuntime(cfg)
	if runtime == nil {
		t.Fatalf("buildSubagentRuntime() returned nil runtime")
	}
	if mode != "agentsdk" {
		t.Fatalf("runtime mode = %q, want %q", mode, "agentsdk")
	}
	if gotType := fmt.Sprintf("%T", runtime); gotType == "" {
		t.Fatalf("runtime type should not be empty")
	}
}
