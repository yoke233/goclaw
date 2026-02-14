package agent

import "testing"

func TestResolveSessionKeyFreshOnDefaultGeneratesUniqueKeys(t *testing.T) {
	seen := map[string]struct{}{}

	for i := 0; i < 50; i++ {
		key, fresh := ResolveSessionKey(SessionKeyOptions{
			Channel:        "cli",
			AccountID:      "default",
			ChatID:         "default",
			FreshOnDefault: true,
		})
		if !fresh {
			t.Fatalf("expected fresh=true when FreshOnDefault is enabled")
		}
		seen[key] = struct{}{}
	}

	if len(seen) != 50 {
		t.Fatalf("expected unique keys for fresh sessions, got %d unique out of 50", len(seen))
	}
}
