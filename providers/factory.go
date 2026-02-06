package providers

import (
	"fmt"
	"strings"

	"github.com/smallnest/dogclaw/goclaw/config"
)

// ProviderType 提供商类型
type ProviderType string

const (
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeAnthropic  ProviderType = "anthropic"
	ProviderTypeOpenRouter ProviderType = "openrouter"
)

// NewProvider 创建提供商
func NewProvider(cfg *config.Config) (Provider, error) {
	// 确定使用哪个提供商
	providerType, model, err := determineProvider(cfg)
	if err != nil {
		return nil, err
	}

	switch providerType {
	case ProviderTypeOpenAI:
		return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.BaseURL, model)
	case ProviderTypeAnthropic:
		return NewAnthropicProvider(cfg.Providers.Anthropic.APIKey, cfg.Providers.Anthropic.BaseURL, model)
	case ProviderTypeOpenRouter:
		return NewOpenRouterProvider(cfg.Providers.OpenRouter.APIKey, cfg.Providers.OpenRouter.BaseURL, model)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// determineProvider 确定提供商
func determineProvider(cfg *config.Config) (ProviderType, string, error) {
	model := cfg.Agents.Defaults.Model

	// 检查模型名称前缀
	if strings.HasPrefix(model, "openrouter:") {
		return ProviderTypeOpenRouter, strings.TrimPrefix(model, "openrouter:"), nil
	}

	if strings.HasPrefix(model, "anthropic:") || strings.HasPrefix(model, "claude-") {
		return ProviderTypeAnthropic, model, nil
	}

	if strings.HasPrefix(model, "openai:") || strings.HasPrefix(model, "gpt-") {
		return ProviderTypeOpenAI, model, nil
	}

	// 根据可用的 API key 决定
	if cfg.Providers.OpenRouter.APIKey != "" {
		return ProviderTypeOpenRouter, model, nil
	}

	if cfg.Providers.Anthropic.APIKey != "" {
		return ProviderTypeAnthropic, model, nil
	}

	if cfg.Providers.OpenAI.APIKey != "" {
		return ProviderTypeOpenAI, model, nil
	}

	return "", "", fmt.Errorf("no LLM provider API key configured")
}
