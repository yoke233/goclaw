package channels

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// DingTalkChannel DingTalk 通道实现
type DingTalkChannel struct {
	*BaseChannelImpl
	config       config.DingTalkChannelConfig
	clientID     string
	clientSecret string
	streamClient *client.StreamClient
	ctx          context.Context
	cancel       context.CancelFunc
	// Map to store session webhooks for each chat
	sessionWebhooks      sync.Map // chatID -> sessionWebhookEntry
	sessionWebhookSize   int64
	sessionWebhookTrimMu sync.Mutex
}

const (
	dingtalkSessionWebhookTTL             = 24 * time.Hour
	dingtalkSessionWebhookMaxEntries      = 5000
	dingtalkSessionWebhookCleanupInterval = 10 * time.Minute
)

type sessionWebhookEntry struct {
	Webhook   string
	UpdatedAt time.Time
}

// NewDingTalkChannel 创建 DingTalk 通道实例
func NewDingTalkChannel(cfg config.DingTalkChannelConfig, bus *bus.MessageBus) (*DingTalkChannel, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("dingtalk client_id and client_secret are required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &DingTalkChannel{
		BaseChannelImpl: NewBaseChannelImpl("dingtalk", "default", baseCfg, bus),
		config:          cfg,
		clientID:        cfg.ClientID,
		clientSecret:    cfg.ClientSecret,
	}, nil
}

// Start 启动 DingTalk 通道 (Stream Mode)
func (c *DingTalkChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting DingTalk channel (Stream Mode)...",
		zap.String("client_id", c.clientID))

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create credential config
	cred := client.NewAppCredentialConfig(c.clientID, c.clientSecret)

	// Create stream client with options
	c.streamClient = client.NewStreamClient(
		client.WithAppCredential(cred),
		client.WithAutoReconnect(true),
	)

	// Register chatbot callback handler
	c.streamClient.RegisterChatBotCallbackRouter(c.onChatBotMessageReceived)

	// Start stream client
	if err := c.streamClient.Start(c.ctx); err != nil {
		return fmt.Errorf("failed to start stream client: %w", err)
	}

	go c.runSessionWebhookJanitor(c.ctx)

	logger.Info("DingTalk channel started (Stream Mode)")
	return nil
}

// Stop 停止 DingTalk 通道
func (c *DingTalkChannel) Stop() error {
	logger.Info("Stopping DingTalk channel...")

	if c.cancel != nil {
		c.cancel()
	}

	if c.streamClient != nil {
		c.streamClient.Close()
	}

	c.clearSessionWebhookCache()

	if err := c.BaseChannelImpl.Stop(); err != nil {
		return err
	}

	logger.Info("DingTalk channel stopped")
	return nil
}

// Send 发送消息到 DingTalk
func (c *DingTalkChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("dingtalk channel not running")
	}

	sessionWebhook, ok := c.getSessionWebhook(msg.ChatID, time.Now())
	if !ok {
		return fmt.Errorf("no session_webhook found for chat %s, cannot send message", msg.ChatID)
	}

	logger.Info("DingTalk message to send",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)))

	// Use session webhook to send reply
	return c.SendDirectReply(sessionWebhook, msg.Content)
}

// SendStream 发送流式消息 (DingTalk 不支持，收集后一次性发送)
func (c *DingTalkChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	var content string

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content += msg.Content
		}

		if msg.IsComplete {
			// Send complete message
			outMsg := &bus.OutboundMessage{
				Channel:   c.Name(),
				ChatID:    chatID,
				Content:   content,
				Timestamp: time.Now(),
			}
			return c.Send(outMsg)
		}
	}

	return nil
}

// onChatBotMessageReceived 处理 DingTalk 机器人消息
func (c *DingTalkChannel) onChatBotMessageReceived(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	// Extract message content from Text field
	content := data.Text.Content
	if content == "" {
		// Try to extract from Content interface{} if Text is empty
		if contentMap, ok := data.Content.(map[string]interface{}); ok {
			if textContent, ok := contentMap["content"].(string); ok {
				content = textContent
			}
		}
	}

	if content == "" {
		return nil, nil // Ignore empty messages
	}

	senderID := data.SenderStaffId
	senderNick := data.SenderNick
	chatID := senderID
	if data.ConversationType != "1" {
		// For group chats (ConversationType: "1" = private, "2" = group)
		chatID = data.ConversationId
	}

	// Check if sender is allowed
	if !c.IsAllowed(senderID) {
		logger.Debug("DingTalk message from unauthorized sender, ignoring",
			zap.String("sender_id", senderID),
			zap.String("sender_nick", senderNick))
		return nil, nil
	}

	now := time.Now()
	// Store session webhook for this chat so we can reply later
	c.setSessionWebhook(chatID, data.SessionWebhook, now)

	logger.Info("DingTalk message received",
		zap.String("sender_nick", senderNick),
		zap.String("sender_id", senderID),
		zap.String("chat_id", chatID),
		zap.String("conversation_type", data.ConversationType),
		zap.Int("content_length", len(content)))

	// Build inbound message
	msg := &bus.InboundMessage{
		Content:   content,
		SenderID:  senderID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: now,
		Metadata: map[string]interface{}{
			"sender_name":       senderNick,
			"conversation_id":   data.ConversationId,
			"conversation_type": data.ConversationType,
			"platform":          "dingtalk",
			"session_webhook":   data.SessionWebhook,
		},
	}

	// Publish inbound message
	_ = c.PublishInbound(ctx, msg)

	// Return nil to indicate we've handled message asynchronously
	return nil, nil
}

