package bus

import (
	"context"
	"testing"
	"time"
)

func TestStreamHandlerOnChunkReentrantAccessShouldNotDeadlock(t *testing.T) {
	streamBus := NewStreamingMessageBus(1)
	handler := NewStreamHandler(streamBus, "chat-1")

	done := make(chan struct{})
	handler.OnChunk(func(msg *StreamMessage) {
		_ = handler.GetContent()
		close(done)
	})
	handler.Start(context.Background())

	err := streamBus.PublishStream(context.Background(), &StreamMessage{
		ChatID:  "chat-1",
		Content: "chunk-1",
	})
	if err != nil {
		t.Fatalf("publish stream failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("onChunk callback with reentrant handler access appears to deadlock")
	}
}

func TestStreamHandlerCompleteCallbackIncludesFinalContent(t *testing.T) {
	streamBus := NewStreamingMessageBus(1)
	handler := NewStreamHandler(streamBus, "chat-final")

	done := make(chan struct{})
	var completed string
	handler.OnComplete(func(s string) {
		completed = s
		close(done)
	})
	handler.Start(context.Background())

	err := streamBus.PublishStream(context.Background(), &StreamMessage{
		ChatID:     "chat-final",
		Content:    "final-answer",
		IsFinal:    true,
		IsComplete: true,
	})
	if err != nil {
		t.Fatalf("publish stream failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected completion callback to fire")
	}

	if completed != "final-answer" {
		t.Fatalf("expected complete payload to include final content, got %q", completed)
	}
}
