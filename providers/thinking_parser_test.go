package providers

import "testing"

func TestThinkingParserHandlesSplitThinkingStartTagAcrossChunks(t *testing.T) {
	p := NewThinkingParser()

	first := p.Parse("<thin")
	if len(first) != 0 {
		t.Fatalf("expected no output for partial opening tag, got %+v", first)
	}

	second := p.Parse("king>reasoning</thinking>")
	if len(second) != 1 {
		t.Fatalf("expected one thinking chunk, got %+v", second)
	}
	if !second[0].IsThinking || second[0].Content != "reasoning" {
		t.Fatalf("expected thinking chunk 'reasoning', got %+v", second[0])
	}
	if p.IsInThinking() {
		t.Fatalf("expected parser to exit thinking mode after closing tag")
	}
}

func TestThinkingParserHandlesSplitFinalClosingTagAcrossChunks(t *testing.T) {
	p := NewThinkingParser()

	first := p.Parse("<final>answer</fi")
	if len(first) != 1 {
		t.Fatalf("expected one final-content chunk, got %+v", first)
	}
	if !first[0].IsFinal || first[0].Content != "answer" {
		t.Fatalf("expected final chunk 'answer', got %+v", first[0])
	}

	second := p.Parse("nal>")
	if len(second) != 0 {
		t.Fatalf("expected no extra plain-text chunk when closing tag completes, got %+v", second)
	}
	if p.IsInFinal() {
		t.Fatalf("expected parser to exit final mode after closing tag")
	}
}
