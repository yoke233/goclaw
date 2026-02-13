package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/memory"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// AgentManager 管理多个 Agent 实例
type AgentManager struct {
	agents         map[string]*Agent        // agentID -> Agent
	bindings       map[string]*BindingEntry // channel:accountID -> BindingEntry
	defaultAgent   *Agent                   // 默认 Agent
	bus            *bus.MessageBus
	sessionMgr     *session.Manager
	tools          *ToolRegistry
	mu             sync.RWMutex
	cfg            *config.Config
	contextBuilder *ContextBuilder
	inbound        *inboundDispatcher
	// 分身支持
	subagentRegistry  *SubagentRegistry
	subagentAnnouncer *SubagentAnnouncer
	subagentRuntime   agentruntime.SubagentRuntime
	mainRuntime       MainRuntime
	taskStore         TaskTracker
	dataDir           string
	workspace         string
}

const (
	taskStatusInProgress = "in_progress"
	taskStatusCompleted  = "completed"
	taskStatusBlocked    = "blocked"
)

// TaskProgressInput describes a subagent progress append operation.
type TaskProgressInput struct {
	TaskID  string
	RunID   string
	Status  string
	Message string
}

// TaskTracker stores task/run mappings and status/progress updates for subagent execution.
type TaskTracker interface {
	LinkSubagentRun(runID, taskID string) error
	ResolveTaskByRun(runID string) (string, error)
	UpdateTaskStatus(taskID string, status string) error
	AppendTaskProgress(input TaskProgressInput) error
}

// BindingEntry Agent 绑定条目
type BindingEntry struct {
	AgentID   string
	Channel   string
	AccountID string
	Agent     *Agent
}

// NewAgentManagerConfig AgentManager 配置
type NewAgentManagerConfig struct {
	Bus             *bus.MessageBus
	SessionMgr      *session.Manager
	Tools           *ToolRegistry
	DataDir         string // 数据目录，用于存储分身注册表
	Workspace       string
	SubagentRuntime agentruntime.SubagentRuntime
	MainRuntime     MainRuntime
	TaskStore       TaskTracker
}

// NewAgentManager 创建 Agent 管理器
func NewAgentManager(cfg *NewAgentManagerConfig) *AgentManager {
	// 创建分身注册表
	subagentRegistry := NewSubagentRegistry(cfg.DataDir)

	// 创建分身宣告器
	subagentAnnouncer := NewSubagentAnnouncer(nil) // 回调在 Start 中设置

	mgr := &AgentManager{
		agents:            make(map[string]*Agent),
		bindings:          make(map[string]*BindingEntry),
		bus:               cfg.Bus,
		sessionMgr:        cfg.SessionMgr,
		tools:             cfg.Tools,
		subagentRegistry:  subagentRegistry,
		subagentAnnouncer: subagentAnnouncer,
		subagentRuntime:   cfg.SubagentRuntime,
		mainRuntime:       cfg.MainRuntime,
		taskStore:         cfg.TaskStore,
		dataDir:           cfg.DataDir,
		workspace:         cfg.Workspace,
	}
	// Keep inbound consumption responsive: per-session serial, cross-session concurrent.
	mgr.inbound = newInboundDispatcher(mgr, inboundDispatcherOptions{})
	return mgr
}

// handleSubagentCompletion 处理分身完成事件
func (m *AgentManager) handleSubagentCompletion(runID string, record *SubagentRunRecord) {
	logger.Info("Subagent completed",
		zap.String("run_id", runID),
		zap.String("task", record.Task))

	// 启动宣告流程
	if record.Outcome != nil {
		announceParams := &SubagentAnnounceParams{
			ChildSessionKey:     record.ChildSessionKey,
			ChildRunID:          record.RunID,
			RequesterSessionKey: record.RequesterSessionKey,
			RequesterOrigin:     record.RequesterOrigin,
			RequesterDisplayKey: record.RequesterDisplayKey,
			Task:                record.Task,
			Label:               record.Label,
			StartedAt:           record.StartedAt,
			EndedAt:             record.EndedAt,
			Outcome:             record.Outcome,
			Cleanup:             record.Cleanup,
			AnnounceType:        SubagentAnnounceTypeTask,
		}

		if err := m.subagentAnnouncer.RunAnnounceFlow(announceParams); err != nil {
			logger.Error("Failed to announce subagent result",
				zap.String("run_id", runID),
				zap.Error(err))
		}

		// 标记清理完成
		m.subagentRegistry.Cleanup(runID, record.Cleanup, true)
	}
}

