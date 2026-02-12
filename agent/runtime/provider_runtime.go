package runtime

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"go.uber.org/zap"
)

// ProviderRuntimeOptions 运行时初始化参数（纯 goclaw provider 路径）
type ProviderRuntimeOptions struct {
	Pool            RolePool
	Provider        providers.Provider
	ModelName       string
	Temperature     float64
	MaxTokens       int
	DefaultTimeoutS int
}

// ProviderRuntime 使用 goclaw provider 执行 subagent 任务。
type ProviderRuntime struct {
	pool            RolePool
	provider        providers.Provider
	modelName       string
	temperature     float64
	maxTokens       int
	defaultTimeoutS int

	mu   sync.RWMutex
	runs map[string]*providerRun
}

type providerRun struct {
	req    SubagentRunRequest
	done   chan struct{}
	result *SubagentRunResult
	cancel context.CancelFunc
}

// NewProviderRuntime 创建 ProviderRuntime
func NewProviderRuntime(opts ProviderRuntimeOptions) *ProviderRuntime {
	pool := opts.Pool
	if pool == nil {
		pool = NewSimpleRolePool(8, map[string]int{
			RoleFrontend: 5,
			RoleBackend:  4,
		})
	}

	timeoutS := opts.DefaultTimeoutS
	if timeoutS <= 0 {
		timeoutS = 900
	}

	return &ProviderRuntime{
		pool:            pool,
		provider:        opts.Provider,
		modelName:       normalizeProviderModelName(opts.ModelName),
		temperature:     opts.Temperature,
		maxTokens:       opts.MaxTokens,
		defaultTimeoutS: timeoutS,
		runs:            make(map[string]*providerRun),
	}
}

// Spawn 启动分身任务
func (r *ProviderRuntime) Spawn(_ context.Context, req SubagentRunRequest) (string, error) {
	if strings.TrimSpace(req.RunID) == "" {
		return "", fmt.Errorf("run id is required")
	}
	if strings.TrimSpace(req.Task) == "" {
		return "", fmt.Errorf("task is required")
	}

	r.mu.Lock()
	if _, exists := r.runs[req.RunID]; exists {
		r.mu.Unlock()
		return "", fmt.Errorf("run already exists: %s", req.RunID)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	run := &providerRun{
		req:    req,
		done:   make(chan struct{}),
		cancel: cancel,
	}
	r.runs[req.RunID] = run
	r.mu.Unlock()

	go r.execute(runCtx, req.RunID)
	return req.RunID, nil
}

// Wait 等待分身任务结束
func (r *ProviderRuntime) Wait(ctx context.Context, runID string) (*SubagentRunResult, error) {
	run, err := r.getRun(runID)
	if err != nil {
		return nil, err
	}

	select {
	case <-run.done:
		r.mu.Lock()
		current, exists := r.runs[runID]
		if exists {
			delete(r.runs, runID)
		}
		r.mu.Unlock()

		if !exists || current.result == nil {
			return &SubagentRunResult{
				Status:   RunStatusError,
				ErrorMsg: "run finished without result",
			}, nil
		}
		return current.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Cancel 取消分身任务
func (r *ProviderRuntime) Cancel(_ context.Context, runID string) error {
	run, err := r.getRun(runID)
	if err != nil {
		return err
	}
	run.cancel()
	return nil
}

func (r *ProviderRuntime) getRun(runID string) (*providerRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	run, ok := r.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	return run, nil
}

func (r *ProviderRuntime) execute(parentCtx context.Context, runID string) {
	run, err := r.getRun(runID)
	if err != nil {
		return
	}
	defer close(run.done)

	role := NormalizeRole(run.req.Role)
	if err := r.pool.Acquire(parentCtx, role); err != nil {
		run.result = &SubagentRunResult{
			Status:   RunStatusError,
			ErrorMsg: fmt.Sprintf("failed to acquire role pool: %v", err),
		}
		return
	}
	defer r.pool.Release(role)

	if err := os.MkdirAll(run.req.WorkDir, 0o755); err != nil {
		run.result = &SubagentRunResult{
			Status:   RunStatusError,
			ErrorMsg: fmt.Sprintf("failed to create workdir: %v", err),
		}
		return
	}

	skillsPrompt, warn := loadRoleSkillsPrompt(run.req.SkillsDir)
	if warn != "" {
		logger.Warn("Subagent skills warning",
			zap.String("run_id", runID),
			zap.String("skills_dir", run.req.SkillsDir),
			zap.String("warning", warn))
	}

	timeoutSeconds := run.req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = r.defaultTimeoutS
	}
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if r.provider == nil {
		run.result = &SubagentRunResult{
			Status:   RunStatusError,
			ErrorMsg: "provider runtime is nil",
		}
		return
	}

	systemPrompt := strings.TrimSpace(run.req.SystemPrompt)
	if skillsPrompt != "" {
		if systemPrompt != "" {
			systemPrompt += "\n\n"
		}
		systemPrompt += skillsPrompt
	}

	reqTask := strings.TrimSpace(StripRolePrefix(run.req.Task))
	if reqTask == "" {
		reqTask = strings.TrimSpace(run.req.Task)
	}

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: reqTask},
	}

	options := []providers.ChatOption{}
	if r.modelName != "" {
		options = append(options, providers.WithModel(r.modelName))
	}
	if r.temperature > 0 {
		options = append(options, providers.WithTemperature(r.temperature))
	}
	if r.maxTokens > 0 {
		options = append(options, providers.WithMaxTokens(r.maxTokens))
	}

	resp, err := r.provider.Chat(ctx, messages, nil, options...)
	if err != nil {
		status := RunStatusError
		errMsg := err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = RunStatusTimeout
			errMsg = "subagent run timed out"
		} else if errors.Is(ctx.Err(), context.Canceled) {
			errMsg = "subagent run canceled"
		}
		run.result = &SubagentRunResult{
			Status:   status,
			ErrorMsg: errMsg,
		}
		return
	}

	output := strings.TrimSpace(resp.Content)
	if warn != "" {
		if output == "" {
			output = warn
		} else {
			output += "\n\n" + warn
		}
	}

	run.result = &SubagentRunResult{
		Status: RunStatusOK,
		Output: output,
	}
}

