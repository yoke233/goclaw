package runtime

import (
	"context"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	coreevents "github.com/cexll/agentsdk-go/pkg/core/events"
)

// PermissionDecider lets the host (main agent / orchestrator) decide whether a
// subagent tool invocation that hit a PermissionAsk rule should be allowed.
//
// It is only invoked when agentsdk-go's sandbox returns an "ask" decision.
// Returning PermissionAsk will cause the tool execution to fail with a
// "requires approval" error, so callers should generally return allow/deny.
type PermissionDecider func(context.Context, SubagentRunRequest, sdkapi.PermissionRequest) (coreevents.PermissionDecisionType, error)

// PermissionDeciderSetter is an optional interface implemented by subagent
// runtimes that support delegating PermissionAsk decisions.
type PermissionDeciderSetter interface {
	SetPermissionDecider(PermissionDecider)
}
