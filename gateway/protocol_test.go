package gateway

import "testing"

func TestMethodRegistryCallNilHandlerShouldNotPanic(t *testing.T) {
	r := NewMethodRegistry()
	r.Register("m", nil)

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("calling nil method handler should return error, got panic: %v", rec)
		}
	}()

	if _, err := r.Call("m", "s1", map[string]interface{}{}); err == nil {
		t.Fatalf("expected error for nil method handler")
	}
}

func TestParseRequestRejectsWhitespaceMethod(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":"1","method":"   ","params":{}}`)

	if _, err := ParseRequest(data); err == nil {
		t.Fatalf("expected parse request to reject whitespace-only method")
	}
}

func TestParseRequestAcceptsNumericID(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"health","params":{}}`)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("expected numeric id request to be accepted, got error: %v", err)
	}
	if req.ID != "1" {
		t.Fatalf("expected numeric id to be normalized to string \"1\", got %q", req.ID)
	}
}
