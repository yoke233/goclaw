package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"
)

var globalConfig *Config

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	// 创建 viper 实例
	v := viper.New()

	// 设置配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 默认配置文件搜索路径（按优先级）
		home, err := ResolveUserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		localConfigDir := filepath.Join(".", ".goclaw")
		homeConfigDir := filepath.Join(home, ".goclaw")

		// 1) 当前工作目录下 .goclaw/config.json
		v.AddConfigPath(localConfigDir)
		// 2) 当前工作目录 ./config.json
		v.AddConfigPath(".")
		// 3) 用户目录 ~/.goclaw/config.json
		v.AddConfigPath(homeConfigDir)
		v.SetConfigName("config")
		v.SetConfigType("json")
	}

	// 设置环境变量前缀
	v.SetEnvPrefix("GOSKILLS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 设置默认值
	setDefaults(v)

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		// 配置文件不存在，使用默认值和环境变量
	}

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

// setDefaults 设置默认配置值
func setDefaults(v *viper.Viper) {
	// Agent 默认配置
	v.SetDefault("agents.defaults.model", "openrouter:anthropic/claude-opus-4-5")
	v.SetDefault("agents.defaults.max_iterations", 15)
	v.SetDefault("agents.defaults.temperature", 0.7)
	v.SetDefault("agents.defaults.max_tokens", 4096)
	v.SetDefault("agents.defaults.inbound.max_concurrent", 4)
	v.SetDefault("agents.defaults.inbound.queue_ack_interval_seconds", 3)
	v.SetDefault("agents.defaults.inbound.session_idle_ttl_seconds", 600)
	v.SetDefault("agents.defaults.subagents.max_concurrent", 8)
	v.SetDefault("agents.defaults.subagents.role_max_concurrent", map[string]int{
		"frontend": 5,
		"backend":  4,
	})
	v.SetDefault("agents.defaults.subagents.archive_after_minutes", 60)
	v.SetDefault("agents.defaults.subagents.timeout_seconds", 900)
	v.SetDefault("agents.defaults.subagents.skills_role_dir", "skills")
	v.SetDefault("agents.defaults.subagents.workdir_base", "subagents")
	v.SetDefault("agents.defaults.history.mode", "session_only")
	v.SetDefault("agents.defaults.history.compare", false)
	v.SetDefault("agents.defaults.history.agentsdk_cleanup_days", 7)

	// Gateway 默认配置
	v.SetDefault("gateway.host", "localhost")
	v.SetDefault("gateway.port", 8080)
	// Use time.Duration defaults; plain integers would become nanoseconds when unmarshaled.
	v.SetDefault("gateway.read_timeout", 30*time.Second)
	v.SetDefault("gateway.write_timeout", 30*time.Second)

	// 工具默认配置
	v.SetDefault("tools.shell.enabled", true)
	v.SetDefault("tools.shell.timeout", 120)
	v.SetDefault("tools.shell.sandbox.enabled", false)
	v.SetDefault("tools.shell.sandbox.image", "goclaw/sandbox:latest")
	v.SetDefault("tools.shell.sandbox.workdir", "/workspace")
	v.SetDefault("tools.shell.sandbox.remove", true)
	v.SetDefault("tools.shell.sandbox.network", "none")
	v.SetDefault("tools.shell.sandbox.privileged", false)
	v.SetDefault("tools.web.search_engine", "travily")
	v.SetDefault("tools.web.timeout", 10)
	v.SetDefault("tools.browser.enabled", false)
	v.SetDefault("browser.headless", true)
	v.SetDefault("browser.timeout", 30)

	// Memory 默认配置（memsearch）
	v.SetDefault("memory.backend", "memsearch")
	v.SetDefault("memory.memsearch.command", "memsearch")
	v.SetDefault("memory.memsearch.collection", "memsearch_chunks")
	v.SetDefault("memory.memsearch.milvus_uri", "~/.memsearch/milvus.db")
	v.SetDefault("memory.memsearch.watch.debounce_ms", 1500)
	v.SetDefault("memory.memsearch.chunking.max_chunk_size", 1500)
	v.SetDefault("memory.memsearch.chunking.overlap_lines", 2)
	v.SetDefault("memory.memsearch.compact.llm_provider", "openai")
	v.SetDefault("memory.memsearch.sessions.enabled", true)
	v.SetDefault("memory.memsearch.sessions.retention_days", 60)
	v.SetDefault("memory.memsearch.sessions.redact", false)
	v.SetDefault("memory.memsearch.context.enabled", false)
	v.SetDefault("memory.memsearch.context.limit", 6)
}

