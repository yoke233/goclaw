package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	sdkmodel "github.com/cexll/agentsdk-go/pkg/model"
	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
	sdktool "github.com/cexll/agentsdk-go/pkg/tool"
	agenttools "github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/config"
)

// AgentSDKMainRuntimeOptions configures the main agentsdk runtime adapter.
type AgentSDKMainRuntimeOptions struct {
	Config           *config.Config
	Tools            *ToolRegistry
	DefaultWorkspace string
	TaskStore        sdktasks.Store
}

// AgentSDKMainRuntime implements MainRuntime via agentsdk-go.
type AgentSDKMainRuntime struct {
	cfg              *config.Config
	tools            *ToolRegistry
	defaultWorkspace string
	taskStore        sdktasks.Store

	mu       sync.Mutex
	runtimes map[string]*sdkRuntimeEntry
}

type sdkRuntimeEntry struct {
	runtime     *sdkapi.Runtime
	workspace   string
	system      string
	model       string
	temperature float64
	maxTokens   int
}

// NewAgentSDKMainRuntime creates a main runtime backed by agentsdk-go.
func NewAgentSDKMainRuntime(opts AgentSDKMainRuntimeOptions) (*AgentSDKMainRuntime, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Tools == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	return &AgentSDKMainRuntime{
		cfg:              opts.Config,
		tools:            opts.Tools,
		defaultWorkspace: strings.TrimSpace(opts.DefaultWorkspace),
		taskStore:        opts.TaskStore,
		runtimes:         make(map[string]*sdkRuntimeEntry),
	}, nil
}

// Run executes a single main-agent turn.
func (r *AgentSDKMainRuntime) Run(ctx context.Context, req MainRunRequest) (*MainRunResult, error) {
	if strings.TrimSpace(req.Prompt) == "" && len(req.Media) == 0 {
		return nil, fmt.Errorf("prompt or media is required")
	}

	runtime, err := r.getOrCreateRuntime(ctx, req)
	if err != nil {
		return nil, err
	}

	request := sdkapi.Request{
		Prompt:    req.Prompt,
		SessionID: strings.TrimSpace(req.SessionKey),
		Metadata:  req.Metadata,
	}
	request.ContentBlocks = buildContentBlocks(req.Prompt, req.Media)

	resp, err := runtime.Run(ctx, request)
	if err != nil {
		return nil, err
	}

	output := ""
	if resp != nil && resp.Result != nil {
		output = strings.TrimSpace(resp.Result.Output)
	}
	return &MainRunResult{Output: output}, nil
}

// Close releases all cached runtime instances.
func (r *AgentSDKMainRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for key, entry := range r.runtimes {
		if entry != nil && entry.runtime != nil {
			if err := entry.runtime.Close(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("close runtime %s: %w", key, err)
			}
		}
		delete(r.runtimes, key)
	}
	return firstErr
}

