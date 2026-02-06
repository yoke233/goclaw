package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"go.uber.org/zap"
)

// Loop Agent 循环
type Loop struct {
	bus          *bus.MessageBus
	provider     providers.Provider
	sessionMgr   *session.Manager
	memory       *MemoryStore
	context      *ContextBuilder
	tools        *tools.Registry
	skillsLoader *SkillsLoader
	subagents    *SubagentManager
	workspace    string
	maxIteration int
	running      bool
}

// Config Loop 配置
type Config struct {
	Bus          *bus.MessageBus
	Provider     providers.Provider
	SessionMgr   *session.Manager
	Memory       *MemoryStore
	Context      *ContextBuilder
	Tools        *tools.Registry
	SkillsLoader *SkillsLoader
	Subagents    *SubagentManager
	Workspace    string
	MaxIteration int
}

// NewLoop 创建 Agent 循环
func NewLoop(cfg *Config) (*Loop, error) {
	if cfg.MaxIteration <= 0 {
		cfg.MaxIteration = 15
	}

	return &Loop{
		bus:          cfg.Bus,
		provider:     cfg.Provider,
		sessionMgr:   cfg.SessionMgr,
		memory:       cfg.Memory,
		context:      cfg.Context,
		tools:        cfg.Tools,
		skillsLoader: cfg.SkillsLoader,
		subagents:    cfg.Subagents,
		workspace:    cfg.Workspace,
		maxIteration: cfg.MaxIteration,
		running:      false,
	}, nil
}

// Start 启动 Agent 循环
func (l *Loop) Start(ctx context.Context) error {
	logger.Info("Starting agent loop")
	l.running = true

	// 启动出站消息分发
	go l.dispatchOutbound(ctx)

	// 主循环
	for l.running {
		select {
		case <-ctx.Done():
			logger.Info("Agent loop stopped by context")
			return ctx.Err()
		default:
			// 消费入站消息
			msg, err := l.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound message", zap.Error(err))
				continue
			}

			// 处理消息
			go l.processMessage(ctx, msg)
		}
	}

	return nil
}

// Stop 停止 Agent 循环
func (l *Loop) Stop() error {
	logger.Info("Stopping agent loop")
	l.running = false
	return nil
}

// processMessage 处理消息
func (l *Loop) processMessage(ctx context.Context, msg *bus.InboundMessage) {
	logger.Info("Processing message",
		zap.String("channel", msg.Channel),
		zap.String("chat_id", msg.ChatID),
	)

	// 检查是否为系统消息
	if msg.IsSystemMessage() {
		l.processSystemMessage(ctx, msg)
		return
	}

	// 获取或创建会话
	sess, err := l.sessionMgr.GetOrCreate(msg.SessionKey())
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return
	}

	// 添加用户消息到会话
	var media []session.Media
	for _, m := range msg.Media {
		media = append(media, session.Media{
			Type:     m.Type,
			URL:      m.URL,
			Base64:   m.Base64,
			MimeType: m.MimeType,
		})
	}

	sess.AddMessage(session.Message{
		Role:      "user",
		Content:   msg.Content,
		Media:     media,
		Timestamp: msg.Timestamp,
	})

	// 运行 Agent 迭代
	response, err := l.runIteration(ctx, sess)
	if err != nil {
		logger.Error("Agent iteration failed", zap.Error(err))

		// 发送错误消息
		_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
			Channel:   msg.Channel,
			ChatID:    msg.ChatID,
			Content:   fmt.Sprintf("抱歉，处理您的请求时出错：%v", err),
			Timestamp: time.Now(),
		})
		return
	}

	// 发送响应
	_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   msg.Channel,
		ChatID:    msg.ChatID,
		Content:   response,
		Timestamp: time.Now(),
	})

	// 添加助手响应到会话
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	// 保存会话
	if err := l.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
	}
}

