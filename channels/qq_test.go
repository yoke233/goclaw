package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
)

func TestParseCQCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "at code",
			input:    "[CQ:at,qq=123456]",
			expected: "@123456",
		},
		{
			name:     "image",
			input:    "[CQ:image,file=test.jpg]",
			expected: "[图片]",
		},
		{
			name:     "face",
			input:    "[CQ:face,id=1]",
			expected: "[表情]",
		},
		{
			name:     "record",
			input:    "[CQ:record,file=test.amr]",
			expected: "[语音]",
		},
		{
			name:     "video",
			input:    "[CQ:video,file=test.mp4]",
			expected: "[视频]",
		},
		{
			name:     "file",
			input:    "[CQ:file,file=test.pdf]",
			expected: "[文件]",
		},
		{
			name:     "share",
			input:    "[CQ:share,url=https://example.com]",
			expected: "[链接分享]",
		},
		{
			name:     "unknown type",
			input:    "[CQ:unknown,param=value]",
			expected: "[unknown]",
		},
		{
			name:     "mixed text",
			input:    "Hello [CQ:at,qq=123] world",
			expected: "Hello @123 world",
		},
		{
			name:     "plain text",
			input:    "just plain text",
			expected: "just plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCQCode(tt.input)
			if result != tt.expected {
				t.Errorf("parseCQCode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNewQQChannel(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.QQChannelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: config.QQChannelConfig{
				Enabled:    true,
				WSURL:      "ws://localhost:3000",
				AccessToken: "test-token",
				AllowedIDs: []string{"123456"},
			},
			wantErr: false,
		},
		{
			name: "missing ws_url",
			cfg: config.QQChannelConfig{
				Enabled:    true,
				WSURL:      "",
				AccessToken: "test-token",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bus.NewMessageBus(10)
			defer b.Close()

			ch, err := NewQQChannel(tt.cfg, b)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewQQChannel() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && ch == nil {
				t.Error("Expected non-nil channel")
			}
		})
	}
}

func TestQQChannelHandleMessage(t *testing.T) {
	b := bus.NewMessageBus(10)
	defer b.Close()

	cfg := config.QQChannelConfig{
		Enabled:    true,
		WSURL:      "ws://localhost:3000",
		AccessToken: "test-token",
		AllowedIDs: []string{"123456"},
	}

	ch, err := NewQQChannel(cfg, b)
	if err != nil {
		t.Fatalf("NewQQChannel() error = %v", err)
	}

	// 准备测试消息
	msgData := map[string]interface{}{
		"post_type":     "message",
		"message_type":  "private",
		"message_id":    float64(123),
		"user_id":       float64(123456),
		"message":       "hello [CQ:at,qq=789]",
		"sender": map[string]interface{}{
			"nickname": "TestUser",
		},
	}

	// 由于 handleMessage 使用了通道发布，我们需要订阅入站消息
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		ch.handleMessage(msgData)
	}()

	// 尝试接收消息
	msg, err := b.ConsumeInbound(ctx)
	if err != nil {
		t.Fatalf("Failed to consume inbound message: %v", err)
	}

	if msg.Channel != "qq" {
		t.Errorf("Expected channel 'qq', got '%s'", msg.Channel)
	}

	if msg.SenderID != "123456" {
		t.Errorf("Expected sender_id '123456', got '%s'", msg.SenderID)
	}

	if !strings.Contains(msg.Content, "@789") {
		t.Errorf("Expected content to contain '@789', got '%s'", msg.Content)
	}
}

