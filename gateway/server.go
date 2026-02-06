package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Server HTTP 网关服务器
type Server struct {
	config      *config.GatewayConfig
	bus         *bus.MessageBus
	channelMgr  *channels.Manager
	server      *http.Server
	mu          sync.RWMutex
	running     bool
}

// NewServer 创建网关服务器
func NewServer(cfg *config.GatewayConfig, messageBus *bus.MessageBus, channelMgr *channels.Manager) *Server {
	return &Server{
		config:     cfg,
		bus:        messageBus,
		channelMgr: channelMgr,
	}
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// 创建 HTTP 路由
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)

	// 飞书 webhook 端点
	mux.HandleFunc("/webhook/feishu", s.handleFeishuWebhook)

	// 通用 webhook 端点
	mux.HandleFunc("/webhook/", s.handleGenericWebhook)

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}

	// 启动服务器
	go func() {
		logger.Info("Gateway server started",
			zap.String("addr", s.server.Addr),
		)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Gateway server error", zap.Error(err))
		}
	}()

	// 监听上下文取消
	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown gateway server", zap.Error(err))
			return err
		}

		logger.Info("Gateway server stopped")
	}

	return nil
}

// IsRunning 检查是否运行中
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// handleHealth 健康检查处理器
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// handleFeishuWebhook 飞书 webhook 处理器
func (s *Server) handleFeishuWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取飞书通道
	_, ok := s.channelMgr.Get("feishu")
	if !ok {
		http.Error(w, "Feishu channel not found", http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// 验证签名（由通道处理）
	// 这里我们需要将请求传递给飞书通道处理
	// 由于接口限制，我们暂时记录日志

	logger.Info("Received Feishu webhook",
		zap.Int("content_length", len(body)),
	)

	// 将事件发布到消息总线（由飞书通道解析）
	// 这里简化处理，实际应该由飞书通道解析并发布

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleGenericWebhook 通用 webhook 处理器
func (s *Server) handleGenericWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 路径提取通道名称
	// /webhook/{channel}
	channelName := r.URL.Path[len("/webhook/"):]
	if channelName == "" {
		http.Error(w, "Channel not specified", http.StatusBadRequest)
		return
	}

	// 获取通道
	_, ok := s.channelMgr.Get(channelName)
	if !ok {
		http.Error(w, fmt.Sprintf("Channel %s not found", channelName), http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body",
			zap.String("channel", channelName),
			zap.Error(err),
		)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	logger.Info("Received webhook",
		zap.String("channel", channelName),
		zap.Int("content_length", len(body)),
	)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