// SetupFromConfig 从配置设置 Agent 和绑定
func (m *AgentManager) SetupFromConfig(cfg *config.Config, contextBuilder *ContextBuilder) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg = cfg
	m.contextBuilder = contextBuilder

	logger.Info("Setting up agents from config")

	// 0. Configure inbound dispatching limits (queue acks, idle TTL, global concurrency).
	m.setupInboundDispatcher(cfg)

	// 1. 先设置分身支持，确保 sessions_spawn 已注册（便于系统提示词感知真实工具集合）
	m.setupSubagentSupport(cfg, contextBuilder)

	// 2. 创建 Agent 实例
	for _, agentCfg := range cfg.Agents.List {
		if err := m.createAgent(agentCfg, contextBuilder, cfg); err != nil {
			logger.Error("Failed to create agent",
				zap.String("agent_id", agentCfg.ID),
				zap.Error(err))
			continue
		}
	}

	// 3. 如果没有配置 Agent，创建默认 Agent
	if len(m.agents) == 0 {
		logger.Info("No agents configured, creating default agent")
		defaultAgentCfg := config.AgentConfig{
			ID:        "default",
			Name:      "Default Agent",
			Default:   true,
			Model:     cfg.Agents.Defaults.Model,
			Workspace: cfg.Workspace.Path,
		}
		if err := m.createAgent(defaultAgentCfg, contextBuilder, cfg); err != nil {
			return fmt.Errorf("failed to create default agent: %w", err)
		}
	}

	// 4. 设置绑定
	for _, binding := range cfg.Bindings {
		if err := m.setupBinding(binding); err != nil {
			logger.Error("Failed to setup binding",
				zap.String("agent_id", binding.AgentID),
				zap.String("channel", binding.Match.Channel),
				zap.String("account_id", binding.Match.AccountID),
				zap.Error(err))
		}
	}

	logger.Info("Agent manager setup complete",
		zap.Int("agents", len(m.agents)),
		zap.Int("bindings", len(m.bindings)))

	return nil
}

func (m *AgentManager) setupInboundDispatcher(cfg *config.Config) {
	if m == nil {
		return
	}
	var opts inboundDispatcherOptions
	if cfg != nil {
		in := cfg.Agents.Defaults.Inbound
		if in.QueueAckIntervalSeconds > 0 {
			opts.AckInterval = time.Duration(in.QueueAckIntervalSeconds) * time.Second
		}
		if in.SessionIdleTTLSeconds > 0 {
			opts.IdleTTL = time.Duration(in.SessionIdleTTLSeconds) * time.Second
		}
		if in.MaxConcurrent > 0 {
			opts.MaxConcurrent = in.MaxConcurrent
		}
	}
	m.inbound = newInboundDispatcher(m, opts)
}

