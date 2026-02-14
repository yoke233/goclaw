package channels

import (
	"context"
	"testing"

	"github.com/smallnest/goclaw/bus"
)

func TestBaseChannelRestartDoesNotKeepClosedStopChan(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	cfg := BaseChannelConfig{
		Enabled: true,
	}
	channel := NewBaseChannelImpl("test", "acc-1", cfg, messageBus)

	if err := channel.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := channel.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := channel.Start(context.Background()); err != nil {
		t.Fatalf("restart failed: %v", err)
	}

	select {
	case <-channel.WaitForStop():
		t.Fatalf("stop channel should not already be closed after restart")
	default:
	}
}

func TestBaseChannelIsAllowedUsesAllowList(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	channel := NewBaseChannelImpl("test", "acc-1", BaseChannelConfig{
		Enabled:    true,
		AllowedIDs: []string{"u1", "u2"},
	}, messageBus)

	if !channel.IsAllowed("u1") {
		t.Fatalf("expected u1 to be allowed")
	}
	if channel.IsAllowed("u3") {
		t.Fatalf("expected u3 to be rejected")
	}
}
