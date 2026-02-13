package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

const (
	defaultQueueAckInterval = 3 * time.Second
	defaultSessionIdleTTL   = 10 * time.Minute
)

// inboundDispatcher routes inbound messages into per-session workers.
// Each session is processed serially, while different sessions can run concurrently.
type inboundDispatcher struct {
	manager *AgentManager

	ackInterval time.Duration
	idleTTL     time.Duration

	mu      sync.Mutex
	workers map[string]*inboundSessionWorker
}

func newInboundDispatcher(mgr *AgentManager) *inboundDispatcher {
	return &inboundDispatcher{
		manager:     mgr,
		ackInterval: defaultQueueAckInterval,
		idleTTL:     defaultSessionIdleTTL,
		workers:     make(map[string]*inboundSessionWorker),
	}
}

func (d *inboundDispatcher) Dispatch(ctx context.Context, msg *bus.InboundMessage) error {
	if d == nil || d.manager == nil {
		return fmt.Errorf("inbound dispatcher is not configured")
	}
	if msg == nil {
		return nil
	}

	sessionKey, _ := ResolveSessionKey(SessionKeyOptions{
		Channel:        msg.Channel,
		AccountID:      msg.AccountID,
		ChatID:         msg.ChatID,
		FreshOnDefault: true,
		Now:            msg.Timestamp,
	})
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return fmt.Errorf("resolved session key is empty")
	}

	worker := d.getOrCreateWorker(ctx, sessionKey)
	queued, ahead := worker.Enqueue(msg)

	if queued && shouldSendQueueAck(msg) && worker.TryAck(time.Now(), d.ackInterval) {
		d.manager.sendQueueAck(sessionKey, msg, ahead)
	}

	return nil
}

func (d *inboundDispatcher) getOrCreateWorker(ctx context.Context, sessionKey string) *inboundSessionWorker {
	sessionKey = strings.TrimSpace(sessionKey)

	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.workers[sessionKey]; ok && existing != nil {
		return existing
	}

	// Declare first so the OnStop closure can safely capture it.
	var worker *inboundSessionWorker
	worker = newInboundSessionWorker(inboundSessionWorkerOptions{
		Manager:    d.manager,
		SessionKey: sessionKey,
		IdleTTL:    d.idleTTL,
		OnStop: func(key string) {
			d.mu.Lock()
			defer d.mu.Unlock()
			// Only delete if the map still points to this worker.
			if cur, ok := d.workers[key]; ok && cur == worker {
				delete(d.workers, key)
			}
		},
	})
	d.workers[sessionKey] = worker

	go worker.Run(ctx)
	return worker
}

func shouldSendQueueAck(msg *bus.InboundMessage) bool {
	// Queue receipts are meant for user-originated messages.
	// Subagent announcements / internal injections typically have empty SenderID.
	if msg == nil {
		return false
	}
	if strings.TrimSpace(msg.SenderID) == "" {
		return false
	}
	if strings.TrimSpace(msg.Channel) == "" {
		return false
	}
	// Avoid noisy receipts for non-chat invocations.
	if strings.EqualFold(strings.TrimSpace(msg.Channel), "cli") {
		return false
	}
	return true
}

type inboundSessionWorkerOptions struct {
	Manager    *AgentManager
	SessionKey string
	IdleTTL    time.Duration
	OnStop     func(sessionKey string)
}

type inboundSessionWorker struct {
	manager    *AgentManager
	sessionKey string

	idleTTL time.Duration
	onStop  func(sessionKey string)

	mu    sync.Mutex
	queue []*bus.InboundMessage
	wake  chan struct{}

	busy       atomic.Bool
	lastAckNS  atomic.Int64
	lastActive atomic.Int64
	stopped    atomic.Bool
}

func newInboundSessionWorker(opts inboundSessionWorkerOptions) *inboundSessionWorker {
	ttl := opts.IdleTTL
	if ttl <= 0 {
		ttl = defaultSessionIdleTTL
	}
	w := &inboundSessionWorker{
		manager:    opts.Manager,
		sessionKey: strings.TrimSpace(opts.SessionKey),
		idleTTL:    ttl,
		onStop:     opts.OnStop,
		wake:       make(chan struct{}, 1),
	}
	w.lastActive.Store(time.Now().UnixNano())
	return w
}

// Enqueue pushes a message onto the session queue and returns whether it was queued behind existing work.
// ahead is a best-effort count of items ahead of the enqueued message (including in-flight work).
func (w *inboundSessionWorker) Enqueue(msg *bus.InboundMessage) (queued bool, ahead int) {
	if w == nil || msg == nil {
		return false, 0
	}

	now := time.Now()
	nowNS := now.UnixNano()

	inFlight := w.busy.Load()

	w.mu.Lock()
	// queued = already processing OR already has backlog (before enqueue)
	queued = inFlight || len(w.queue) > 0
	ahead = len(w.queue)
	if inFlight {
		ahead++
	}
	w.queue = append(w.queue, msg)
	w.mu.Unlock()

	w.lastActive.Store(nowNS)

	// Wake the worker without blocking.
	select {
	case w.wake <- struct{}{}:
	default:
	}

	return queued, ahead
}

func (w *inboundSessionWorker) TryAck(now time.Time, interval time.Duration) bool {
	if w == nil {
		return false
	}
	if interval <= 0 {
		interval = defaultQueueAckInterval
	}
	nowNS := now.UnixNano()
	minDelta := interval.Nanoseconds()
	for {
		last := w.lastAckNS.Load()
		if last != 0 && nowNS-last < minDelta {
			return false
		}
		if w.lastAckNS.CompareAndSwap(last, nowNS) {
			return true
		}
	}
}

func (w *inboundSessionWorker) Run(ctx context.Context) {
	if w == nil || w.manager == nil {
		return
	}
	if w.sessionKey == "" {
		return
	}
	defer w.stop()

	timer := time.NewTimer(w.idleTTL)
	defer timer.Stop()

	for {
		msg := w.dequeue()
		if msg == nil {
			// Wait for new work or idle TTL expiry.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.idleTTL)

			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			case <-timer.C:
				if w.isIdle() {
					return
				}
				continue
			}
		}

		w.busy.Store(true)
		if err := w.manager.RouteInbound(ctx, msg); err != nil {
			logger.Error("Failed to route inbound (session worker)",
				zap.String("session_key", w.sessionKey),
				zap.String("channel", msg.Channel),
				zap.String("account_id", msg.AccountID),
				zap.String("chat_id", msg.ChatID),
				zap.Error(err))
		}
		w.busy.Store(false)
		w.lastActive.Store(time.Now().UnixNano())
	}
}

func (w *inboundSessionWorker) dequeue() *bus.InboundMessage {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) == 0 {
		return nil
	}
	msg := w.queue[0]
	w.queue[0] = nil
	w.queue = w.queue[1:]
	return msg
}

func (w *inboundSessionWorker) isIdle() bool {
	if w == nil {
		return true
	}
	if w.busy.Load() {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.queue) == 0
}

func (w *inboundSessionWorker) stop() {
	if w == nil {
		return
	}
	if w.stopped.Swap(true) {
		return
	}
	if w.onStop != nil {
		w.onStop(w.sessionKey)
	}
}
