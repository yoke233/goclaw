package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/smallnest/goclaw/agent"
	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	tasksdk "github.com/smallnest/goclaw/agent/tasksdk"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/cli/input"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/memory"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	tuiURL          string
	tuiToken        string
	tuiPassword     string
	tuiSession      string
	tuiDeliver      bool
	tuiThinking     bool
	tuiMessage      string
	tuiTimeoutMs    int
	tuiHistoryLimit int
)

// TUICommand returns the tui command
func TUICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open Terminal UI for goclaw",
		Long:  `Open an interactive terminal UI for interacting with goclaw agent.`,
		Run:   runTUI,
	}

	cmd.Flags().StringVar(&tuiURL, "url", "", "Gateway URL (default: ws://localhost:18789)")
	cmd.Flags().StringVar(&tuiToken, "token", "", "Authentication token")
	cmd.Flags().StringVar(&tuiPassword, "password", "", "Password for authentication")
	cmd.Flags().StringVar(&tuiSession, "session", "", "Session ID to resume")
	cmd.Flags().BoolVar(&tuiDeliver, "deliver", false, "Enable message delivery notifications")
	cmd.Flags().BoolVar(&tuiThinking, "thinking", false, "Show thinking indicator")
	cmd.Flags().StringVar(&tuiMessage, "message", "", "Send message on start")
	cmd.Flags().IntVar(&tuiTimeoutMs, "timeout-ms", 600000, "Timeout in milliseconds")
	cmd.Flags().IntVar(&tuiHistoryLimit, "history-limit", 50, "History limit")

	return cmd
}