func TestNewWeWorkChannel(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.WeWorkChannelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: config.WeWorkChannelConfig{
				Enabled:        true,
				CorpID:         "test-corp",
				AgentID:        "test-agent",
				Secret:         "test-secret",
				Token:          "test-token",
				EncodingAESKey: "test-key",
				WebhookPort:    8766,
				AllowedIDs:     []string{"user1"},
			},
			wantErr: false,
		},
		{
			name: "missing corp_id",
			cfg: config.WeWorkChannelConfig{
				Enabled: true,
				CorpID:  "",
				AgentID: "test-agent",
				Secret:  "test-secret",
			},
			wantErr: true,
		},
		{
			name: "missing secret",
			cfg: config.WeWorkChannelConfig{
				Enabled: true,
				CorpID:  "test-corp",
				AgentID: "test-agent",
				Secret:  "",
			},
			wantErr: true,
		},
		{
			name: "missing agent_id",
			cfg: config.WeWorkChannelConfig{
				Enabled: true,
				CorpID:  "test-corp",
				Secret:  "test-secret",
				AgentID: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bus.NewMessageBus(10)
			defer b.Close()

			ch, err := NewWeWorkChannel(tt.cfg, b)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWeWorkChannel() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && ch == nil {
				t.Error("Expected non-nil channel")
			}

			if !tt.wantErr && ch != nil {
				// 检查 HTTP 客户端是否已初始化
				if ch.httpClient == nil {
					t.Error("Expected httpClient to be initialized")
				}
			}
		})
	}
}

func TestWeWorkChannelVerifySignature(t *testing.T) {
	cfg := config.WeWorkChannelConfig{
		Enabled:        true,
		CorpID:         "test-corp",
		AgentID:        "test-agent",
		Secret:         "test-secret",
		Token:          "test-token",
		EncodingAESKey: "test-key" + strings.Repeat("a", 23), // 43 chars total
		WebhookPort:    8766,
		AllowedIDs:     []string{"user1"},
	}

	b := bus.NewMessageBus(10)
	defer b.Close()

	ch, err := NewWeWorkChannel(cfg, b)
	if err != nil {
		t.Fatalf("NewWeWorkChannel() error = %v", err)
	}

	// 测试签名验证
	validSig := ch.computeSignature("test-token", "1234567890", "random-nonce", "test-data")

	tests := []struct {
		name      string
		token     string
		timestamp string
		nonce     string
		data      string
		signature string
		expected  bool
	}{
		{
			name:      "valid signature",
			token:     "test-token",
			timestamp: "1234567890",
			nonce:     "random-nonce",
			data:      "test-data",
			signature: validSig,
			expected:  true,
		},
		{
			name:      "invalid signature",
			token:     "test-token",
			timestamp: "1234567890",
			nonce:     "random-nonce",
			data:      "test-data",
			signature: "invalid-signature",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ch.verifySignature(tt.token, tt.timestamp, tt.nonce, tt.data, tt.signature)
			if result != tt.expected {
				t.Errorf("verifySignature() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWeWorkChannelHandleWebhook(t *testing.T) {
	cfg := config.WeWorkChannelConfig{
		Enabled:        true,
		CorpID:         "test-corp",
		AgentID:        "test-agent",
		Secret:         "test-secret",
		Token:          "test-token",
		EncodingAESKey: "", // 不使用加密模式以便测试
		WebhookPort:    8766,
		AllowedIDs:     []string{"user1"},
	}

	b := bus.NewMessageBus(10)
	defer b.Close()

	ch, err := NewWeWorkChannel(cfg, b)
	if err != nil {
		t.Fatalf("NewWeWorkChannel() error = %v", err)
	}

	// 测试 POST 请求 - 当没有 Encrypt 字段时（明文模式）
	xmlBody := `<xml>
		<ToUserName>test</ToUserName>
		<FromUserName>user1</FromUserName>
		<CreateTime>1234567890</CreateTime>
		<MsgType>text</MsgType>
		<Content>Hello</Content>
		<MsgId>msg123</MsgId>
		<AgentID>test-agent</AgentID>
	</xml>`

	req := httptest.NewRequest("POST", "/wework/event?msg_signature=sign&timestamp=123&nonce=456", strings.NewReader(xmlBody))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()

	ch.handleWebhook(w, req)

	// 明文模式应该返回 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for plaintext mode, got %d", w.Code)
	}

	// 测试 GET 请求 (URL 验证)
	req = httptest.NewRequest("GET", "/wework/event?msg_signature=sign&timestamp=123&nonce=456&echostr=echo", nil)
	w = httptest.NewRecorder()

	ch.handleWebhook(w, req)

	// 无效签名应该返回 401
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for invalid signature, got %d", w.Code)
	}
}
