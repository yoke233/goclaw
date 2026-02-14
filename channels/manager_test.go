package channels

import (
	"context"
	"testing"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
)

type testChannel struct {
	name      string
	accountID string
}

func (c *testChannel) Name() string { return c.name }

func (c *testChannel) AccountID() string { return c.accountID }

func (c *testChannel) Start(ctx context.Context) error { return nil }

func (c *testChannel) Stop() error { return nil }

func (c *testChannel) Send(msg *bus.OutboundMessage) error { return nil }

func (c *testChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error { return nil }

func (c *testChannel) IsAllowed(senderID string) bool { return true }

func TestManagerRegisterNilChannelShouldNotPanic(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	mgr := NewManager(messageBus)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering nil channel should return error, got panic: %v", r)
		}
	}()

	var ch BaseChannel
	if err := mgr.Register(ch); err == nil {
		t.Fatalf("expected error when registering nil channel")
	}
}

func TestManagerStatusUsesRegisteredAliasName(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	mgr := NewManager(messageBus)
	ch := &testChannel{name: "telegram", accountID: "acc1"}
	if err := mgr.RegisterWithName(ch, "telegram:acc1"); err != nil {
		t.Fatalf("register with name failed: %v", err)
	}

	status, err := mgr.Status("telegram:acc1")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	if status["name"] != "telegram:acc1" {
		t.Fatalf("expected status name to use registered alias, got %v", status["name"])
	}
}

func TestManagerSetupFromConfigFeishuAccountsShouldInheritGlobalWebhookSettings(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	mgr := NewManager(messageBus)
	cfg := &config.Config{}
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.VerificationToken = "global-verify-token"
	cfg.Channels.Feishu.EncryptKey = "global-encrypt-key"
	cfg.Channels.Feishu.WebhookPort = 9123
	cfg.Channels.Feishu.Accounts = map[string]config.ChannelAccountConfig{
		"acc1": {
			Enabled:   true,
			AppID:     "cli_xxx",
			AppSecret: "sec_xxx",
		},
	}

	if err := mgr.SetupFromConfig(cfg); err != nil {
		t.Fatalf("setup from config failed: %v", err)
	}

	raw, ok := mgr.Get("feishu:acc1")
	if !ok {
		t.Fatalf("expected feishu:acc1 channel to be registered")
	}

	ch, ok := raw.(*FeishuChannel)
	if !ok {
		t.Fatalf("expected *FeishuChannel, got %T", raw)
	}

	if ch.verificationToken != cfg.Channels.Feishu.VerificationToken {
		t.Fatalf("expected verification_token to inherit global config, got %q", ch.verificationToken)
	}
	if ch.encryptKey != cfg.Channels.Feishu.EncryptKey {
		t.Fatalf("expected encrypt_key to inherit global config, got %q", ch.encryptKey)
	}
	if ch.webhookPort != cfg.Channels.Feishu.WebhookPort {
		t.Fatalf("expected webhook_port %d, got %d", cfg.Channels.Feishu.WebhookPort, ch.webhookPort)
	}
}

func TestManagerSetupFromConfigWeWorkAccountsShouldInheritGlobalWebhookSettings(t *testing.T) {
	messageBus := bus.NewMessageBus(1)
	defer func() { _ = messageBus.Close() }()

	mgr := NewManager(messageBus)
	cfg := &config.Config{}
	cfg.Channels.WeWork.Enabled = true
	cfg.Channels.WeWork.Token = "global-token"
	cfg.Channels.WeWork.EncodingAESKey = "global-aes-key"
	cfg.Channels.WeWork.WebhookPort = 9234
	cfg.Channels.WeWork.Accounts = map[string]config.ChannelAccountConfig{
		"corp-a": {
			Enabled:   true,
			CorpID:    "ww_corp_id",
			AgentID:   "1000002",
			AppSecret: "secret_xxx",
		},
	}

	if err := mgr.SetupFromConfig(cfg); err != nil {
		t.Fatalf("setup from config failed: %v", err)
	}

	raw, ok := mgr.Get("wework:corp-a")
	if !ok {
		t.Fatalf("expected wework:corp-a channel to be registered")
	}

	ch, ok := raw.(*WeWorkChannel)
	if !ok {
		t.Fatalf("expected *WeWorkChannel, got %T", raw)
	}

	if ch.token != cfg.Channels.WeWork.Token {
		t.Fatalf("expected token to inherit global config, got %q", ch.token)
	}
	if ch.encodingAESKey != cfg.Channels.WeWork.EncodingAESKey {
		t.Fatalf("expected encoding_aes_key to inherit global config, got %q", ch.encodingAESKey)
	}
	if ch.webhookPort != cfg.Channels.WeWork.WebhookPort {
		t.Fatalf("expected webhook_port %d, got %d", cfg.Channels.WeWork.WebhookPort, ch.webhookPort)
	}
}