// runTUI runs the terminal UI
func runTUI(cmd *cobra.Command, args []string) {
	// 确保内置技能被复制到用户目录
	if err := internal.EnsureBuiltinSkills(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to ensure builtin skills: %v\n", err)
	}

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logLevel := "info"
	if tuiThinking {
		logLevel = "debug"
	}
	if err := logger.Init(logLevel, false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // nolint:errcheck

	fmt.Println("🐾 goclaw Terminal UI")
	fmt.Println()

	// Create workspace
	workspace, err := config.GetWorkspacePath(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve workspace: %v\n", err)
		os.Exit(1)
	}

	// Create message bus
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Create session manager
	homeDir, err := config.ResolveUserHomeDir()
	if err != nil {
		homeDir = ""
	}
	sessionDir := filepath.Join(homeDir, ".goclaw", "sessions")
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session manager: %v\n", err)
		os.Exit(1)
	}

	// Create memory store
	var searchMgr memory.MemorySearchManager
	searchMgr, err = memory.GetMemorySearchManager(cfg.Memory, workspace)
	if err != nil {
		logger.Warn("Failed to create memory search manager", zap.Error(err))
	}

	contextCfg := cfg.Memory.Memsearch.Context
	if contextCfg.Limit == 0 {
		contextCfg.Limit = 6
	}
	memoryStore := agent.NewMemoryStore(workspace, searchMgr, contextCfg.Query, contextCfg.Limit, contextCfg.Enabled)
	_ = memoryStore.EnsureBootstrapFiles()

	// Create tool registry
	toolRegistry := agent.NewToolRegistry()
	contextBuilder := agent.NewContextBuilder(memoryStore, workspace)
	contextBuilder.SetToolRegistry(toolRegistry)

	// Runtime invalidator (tools call this; mainRuntime is assigned later).
	var mainRuntime *agent.AgentSDKMainRuntime
	invalidateRuntime := tools.RuntimeInvalidator(func(ctx context.Context, agentID string) error {
		if mainRuntime == nil {
			return fmt.Errorf("main runtime is not initialized")
		}
		return mainRuntime.Invalidate(strings.TrimSpace(agentID))
	})

	// Register memory tools
	if searchMgr != nil {
		_ = toolRegistry.RegisterExisting(tools.NewMemoryTool(searchMgr))
		_ = toolRegistry.RegisterExisting(tools.NewMemoryAddTool(searchMgr))
	}

	// Register file system tool
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths, workspace)
	for _, tool := range fsTool.GetTools() {
		_ = toolRegistry.RegisterExisting(tool)
	}

	// Register use_skill tool
	_ = toolRegistry.RegisterExisting(tools.NewUseSkillTool())

	// Register skills + MCP management tools (conversation-accessible)
	skillsRoleDir := "skills"
	if sub := cfg.Agents.Defaults.Subagents; sub != nil {
		if strings.TrimSpace(sub.SkillsRoleDir) != "" {
			skillsRoleDir = strings.TrimSpace(sub.SkillsRoleDir)
		}
	}
	for _, tool := range []tools.Tool{
		tools.NewSkillsListTool(workspace, skillsRoleDir),
		tools.NewSkillsGetTool(workspace, skillsRoleDir),
		tools.NewSkillsPutTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewSkillsDeleteTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewSkillsSetEnabledTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewMCPListTool(workspace, skillsRoleDir),
		tools.NewMCPPutServerTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewMCPDeleteServerTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewMCPSetEnabledTool(workspace, skillsRoleDir, invalidateRuntime),
		tools.NewRuntimeReloadTool(invalidateRuntime),
	} {
		if tool == nil {
			continue
		}
		_ = toolRegistry.RegisterExisting(tool)
	}

	// Register shell tool
	shellTool := tools.NewShellTool(
		cfg.Tools.Shell.Enabled,
		cfg.Tools.Shell.AllowedCmds,
		cfg.Tools.Shell.DeniedCmds,
		cfg.Tools.Shell.Timeout,
		cfg.Tools.Shell.WorkingDir,
		cfg.Tools.Shell.Sandbox,
	)
	for _, tool := range shellTool.GetTools() {
		_ = toolRegistry.RegisterExisting(tool)
	}

	// Register web tool
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		_ = toolRegistry.RegisterExisting(tool)
	}

	// Register smart search
	browserTimeout := 30
	if cfg.Tools.Browser.Timeout > 0 {
		browserTimeout = cfg.Tools.Browser.Timeout
	}
	_ = toolRegistry.RegisterExisting(tools.NewSmartSearch(webTool, true, browserTimeout).GetTool())

	// Register browser tool
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			_ = toolRegistry.RegisterExisting(tool)
		}
	}

	// Create skills loader（统一使用 ~/.goclaw/skills 目录）
	goclawDir := filepath.Join(homeDir, ".goclaw")
	skillsDir := filepath.Join(goclawDir, "skills")
	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{skillsDir})
	if err := skillsLoader.Discover(); err != nil {
		logger.Warn("Failed to discover skills", zap.Error(err))
	} else {
		skills := skillsLoader.List()
		if len(skills) > 0 {
			logger.Info("Skills loaded", zap.Int("count", len(skills)))
		}
	}

	agentSDKTaskStore, err := tasksdk.NewSQLiteStore(filepath.Join(workspace, "data", "agentsdk_tasks.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize agentsdk task store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = agentSDKTaskStore.Close() }()

	taskTracker, err := tasksdk.NewTracker(agentSDKTaskStore, filepath.Join(workspace, "data", "subagent_task_tracker.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize subagent task tracker: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = taskTracker.Close() }()

	mainRuntime, err = agent.NewAgentSDKMainRuntime(agent.AgentSDKMainRuntimeOptions{
		Config:           cfg,
		Tools:            toolRegistry,
		DefaultWorkspace: workspace,
		TaskStore:        agentSDKTaskStore,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create main runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = mainRuntime.Close() }()

	subagentRuntime, _ := buildSubagentRuntimeForTUI(cfg)
	agentManager := agent.NewAgentManager(&agent.NewAgentManagerConfig{
		Bus:             messageBus,
		SessionMgr:      sessionMgr,
		Tools:           toolRegistry,
		DataDir:         workspace,
		Workspace:       workspace,
		SubagentRuntime: subagentRuntime,
		MainRuntime:     mainRuntime,
		TaskStore:       taskTracker,
	})
	if err := agentManager.SetupFromConfig(cfg, contextBuilder); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup agent manager: %v\n", err)
		os.Exit(1)
	}

	// Always create a new session unless --session 显式指定
	sessionKey, _ := agent.ResolveSessionKey(agent.SessionKeyOptions{
		Explicit:       tuiSession,
		Channel:        "tui",
		AccountID:      "tui",
		ChatID:         "default",
		FreshOnDefault: true,
		Now:            time.Now(),
	})

	sess, err := sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("New Session: %s\n", sessionKey)
	fmt.Printf("History limit: %d\n", tuiHistoryLimit)
	fmt.Printf("Timeout: %d ms\n", tuiTimeoutMs)
	fmt.Println()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create command registry for slash commands
	cmdRegistry := NewCommandRegistry()
	cmdRegistry.SetSessionManager(sessionMgr)
	cmdRegistry.SetToolGetter(func() (map[string]interface{}, error) {
		// 从 toolRegistry 获取工具信息
		existingTools := toolRegistry.ListExisting()
		result := make(map[string]interface{})
		for _, tool := range existingTools {
			result[tool.Name()] = map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.Parameters(),
			}
		}
		return result, nil
	})

	cmdRegistry.SetSkillsGetter(func() ([]*SkillInfo, error) {
		// 从 skillsLoader 获取技能信息
		agentSkills := skillsLoader.List()
		result := make([]*SkillInfo, 0, len(agentSkills))
		for _, skill := range agentSkills {
			skillInfo := &SkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
				Version:     skill.Version,
				Author:      skill.Author,
				Homepage:    skill.Homepage,
				Always:      skill.Always,
				Emoji:       skill.Metadata.OpenClaw.Emoji,
			}
			// 转换缺失依赖信息
			if skill.MissingDeps != nil {
				skillInfo.MissingDeps = &MissingDepsInfo{
					Bins:       skill.MissingDeps.Bins,
					AnyBins:    skill.MissingDeps.AnyBins,
					Env:        skill.MissingDeps.Env,
					PythonPkgs: skill.MissingDeps.PythonPkgs,
					NodePkgs:   skill.MissingDeps.NodePkgs,
				}
			}
			result = append(result, skillInfo)
		}
		return result, nil
	})

	// Handle message flag
	if tuiMessage != "" {
		fmt.Printf("Sending message: %s\n", tuiMessage)
		sess.AddMessage(session.Message{
			Role:    "user",
			Content: tuiMessage,
		})

		timeout := time.Duration(tuiTimeoutMs) * time.Millisecond
		msgCtx, msgCancel := context.WithTimeout(ctx, timeout)
		defer msgCancel()

		response, err := runAgentIteration(msgCtx, sess, mainRuntime, toolRegistry, cmdRegistry, agentManager)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Println("\n" + response + "\n")
			sess.AddMessage(session.Message{
				Role:    "assistant",
				Content: response,
			})
			_ = sessionMgr.Save(sess)
			exportSessionMarkdown(cfg, sessionMgr, sess)
		}

		if !tuiDeliver {
			return
		}
	}

	// Start interactive mode
	fmt.Println("Starting interactive TUI mode...")
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println()
	fmt.Println("Arrow keys: ↑/↓ for history, ←/→ for edit")
	fmt.Println()

	// Create persistent readline instance for history navigation
	rl, err := input.NewReadline("➤ ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	// Initialize history from session
	input.InitReadlineHistory(rl, getUserInputHistory(sess))

	// Input loop with persistent readline
	fmt.Println("Enter your message (or /help for commands):")
	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		// Save non-empty input to history
		if line != "" {
			_ = rl.SaveHistory(line)
		}

		if line == "" {
			continue
		}

		// Echo the input with prompt (readline doesn't automatically print after Enter)
		fmt.Printf("%s%s\n", "➤ ", line)

		// Check for commands
		result, isCommand, shouldExit := cmdRegistry.Execute(line)
		if isCommand {
			if shouldExit {
				fmt.Println("Goodbye!")
				break
			}
			if result != "" {
				fmt.Println(result)
			}
			continue
		}

		// Add user message
		sess.AddMessage(session.Message{
			Role:    "user",
			Content: line,
		})

		// Run agent
		timeout := time.Duration(tuiTimeoutMs) * time.Millisecond
		msgCtx, msgCancel := context.WithTimeout(ctx, timeout)

		response, err := runAgentIteration(msgCtx, sess, mainRuntime, toolRegistry, cmdRegistry, agentManager)
		msgCancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Println("\n" + response + "\n")
			sess.AddMessage(session.Message{
				Role:    "assistant",
				Content: response,
			})
			_ = sessionMgr.Save(sess)
			exportSessionMarkdown(cfg, sessionMgr, sess)
		}

		// Force readline to refresh terminal state
		rl.Refresh()
	}
}