// Save 保存配置到文件
func Save(cfg *Config, path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 转换为 JSON（带缩进）
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Get 获取全局配置
func Get() *Config {
	return globalConfig
}

// GetDefaultConfigPath 获取默认配置文件路径
func GetDefaultConfigPath() (string, error) {
	home, err := ResolveUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "config.json"), nil
}

// GetWorkspacePath 获取 workspace 目录路径
func GetWorkspacePath(cfg *Config) (string, error) {
	if cfg.Workspace.Path != "" {
		// 使用配置中的自定义路径（支持 "~" 展开）
		p := strings.TrimSpace(cfg.Workspace.Path)
		if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
			home, err := ResolveUserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			rest := strings.TrimLeft(p[1:], "/\\")
			if rest == "" {
				return filepath.Clean(home), nil
			}
			return filepath.Join(home, filepath.FromSlash(rest)), nil
		}
		return p, nil
	}
	// 使用默认路径
	home, err := ResolveUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// ResolveUserHomeDir returns the best-effort user home directory.
// On Windows, prefer USERPROFILE or HOMEDRIVE+HOMEPATH to avoid HOME drift.
func ResolveUserHomeDir() (string, error) {
	// Prefer explicit Windows profile variables to avoid environment drift.
	if runtime.GOOS == "windows" {
		if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
			return profile, nil
		}
		drive := strings.TrimSpace(os.Getenv("HOMEDRIVE"))
		path := strings.TrimSpace(os.Getenv("HOMEPATH"))
		if drive != "" && path != "" {
			return filepath.Clean(drive + path), nil
		}
	}
	return os.UserHomeDir()
}

// Validate 验证配置
func Validate(cfg *Config) error {
	if err := validateAgents(cfg); err != nil {
		return fmt.Errorf("agents config invalid: %w", err)
	}

	if err := validateProviders(cfg); err != nil {
		return fmt.Errorf("providers config invalid: %w", err)
	}

	if err := validateChannels(cfg); err != nil {
		return fmt.Errorf("channels config invalid: %w", err)
	}

	if err := validateTools(cfg); err != nil {
		return fmt.Errorf("tools config invalid: %w", err)
	}

	if err := validateGateway(cfg); err != nil {
		return fmt.Errorf("gateway config invalid: %w", err)
	}

	return nil
}

// validateAgents 验证 Agent 配置
func validateAgents(cfg *Config) error {
	if cfg.Agents.Defaults.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	if cfg.Agents.Defaults.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be positive")
	}

	if cfg.Agents.Defaults.Temperature < 0 || cfg.Agents.Defaults.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}

	if cfg.Agents.Defaults.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Agents.Defaults.History.Mode))
	if mode == "" {
		mode = "session_only"
	}
	switch mode {
	case "session_only", "dual", "agentsdk_only":
	default:
		return fmt.Errorf("agents.defaults.history.mode must be session_only, dual, or agentsdk_only")
	}
	if cfg.Agents.Defaults.History.AgentsdkCleanupDays < 0 {
		return fmt.Errorf("agents.defaults.history.agentsdk_cleanup_days must be non-negative")
	}

	return nil
}

// validateProviders 验证 LLM 提供商配置
func validateProviders(cfg *Config) error {
	// 至少需要一个提供商配置了 API 密钥
	hasProvider := false

	if cfg.Providers.OpenRouter.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.OpenRouter.APIKey); err != nil {
			return fmt.Errorf("openrouter: %w", err)
		}
	}

	if cfg.Providers.OpenAI.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.OpenAI.APIKey); err != nil {
			return fmt.Errorf("openai: %w", err)
		}
	}

	if cfg.Providers.Anthropic.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.Anthropic.APIKey); err != nil {
			return fmt.Errorf("anthropic: %w", err)
		}
	}

	if !hasProvider {
		return fmt.Errorf("at least one provider must be configured with an API key")
	}

	return nil
}

