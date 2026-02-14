package cron

import (
	"testing"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/session"
)

func TestSchedulerAddJobNilShouldNotPanic(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	sessionMgr, err := session.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	s := NewScheduler(messageBus, nil, sessionMgr)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("adding nil job should return error, got panic: %v", r)
		}
	}()

	if err := s.AddJob(nil); err == nil {
		t.Fatalf("expected error when adding nil job")
	}
}