func (c *DingTalkChannel) runSessionWebhookJanitor(ctx context.Context) {
	ticker := time.NewTicker(dingtalkSessionWebhookCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.trimSessionWebhookCache(time.Now())
		}
	}
}

func (c *DingTalkChannel) clearSessionWebhookCache() {
	c.sessionWebhooks.Range(func(key, _ interface{}) bool {
		c.sessionWebhooks.Delete(key)
		return true
	})
	atomic.StoreInt64(&c.sessionWebhookSize, 0)
}

func (c *DingTalkChannel) setSessionWebhook(chatID, sessionWebhook string, now time.Time) {
	if chatID == "" || sessionWebhook == "" {
		return
	}
	if _, loaded := c.sessionWebhooks.Load(chatID); !loaded {
		atomic.AddInt64(&c.sessionWebhookSize, 1)
	}
	c.sessionWebhooks.Store(chatID, sessionWebhookEntry{
		Webhook:   sessionWebhook,
		UpdatedAt: now,
	})

	if atomic.LoadInt64(&c.sessionWebhookSize) > dingtalkSessionWebhookMaxEntries {
		c.trimSessionWebhookCache(now)
	}
}

func (c *DingTalkChannel) getSessionWebhook(chatID string, now time.Time) (string, bool) {
	raw, ok := c.sessionWebhooks.Load(chatID)
	if !ok {
		return "", false
	}

	switch entry := raw.(type) {
	case sessionWebhookEntry:
		if entry.Webhook == "" || now.Sub(entry.UpdatedAt) > dingtalkSessionWebhookTTL {
			c.deleteSessionWebhook(chatID)
			return "", false
		}
		return entry.Webhook, true
	case string:
		if entry == "" {
			c.deleteSessionWebhook(chatID)
			return "", false
		}
		c.sessionWebhooks.Store(chatID, sessionWebhookEntry{
			Webhook:   entry,
			UpdatedAt: now,
		})
		return entry, true
	default:
		c.deleteSessionWebhook(chatID)
		return "", false
	}
}

func (c *DingTalkChannel) deleteSessionWebhook(chatID string) {
	if _, loaded := c.sessionWebhooks.LoadAndDelete(chatID); loaded {
		size := atomic.AddInt64(&c.sessionWebhookSize, -1)
		if size < 0 {
			atomic.StoreInt64(&c.sessionWebhookSize, 0)
		}
	}
}

func (c *DingTalkChannel) trimSessionWebhookCache(now time.Time) {
	c.sessionWebhookTrimMu.Lock()
	defer c.sessionWebhookTrimMu.Unlock()

	type webhookItem struct {
		chatID    string
		updatedAt time.Time
	}

	active := make([]webhookItem, 0)

	c.sessionWebhooks.Range(func(key, value interface{}) bool {
		chatID, ok := key.(string)
		if !ok {
			c.sessionWebhooks.Delete(key)
			return true
		}

		var entry sessionWebhookEntry
		switch data := value.(type) {
		case sessionWebhookEntry:
			entry = data
		case string:
			if data == "" {
				c.deleteSessionWebhook(chatID)
				return true
			}
			entry = sessionWebhookEntry{
				Webhook:   data,
				UpdatedAt: now,
			}
			c.sessionWebhooks.Store(chatID, entry)
		default:
			c.deleteSessionWebhook(chatID)
			return true
		}

		if entry.Webhook == "" || now.Sub(entry.UpdatedAt) > dingtalkSessionWebhookTTL {
			c.deleteSessionWebhook(chatID)
			return true
		}

		active = append(active, webhookItem{
			chatID:    chatID,
			updatedAt: entry.UpdatedAt,
		})
		return true
	})

	if len(active) > dingtalkSessionWebhookMaxEntries {
		sort.Slice(active, func(i, j int) bool {
			return active[i].updatedAt.Before(active[j].updatedAt)
		})

		overflow := len(active) - dingtalkSessionWebhookMaxEntries
		for i := 0; i < overflow; i++ {
			c.deleteSessionWebhook(active[i].chatID)
		}

		logger.Warn("Trimmed DingTalk session webhook cache",
			zap.Int("removed_entries", overflow),
			zap.Int("max_entries", dingtalkSessionWebhookMaxEntries))
	}

	var actualSize int64
	c.sessionWebhooks.Range(func(_, _ interface{}) bool {
		actualSize++
		return true
	})
	atomic.StoreInt64(&c.sessionWebhookSize, actualSize)
}

// SendDirectReply 使用 session webhook 发送直接回复
func (c *DingTalkChannel) SendDirectReply(sessionWebhook, content string) error {
	replier := chatbot.NewChatbotReplier()

	// Convert string content to []byte for API
	contentBytes := []byte(content)
	titleBytes := []byte("GoClaw")

	// Send markdown formatted reply
	err := replier.SimpleReplyMarkdown(
		context.Background(),
		sessionWebhook,
		titleBytes,
		contentBytes,
	)

	if err != nil {
		logger.Error("Failed to send DingTalk reply",
			zap.Error(err))
		return fmt.Errorf("failed to send reply: %w", err)
	}

	return nil
}
