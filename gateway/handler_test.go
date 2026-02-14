package gateway

import (
	"strings"
	"testing"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/session"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	messageBus := bus.NewMessageBus(4)
	t.Cleanup(func() { _ = messageBus.Close() })

	sessionMgr, err := session.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	channelMgr := channels.NewManager(messageBus)
	return NewHandler(messageBus, sessionMgr, channelMgr)
}

func TestHandleRequestNilRequestShouldNotPanic(t *testing.T) {
	h := newTestHandler(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil request should return error response, got panic: %v", r)
		}
	}()

	resp := h.HandleRequest("s1", nil)
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected error response for nil request")
	}
}

func TestHandleRequestUnknownMethodReturnsMethodNotFound(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "does.not.exist",
		Params:  map[string]interface{}{},
	})

	if resp == nil || resp.Error == nil {
		t.Fatalf("expected error response for unknown method")
	}
	if resp.Error.Code != ErrorMethodNotFound {
		t.Fatalf("expected method not found code %d, got %d", ErrorMethodNotFound, resp.Error.Code)
	}
}

func TestHandleRequestAgentWaitTimesOutWithoutResponse(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "2",
		Method:  "agent.wait",
		Params: map[string]interface{}{
			"content": "hello",
			"timeout": 0.01,
		},
	})

	if resp == nil || resp.Error == nil {
		t.Fatalf("expected timeout error when no response is produced")
	}
	if !strings.Contains(strings.ToLower(resp.Error.Message), "timeout") {
		t.Fatalf("expected timeout message, got: %v", resp.Error.Message)
	}
}

func TestHandleRequestLogsGetRejectsNegativeLines(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "3",
		Method:  "logs.get",
		Params: map[string]interface{}{
			"lines": -5.0,
		},
	})

	if resp == nil || resp.Error != nil {
		t.Fatalf("expected success response for logs.get call, got error: %+v", resp)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	lines, _ := result["lines"].(int)
	if lines < 0 {
		t.Fatalf("expected non-negative lines count, got %d", lines)
	}
}

func TestHandleRequestAgentWaitSupportsFractionalTimeoutSeconds(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "4",
		Method:  "agent.wait",
		Params: map[string]interface{}{
			"content": "hello",
			"timeout": 0.5,
		},
	})

	if resp == nil {
		t.Fatalf("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("expected positive fractional timeout to be accepted, got error: %+v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if status, _ := result["status"].(string); status != "waiting" {
		t.Fatalf("expected status=waiting, got %q", status)
	}
}

func TestHandleRequestAgentWaitShouldTimeoutWithoutResponse(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "5",
		Method:  "agent.wait",
		Params: map[string]interface{}{
			"content": "hello",
			"timeout": 1.0,
		},
	})

	if resp == nil || resp.Error == nil {
		t.Fatalf("expected agent.wait to timeout when no outbound response is available")
	}
	if !strings.Contains(strings.ToLower(resp.Error.Message), "timeout") {
		t.Fatalf("expected timeout message, got: %v", resp.Error.Message)
	}
}

func TestHandleRequestAgentMissingContentReturnsInvalidParams(t *testing.T) {
	h := newTestHandler(t)

	resp := h.HandleRequest("s1", &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "6",
		Method:  "agent",
		Params:  map[string]interface{}{},
	})

	if resp == nil || resp.Error == nil {
		t.Fatalf("expected error response when content is missing")
	}
	if resp.Error.Code != ErrorInvalidParams {
		t.Fatalf("expected invalid params code %d, got %d", ErrorInvalidParams, resp.Error.Code)
	}
}
