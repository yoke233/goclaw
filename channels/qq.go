package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// QQChannel QQ 通道 (基于 OneBot V11)
type QQChannel struct {
	*BaseChannelImpl
	wsURL       string
	accessToken string
	conn        *websocket.Conn
	mu          sync.Mutex
	apiCallbacks map[string]chan map[string]interface{}
	callbackMu   sync.Mutex
}

// NewQQChannel 创建 QQ 通道
func NewQQChannel(cfg config.QQChannelConfig, bus *bus.MessageBus) (*QQChannel, error) {
	if cfg.WSURL == "" {
		return nil, fmt.Errorf("qq ws_url is required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &QQChannel{
		BaseChannelImpl: NewBaseChannelImpl("qq", baseCfg, bus),
		wsURL:           cfg.WSURL,
		accessToken:     cfg.AccessToken,
		apiCallbacks:    make(map[string]chan map[string]interface{}),
	}, nil
}

// Start 启动 QQ 通道
func (c *QQChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting QQ channel", zap.String("ws_url", c.wsURL))

	go c.connectAndListen(ctx)

	return nil
}

// connectAndListen 连接 WebSocket 并监听消息
func (c *QQChannel) connectAndListen(ctx context.Context) {
	reconnectDelay := 1 * time.Second
	maxReconnectDelay := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.WaitForStop():
			return
		default:
			if err := c.connect(); err != nil {
				logger.Error("Failed to connect to QQ WebSocket", zap.Error(err))
				select {
				case <-time.After(reconnectDelay):
					// 指数退避
					reconnectDelay = reconnectDelay * 2
					if reconnectDelay > maxReconnectDelay {
						reconnectDelay = maxReconnectDelay
					}
					continue
				case <-ctx.Done():
					return
				}
			}

			// 重置重连延迟
			reconnectDelay = 1 * time.Second

			// 读取循环
			c.readLoop(ctx)

			// 连接断开，清理资源
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
			}
			c.mu.Unlock()
		}
	}
}

func (c *QQChannel) connect() error {
	header := http.Header{}
	if c.accessToken != "" {
		header.Set("Authorization", "Bearer "+c.accessToken)
	}

	conn, _, err := websocket.DefaultDialer.Dial(c.wsURL, header)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	logger.Info("Connected to QQ OneBot")
	return nil
}

func (c *QQChannel) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 设置读取超时，避免永久阻塞
			c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.Info("QQ WebSocket connection closed normally")
				} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					logger.Debug("WebSocket read timeout, checking context...")
					continue
				} else {
					logger.Error("QQ WebSocket read error", zap.Error(err))
				}
				return
			}

			var data map[string]interface{}
			if err := json.Unmarshal(message, &data); err != nil {
				logger.Warn("Invalid JSON from QQ WebSocket", zap.Error(err))
				continue
			}

			// 处理 API 响应
			if echo, ok := data["echo"].(string); ok {
				c.callbackMu.Lock()
				if ch, ok := c.apiCallbacks[echo]; ok {
					select {
					case ch <- data:
					case <-time.After(5 * time.Second):
						logger.Warn("Callback channel timeout", zap.String("echo", echo))
					}
					close(ch)
					delete(c.apiCallbacks, echo)
				}
				c.callbackMu.Unlock()
				continue
			}

			// 处理事件
			postType, _ := data["post_type"].(string)
			if postType == "message" {
				c.handleMessage(data)
			}
		}
	}
}

var cqCodeRegex = regexp.MustCompile(`\[CQ:([^,\]]+),?([^\]]*)\]`)

// parseCQCode 解析 CQ 码
func parseCQCode(message string) string {
	return cqCodeRegex.ReplaceAllStringFunc(message, func(match string) string {
		submatch := cqCodeRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		cqType := submatch[1]
		params := submatch[2]

		switch cqType {
		case "at":
			// [CQ:at,qq=123] -> @123
			re := regexp.MustCompile(`qq=(\d+)`)
			if m := re.FindStringSubmatch(params); len(m) > 1 {
				return "@" + m[1]
			}
		case "image":
			return "[图片]"
		case "face":
			return "[表情]"
		case "record":
			return "[语音]"
		case "video":
			return "[视频]"
		case "file":
			return "[文件]"
		case "share":
			return "[链接分享]"
		}
		return "[" + cqType + "]"
	})
}

func (c *QQChannel) handleMessage(data map[string]interface{}) {
	// 安全解析 user_id
	userIDFloat, ok := data["user_id"].(float64)
	if !ok {
		logger.Warn("Invalid user_id type in QQ message")
		return
	}
	userID := fmt.Sprintf("%.0f", userIDFloat)

	// 检查权限
	if !c.IsAllowed(userID) {
		return
	}

	// 解析发送者
	sender, _ := data["sender"].(map[string]interface{})
	senderName := ""
	if sender != nil {
		if name, ok := sender["nickname"].(string); ok {
			senderName = name
		}
	}

	// 解析消息类型
	messageType, _ := data["message_type"].(string)
	rawMessage, _ := data["message"].(string)

	chatID := userID
	chatType := "private"
	if messageType == "group" {
		groupIDFloat, ok := data["group_id"].(float64)
		if !ok {
			logger.Warn("Invalid group_id type in QQ message")
			return
		}
		chatID = fmt.Sprintf("%.0f", groupIDFloat)
		chatType = "group"
	}

	// 解析消息 ID
	messageIDFloat, ok := data["message_id"].(float64)
	if !ok {
		logger.Warn("Invalid message_id type in QQ message")
		return
	}
	messageID := fmt.Sprintf("%.0f", messageIDFloat)

	// 处理 CQ 码
	content := parseCQCode(rawMessage)

	msg := &bus.InboundMessage{
		ID:        messageID,
		Content:   content,
		SenderID:  userID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"sender_name": senderName,
			"chat_type":   chatType,
			"raw_message": rawMessage,
		},
	}

	c.PublishInbound(context.Background(), msg)
}

// Send 发送消息
func (c *QQChannel) Send(msg *bus.OutboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("qq channel not connected")
	}

	payload := map[string]interface{}{
		"action": "send_msg",
		"params": map[string]interface{}{
			"message": msg.Content,
		},
	}
	
	// 判断是群还是私聊
	// 这里通过 ChatID 的特征或元数据判断，或者尝试两个都发
	// 简单起见，如果 msg.Metadata["chat_type"] 存在则使用，否则尝试推断
	
	chatType, ok := msg.Metadata["chat_type"].(string)
	if !ok {
		// 默认私聊
		payload["params"].(map[string]interface{})["user_id"] = msg.ChatID
	} else if chatType == "group" {
		payload["params"].(map[string]interface{})["group_id"] = msg.ChatID
	} else {
		payload["params"].(map[string]interface{})["user_id"] = msg.ChatID
	}

	return c.conn.WriteJSON(payload)
}

// Stop 停止 QQ 通道
func (c *QQChannel) Stop() error {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
	return c.BaseChannelImpl.Stop()
}