// validateChannels 验证通道配置
func validateChannels(cfg *Config) error {
	validateTelegramToken := func(token string) error {
		// Telegram bot tokens are commonly "<bot_id>:<secret>" (no "bot" prefix).
		// Keep validation lightweight: require a single ':' separator and no whitespace.
		tok := strings.TrimSpace(token)
		if tok != token {
			return fmt.Errorf("telegram token must not contain leading/trailing whitespace")
		}
		parts := strings.Split(tok, ":")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("telegram token must look like '<bot_id>:<secret>'")
		}
		return nil
	}

	validateWhatsAppBridgeURL := func(url string) error {
		u := strings.TrimSpace(url)
		if u == "" {
			return fmt.Errorf("whatsapp bridge_url is required when enabled")
		}
		if !strings.HasPrefix(u, "http") {
			return fmt.Errorf("whatsapp bridge_url must be a valid URL")
		}
		return nil
	}

	// Telegram
	if cfg.Channels.Telegram.Enabled {
		if len(cfg.Channels.Telegram.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.Telegram.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if err := validateTelegramToken(accountCfg.Token); err != nil {
					return err
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level token as fallback.
				if strings.TrimSpace(cfg.Channels.Telegram.Token) == "" {
					return fmt.Errorf("telegram token is required when enabled")
				}
				if err := validateTelegramToken(cfg.Channels.Telegram.Token); err != nil {
					return err
				}
			}
		} else {
			if strings.TrimSpace(cfg.Channels.Telegram.Token) == "" {
				return fmt.Errorf("telegram token is required when enabled")
			}
			if err := validateTelegramToken(cfg.Channels.Telegram.Token); err != nil {
				return err
			}
		}
	}

	// WhatsApp
	if cfg.Channels.WhatsApp.Enabled {
		if len(cfg.Channels.WhatsApp.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.WhatsApp.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if err := validateWhatsAppBridgeURL(accountCfg.BridgeURL); err != nil {
					return err
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level bridge_url as fallback.
				if err := validateWhatsAppBridgeURL(cfg.Channels.WhatsApp.BridgeURL); err != nil {
					return err
				}
			}
		} else {
			if err := validateWhatsAppBridgeURL(cfg.Channels.WhatsApp.BridgeURL); err != nil {
				return err
			}
		}
	}

	// Feishu
	if cfg.Channels.Feishu.Enabled {
		// Verification token is a shared webhook security setting (not per-account).
		if strings.TrimSpace(cfg.Channels.Feishu.VerificationToken) == "" {
			return fmt.Errorf("feishu verification_token is required when enabled")
		}

		if len(cfg.Channels.Feishu.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.Feishu.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if strings.TrimSpace(accountCfg.AppID) == "" {
					return fmt.Errorf("feishu app_id is required when enabled")
				}
				if strings.TrimSpace(accountCfg.AppSecret) == "" {
					return fmt.Errorf("feishu app_secret is required when enabled")
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level credentials as fallback.
				if strings.TrimSpace(cfg.Channels.Feishu.AppID) == "" {
					return fmt.Errorf("feishu app_id is required when enabled")
				}
				if strings.TrimSpace(cfg.Channels.Feishu.AppSecret) == "" {
					return fmt.Errorf("feishu app_secret is required when enabled")
				}
			}
		} else {
			if strings.TrimSpace(cfg.Channels.Feishu.AppID) == "" {
				return fmt.Errorf("feishu app_id is required when enabled")
			}
			if strings.TrimSpace(cfg.Channels.Feishu.AppSecret) == "" {
				return fmt.Errorf("feishu app_secret is required when enabled")
			}
		}
	}

	// QQ
	if cfg.Channels.QQ.Enabled {
		if len(cfg.Channels.QQ.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.QQ.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if strings.TrimSpace(accountCfg.AppID) == "" {
					return fmt.Errorf("qq app_id is required when enabled")
				}
				if strings.TrimSpace(accountCfg.AppSecret) == "" {
					return fmt.Errorf("qq app_secret is required when enabled")
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level credentials as fallback.
				if strings.TrimSpace(cfg.Channels.QQ.AppID) == "" {
					return fmt.Errorf("qq app_id is required when enabled")
				}
				if strings.TrimSpace(cfg.Channels.QQ.AppSecret) == "" {
					return fmt.Errorf("qq app_secret is required when enabled")
				}
			}
		} else {
			if strings.TrimSpace(cfg.Channels.QQ.AppID) == "" {
				return fmt.Errorf("qq app_id is required when enabled")
			}
			if strings.TrimSpace(cfg.Channels.QQ.AppSecret) == "" {
				return fmt.Errorf("qq app_secret is required when enabled")
			}
		}
	}

	// WeWork (企业微信)
	if cfg.Channels.WeWork.Enabled {
		if cfg.Channels.WeWork.WebhookPort < 0 || cfg.Channels.WeWork.WebhookPort > 65535 {
			return fmt.Errorf("wework webhook_port must be between 0 and 65535")
		}

		if len(cfg.Channels.WeWork.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.WeWork.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if strings.TrimSpace(accountCfg.CorpID) == "" {
					return fmt.Errorf("wework corp_id is required when enabled")
				}
				// accountCfg.AgentID is reused by WeWork.
				if strings.TrimSpace(accountCfg.AgentID) == "" {
					return fmt.Errorf("wework agent_id is required when enabled")
				}
				// accountCfg.AppSecret is reused as WeWork secret.
				if strings.TrimSpace(accountCfg.AppSecret) == "" {
					return fmt.Errorf("wework secret is required when enabled")
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level credentials as fallback.
				if strings.TrimSpace(cfg.Channels.WeWork.CorpID) == "" {
					return fmt.Errorf("wework corp_id is required when enabled")
				}
				if strings.TrimSpace(cfg.Channels.WeWork.Secret) == "" {
					return fmt.Errorf("wework secret is required when enabled")
				}
				if strings.TrimSpace(cfg.Channels.WeWork.AgentID) == "" {
					return fmt.Errorf("wework agent_id is required when enabled")
				}
			}
		} else {
			if strings.TrimSpace(cfg.Channels.WeWork.CorpID) == "" {
				return fmt.Errorf("wework corp_id is required when enabled")
			}
			if strings.TrimSpace(cfg.Channels.WeWork.Secret) == "" {
				return fmt.Errorf("wework secret is required when enabled")
			}
			if strings.TrimSpace(cfg.Channels.WeWork.AgentID) == "" {
				return fmt.Errorf("wework agent_id is required when enabled")
			}
		}
	}

	// DingTalk
	if cfg.Channels.DingTalk.Enabled {
		if len(cfg.Channels.DingTalk.Accounts) > 0 {
			enabledCount := 0
			for _, accountCfg := range cfg.Channels.DingTalk.Accounts {
				if !accountCfg.Enabled {
					continue
				}
				enabledCount++
				if strings.TrimSpace(accountCfg.ClientID) == "" {
					return fmt.Errorf("dingtalk client_id is required when enabled")
				}
				if strings.TrimSpace(accountCfg.ClientSecret) == "" {
					return fmt.Errorf("dingtalk client_secret is required when enabled")
				}
			}
			if enabledCount == 0 {
				// Enabled=true but no enabled accounts; allow legacy top-level credentials as fallback.
				if strings.TrimSpace(cfg.Channels.DingTalk.ClientID) == "" {
					return fmt.Errorf("dingtalk client_id is required when enabled")
				}
				if strings.TrimSpace(cfg.Channels.DingTalk.ClientSecret) == "" {
					return fmt.Errorf("dingtalk client_secret is required when enabled")
				}
			}
		} else {
			if strings.TrimSpace(cfg.Channels.DingTalk.ClientID) == "" {
				return fmt.Errorf("dingtalk client_id is required when enabled")
			}
			if strings.TrimSpace(cfg.Channels.DingTalk.ClientSecret) == "" {
				return fmt.Errorf("dingtalk client_secret is required when enabled")
			}
		}
	}

	return nil
}

