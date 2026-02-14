package extensions

import "testing"

func TestNormalizeToolSelectorPatternTrimsWhitespace(t *testing.T) {
	got := normalizeToolSelectorPattern("  Bash  ")
	if got != "Bash" {
		t.Fatalf("expected trimmed matcher %q, got %q", "Bash", got)
	}
}
