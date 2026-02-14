package bus

import (
	"context"
	"testing"
	"time"
)

func TestMessageBusCloseNotBlockedByPendingConsumeInbound(t *testing.T) {
	b := NewMessageBus(1)

	consumeStarted := make(chan struct{})
	go func() {
		close(consumeStarted)
		_, _ = b.ConsumeInbound(context.Background())
	}()
	<-consumeStarted
	time.Sleep(20 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		_ = b.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("Close is blocked by pending ConsumeInbound")
	}
}

func TestPublishInboundSetsIDAndTimestamp(t *testing.T) {
	b := NewMessageBus(2)
	defer func() { _ = b.Close() }()

	msg := &InboundMessage{
		Channel: "cli",
		ChatID:  "c1",
		Content: "hello",
	}
	if err := b.PublishInbound(context.Background(), msg); err != nil {
		t.Fatalf("publish inbound failed: %v", err)
	}
	if msg.ID == "" {
		t.Fatalf("expected publish to set message ID")
	}
	if msg.Timestamp.IsZero() {
		t.Fatalf("expected publish to set timestamp")
	}
}

func TestSubscribeOutboundAfterCloseShouldNotReturnActiveChannel(t *testing.T) {
	b := NewMessageBus(1)
	_ = b.Close()

	sub := b.SubscribeOutbound()

	select {
	case _, ok := <-sub.Channel:
		if ok {
			t.Fatalf("expected subscription channel to be closed after bus close")
		}
	default:
		t.Fatalf("expected closed subscription channel after bus close")
	}
}

func TestPublishInboundNilMessageShouldNotPanic(t *testing.T) {
	b := NewMessageBus(1)
	defer func() { _ = b.Close() }()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("publish inbound with nil message should return error, got panic: %v", r)
		}
	}()

	if err := b.PublishInbound(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil message")
	}
}

func TestConsumeOutboundCanReadPreviouslyPublishedMessage(t *testing.T) {
	b := NewMessageBus(2)
	defer func() { _ = b.Close() }()

	msg := &OutboundMessage{
		Channel: "telegram",
		ChatID:  "chat-1",
		Content: "hello",
	}
	if err := b.PublishOutbound(context.Background(), msg); err != nil {
		t.Fatalf("publish outbound failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	got, err := b.ConsumeOutbound(ctx)
	if err != nil {
		t.Fatalf("expected to consume previously published message, got error: %v", err)
	}
	if got == nil || got.Content != "hello" {
		t.Fatalf("unexpected consumed message: %+v", got)
	}
}