// validateTools 验证工具配置
func validateTools(cfg *Config) error {
	// Shell 工具配置验证
	if cfg.Tools.Shell.Enabled {
		// 检查危险命令是否在拒绝列表中
		dangerousCmds := []string{"rm -rf", "dd", "mkfs"}
		for _, dangerous := range dangerousCmds {
			found := false
			for _, denied := range cfg.Tools.Shell.DeniedCmds {
				if strings.Contains(denied, dangerous) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("shell tool: dangerous command '%s' should be in denied_cmds", dangerous)
			}
		}

		if cfg.Tools.Shell.Timeout <= 0 {
			return fmt.Errorf("shell timeout must be positive")
		}
	}

	// Web 工具配置验证
	if cfg.Tools.Web.SearchAPIKey != "" {
		if cfg.Tools.Web.SearchEngine == "" {
			return fmt.Errorf("web search_engine is required when search_api_key is set")
		}
	}

	if cfg.Tools.Web.Timeout <= 0 {
		return fmt.Errorf("web timeout must be positive")
	}

	// 浏览器工具配置验证
	if cfg.Tools.Browser.Enabled {
		if cfg.Tools.Browser.Timeout <= 0 {
			return fmt.Errorf("browser timeout must be positive")
		}
	}

	return nil
}

// validateGateway 验证网关配置
func validateGateway(cfg *Config) error {
	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		return fmt.Errorf("gateway port must be between 1 and 65535")
	}

	if cfg.Gateway.ReadTimeout <= 0 {
		return fmt.Errorf("gateway read_timeout must be positive")
	}

	if cfg.Gateway.WriteTimeout <= 0 {
		return fmt.Errorf("gateway write_timeout must be positive")
	}

	return nil
}

// validateAPIKey 验证 API 密钥格式
func validateAPIKey(key string) error {
	key = strings.TrimSpace(key)

	if len(key) < 10 {
		return fmt.Errorf("API key too short (minimum 10 characters)")
	}

	// 检查是否包含空格
	if strings.Contains(key, " ") {
		return fmt.Errorf("API key cannot contain spaces")
	}

	return nil
}
