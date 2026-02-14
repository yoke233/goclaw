package session

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPrunerUnknownStrategyReturnsError(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	config := DefaultPruneConfig()
	config.Strategy = PruneStrategy("unexpected")
	pruner := NewPruner(manager, config)

	err = pruner.PruneSessions()
	if err == nil {
		t.Fatalf("expected unknown strategy error")
	}
	if !strings.Contains(err.Error(), "unknown prune strategy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrunerPruneMessagesKeepsMostRecent(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	session, err := manager.GetOrCreate("s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	for i := 0; i < 5; i++ {
		session.AddMessage(Message{
			Role:      "user",
			Content:   fmt.Sprintf("m%d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	pruner := NewPruner(manager, DefaultPruneConfig())
	if err := pruner.PruneMessages("s1", 2); err != nil {
		t.Fatalf("failed to prune messages: %v", err)
	}

	if len(session.Messages) != 2 {
		t.Fatalf("expected exactly 2 messages preserved, got %d", len(session.Messages))
	}
	if session.Messages[0].Content != "m3" || session.Messages[1].Content != "m4" {
		t.Fatalf("expected to keep newest messages [m3 m4], got [%s %s]", session.Messages[0].Content, session.Messages[1].Content)
	}
}

func TestPrunerPruneMessagesByTTLRemovesAllExpired(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	session, err := manager.GetOrCreate("ttl-all-expired")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	session.AddMessage(Message{
		Role:      "user",
		Content:   "old-1",
		Timestamp: time.Now().Add(-4 * time.Hour),
	})
	session.AddMessage(Message{
		Role:      "assistant",
		Content:   "old-2",
		Timestamp: time.Now().Add(-3 * time.Hour),
	})

	cfg := DefaultPruneConfig()
	cfg.DefaultMessageTTL = 30 * time.Minute
	pruner := NewPruner(manager, cfg)
	if err := pruner.PruneMessagesByTTL("ttl-all-expired"); err != nil {
		t.Fatalf("failed to prune by ttl: %v", err)
	}

	if len(session.Messages) != 0 {
		t.Fatalf("expected all expired messages to be removed, got %d", len(session.Messages))
	}
}

func TestPrunerCleanupReturnsWithoutDeadlock(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	if _, err := manager.GetOrCreate("s1"); err != nil {
		t.Fatalf("failed to seed session: %v", err)
	}

	cfg := DefaultPruneConfig()
	cfg.Strategy = PruneStrategyLRU
	cfg.MaxTotalSessions = 100
	pruner := NewPruner(manager, cfg)

	done := make(chan struct{})
	go func() {
		_ = pruner.Cleanup()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("cleanup appears to deadlock")
	}
}

func TestPrunerPruneMessagesByTTLShouldRemoveExpiredMessagesEvenWhenOutOfOrder(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	session, err := manager.GetOrCreate("ttl-out-of-order")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	now := time.Now()
	session.AddMessage(Message{
		Role:      "user",
		Content:   "new-1",
		Timestamp: now,
	})
	session.AddMessage(Message{
		Role:      "assistant",
		Content:   "old-middle",
		Timestamp: now.Add(-2 * time.Hour),
	})
	session.AddMessage(Message{
		Role:      "user",
		Content:   "new-2",
		Timestamp: now.Add(2 * time.Minute),
	})

	cfg := DefaultPruneConfig()
	cfg.DefaultMessageTTL = 30 * time.Minute
	pruner := NewPruner(manager, cfg)
	if err := pruner.PruneMessagesByTTL("ttl-out-of-order"); err != nil {
		t.Fatalf("failed to prune by ttl: %v", err)
	}

	if len(session.Messages) != 2 {
		t.Fatalf("expected expired message to be removed regardless of order, got %d messages", len(session.Messages))
	}
	if session.Messages[0].Content != "new-1" || session.Messages[1].Content != "new-2" {
		t.Fatalf("expected remaining messages [new-1 new-2], got [%s %s]", session.Messages[0].Content, session.Messages[1].Content)
	}
}
