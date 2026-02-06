package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
)

// MessageTool 消息工具
type MessageTool struct {
	bus         *bus.MessageBus
	currentChan string
	currentChat string
}

// NewMessageTool 创建消息工具
func NewMessageTool(bus *bus.MessageBus) *MessageTool {
	return &MessageTool{
		bus: bus,
	}
}

// SetCurrent 设置当前通道和聊天
func (t *MessageTool) SetCurrent(channel, chatID string) {
	t.currentChan = channel
	t.currentChat = chatID
}

// SendMessage 发送消息
func (t *MessageTool) SendMessage(ctx context.Context, params map[string]interface{}) (string, error) {
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required")
	}

	// 获取目标通道
	channel := t.currentChan
	if ch, ok := params["channel"].(string); ok && ch != "" {
		channel = ch
	}

	chatID := t.currentChat
	if cid, ok := params["chat_id"].(string); ok && cid != "" {
		chatID = cid
	}

	if channel == "" || chatID == "" {
		return "", fmt.Errorf("channel and chat_id are required")
	}

	// 发送消息
	msg := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
	}

	if err := t.bus.PublishOutbound(ctx, msg); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return fmt.Sprintf("Message sent to %s:%s", channel, chatID), nil
}

// GetTools 获取所有消息工具
func (t *MessageTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"message",
			"Send a message to a chat channel",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Message content to send",
					},
					"channel": map[string]interface{}{
						"type":        "string",
						"description": "Target channel (default: current)",
					},
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "Target chat ID (default: current)",
					},
				},
				"required": []string{"content"},
			},
			t.SendMessage,
		),
	}
}