// runAgentIteration runs a single agent turn via the shared main runtime.
func runAgentIteration(
	ctx context.Context,
	sess *session.Session,
	mainRuntime agent.MainRuntime,
	toolRegistry *agent.ToolRegistry,
	cmdRegistry *CommandRegistry,
	agentManager *agent.AgentManager,
) (string, error) {
	if cmdRegistry != nil && cmdRegistry.IsStopped() {
		return "", nil
	}
	if mainRuntime == nil {
		return "", fmt.Errorf("main runtime is not initialized")
	}
	if sess == nil {
		return "", fmt.Errorf("session is nil")
	}
	if len(toolRegistry.ListExisting()) == 0 {
		logger.Warn("No tools registered for TUI main runtime")
	}

	history := sess.GetHistory(1)
	prompt := ""
	if len(history) > 0 {
		prompt = strings.TrimSpace(history[len(history)-1].Content)
	}
	if prompt == "" {
		return "", nil
	}

	runAgentID := "default"
	runSystemPrompt := ""
	runWorkspace := ""

	if agentManager != nil {
		selectedAgent, ok := agentManager.GetAgent(runAgentID)
		if !ok {
			if defaultAgent := agentManager.GetDefaultAgent(); defaultAgent != nil {
				selectedAgent = defaultAgent
				if id := resolveAgentID(agentManager, defaultAgent); id != "" {
					runAgentID = id
				}
			}
		}
		if selectedAgent != nil {
			if state := selectedAgent.GetState(); strings.TrimSpace(state.SystemPrompt) != "" {
				runSystemPrompt = strings.TrimSpace(state.SystemPrompt)
			}
			if ws := strings.TrimSpace(selectedAgent.GetWorkspace()); ws != "" {
				runWorkspace = ws
			}
		}
	}

	channel, accountID, chatID := parseSessionKey(sess.Key)
	runCtx := context.WithValue(ctx, agentruntime.CtxSessionKey, sess.Key)
	runCtx = context.WithValue(runCtx, agentruntime.CtxAgentID, runAgentID)
	runCtx = context.WithValue(runCtx, agentruntime.CtxChannel, channel)
	runCtx = context.WithValue(runCtx, agentruntime.CtxAccountID, accountID)
	runCtx = context.WithValue(runCtx, agentruntime.CtxChatID, chatID)

	resp, err := mainRuntime.Run(runCtx, agent.MainRunRequest{
		AgentID:      runAgentID,
		SessionKey:   sess.Key,
		Prompt:       prompt,
		SystemPrompt: runSystemPrompt,
		Workspace:    runWorkspace,
		Metadata: map[string]any{
			"channel":    channel,
			"account_id": accountID,
			"chat_id":    chatID,
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return strings.TrimSpace(resp.Output), nil
}

// getLoadedSkills from session
func getLoadedSkills(sess *session.Session) []string {
	if sess.Metadata == nil {
		return []string{}
	}
	if v, ok := sess.Metadata["loaded_skills"].([]string); ok {
		return v
	}
	return []string{}
}

// setLoadedSkills in session
func setLoadedSkills(sess *session.Session, skills []string) {
	if sess.Metadata == nil {
		sess.Metadata = make(map[string]interface{})
	}
	sess.Metadata["loaded_skills"] = skills
}

// getUserInputHistory extracts user message history for readline
func getUserInputHistory(sess *session.Session) []string {
	history := sess.GetHistory(100)
	userInputs := make([]string, 0, len(history))

	// Extract only user messages (in reverse order - most recent first)
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			userInputs = append(userInputs, history[i].Content)
		}
	}

	return userInputs
}

// findMostRecentTUISession finds the most recently updated tui session
func findMostRecentTUISession(mgr *session.Manager) string {
	keys, err := mgr.List()
	if err != nil {
		return ""
	}

	// Filter and collect tui sessions with their update time
	type sessionInfo struct {
		key       string
		updatedAt time.Time
	}

	var tuiSessions []sessionInfo
	for _, key := range keys {
		// Only consider sessions starting with "tui:" or "tui_"
		if !strings.HasPrefix(key, "tui:") && !strings.HasPrefix(key, "tui_") {
			continue
		}

		// Load the session to get its update time
		sess, err := mgr.GetOrCreate(key)
		if err != nil {
			continue
		}

		tuiSessions = append(tuiSessions, sessionInfo{
			key:       key,
			updatedAt: sess.UpdatedAt,
		})
	}

	// If no tui sessions found, return empty
	if len(tuiSessions) == 0 {
		return ""
	}

	// Sort by updated time (most recent first)
	sort.Slice(tuiSessions, func(i, j int) bool {
		return tuiSessions[i].updatedAt.After(tuiSessions[j].updatedAt)
	})

	return tuiSessions[0].key
}

func parseSessionKey(sessionKey string) (channel string, accountID string, chatID string) {
	parts := strings.Split(strings.TrimSpace(sessionKey), ":")
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

func exportSessionMarkdown(cfg *config.Config, sessionMgr *session.Manager, sess *session.Session) {
	if cfg == nil || sessionMgr == nil || sess == nil {
		return
	}

	memCfg := cfg.Memory
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

	jsonlPath := sessionMgr.SessionPath(sess.Key)
	if strings.TrimSpace(jsonlPath) == "" {
		return
	}

	if _, err := memory.ExportSessionJSONLToMarkdown(jsonlPath, exportDir, ms.Sessions.Redact); err != nil {
		logger.Warn("Failed to export session markdown", zap.Error(err))
	}
}

func resolveAgentID(manager *agent.AgentManager, target *agent.Agent) string {
	if manager == nil || target == nil {
		return ""
	}
	for _, id := range manager.ListAgents() {
		current, ok := manager.GetAgent(id)
		if !ok {
			continue
		}
		if current == target {
			return id
		}
	}
	return ""
}

func buildSubagentRuntimeForTUI(cfg *config.Config) (agentruntime.SubagentRuntime, string) {
	subagentCfg := cfg.Agents.Defaults.Subagents
	roleLimits := map[string]int{}
	defaultMaxConcurrent := 8
	if subagentCfg != nil {
		if subagentCfg.MaxConcurrent > 0 {
			defaultMaxConcurrent = subagentCfg.MaxConcurrent
		}
		for role, limit := range subagentCfg.RoleMaxConcurrent {
			if limit <= 0 {
				continue
			}
			roleLimits[role] = limit
		}
	}
	rolePool := agentruntime.NewSimpleRolePool(defaultMaxConcurrent, roleLimits)

	subagentModel := "claude-sonnet-4-5"
	if subagentCfg != nil && strings.TrimSpace(subagentCfg.Model) != "" {
		subagentModel = strings.TrimSpace(subagentCfg.Model)
	}

	maxTokens := cfg.Agents.Defaults.MaxTokens
	temperature := cfg.Agents.Defaults.Temperature

	return agentruntime.NewAgentsdkRuntime(agentruntime.AgentsdkRuntimeOptions{
		Pool:             rolePool,
		AnthropicAPIKey:  strings.TrimSpace(cfg.Providers.Anthropic.APIKey),
		AnthropicBaseURL: strings.TrimSpace(cfg.Providers.Anthropic.BaseURL),
		ModelName:        subagentModel,
		MaxTokens:        maxTokens,
		Temperature:      temperature,
		MaxIterations:    cfg.Agents.Defaults.MaxIterations,
	}), "agentsdk"
}

// FailureTracker 追踪工具调用失败
type FailureTracker struct {
	toolFailures map[string]int // tool_name -> failure count
	totalCount   int
}

// NewFailureTracker 创建失败追踪器
func NewFailureTracker() *FailureTracker {
	return &FailureTracker{
		toolFailures: make(map[string]int),
		totalCount:   0,
	}
}

// RecordFailure 记录工具失败
func (ft *FailureTracker) RecordFailure(toolName string) {
	ft.toolFailures[toolName]++
	ft.totalCount++
	logger.Debug("Tool failure recorded",
		zap.String("tool", toolName),
		zap.Int("count", ft.toolFailures[toolName]),
		zap.Int("total", ft.totalCount))
}

// RecordSuccess 记录工具成功
func (ft *FailureTracker) RecordSuccess(toolName string) {
	// 同一工具成功后，可以重置其失败计数
	if count, ok := ft.toolFailures[toolName]; ok && count > 0 {
		ft.toolFailures[toolName] = 0
	}
}

// HasConsecutiveFailures 检查是否有连续失败
func (ft *FailureTracker) HasConsecutiveFailures(threshold int) bool {
	return ft.totalCount >= threshold
}

// GetFailedToolNames 获取失败的工具名称列表
func (ft *FailureTracker) GetFailedToolNames() []string {
	var names []string
	for name, count := range ft.toolFailures {
		if count > 0 {
			names = append(names, name)
		}
	}
	return names
}

// formatToolError 格式化工具错误，提供替代建议
func formatToolError(toolName string, params map[string]interface{}, err error, availableTools []string) string {
	errorMsg := err.Error()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 工具执行失败: `%s`\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**错误**: %s\n\n", errorMsg))

	// 提供降级建议
	var suggestions []string
	switch toolName {
	case "write_file":
		suggestions = []string{
			"1. **输出到控制台**: 直接将内容显示给用户",
			"2. **使用相对路径**: 尝试使用 `./filename`",
			"3. **使用完整路径**: 尝试使用绝对路径",
			"4. **检查权限**: 确认当前目录有写入权限",
		}
	case "read_file":
		suggestions = []string{
			"1. **检查路径**: 确认文件路径是否正确",
			"2. **列出目录**: 使用 `list_dir` 工具查看目录内容",
			"3. **使用相对路径**: 尝试使用 `./filename`",
		}
	case "smart_search", "web_search":
		suggestions = []string{
			"1. **简化查询**: 使用更简单的关键词",
			"2. **稍后重试**: 网络暂时不可用",
			"3. **告知用户**: 让用户自己搜索并提供结果",
		}
	case "browser":
		suggestions = []string{
			"1. **检查URL**: 确认URL格式正确",
			"2. **使用web_reader**: 尝试使用 web_reader 工具替代",
		}
	default:
		suggestions = []string{
			"1. **检查参数**: 确认工具参数是否正确",
			"2. **尝试替代方案**: 使用其他工具或方法",
		}
	}

	if len(suggestions) > 0 {
		sb.WriteString("**建议的替代方案**:\n\n")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("%s\n", s))
		}
	}

	// 显示可用的替代工具
	if len(availableTools) > 0 {
		sb.WriteString("\n**可用的工具列表**:\n\n")
		for _, tool := range availableTools {
			if tool != toolName {
				sb.WriteString(fmt.Sprintf("- %s\n", tool))
			}
		}
	}

	return sb.String()
}

// shouldUseErrorGuidance 判断是否需要添加错误处理指导
func shouldUseErrorGuidance(history []session.Message) bool {
	// 检查最近的消息中是否有工具失败
	if len(history) == 0 {
		return false
	}

	consecutiveFailures := 0
	for i := len(history) - 1; i >= 0 && i >= len(history)-6; i-- {
		msg := history[i]
		if msg.Role == "tool" {
			if strings.Contains(msg.Content, "## 工具执行失败") ||
				strings.Contains(msg.Content, "Error:") {
				consecutiveFailures++
			} else {
				break // 遇到成功的工具调用就停止
			}
		}
	}

	return consecutiveFailures >= 2
}

// getAvailableToolNames 获取可用的工具名称列表
func getAvailableToolNames(toolRegistry *tools.Registry) []string {
	if toolRegistry == nil {
		return []string{}
	}

	tools := toolRegistry.List()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	return names
}
