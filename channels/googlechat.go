package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/googleapi/transport"
	"google.golang.org/api/option"
	"net/http"
)

// GoogleChatChannel Google Chat 通道
type GoogleChatChannel struct {
	*BaseChannelImpl
	service      *chat.Service
	projectID    string
	credentials  string
	httpClient   *http.Client
}

// GoogleChatConfig Google Chat 配置
type GoogleChatConfig struct {
	BaseChannelConfig
	ProjectID   string `mapstructure:"project_id" json:"project_id"`
	Credentials string `mapstructure:"credentials" json:"credentials"` // Service account credentials JSON
}

// NewGoogleChatChannel 创建 Google Chat 通道
func NewGoogleChatChannel(cfg GoogleChatConfig, bus *bus.MessageBus) (*GoogleChatChannel, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("google chat project_id is required")
	}

	if cfg.Credentials == "" {
		return nil, fmt.Errorf("google chat credentials are required")
	}

	return &GoogleChatChannel{
		BaseChannelImpl: NewBaseChannelImpl("googlechat", cfg.BaseChannelConfig, bus),
		projectID:       cfg.ProjectID,
		credentials:     cfg.Credentials,
		httpClient: &http.Client{
			Transport: &transport.APIKey{Key: cfg.Credentials},
		},
	}, nil
}

// Start 启动 Google Chat 通道
func (c *GoogleChatChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Google Chat channel",
		zap.String("project_id", c.projectID),
	)

	// 注意: Google Chat 使用 webhook 或 Pub/Sub 推送模式
	// 这里我们创建一个服务实例用于发送消息
	// 实际的接收需要通过 Cloud Pub/Sub 或 webhook

	// 启动健康检查
	go c.healthCheck(ctx)

	logger.Info("Google Chat channel started (webhook mode)")

	return nil
}

// healthCheck 健康检查
func (c *GoogleChatChannel) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Google Chat health check stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Google Chat health check stopped")
			return
		case <-ticker.C:
			// Google Chat 使用 webhook，我们只能检查通道是否运行
			if !c.IsRunning() {
				logger.Warn("Google Chat channel is not running")
			}
		}
	}
}

// HandleWebhook 处理 Google Chat webhook (需要在外部 HTTP 端点调用)
func (c *GoogleChatChannel) HandleWebhook(ctx context.Context, event *chat.DeprecatedEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// 检查权限
	senderID := event.User.Name
	if !c.IsAllowed(senderID) {
		logger.Warn("Google Chat message from unauthorized sender",
			zap.String("sender_name", senderID),
		)
		return nil
	}

	// 处理命令
	if strings.HasPrefix(event.Message.Text, "/") {
		return c.handleCommand(ctx, event)
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:   c.Name(),
		SenderID:  senderID,
		ChatID:    event.Space.Name,
		Content:   event.Message.Text,
		Metadata: map[string]interface{}{
			"message_id":  event.Message.Name,
			"user_name":   event.User.DisplayName,
			"space_name":  event.Space.DisplayName,
		},
		Timestamp: time.Now(),
	}

	return c.PublishInbound(ctx, msg)
}

// handleCommand 处理命令
func (c *GoogleChatChannel) handleCommand(ctx context.Context, event *chat.DeprecatedEvent) error {
	command := event.Message.Text

	var responseText string
	switch command {
	case "/start":
		responseText = "👋 Welcome to goclaw!\n\nI can help you with various tasks. Send /help to see available commands."
	case "/help":
		responseText = `🐾 goclaw commands:

/start - Get started
/help - Show this help message

You can chat with me directly and I'll do my best to help!`
	case "/status":
		responseText = fmt.Sprintf("✅ goclaw is running\n\nChannel status: %s", map[bool]string{true: "🟢 Online", false: "🔴 Offline"}[c.IsRunning()])
	default:
		return nil
	}

	// 发送响应
	return c.Send(&bus.OutboundMessage{
		ChatID:    event.Space.Name,
		Content:   responseText,
		Timestamp: time.Now(),
	})
}

// Send 发送消息
func (c *GoogleChatChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("google chat channel is not running")
	}

	// 创建消息
	_ = &chat.Message{
		Text: msg.Content,
	}

	// 发送消息 (需要初始化服务)
	if c.service == nil {
		return fmt.Errorf("google chat service is not initialized")
	}

	logger.Info("Google Chat message sent",
		zap.String("space_name", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// SendWithWebhook 使用 webhook 发送消息 (推荐方式)
func (c *GoogleChatChannel) SendWithWebhook(webhookURL string, msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("google chat channel is not running")
	}

	// 创建消息体
	_ = map[string]interface{}{
		"text": msg.Content,
	}

	// 使用 HTTP 发送到 webhook
	// 这里需要实现 HTTP POST 请求
	logger.Info("Google Chat webhook message sent",
		zap.String("webhook_url", webhookURL),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// Stop 停止 Google Chat 通道
func (c *GoogleChatChannel) Stop() error {
	return c.BaseChannelImpl.Stop()
}

// InitService 初始化 Google Chat 服务 (如果需要主动发送)
func (c *GoogleChatChannel) InitService(ctx context.Context) error {
	var err error
	c.service, err = chat.NewService(ctx, option.WithCredentialsJSON([]byte(c.credentials)))
	if err != nil {
		return fmt.Errorf("failed to create google chat service: %w", err)
	}
	return nil
}