// setupSubagentSupport 设置分身支持
func (m *AgentManager) setupSubagentSupport(cfg *config.Config, contextBuilder *ContextBuilder) {
	// 加载分身注册表
	if err := m.subagentRegistry.LoadFromDisk(); err != nil {
		logger.Warn("Failed to load subagent registry", zap.Error(err))
	}

	// 设置分身运行完成回调
	m.subagentRegistry.SetOnRunComplete(func(runID string, record *SubagentRunRecord) {
		m.handleSubagentCompletion(runID, record)
	})

	// 更新宣告器回调
	m.subagentAnnouncer = NewSubagentAnnouncer(func(sessionKey, message string) error {
		// 发送宣告消息到指定会话
		return m.sendToSession(sessionKey, message)
	})

	// 创建分身注册表适配器
	registryAdapter := &subagentRegistryAdapter{registry: m.subagentRegistry}

	// 注册 sessions_spawn 工具
	spawnTool := tools.NewSubagentSpawnTool(registryAdapter)
	spawnTool.SetAgentConfigGetter(func(agentID string) *config.AgentConfig {
		for _, agentCfg := range cfg.Agents.List {
			if agentCfg.ID == agentID {
				return &agentCfg
			}
		}
		return nil
	})
	spawnTool.SetDefaultConfigGetter(func() *config.AgentDefaults {
		return &cfg.Agents.Defaults
	})
	spawnTool.SetAgentIDGetter(func(sessionKey string) string {
		// 从会话密钥中解析 agent ID
		agentID, _, _ := ParseAgentSessionKey(sessionKey)
		if agentID == "" {
			// 尝试从绑定中查找
			for _, entry := range m.bindings {
				if entry.Agent != nil {
					return entry.AgentID
				}
			}
		}
		return agentID
	})
	spawnTool.SetOnSpawn(func(result *tools.SubagentSpawnResult) error {
		return m.handleSubagentSpawn(result)
	})

	// 注册工具
	if err := m.tools.RegisterExisting(spawnTool); err != nil {
		logger.Error("Failed to register sessions_spawn tool", zap.Error(err))
	}

	logger.Info("Subagent support configured")
}

// subagentRegistryAdapter 分身注册表适配器
type subagentRegistryAdapter struct {
	registry *SubagentRegistry
}

// RegisterRun 注册分身运行
func (a *subagentRegistryAdapter) RegisterRun(params *tools.SubagentRunParams) error {
	// 转换 RequesterOrigin
	var requesterOrigin *DeliveryContext
	if params.RequesterOrigin != nil {
		requesterOrigin = &DeliveryContext{
			Channel:   params.RequesterOrigin.Channel,
			AccountID: params.RequesterOrigin.AccountID,
			To:        params.RequesterOrigin.To,
			ThreadID:  params.RequesterOrigin.ThreadID,
		}
	}

	return a.registry.RegisterRun(&SubagentRunParams{
		RunID:               params.RunID,
		ChildSessionKey:     params.ChildSessionKey,
		RequesterSessionKey: params.RequesterSessionKey,
		RequesterOrigin:     requesterOrigin,
		RequesterDisplayKey: params.RequesterDisplayKey,
		Task:                params.Task,
		TaskID:              params.TaskID,
		RepoDir:             params.RepoDir,
		MCPConfigPath:       params.MCPConfigPath,
		Cleanup:             params.Cleanup,
		Label:               params.Label,
		TimeoutSeconds:      params.TimeoutSeconds,
		ArchiveAfterMinutes: params.ArchiveAfterMinutes,
	})
}