func loadRoleSkillsPrompt(skillsDir string) (string, string) {
	if strings.TrimSpace(skillsDir) == "" {
		return "", "skills directory is empty"
	}

	stat, err := os.Stat(skillsDir)
	if err != nil {
		return "", fmt.Sprintf("skills directory missing: %s", skillsDir)
	}
	if !stat.IsDir() {
		return "", fmt.Sprintf("skills path is not a directory: %s", skillsDir)
	}

	var snippets []string
	stopErr := errors.New("stop walk")
	walkErr := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		title := filepath.Base(filepath.Dir(path))
		snippet := truncateSkillsText(strings.TrimSpace(string(content)), 1200)
		snippets = append(snippets, fmt.Sprintf("### %s\n%s", title, snippet))
		if len(snippets) >= 3 {
			return stopErr
		}
		return nil
	})

	if walkErr != nil && !errors.Is(walkErr, stopErr) {
		return "", fmt.Sprintf("failed to load skills: %v", walkErr)
	}
	if len(snippets) == 0 {
		return "", fmt.Sprintf("no SKILL.md found under: %s", skillsDir)
	}

	prompt := strings.Join([]string{
		"## Role Skills",
		"Use the following role skills when they are relevant to the task:",
		strings.Join(snippets, "\n\n"),
	}, "\n\n")
	return prompt, ""
}

func truncateSkillsText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

func normalizeProviderModelName(raw string) string {
	model := strings.TrimSpace(raw)
	if model == "" {
		return ""
	}
	if idx := strings.Index(model, ":"); idx >= 0 && idx+1 < len(model) {
		model = model[idx+1:]
	}
	if idx := strings.LastIndex(model, "/"); idx >= 0 && idx+1 < len(model) {
		model = model[idx+1:]
	}
	return strings.TrimSpace(model)
}
