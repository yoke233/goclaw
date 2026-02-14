package runtime

import (
	"context"
	"testing"
)

type alwaysFailPool struct{}

func (alwaysFailPool) Acquire(_ context.Context, _ string) error { return context.Canceled }
func (alwaysFailPool) Release(_ string)                          {}

func TestAgentsdkRuntimeSpawnHonorsCanceledContext(t *testing.T) {
	rt := NewAgentsdkRuntime(AgentsdkRuntimeOptions{
		Pool: alwaysFailPool{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Spawn(ctx, SubagentRunRequest{
		RunID: "run-canceled",
		Task:  "do work",
	})
	if err == nil {
		t.Fatalf("expected Spawn to fail when context is canceled")
	}
}