// handleSubagentSpawn 处理分身生成
func (m *AgentManager) handleSubagentSpawn(result *tools.SubagentSpawnResult) error {
	if m.subagentRuntime == nil {
		return fmt.Errorf("subagent runtime is not configured")
	}

	record, ok := m.subagentRegistry.GetRun(result.RunID)
	if !ok {
		return fmt.Errorf("subagent run not found: %s", result.RunID)
	}

	role := agentruntime.ParseRole(record.Task, record.Label)
	subCfg := m.getSubagentsConfig()

	workdirBase := "subagents"
	skillsRoleDir := "skills"
	timeoutSeconds := 900
	if subCfg != nil {
		if strings.TrimSpace(subCfg.WorkdirBase) != "" {
			workdirBase = strings.TrimSpace(subCfg.WorkdirBase)
		}
		if strings.TrimSpace(subCfg.SkillsRoleDir) != "" {
			skillsRoleDir = strings.TrimSpace(subCfg.SkillsRoleDir)
		}
		if subCfg.TimeoutSeconds > 0 {
			timeoutSeconds = subCfg.TimeoutSeconds
		}
	}
	if record.TimeoutSeconds > 0 {
		timeoutSeconds = record.TimeoutSeconds
	}

	workspaceRoot := m.getWorkspaceRoot()
	runRoot := filepath.Join(workspaceRoot, workdirBase, record.RunID)
	defaultRepoDir := filepath.Join(runRoot, "repo")
	repoDir := strings.TrimSpace(record.RepoDir)
	if repoDir == "" {
		repoDir = defaultRepoDir
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return fmt.Errorf("failed to create subagent repo dir: %w", err)
		}
	} else {
		stat, err := os.Stat(repoDir)
		if err != nil {
			return fmt.Errorf("subagent repo_dir does not exist: %w", err)
		}
		if stat == nil || !stat.IsDir() {
			return fmt.Errorf("subagent repo_dir is not a directory: %s", repoDir)
		}
	}

	// Role pack directory (may be missing). When it doesn't exist or doesn't
	// contain a valid .agents pack, runtime will fall back to GoClawDir.
	roleDir := filepath.Join(workspaceRoot, skillsRoleDir, role)

	systemPrompt := BuildSubagentSystemPrompt(&SubagentSystemPromptParams{
		RequesterSessionKey: record.RequesterSessionKey,
		RequesterOrigin:     record.RequesterOrigin,
		ChildSessionKey:     record.ChildSessionKey,
		Label:               record.Label,
		Task:                record.Task,
	})

	runReq := agentruntime.SubagentRunRequest{
		RunID:          record.RunID,
		Task:           record.Task,
		Role:           role,
		GoClawDir:      workspaceRoot,
		RoleDir:        roleDir,
		RepoDir:        repoDir,
		MCPConfigPath:  strings.TrimSpace(record.MCPConfigPath),
		SystemPrompt:   systemPrompt,
		TimeoutSeconds: timeoutSeconds,
	}

	if m.taskStore != nil && strings.TrimSpace(record.TaskID) != "" {
		taskID := strings.TrimSpace(record.TaskID)
		if err := m.taskStore.LinkSubagentRun(record.RunID, taskID); err != nil {
			logger.Warn("Failed to link subagent run to task",
				zap.String("run_id", record.RunID),
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		if err := m.taskStore.UpdateTaskStatus(taskID, taskStatusInProgress); err != nil {
			logger.Warn("Failed to update task status to doing",
				zap.String("run_id", record.RunID),
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		if err := m.taskStore.AppendTaskProgress(TaskProgressInput{
			TaskID:  taskID,
			RunID:   record.RunID,
			Status:  taskStatusInProgress,
			Message: fmt.Sprintf("subagent started (role=%s, timeout=%ds)", role, timeoutSeconds),
		}); err != nil {
			logger.Warn("Failed to append task progress for subagent start",
				zap.String("run_id", record.RunID),
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}

	if _, err := m.subagentRuntime.Spawn(context.Background(), runReq); err != nil {
		endedAt := time.Now().UnixMilli()
		_ = m.subagentRegistry.MarkCompleted(record.RunID, &SubagentRunOutcome{
			Status: agentruntime.RunStatusError,
			Error:  err.Error(),
		}, &endedAt)
		if m.taskStore != nil && strings.TrimSpace(record.TaskID) != "" {
			taskID := strings.TrimSpace(record.TaskID)
			_ = m.taskStore.UpdateTaskStatus(taskID, taskStatusBlocked)
			_ = m.taskStore.AppendTaskProgress(TaskProgressInput{
				TaskID:  taskID,
				RunID:   record.RunID,
				Status:  taskStatusBlocked,
				Message: fmt.Sprintf("subagent failed to start: %v", err),
			})
		}
		return fmt.Errorf("failed to spawn subagent runtime: %w", err)
	}

	go m.waitSubagentResult(record.RunID)

	logger.Info("Subagent runtime started",
		zap.String("run_id", result.RunID),
		zap.String("role", role),
		zap.String("goclawdir", workspaceRoot),
		zap.String("roledir", roleDir),
		zap.String("repodir", repoDir))

	return nil
}

// sendToSession 发送消息到指定会话
func (m *AgentManager) sendToSession(sessionKey, message string) error {
	channel, accountID, chatID := parseRequesterSessionKey(sessionKey)
	if channel == "" {
		channel = "cli"
	}
	if accountID == "" {
		accountID = "default"
	}
	if chatID == "" {
		chatID = "default"
	}
	inbound := &bus.InboundMessage{
		Channel:   channel,
		AccountID: accountID,
		ChatID:    chatID,
		Content:   message,
		Metadata: map[string]interface{}{
			"source":                "subagent_announce",
			"requester_session_key": sessionKey,
		},
		Timestamp: time.Now(),
	}

	return m.RouteInbound(context.Background(), inbound)
}

func (m *AgentManager) waitSubagentResult(runID string) {
	taskID := ""
	if record, ok := m.subagentRegistry.GetRun(runID); ok {
		taskID = strings.TrimSpace(record.TaskID)
	}

	res, err := m.subagentRuntime.Wait(context.Background(), runID)
	endedAt := time.Now().UnixMilli()
	outcome := &SubagentRunOutcome{
		Status: agentruntime.RunStatusError,
	}

	if err != nil {
		outcome.Error = err.Error()
	} else if res == nil {
		outcome.Error = "subagent runtime returned nil result"
	} else {
		outcome.Status = normalizeRuntimeStatus(res.Status)
		outcome.Result = strings.TrimSpace(res.Output)
		if strings.TrimSpace(res.ErrorMsg) != "" {
			outcome.Error = strings.TrimSpace(res.ErrorMsg)
		}
	}

	if markErr := m.subagentRegistry.MarkCompleted(runID, outcome, &endedAt); markErr != nil {
		logger.Error("Failed to mark subagent run completed",
			zap.String("run_id", runID),
			zap.Error(markErr))
	}

	if m.taskStore == nil {
		return
	}

	if taskID == "" {
		resolvedTaskID, resolveErr := m.taskStore.ResolveTaskByRun(runID)
		if resolveErr != nil {
			logger.Warn("Failed to resolve task by run",
				zap.String("run_id", runID),
				zap.Error(resolveErr))
		}
		taskID = strings.TrimSpace(resolvedTaskID)
	}
	if taskID == "" {
		return
	}

	nextStatus := taskStatusBlocked
	switch outcome.Status {
	case agentruntime.RunStatusOK:
		nextStatus = taskStatusCompleted
	case agentruntime.RunStatusTimeout:
		nextStatus = taskStatusBlocked
	case agentruntime.RunStatusError:
		nextStatus = taskStatusBlocked
	}

	if err := m.taskStore.UpdateTaskStatus(taskID, nextStatus); err != nil {
		logger.Warn("Failed to update task status by subagent outcome",
			zap.String("run_id", runID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	progressMsg := strings.TrimSpace(outcome.Result)
	if progressMsg == "" {
		progressMsg = strings.TrimSpace(outcome.Error)
	}
	if progressMsg == "" {
		progressMsg = "subagent finished with no output"
	}

	if err := m.taskStore.AppendTaskProgress(TaskProgressInput{
		TaskID:  taskID,
		RunID:   runID,
		Status:  outcome.Status,
		Message: progressMsg,
	}); err != nil {
		logger.Warn("Failed to append task progress by subagent outcome",
			zap.String("run_id", runID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func normalizeRuntimeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case agentruntime.RunStatusOK:
		return agentruntime.RunStatusOK
	case agentruntime.RunStatusTimeout:
		return agentruntime.RunStatusTimeout
	case agentruntime.RunStatusError:
		return agentruntime.RunStatusError
	default:
		return agentruntime.RunStatusError
	}
}

func parseRequesterSessionKey(sessionKey string) (channel string, accountID string, chatID string) {
	parts := strings.Split(sessionKey, ":")
	switch {
	case len(parts) >= 3:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(strings.Join(parts[2:], ":"))
	case len(parts) == 2:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), "default"
	case len(parts) == 1 && strings.TrimSpace(parts[0]) != "":
		return "cli", "default", strings.TrimSpace(parts[0])
	default:
		return "cli", "default", "default"
	}
}

func (m *AgentManager) getWorkspaceRoot() string {
	if strings.TrimSpace(m.workspace) != "" {
		return strings.TrimSpace(m.workspace)
	}
	if m.cfg != nil && strings.TrimSpace(m.cfg.Workspace.Path) != "" {
		return strings.TrimSpace(m.cfg.Workspace.Path)
	}
	if strings.TrimSpace(m.dataDir) != "" {
		return strings.TrimSpace(m.dataDir)
	}
	return "."
}

func (m *AgentManager) getSubagentsConfig() *config.SubagentsConfig {
	if m.cfg == nil {
		return nil
	}
	return m.cfg.Agents.Defaults.Subagents
}

// createAgent 创建 Agent 实例
func (m *AgentManager) createAgent(cfg config.AgentConfig, contextBuilder *ContextBuilder, globalCfg *config.Config) error {
	// 获取 workspace 路径
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = globalCfg.Workspace.Path
	}

	// 获取模型
	model := cfg.Model
	if model == "" {
		model = globalCfg.Agents.Defaults.Model
	}

	// 获取最大迭代次数
	maxIterations := globalCfg.Agents.Defaults.MaxIterations
	if maxIterations == 0 {
		maxIterations = 15
	}

	// 创建 Agent
	agent, err := NewAgent(&NewAgentConfig{
		Bus:          m.bus,
		SessionMgr:   m.sessionMgr,
		Tools:        m.tools,
		Context:      contextBuilder,
		Workspace:    workspace,
		MaxIteration: maxIterations,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent %s: %w", cfg.ID, err)
	}

	// 设置系统提示词
	if cfg.SystemPrompt != "" {
		agent.SetSystemPrompt(cfg.SystemPrompt)
	}

	// 存储到管理器
	m.agents[cfg.ID] = agent

	// 如果是默认 Agent，设置默认
	if cfg.Default {
		m.defaultAgent = agent
	}

	logger.Info("Agent created",
		zap.String("agent_id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("workspace", workspace),
		zap.String("model", model),
		zap.Bool("is_default", cfg.Default))

	return nil
}

// setupBinding 设置 Agent 绑定
func (m *AgentManager) setupBinding(binding config.BindingConfig) error {
	// 获取 Agent
	agent, ok := m.agents[binding.AgentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", binding.AgentID)
	}

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", binding.Match.Channel, binding.Match.AccountID)

	// 存储绑定
	m.bindings[bindingKey] = &BindingEntry{
		AgentID:   binding.AgentID,
		Channel:   binding.Match.Channel,
		AccountID: binding.Match.AccountID,
		Agent:     agent,
	}

	logger.Info("Binding setup",
		zap.String("binding_key", bindingKey),
		zap.String("agent_id", binding.AgentID))

	return nil
}

// RouteInbound 路由入站消息到对应的 Agent
func (m *AgentManager) RouteInbound(ctx context.Context, msg *bus.InboundMessage) error {
	m.mu.RLock()

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", msg.Channel, msg.AccountID)

	// 查找绑定的 Agent
	entry, ok := m.bindings[bindingKey]
	var agent *Agent
	agentID := ""
	if ok {
		agent = entry.Agent
		agentID = entry.AgentID
		logger.Debug("Message routed by binding",
			zap.String("binding_key", bindingKey),
			zap.String("agent_id", entry.AgentID))
	} else if m.defaultAgent != nil {
		// 使用默认 Agent
		agent = m.defaultAgent
		for id, a := range m.agents {
			if a == agent {
				agentID = id
				break
			}
		}
		if agentID == "" {
			agentID = "default"
		}
		logger.Debug("Message routed to default agent",
			zap.String("channel", msg.Channel),
			zap.String("account_id", msg.AccountID),
			zap.String("agent_id", agentID))
	} else {
		m.mu.RUnlock()
		return fmt.Errorf("no agent found for message: %s", bindingKey)
	}
	m.mu.RUnlock()

	// 处理消息
	return m.handleInboundMessage(ctx, msg, agent, agentID)
}

// handleInboundMessage 处理入站消息
func (m *AgentManager) handleInboundMessage(ctx context.Context, msg *bus.InboundMessage, agent *Agent, agentID string) error {
	// 调用 Agent 处理消息（内部逻辑和 agent.go 中的 handleInboundMessage 类似）
	logger.Info("Processing inbound message",
		zap.String("channel", msg.Channel),
		zap.String("account_id", msg.AccountID),
		zap.String("chat_id", msg.ChatID))

	// 生成会话键（显式 chat 会复用，默认 chat 自动生成新会话）
	sessionKey, fresh := ResolveSessionKey(SessionKeyOptions{
		Channel:        msg.Channel,
		AccountID:      msg.AccountID,
		ChatID:         msg.ChatID,
		FreshOnDefault: true,
		Now:            msg.Timestamp,
	})
	if fresh {
		logger.Info("Creating fresh session", zap.String("session_key", sessionKey))
	}

	// 获取或创建会话
	sess, err := m.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return err
	}

	// 转换为 Agent 消息
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: msg.Timestamp.UnixMilli(),
	}

	// 添加媒体内容
	for _, media := range msg.Media {
		if media.Type == "image" {
			agentMsg.Content = append(agentMsg.Content, ImageContent{
				URL:      media.URL,
				Data:     media.Base64,
				MimeType: media.MimeType,
			})
		}
	}

	// 透传会话上下文，供 sessions_spawn 等工具读取请求来源
	ctx = context.WithValue(ctx, agentruntime.CtxSessionKey, sessionKey)
	ctx = context.WithValue(ctx, agentruntime.CtxAgentID, strings.TrimSpace(agentID))
	ctx = context.WithValue(ctx, agentruntime.CtxChannel, strings.TrimSpace(msg.Channel))
	ctx = context.WithValue(ctx, agentruntime.CtxAccountID, strings.TrimSpace(msg.AccountID))
	ctx = context.WithValue(ctx, agentruntime.CtxChatID, strings.TrimSpace(msg.ChatID))

	if m.mainRuntime == nil {
		return fmt.Errorf("main runtime is not configured")
	}

	media := make([]MainRunMedia, 0, len(msg.Media))
	for _, item := range msg.Media {
		media = append(media, MainRunMedia{
			Type:     item.Type,
			URL:      item.URL,
			Base64:   item.Base64,
			MimeType: item.MimeType,
		})
	}

	runResp, runErr := m.mainRuntime.Run(ctx, MainRunRequest{
		AgentID:      strings.TrimSpace(agentID),
		SessionKey:   sessionKey,
		Prompt:       msg.Content,
		SystemPrompt: agent.GetState().SystemPrompt,
		Workspace:    agent.GetWorkspace(),
		Media:        media,
		Metadata: map[string]any{
			"channel":    msg.Channel,
			"account_id": msg.AccountID,
			"chat_id":    msg.ChatID,
		},
	})
	if runErr != nil {
		logger.Error("Main runtime execution failed", zap.Error(runErr))
		return runErr
	}

	output := ""
	if runResp != nil {
		output = strings.TrimSpace(runResp.Output)
	}
	if output == "" {
		output = "(no output)"
	}

	finalMessages := []AgentMessage{
		agentMsg,
		{
			Role:      RoleAssistant,
			Content:   []ContentBlock{TextContent{Text: output}},
			Timestamp: time.Now().UnixMilli(),
		},
	}

	// 更新会话
	m.updateSession(sess, finalMessages)

	// 发布响应
	if len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			m.publishToBus(ctx, msg.Channel, msg.ChatID, msg.Metadata, lastMsg)
		}
	}

	return nil
}

// updateSession 更新会话
func (m *AgentManager) updateSession(sess *session.Session, messages []AgentMessage) {
	for _, msg := range messages {
		sessMsg := session.Message{
			Role:      string(msg.Role),
			Content:   extractTextContent(msg),
			Timestamp: time.Unix(msg.Timestamp/1000, 0),
		}

		if msg.Role == RoleAssistant {
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					sessMsg.ToolCalls = []session.ToolCall{
						{ID: tc.ID, Name: tc.Name, Params: tc.Arguments},
					}
				}
			}
		}

		if msg.Role == RoleToolResult {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				sessMsg.ToolCallID = id
			}
		}

		sess.AddMessage(sessMsg)
	}

	if err := m.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
		return
	}

	m.exportSessionMarkdown(sess)
}

func (m *AgentManager) exportSessionMarkdown(sess *session.Session) {
	if m == nil || sess == nil || m.cfg == nil || m.sessionMgr == nil {
		return
	}

	memCfg := m.cfg.Memory
	if strings.TrimSpace(memCfg.Backend) != "" && strings.TrimSpace(memCfg.Backend) != "memsearch" {
		return
	}

	ms := memCfg.Memsearch
	if !ms.Sessions.Enabled {
		return
	}

	exportDir := strings.TrimSpace(ms.Sessions.ExportDir)
	if exportDir == "" {
		homeDir, err := config.ResolveUserHomeDir()
		if err != nil {
			return
		}
		exportDir = filepath.Join(homeDir, ".goclaw", "sessions", "export")
	}
	exportDir = config.ExpandUserPath(exportDir)
	if exportDir == "" {
		return
	}

	jsonlPath := m.sessionMgr.SessionPath(sess.Key)
	if strings.TrimSpace(jsonlPath) == "" {
		return
	}

	if _, err := memory.ExportSessionJSONLToMarkdown(jsonlPath, exportDir, ms.Sessions.Redact); err != nil {
		logger.Warn("Failed to export session markdown", zap.String("session", sess.Key), zap.Error(err))
	}
}

// publishToBus 发布消息到总线
func (m *AgentManager) publishToBus(ctx context.Context, channel, chatID string, metadata map[string]interface{}, msg AgentMessage) {
	content := extractTextContent(msg)
	outboundMetadata := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		outboundMetadata[k] = v
	}

	outbound := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Unix(msg.Timestamp/1000, 0),
		Metadata:  outboundMetadata,
	}

	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish outbound", zap.Error(err))
	}
}

