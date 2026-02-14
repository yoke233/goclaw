package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	corehooks "github.com/cexll/agentsdk-go/pkg/core/hooks"
	sdkmodel "github.com/cexll/agentsdk-go/pkg/model"
	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
	sdktool "github.com/cexll/agentsdk-go/pkg/tool"
	agenttools "github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/extensions"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
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
	inUse       int
	invalidated bool
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

	entry, agentID, err := r.getOrCreateRuntimeEntry(ctx, req)
	if err != nil {
		return nil, err
	}
	defer r.releaseRuntime(agentID, entry)

	if entry == nil || entry.runtime == nil {
		return nil, fmt.Errorf("runtime is not available")
	}
	runtime := entry.runtime

	request := sdkapi.Request{
		Prompt:        req.Prompt,
		SessionID:     strings.TrimSpace(req.SessionKey),
		Metadata:      req.Metadata,
		ToolWhitelist: append([]string(nil), req.ToolWhitelist...),
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

// RunStream executes a single main-agent turn with streaming events.
func (r *AgentSDKMainRuntime) RunStream(ctx context.Context, req MainRunRequest) (<-chan StreamEvent, error) {
	if strings.TrimSpace(req.Prompt) == "" && len(req.Media) == 0 {
		return nil, fmt.Errorf("prompt or media is required")
	}

	entry, agentID, err := r.getOrCreateRuntimeEntry(ctx, req)
	if err != nil {
		return nil, err
	}

	if entry == nil || entry.runtime == nil {
		r.releaseRuntime(agentID, entry)
		return nil, fmt.Errorf("runtime is not available")
	}
	runtime := entry.runtime

	request := sdkapi.Request{
		Prompt:        req.Prompt,
		SessionID:     strings.TrimSpace(req.SessionKey),
		Metadata:      req.Metadata,
		ToolWhitelist: append([]string(nil), req.ToolWhitelist...),
	}
	request.ContentBlocks = buildContentBlocks(req.Prompt, req.Media)

	stream, err := runtime.RunStream(ctx, request)
	if err != nil {
		r.releaseRuntime(agentID, entry)
		return nil, err
	}

	out := make(chan StreamEvent, 128)
	go func() {
		defer close(out)
		defer r.releaseRuntime(agentID, entry)
		for evt := range stream {
			out <- evt
		}
	}()
	return out, nil
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

// Invalidate marks the cached runtime for an agent as stale. The runtime will be
// recreated on the next turn. If the runtime is currently in-use, the actual
// Close is deferred until the last in-flight request completes.
func (r *AgentSDKMainRuntime) Invalidate(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = "default"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.runtimes[agentID]
	if !ok || entry == nil {
		return nil
	}
	entry.invalidated = true
	if entry.inUse > 0 {
		return nil
	}
	if entry.runtime != nil {
		if err := entry.runtime.Close(); err != nil {
			return err
		}
		entry.runtime = nil
	}
	delete(r.runtimes, agentID)
	return nil
}

func (r *AgentSDKMainRuntime) releaseRuntime(agentID string, entry *sdkRuntimeEntry) {
	if entry == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = "default"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if entry.inUse > 0 {
		entry.inUse--
	}

	if entry.inUse != 0 || !entry.invalidated {
		return
	}

	// Safe to close now.
	if entry.runtime != nil {
		_ = entry.runtime.Close()
		entry.runtime = nil
	}

	// Only delete if the map still points to this entry (it might have been replaced).
	if current, ok := r.runtimes[agentID]; ok && current == entry {
		delete(r.runtimes, agentID)
	}
}

func (r *AgentSDKMainRuntime) getOrCreateRuntimeEntry(ctx context.Context, req MainRunRequest) (*sdkRuntimeEntry, string, error) {
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
	if existing, ok := r.runtimes[agentID]; ok && existing != nil && existing.runtime != nil && !existing.invalidated {
		if existing.workspace == workspace &&
			existing.system == systemPrompt &&
			existing.model == modelName &&
			existing.temperature == temperature &&
			existing.maxTokens == maxTokens {
			existing.inUse++
			r.mu.Unlock()
			return existing, agentID, nil
		}
	}
	r.mu.Unlock()

	modelFactory, err := buildAgentSDKModelFactory(r.cfg, modelName, maxTokens, temperature)
	if err != nil {
		return nil, agentID, err
	}

	maxIterations := r.cfg.Agents.Defaults.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 15
	}

	runtimeTimeout := 5 * time.Minute
	if sub := r.cfg.Agents.Defaults.Subagents; sub != nil && sub.TimeoutSeconds > 0 {
		runtimeTimeout = time.Duration(sub.TimeoutSeconds) * time.Second
	}

	// Load workspace extensions (skills + MCP config) at runtime creation time.
	skillsRoleDir := "skills"
	if sub := r.cfg.Agents.Defaults.Subagents; sub != nil {
		if strings.TrimSpace(sub.SkillsRoleDir) != "" {
			skillsRoleDir = strings.TrimSpace(sub.SkillsRoleDir)
		}
	}
	mainRoleRoot := filepath.Join(workspace, skillsRoleDir, "main")
	mainSkillsDir := extensions.AgentsSkillsDir(mainRoleRoot)
	skillRegs, hookRegs, skillWarnings := loadAgentSDKRegistrations(mainSkillsDir)
	for _, w := range skillWarnings {
		logger.Warn("Main skills warning",
			zap.String("agent_id", agentID),
			zap.String("skills_dir", mainSkillsDir),
			zap.String("warning", w))
	}
	pluginResult := extensions.LoadClaudePlugins(workspace)
	for _, w := range pluginResult.Warnings {
		logger.Warn("Main plugin warning",
			zap.String("agent_id", agentID),
			zap.String("workspace", workspace),
			zap.String("warning", w))
	}

	mergedSkills := mergeSkillRegistrations(pluginResult.Skills, skillRegs)
	mergedHooks := append([]corehooks.ShellHook{}, pluginResult.Hooks...)
	mergedHooks = append(mergedHooks, hookRegs...)
	mergedCommands := mergeCommandRegistrations(pluginResult.Commands, nil)
	mergedSubagents := mergeSubagentRegistrations(pluginResult.Subagents, nil)

	settingsOverrides, mcpWarnings := buildAgentSDKSettingsOverrides(r.cfg, workspace, pluginResult.MCP)
	for _, w := range mcpWarnings {
		logger.Warn("Main MCP warning",
			zap.String("agent_id", agentID),
			zap.String("workspace", workspace),
			zap.String("warning", w))
	}

	runtime, err := sdkapi.New(ctx, sdkapi.Options{
		ProjectRoot:       workspace,
		ModelFactory:      modelFactory,
		SystemPrompt:      systemPrompt,
		MaxIterations:     maxIterations,
		MaxSessions:       1000,
		Timeout:           runtimeTimeout,
		TaskStore:         r.taskStore,
		Tools:             buildAgentSDKTools(r.tools.ListExisting()),
		Skills:            mergedSkills,
		Commands:          mergedCommands,
		Subagents:         mergedSubagents,
		TypedHooks:        mergedHooks,
		SettingsOverrides: settingsOverrides,
	})
	if err != nil {
		return nil, agentID, fmt.Errorf("failed to initialize main agentsdk runtime: %w", err)
	}

	newEntry := &sdkRuntimeEntry{
		runtime:     runtime,
		workspace:   workspace,
		system:      systemPrompt,
		model:       modelName,
		temperature: temperature,
		maxTokens:   maxTokens,
		inUse:       1,
	}

	r.mu.Lock()
	// Another goroutine might have created a fresh runtime for this agent while we were initializing.
	if existing, ok := r.runtimes[agentID]; ok && existing != nil && existing.runtime != nil && !existing.invalidated {
		if existing.workspace == workspace &&
			existing.system == systemPrompt &&
			existing.model == modelName &&
			existing.temperature == temperature &&
			existing.maxTokens == maxTokens {
			existing.inUse++
			r.mu.Unlock()
			_ = runtime.Close()
			return existing, agentID, nil
		}
	}

	// Replace existing entry (if any). Old entry is invalidated and will be closed once unused.
	if old, ok := r.runtimes[agentID]; ok && old != nil {
		old.invalidated = true
		if old.inUse == 0 && old.runtime != nil {
			_ = old.runtime.Close()
			old.runtime = nil
		}
	}
	r.runtimes[agentID] = newEntry
	r.mu.Unlock()

	return newEntry, agentID, nil
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
