package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/session"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	messageBus := bus.NewMessageBus(4)
	t.Cleanup(func() { _ = messageBus.Close() })

	sessionMgr, err := session.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	channelMgr := channels.NewManager(messageBus)
	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return NewServer(cfg, messageBus, channelMgr, sessionMgr)
}

func TestSetWebSocketConfigNilShouldNotPanic(t *testing.T) {
	s := newTestServer(t)

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("SetWebSocketConfig(nil) should return/ignore gracefully, got panic: %v", rec)
		}
	}()

	s.SetWebSocketConfig(nil)
}

func TestHandleGenericWebhookShortPathShouldNotPanic(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if recov := recover(); recov != nil {
			t.Fatalf("short webhook path should be handled safely, got panic: %v", recov)
		}
	}()

	s.handleGenericWebhook(rec, req)
}
