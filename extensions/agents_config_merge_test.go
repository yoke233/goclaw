package extensions

import "testing"

func TestMergeAgentsConfigHigherEmptyHeadersShouldClearLowerHeaders(t *testing.T) {
	lower := &AgentsConfig{
		MCPServers: map[string]MCPServerConfig{
			"time": {
				HTTPHeaders: map[string]string{
					"Authorization": "Bearer base",
				},
			},
		},
	}
	higher := &AgentsConfig{
		MCPServers: map[string]MCPServerConfig{
			"time": {
				HTTPHeaders: map[string]string{},
			},
		},
	}

	merged := MergeAgentsConfig(lower, higher)
	if merged == nil {
		t.Fatalf("merged config is nil")
	}
	got := merged.MCPServers["time"].HTTPHeaders
	if len(got) != 0 {
		t.Fatalf("expected higher empty http_headers to clear inherited headers, got %v", got)
	}
}

func TestMergeAgentsConfigHigherEmptyEnvShouldClearLowerEnv(t *testing.T) {
	lower := &AgentsConfig{
		MCPServers: map[string]MCPServerConfig{
			"time": {
				Env: map[string]string{
					"TOKEN": "base-token",
				},
			},
		},
	}
	higher := &AgentsConfig{
		MCPServers: map[string]MCPServerConfig{
			"time": {
				Env: map[string]string{},
			},
		},
	}

	merged := MergeAgentsConfig(lower, higher)
	if merged == nil {
		t.Fatalf("merged config is nil")
	}
	got := merged.MCPServers["time"].Env
	if len(got) != 0 {
		t.Fatalf("expected higher empty env to clear inherited env, got %v", got)
	}
}
