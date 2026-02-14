package agent

import (
	"context"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
)

// StreamEvent mirrors agentsdk-go streaming events.
type StreamEvent = sdkapi.StreamEvent

// MainRuntimeStreamer extends MainRuntime with streaming support.
type MainRuntimeStreamer interface {
	RunStream(ctx context.Context, req MainRunRequest) (<-chan StreamEvent, error)
}