// processSystemMessage 处理系统消息
func (l *Loop) processSystemMessage(ctx context.Context, msg *bus.InboundMessage) {
	logger.Info("Processing system message",
		zap.String("task_id", msg.Metadata["task_id"].(string)),
	)

	// 从元数据中获取原始频道和聊天ID
	originChannel, _ := msg.Metadata["origin_channel"].(string)
	originChatID, _ := msg.Metadata["origin_chat_id"].(string)

	if originChannel == "" || originChatID == "" {
		logger.Warn("System message missing origin info")
		return
	}

	// 获取会话
	sess, err := l.sessionMgr.GetOrCreate(originChannel + ":" + originChatID)
	if err != nil {
		logger.Error("Failed to get session for system message", zap.Error(err))
		return
	}

	// 生成总结
	summary := l.generateSummary(ctx, msg)

	// 发送总结
	_ = l.bus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   originChannel,
		ChatID:    originChatID,
		Content:   summary,
		Timestamp: time.Now(),
	})

	// 添加到会话
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   summary,
		Timestamp: time.Now(),
	})

	// 保存会话
	if err := l.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session after system message", zap.Error(err))
	}
}

// runIteration 运行 Agent 迭代
func (l *Loop) runIteration(ctx context.Context, sess *session.Session) (string, error) {
	iteration := 0
	var lastResponse string

	for iteration < l.maxIteration {
		iteration++

		logger.Info("Agent iteration", zap.Int("iteration", iteration))

		// 获取可用技能
		var skills []*Skill
		if l.skillsLoader != nil {
			skills = l.skillsLoader.List()
		}

		// 构建上下文
		history := sess.GetHistory(50)
		messages := l.context.BuildMessages(history, "", skills)

		providerMessages := make([]providers.Message, len(messages))
		for i, msg := range messages {
			var tcs []providers.ToolCall
			for _, tc := range msg.ToolCalls {
				tcs = append(tcs, providers.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
			}
			providerMessages[i] = providers.Message{
				Role:       msg.Role,
				Content:    msg.Content,
				Images:     msg.Images,
				ToolCallID: msg.ToolCallID,
				ToolCalls:  tcs,
			}
		}

		// 准备工具定义
		var toolDefs []providers.ToolDefinition
		if l.tools != nil {
			toolList := l.tools.List()
			for _, t := range toolList {
				toolDefs = append(toolDefs, providers.ToolDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				})
			}
		}

		// 调用 LLM
		response, err := l.provider.Chat(ctx, providerMessages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// 检查是否有工具调用
		if len(response.ToolCalls) > 0 {
			// 重要：必须先把带有工具调用的助手消息存入历史记录
			var assistantToolCalls []session.ToolCall
			for _, tc := range response.ToolCalls {
				assistantToolCalls = append(assistantToolCalls, session.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
			}
			sess.AddMessage(session.Message{
				Role:      "assistant",
				Content:   response.Content,
				Timestamp: time.Now(),
				ToolCalls: assistantToolCalls,
			})

			// 执行工具调用
			for _, tc := range response.ToolCalls {
				result, err := l.tools.Execute(ctx, tc.Name, tc.Params)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}

				// 添加工具结果到会话
				sess.AddMessage(session.Message{
					Role:       "tool",
					Content:    result,
					Timestamp:  time.Now(),
					ToolCallID: tc.ID,
					Metadata: map[string]interface{}{
						"tool_name": tc.Name,
					},
				})
			}

			// 继续下一次迭代
			continue
		}

		// 没有工具调用，返回响应
		lastResponse = response.Content
		break
	}

	if iteration >= l.maxIteration {
		logger.Warn("Agent reached max iterations", zap.Int("max", l.maxIteration))
	}

	return lastResponse, nil
}

// generateSummary 生成子代理结果的总结
func (l *Loop) generateSummary(ctx context.Context, msg *bus.InboundMessage) string {
	// 简单实现：直接返回内容
	// 实际应该调用 LLM 生成更友好的总结
	return fmt.Sprintf("任务完成：%s", msg.Content)
}

// dispatchOutbound 分发出站消息
func (l *Loop) dispatchOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := l.bus.ConsumeOutbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume outbound message", zap.Error(err))
				continue
			}

			logger.Info("Dispatching outbound message",
				zap.String("channel", msg.Channel),
				zap.String("chat_id", msg.ChatID),
			)

			// 这里应该根据 channel 调用对应的通道发送器
			// 暂时只记录日志
		}
	}
}
