package extensions

import "testing"

func TestParsePluginMCPServersEmptyWrapperShouldReturnNoServers(t *testing.T) {
	raw := []byte(`{"mcpServers": {}}`)

	servers, warnings := parsePluginMCPServers(raw, "demo-plugin")
	if len(warnings) != 0 {
		t.Fatalf("warnings=%v, want empty", warnings)
	}
	if len(servers) != 0 {
		t.Fatalf("expected no servers for empty mcpServers wrapper, got %v", servers)
	}
}
