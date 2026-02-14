package agent

import "testing"

func TestWaitForSubagentCompletionPassesEffectiveTimeoutToWaitFunc(t *testing.T) {
	captured := -1

	_, _ = WaitForSubagentCompletion("run-1", 0, func(runID string, timeoutSeconds int) (*SubagentCompletion, error) {
		captured = timeoutSeconds
		return &SubagentCompletion{Status: "ok"}, nil
	})

	if captured <= 0 {
		t.Fatalf("expected waitFunc to receive positive effective timeout, got %d", captured)
	}
}
