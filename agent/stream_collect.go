package agent

import (
	"fmt"
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
)

// ExtractTextDelta returns the text delta for a streaming event, if any.
func ExtractTextDelta(evt StreamEvent) string {
	if evt.Type != sdkapi.EventContentBlockDelta {
		return ""
	}
	if evt.Delta == nil {
		return ""
	}
	if evt.Delta.Text == "" {
		return ""
	}
	return evt.Delta.Text
}

// StreamErrorFromEvent returns a non-nil error if the event indicates a failure.
func StreamErrorFromEvent(evt StreamEvent) error {
	if evt.Type == sdkapi.EventError {
		if evt.Output != nil {
			return fmt.Errorf("%v", evt.Output)
		}
		return fmt.Errorf("stream error")
	}
	if evt.IsError != nil && *evt.IsError {
		if evt.Output != nil {
			return fmt.Errorf("%v", evt.Output)
		}
		return fmt.Errorf("stream error")
	}
	return nil
}

// CollectStreamOutput drains a stream and returns the concatenated text output.
// The onEvent callback is invoked for each event when non-nil.
func CollectStreamOutput(events <-chan StreamEvent, onEvent func(StreamEvent)) (string, error) {
	var out strings.Builder
	var runErr error

	for evt := range events {
		if onEvent != nil {
			onEvent(evt)
		}
		if runErr == nil {
			if err := StreamErrorFromEvent(evt); err != nil {
				runErr = err
			}
		}
		if delta := ExtractTextDelta(evt); delta != "" {
			out.WriteString(delta)
		}
	}

	result := out.String()
	if runErr != nil {
		return result, runErr
	}
	return result, nil
}
