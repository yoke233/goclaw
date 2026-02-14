package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func minimalValidConfig() *Config {
	return &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:         "openai:gpt-4o-mini",
				MaxIterations: 5,
				Temperature:   0.7,
				MaxTokens:     2048,
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{
				APIKey: "sk-test-valid-api-key",
			},
		},
		Gateway: GatewayConfig{
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Tools: ToolsConfig{
			Shell: ShellToolConfig{
				Enabled: false,
			},
			Web: WebToolConfig{
				Timeout: 10,
			},
			Browser: BrowserToolConfig{
				Enabled: false,
			},
		},
	}
}

func TestValidateTelegramTokenWithoutBotPrefix(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.Telegram.Enabled = true
	// Telegram token 常见格式是 "<bot_id>:<secret>", 本身不带 "bot" 前缀。
	cfg.Channels.Telegram.Token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected telegram token format to be accepted, got error: %v", err)
	}
}

func TestGetWorkspacePathExpandsTilde(t *testing.T) {
	home, err := ResolveUserHomeDir()
	if err != nil {
		t.Fatalf("failed to resolve home dir: %v", err)
	}

	cfg := minimalValidConfig()
	cfg.Workspace.Path = "~/my-workspace"

	got, err := GetWorkspacePath(cfg)
	if err != nil {
		t.Fatalf("get workspace path failed: %v", err)
	}

	want := filepath.Join(home, "my-workspace")
	if got != want {
		t.Fatalf("expected expanded path %q, got %q", want, got)
	}
}

func TestSetDefaultsGatewayTimeoutUsesSecondGranularity(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("failed to unmarshal defaults: %v", err)
	}

	if cfg.Gateway.ReadTimeout < time.Second {
		t.Fatalf("expected read timeout to be at least 1s granularity, got %v", cfg.Gateway.ReadTimeout)
	}
	if cfg.Gateway.WriteTimeout < time.Second {
		t.Fatalf("expected write timeout to be at least 1s granularity, got %v", cfg.Gateway.WriteTimeout)
	}
}

func TestValidateChannelsMultiAccountMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  func() *Config
	}{
		{
			name: "telegram with account token only",
			cfg: func() *Config {
				c := minimalValidConfig()
				c.Channels.Telegram.Enabled = true
				c.Channels.Telegram.Accounts = map[string]ChannelAccountConfig{
					"acc1": {
						Enabled: true,
						Token:   "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi",
					},
				}
				return c
			},
		},
		{
			name: "whatsapp with account bridge only",
			cfg: func() *Config {
				c := minimalValidConfig()
				c.Channels.WhatsApp.Enabled = true
				c.Channels.WhatsApp.Accounts = map[string]ChannelAccountConfig{
					"acc1": {
						Enabled:   true,
						BridgeURL: "https://example.com/bridge",
					},
				}
				return c
			},
		},
		{
			name: "feishu with account app credentials and shared verification token",
			cfg: func() *Config {
				c := minimalValidConfig()
				c.Channels.Feishu.Enabled = true
				c.Channels.Feishu.VerificationToken = "shared-token"
				c.Channels.Feishu.Accounts = map[string]ChannelAccountConfig{
					"acc1": {
						Enabled:   true,
						AppID:     "cli_xxx",
						AppSecret: "sec_xxx",
					},
				}
				return c
			},
		},
		{
			name: "qq with account app credentials only",
			cfg: func() *Config {
				c := minimalValidConfig()
				c.Channels.QQ.Enabled = true
				c.Channels.QQ.Accounts = map[string]ChannelAccountConfig{
					"acc1": {
						Enabled:   true,
						AppID:     "qq_appid",
						AppSecret: "qq_secret",
					},
				}
				return c
			},
		},
		{
			name: "wework with account credentials only",
			cfg: func() *Config {
				c := minimalValidConfig()
				c.Channels.WeWork.Enabled = true
				c.Channels.WeWork.Accounts = map[string]ChannelAccountConfig{
					"acc1": {
						Enabled:   true,
						CorpID:    "ww_corp",
						AgentID:   "1000002",
						AppSecret: "ww_secret",
					},
				}
				return c
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg()
			if err := Validate(cfg); err != nil {
				t.Fatalf("expected multi-account config to pass validation, got: %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidTelegramAccountToken(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"
	cfg.Channels.Telegram.Accounts = map[string]ChannelAccountConfig{
		"bad": {
			Enabled: true,
			Token:   "invalid-token-without-colon",
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid telegram account token to be rejected")
	}
}

func TestValidateRejectsInvalidWhatsAppAccountBridgeURL(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.WhatsApp.Enabled = true
	cfg.Channels.WhatsApp.BridgeURL = "https://example.com/bridge"
	cfg.Channels.WhatsApp.Accounts = map[string]ChannelAccountConfig{
		"bad": {
			Enabled:   true,
			BridgeURL: "not-a-url",
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid whatsapp account bridge_url to be rejected")
	}
}

func TestValidateRejectsWhenAnyEnabledTelegramAccountIsInvalid(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Accounts = map[string]ChannelAccountConfig{
		"good": {
			Enabled: true,
			Token:   "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi",
		},
		"bad": {
			Enabled: true,
			Token:   "invalid-without-colon",
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation to fail when any enabled telegram account is invalid")
	}
}

func TestValidateRejectsEnabledDingTalkWithoutCredentials(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.DingTalk.Enabled = true

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation to fail when dingtalk is enabled without credentials")
	}
}

func TestValidateRejectsEnabledDingTalkAccountWithoutClientSecret(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.DingTalk.Enabled = true
	cfg.Channels.DingTalk.Accounts = map[string]ChannelAccountConfig{
		"acc1": {
			Enabled:  true,
			ClientID: "ding_app_id",
			// missing ClientSecret
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation to fail when enabled dingtalk account misses client_secret")
	}
}

func TestValidateRejectsInvalidQQAccountWhenTopLevelIsValid(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.QQ.Enabled = true
	cfg.Channels.QQ.AppID = "qq_top_appid"
	cfg.Channels.QQ.AppSecret = "qq_top_secret"
	cfg.Channels.QQ.Accounts = map[string]ChannelAccountConfig{
		"bad": {
			Enabled: true,
			AppID:   "qq_only_appid",
			// missing AppSecret
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid enabled qq account to be rejected")
	}
}

func TestValidateRejectsInvalidFeishuAccountWhenTopLevelIsValid(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = "cli_top"
	cfg.Channels.Feishu.AppSecret = "sec_top"
	cfg.Channels.Feishu.VerificationToken = "verify_top"
	cfg.Channels.Feishu.Accounts = map[string]ChannelAccountConfig{
		"bad": {
			Enabled: true,
			AppID:   "cli_only",
			// missing AppSecret
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid enabled feishu account to be rejected")
	}
}

func TestValidateRejectsInvalidWeWorkAccountWhenTopLevelIsValid(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.Channels.WeWork.Enabled = true
	cfg.Channels.WeWork.CorpID = "ww_top_corp"
	cfg.Channels.WeWork.Secret = "ww_top_secret"
	cfg.Channels.WeWork.AgentID = "1000002"
	cfg.Channels.WeWork.Accounts = map[string]ChannelAccountConfig{
		"bad": {
			Enabled:   true,
			CorpID:    "ww_bad_corp",
			AppSecret: "ww_bad_secret",
			// missing AgentID
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected invalid enabled wework account to be rejected")
	}
}
