package agent

import (
	"testing"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
)

func TestCollectStreamOutputPreservesWhitespaceSemantics(t *testing.T) {
	tests := []struct {
		name   string
		events []StreamEvent
		want   string
	}{
		{
			name: "middle whitespace delta",
			events: []StreamEvent{
				{Type: sdkapi.EventContentBlockDelta, Delta: &sdkapi.Delta{Text: "Hello"}},
				{Type: sdkapi.EventContentBlockDelta, Delta: &sdkapi.Delta{Text: " "}},
				{Type: sdkapi.EventContentBlockDelta, Delta: &sdkapi.Delta{Text: "World"}},
			},
			want: "Hello World",
		},
		{
			name: "leading and trailing spaces",
			events: []StreamEvent{
				{Type: sdkapi.EventContentBlockDelta, Delta: &sdkapi.Delta{Text: "  hello  "}},
			},
			want: "  hello  ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan StreamEvent, len(tc.events))
			for _, evt := range tc.events {
				ch <- evt
			}
			close(ch)

			got, err := CollectStreamOutput(ch, nil)
			if err != nil {
				t.Fatalf("collect stream output failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
