package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	coreevents "github.com/cexll/agentsdk-go/pkg/core/events"
	corehooks "github.com/cexll/agentsdk-go/pkg/core/hooks"
	sdkmodel "github.com/cexll/agentsdk-go/pkg/model"
	sdkprompts "github.com/cexll/agentsdk-go/pkg/prompts"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// AgentsdkRuntimeOptions 运行时初始化参数。
type AgentsdkRuntimeOptions struct {
	Pool             RolePool
	AnthropicAPIKey  string
	AnthropicBaseURL string
	ModelName        string
	Temperature      float64
	MaxTokens        int
	MaxIterations    int
}

// AgentsdkRuntime 为 sessions_spawn 提供独立执行能力。
// 阶段 1 采用进程内执行，并保留统一接口，后续可替换为真实 agentsdk-go 执行器。
type AgentsdkRuntime struct {
	pool              RolePool
	anthropicAPIKey   string
	anthropicBaseURL  string
	modelName         string
	temperature       float64
	maxTokens         int
	maxIterations     int
	permissionDecider PermissionDecider

	mu   sync.RWMutex
	runs map[string]*subagentRun
}

type subagentRun struct {
	req    SubagentRunRequest
	done   chan struct{}
	result *SubagentRunResult
	cancel context.CancelFunc
}

func NewAgentsdkRuntime(opts AgentsdkRuntimeOptions) *AgentsdkRuntime {
	pool := opts.Pool
	if pool == nil {
		pool = NewSimpleRolePool(8, map[string]int{
			RoleFrontend: 5,
			RoleBackend:  4,
		})
	}

	return &AgentsdkRuntime{
		pool:             pool,
		anthropicAPIKey:  strings.TrimSpace(opts.AnthropicAPIKey),
		anthropicBaseURL: strings.TrimSpace(opts.AnthropicBaseURL),
		modelName:        normalizeModelName(opts.ModelName),
		temperature:      opts.Temperature,
		maxTokens:        opts.MaxTokens,
		maxIterations:    opts.MaxIterations,
		runs:             make(map[string]*subagentRun),
	}
}

func (r *AgentsdkRuntime) SetPermissionDecider(decider PermissionDecider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.permissionDecider = decider
	r.mu.Unlock()
}

func (r *AgentsdkRuntime) Spawn(ctx context.Context, req SubagentRunRequest) (string, error) {
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
	run := &subagentRun{
		req:    req,
		done:   make(chan struct{}),
		cancel: cancel,
	}
	r.runs[req.RunID] = run
	r.mu.Unlock()

	go r.execute(runCtx, req.RunID)
	return req.RunID, nil
}

