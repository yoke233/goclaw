package providers

import "testing"

func TestStreamBufferEnforcesMaxSizeWithIncomingChunk(t *testing.T) {
	buf := NewStreamBuffer(5)

	if err := buf.Add(StreamChunk{Content: "12345"}); err != nil {
		t.Fatalf("unexpected add error: %v", err)
	}

	if err := buf.Add(StreamChunk{Content: "6"}); err == nil {
		t.Fatalf("expected buffer size overflow error when adding extra chunk")
	}
}