// GetAgent 获取 Agent
func (m *AgentManager) GetAgent(agentID string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	return agent, ok
}

// ListAgents 列出所有 Agent ID
func (m *AgentManager) ListAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	return ids
}

// Start 启动所有 Agent
func (m *AgentManager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.mainRuntime == nil {
		return fmt.Errorf("main runtime is not configured")
	}

	// 启动消息处理器
	go m.processMessages(ctx)

	return nil
}

// Stop 停止所有 Agent
func (m *AgentManager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.mainRuntime == nil {
		return nil
	}

	return m.mainRuntime.Close()
}

// processMessages 处理入站消息
func (m *AgentManager) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent manager message processor stopped")
			return
		default:
			msg, err := m.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound", zap.Error(err))
				continue
			}

			// Route inbound via dispatcher to avoid blocking the consumer goroutine.
			if m.inbound != nil {
				err = m.inbound.Dispatch(ctx, msg)
			} else {
				err = m.RouteInbound(ctx, msg)
			}
			if err != nil {
				logger.Error("Failed to route message",
					zap.String("channel", msg.Channel),
					zap.String("account_id", msg.AccountID),
					zap.Error(err))
			}
		}
	}
}

func (m *AgentManager) sendQueueAck(sessionKey string, msg *bus.InboundMessage, ahead int) {
	if m == nil || m.bus == nil || msg == nil {
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	chatID := strings.TrimSpace(msg.ChatID)
	if channel == "" || chatID == "" {
		return
	}

	// Only a lightweight receipt; the real response will follow later.
	content := "已收到，正在处理。"
	if ahead > 0 {
		content = fmt.Sprintf("已收到，正在处理（队列前方约 %d 条）。", ahead)
	}

	metadata := map[string]interface{}{
		"type":        "queue_ack",
		"session_key": strings.TrimSpace(sessionKey),
		"ahead":       ahead,
	}

	out := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		ReplyTo:   strings.TrimSpace(msg.ID),
		Metadata:  metadata,
		Timestamp: time.Now(),
	}
	if err := m.bus.PublishOutbound(context.Background(), out); err != nil {
		logger.Debug("Failed to send queue ack", zap.Error(err))
	}
}

// GetDefaultAgent 获取默认 Agent
func (m *AgentManager) GetDefaultAgent() *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultAgent
}

// GetToolsInfo 获取工具信息
func (m *AgentManager) GetToolsInfo() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从 tool registry 获取工具列表
	existingTools := m.tools.ListExisting()
	result := make(map[string]interface{})

	for _, tool := range existingTools {
		result[tool.Name()] = map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		}
	}

	return result, nil
}