func (r *AgentSDKMainRuntime) getOrCreateRuntime(ctx context.Context, req MainRunRequest) (*sdkapi.Runtime, error) {
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = "default"
	}

	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		workspace = r.defaultWorkspace
	}
	if workspace == "" {
		workspace = "."
	}

	modelName := strings.TrimSpace(r.cfg.Agents.Defaults.Model)
	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	temperature := r.cfg.Agents.Defaults.Temperature
	maxTokens := r.cfg.Agents.Defaults.MaxTokens

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.runtimes[agentID]; ok && existing != nil && existing.runtime != nil {
		if existing.workspace == workspace &&
			existing.system == systemPrompt &&
			existing.model == modelName &&
			existing.temperature == temperature &&
			existing.maxTokens == maxTokens {
			return existing.runtime, nil
		}
		_ = existing.runtime.Close()
		delete(r.runtimes, agentID)
	}

	modelFactory, err := buildAgentSDKModelFactory(r.cfg, modelName, maxTokens, temperature)
	if err != nil {
		return nil, err
	}

	maxIterations := r.cfg.Agents.Defaults.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 15
	}

	runtimeTimeout := 5 * time.Minute
	if sub := r.cfg.Agents.Defaults.Subagents; sub != nil && sub.TimeoutSeconds > 0 {
		runtimeTimeout = time.Duration(sub.TimeoutSeconds) * time.Second
	}

	runtime, err := sdkapi.New(ctx, sdkapi.Options{
		ProjectRoot:   workspace,
		ModelFactory:  modelFactory,
		SystemPrompt:  systemPrompt,
		MaxIterations: maxIterations,
		MaxSessions:   1000,
		Timeout:       runtimeTimeout,
		TaskStore:     r.taskStore,
		Tools:         buildAgentSDKTools(r.tools.ListExisting()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize main agentsdk runtime: %w", err)
	}

	r.runtimes[agentID] = &sdkRuntimeEntry{
		runtime:     runtime,
		workspace:   workspace,
		system:      systemPrompt,
		model:       modelName,
		temperature: temperature,
		maxTokens:   maxTokens,
	}
	return runtime, nil
}

func buildAgentSDKModelFactory(cfg *config.Config, modelName string, maxTokens int, temperature float64) (sdkapi.ModelFactory, error) {
	modelName = strings.TrimSpace(modelName)
	tempPtr := (*float64)(nil)
	if temperature > 0 {
		t := temperature
		tempPtr = &t
	}

	switch {
	case strings.HasPrefix(modelName, "anthropic:"):
		return &sdkmodel.AnthropicProvider{
			APIKey:      strings.TrimSpace(cfg.Providers.Anthropic.APIKey),
			BaseURL:     strings.TrimSpace(cfg.Providers.Anthropic.BaseURL),
			ModelName:   strings.TrimSpace(strings.TrimPrefix(modelName, "anthropic:")),
			MaxTokens:   maxTokens,
			Temperature: tempPtr,
		}, nil
	case strings.HasPrefix(modelName, "openai:"):
		return &sdkmodel.OpenAIProvider{
			APIKey:      strings.TrimSpace(cfg.Providers.OpenAI.APIKey),
			BaseURL:     strings.TrimSpace(cfg.Providers.OpenAI.BaseURL),
			ModelName:   strings.TrimSpace(strings.TrimPrefix(modelName, "openai:")),
			MaxTokens:   maxTokens,
			Temperature: tempPtr,
		}, nil
	case strings.HasPrefix(modelName, "openrouter:"):
		baseURL := strings.TrimSpace(cfg.Providers.OpenRouter.BaseURL)
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		return &sdkmodel.OpenAIProvider{
			APIKey:      strings.TrimSpace(cfg.Providers.OpenRouter.APIKey),
			BaseURL:     baseURL,
			ModelName:   strings.TrimSpace(strings.TrimPrefix(modelName, "openrouter:")),
			MaxTokens:   maxTokens,
			Temperature: tempPtr,
		}, nil
	case strings.HasPrefix(modelName, "claude-"):
		return &sdkmodel.AnthropicProvider{
			APIKey:      strings.TrimSpace(cfg.Providers.Anthropic.APIKey),
			BaseURL:     strings.TrimSpace(cfg.Providers.Anthropic.BaseURL),
			ModelName:   modelName,
			MaxTokens:   maxTokens,
			Temperature: tempPtr,
		}, nil
	default:
		baseURL := strings.TrimSpace(cfg.Providers.OpenRouter.BaseURL)
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		return &sdkmodel.OpenAIProvider{
			APIKey:      strings.TrimSpace(cfg.Providers.OpenRouter.APIKey),
			BaseURL:     baseURL,
			ModelName:   modelName,
			MaxTokens:   maxTokens,
			Temperature: tempPtr,
		}, nil
	}
}

func buildAgentSDKTools(existing []agenttools.Tool) []sdktool.Tool {
	result := make([]sdktool.Tool, 0, len(existing))
	for _, t := range existing {
		if t == nil {
			continue
		}
		result = append(result, &sdkToolAdapter{tool: t})
	}
	return result
}

func buildContentBlocks(prompt string, media []MainRunMedia) []sdkmodel.ContentBlock {
	blocks := make([]sdkmodel.ContentBlock, 0, len(media)+1)
	if strings.TrimSpace(prompt) != "" {
		blocks = append(blocks, sdkmodel.ContentBlock{
			Type: sdkmodel.ContentBlockText,
			Text: prompt,
		})
	}
	for _, m := range media {
		mediaType := strings.ToLower(strings.TrimSpace(m.Type))
		switch {
		case strings.HasPrefix(mediaType, "image"), strings.HasPrefix(strings.ToLower(m.MimeType), "image/"):
			blocks = append(blocks, sdkmodel.ContentBlock{
				Type:      sdkmodel.ContentBlockImage,
				MediaType: strings.TrimSpace(m.MimeType),
				Data:      strings.TrimSpace(m.Base64),
				URL:       strings.TrimSpace(m.URL),
			})
		case strings.HasPrefix(mediaType, "document"), strings.HasPrefix(strings.ToLower(m.MimeType), "application/"):
			blocks = append(blocks, sdkmodel.ContentBlock{
				Type:      sdkmodel.ContentBlockDocument,
				MediaType: strings.TrimSpace(m.MimeType),
				Data:      strings.TrimSpace(m.Base64),
				URL:       strings.TrimSpace(m.URL),
			})
		}
	}
	return blocks
}

type sdkToolAdapter struct {
	tool agenttools.Tool
}

func (a *sdkToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *sdkToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *sdkToolAdapter) Schema() *sdktool.JSONSchema {
	return convertToSDKSchema(a.tool.Parameters())
}

func (a *sdkToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (*sdktool.ToolResult, error) {
	output, err := a.tool.Execute(ctx, params)
	if err != nil {
		return &sdktool.ToolResult{
			Success: false,
			Output:  err.Error(),
			Error:   err,
		}, nil
	}
	return &sdktool.ToolResult{
		Success: true,
		Output:  output,
	}, nil
}

func convertToSDKSchema(raw map[string]interface{}) *sdktool.JSONSchema {
	schema := &sdktool.JSONSchema{
		Type:       "object",
		Properties: map[string]interface{}{},
		Required:   []string{},
	}
	if len(raw) == 0 {
		return schema
	}

	if t, ok := raw["type"].(string); ok && strings.TrimSpace(t) != "" {
		schema.Type = strings.TrimSpace(t)
	}
	if props, ok := raw["properties"].(map[string]interface{}); ok && props != nil {
		schema.Properties = props
	}
	if req, ok := raw["required"].([]string); ok {
		schema.Required = append(schema.Required, req...)
	} else if reqAny, ok := raw["required"].([]interface{}); ok {
		for _, item := range reqAny {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				continue
			}
			schema.Required = append(schema.Required, strings.TrimSpace(s))
		}
	}
	return schema
}