func (r *AgentsdkRuntime) Wait(ctx context.Context, runID string) (*SubagentRunResult, error) {
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

func (r *AgentsdkRuntime) Cancel(_ context.Context, runID string) error {
	run, err := r.getRun(runID)
	if err != nil {
		return err
	}
	run.cancel()
	return nil
}

func (r *AgentsdkRuntime) getRun(runID string) (*subagentRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	run, ok := r.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	return run, nil
}

func (r *AgentsdkRuntime) execute(parentCtx context.Context, runID string) {
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

	repoDir := strings.TrimSpace(run.req.RepoDir)
	if repoDir == "" {
		run.result = &SubagentRunResult{
			Status:   RunStatusError,
			ErrorMsg: "repo dir is empty",
		}
		return
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		run.result = &SubagentRunResult{
			Status:   RunStatusError,
			ErrorMsg: fmt.Sprintf("failed to create repo dir: %v", err),
		}
		return
	}

	skillsRegs, hookRegs, warnings := buildSubagentSkillRegistrations(run.req)
	for _, warning := range warnings {
		logger.Warn("Subagent skills warning",
			zap.String("run_id", runID),
			zap.String("goclawdir", strings.TrimSpace(run.req.GoClawDir)),
			zap.String("roledir", strings.TrimSpace(run.req.RoleDir)),
			zap.String("repodir", repoDir),
			zap.String("warning", warning))
	}

	settingsOverrides, mcpWarnings := buildSubagentSDKSettingsOverrides(run.req)
	for _, w := range mcpWarnings {
		logger.Warn("Subagent MCP warning",
			zap.String("run_id", runID),
			zap.String("goclawdir", strings.TrimSpace(run.req.GoClawDir)),
			zap.String("roledir", strings.TrimSpace(run.req.RoleDir)),
			zap.String("repodir", repoDir),
			zap.String("mcp_config_path", strings.TrimSpace(run.req.MCPConfigPath)),
			zap.String("warning", w))
	}

	timeoutSeconds := run.req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 900
	}
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	r.mu.RLock()
	decider := r.permissionDecider
	r.mu.RUnlock()
	var permissionHandler sdkapi.PermissionRequestHandler
	if decider != nil {
		permissionHandler = func(handlerCtx context.Context, req sdkapi.PermissionRequest) (coreevents.PermissionDecisionType, error) {
			return decider(handlerCtx, run.req, req)
		}
	}

	modelName := normalizeModelName(r.modelName)
	if modelName == "" {
		modelName = "claude-sonnet-4-5"
	}

	var tempPtr *float64
	if r.temperature > 0 {
		t := r.temperature
		tempPtr = &t
	}

	modelProvider := &sdkmodel.AnthropicProvider{
		APIKey:      strings.TrimSpace(r.anthropicAPIKey),
		BaseURL:     strings.TrimSpace(r.anthropicBaseURL),
		ModelName:   modelName,
		MaxTokens:   r.maxTokens,
		Temperature: tempPtr,
	}

	maxIterations := r.maxIterations
	if maxIterations <= 0 {
		maxIterations = 15
	}

	rt, err := sdkapi.New(ctx, sdkapi.Options{
		ProjectRoot:              repoDir,
		ModelFactory:             modelProvider,
		SystemPrompt:             strings.TrimSpace(run.req.SystemPrompt),
		Skills:                   skillsRegs,
		TypedHooks:               hookRegs,
		MaxIterations:            maxIterations,
		Timeout:                  time.Duration(timeoutSeconds) * time.Second,
		SettingsOverrides:        settingsOverrides,
		PermissionRequestHandler: permissionHandler,
	})
	if err != nil {
		status := RunStatusError
		errMsg := fmt.Sprintf("failed to initialize agentsdk runtime: %v", err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = RunStatusTimeout
			errMsg = "subagent runtime initialization timed out"
		}
		run.result = &SubagentRunResult{
			Status:   status,
			ErrorMsg: errMsg,
		}
		return
	}
	defer rt.Close()

	reqTask := strings.TrimSpace(StripRolePrefix(run.req.Task))
	if reqTask == "" {
		reqTask = strings.TrimSpace(run.req.Task)
	}

	resp, err := rt.Run(ctx, sdkapi.Request{
		Prompt:    reqTask,
		SessionID: run.req.RunID,
		Metadata: map[string]any{
			"role": NormalizeRole(run.req.Role),
		},
	})
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

	output := ""
	if resp != nil && resp.Result != nil {
		output = strings.TrimSpace(resp.Result.Output)
	}
	if output == "" && resp != nil && resp.Subagent != nil {
		switch v := resp.Subagent.Output.(type) {
		case string:
			output = strings.TrimSpace(v)
		case nil:
		default:
			output = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if len(warnings) > 0 {
		warnText := strings.Join(warnings, "\n")
		if output == "" {
			output = warnText
		} else {
			output += "\n\n" + warnText
		}
	}

	run.result = &SubagentRunResult{
		Status: RunStatusOK,
		Output: output,
	}
}

func loadRoleRegistrations(skillsDir string) ([]sdkapi.SkillRegistration, []corehooks.ShellHook, []string) {
	if strings.TrimSpace(skillsDir) == "" {
		return nil, nil, []string{"skills directory is empty"}
	}

	stat, err := os.Stat(skillsDir)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("skills directory missing: %s", skillsDir)}
	}
	if !stat.IsDir() {
		return nil, nil, []string{fmt.Sprintf("skills path is not a directory: %s", skillsDir)}
	}

	builtins := sdkprompts.ParseWithOptions(os.DirFS(skillsDir), sdkprompts.ParseOptions{
		SkillsDir:    ".",
		CommandsDir:  "__none__",
		SubagentsDir: "__none__",
		HooksDir:     "__none__",
	})

	warnings := make([]string, 0, len(builtins.Errors)+1)
	for _, parseErr := range builtins.Errors {
		if parseErr != nil {
			warnings = append(warnings, fmt.Sprintf("skills parse warning: %v", parseErr))
		}
	}
	if len(builtins.Skills) == 0 {
		warnings = append(warnings, fmt.Sprintf("no SKILL.md found under: %s", skillsDir))
	}

	regs := make([]sdkapi.SkillRegistration, 0, len(builtins.Skills))
	for _, entry := range builtins.Skills {
		regs = append(regs, sdkapi.SkillRegistration{
			Definition: entry.Definition,
			Handler:    entry.Handler,
		})
	}
	return regs, builtins.Hooks, warnings
}

func normalizeModelName(raw string) string {
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
