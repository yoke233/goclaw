package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/goclaw/agent"
	tasksdk "github.com/smallnest/goclaw/agent/tasksdk"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/memory"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run one agent turn",
	Long:  `Execute a single agent interaction with a message and optional parameters.`,
	Run:   runAgent,
}

// Flags for agent command
var (
	agentMessage   string
	agentTo        string
	agentSessionID string
	agentThinking  bool
	agentVerbose   bool
	agentChannel   string
	agentLocal     bool
	agentDeliver   bool
	agentJSON      bool
	agentTimeout   int
)

func init() {
	agentCmd.Flags().StringVar(&agentMessage, "message", "", "Message to send to the agent (required)")
	agentCmd.Flags().StringVar(&agentTo, "to", "", "Target agent name")
	agentCmd.Flags().StringVar(&agentSessionID, "session-id", "", "Session ID to use")
	agentCmd.Flags().BoolVar(&agentThinking, "thinking", false, "Show thinking process")
	agentCmd.Flags().BoolVar(&agentVerbose, "verbose", false, "Enable verbose output")
	agentCmd.Flags().StringVar(&agentChannel, "channel", "cli", "Channel to use (cli, telegram, etc.)")
	agentCmd.Flags().BoolVar(&agentLocal, "local", false, "Run in local mode without connecting to channels")
	agentCmd.Flags().BoolVar(&agentDeliver, "deliver", false, "Deliver response through the channel")
	agentCmd.Flags().BoolVar(&agentJSON, "json", false, "Output in JSON format")
	agentCmd.Flags().IntVar(&agentTimeout, "timeout", 120, "Timeout in seconds")

	_ = agentCmd.MarkFlagRequired("message")
}

// runAgent executes a single agent turn
func runAgent(cmd *cobra.Command, args []string) {
	// Validate message
	if agentMessage == "" {
		fmt.Fprintf(os.Stderr, "Error: --message is required\n")
		os.Exit(1)
	}

	// Initialize logger if verbose or thinking mode is enabled
	if agentVerbose || agentThinking {
		if err := logger.Init("debug", false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = logger.Sync() }()
	}

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create workspace
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create workspace: %v\n", err)
		os.Exit(1)
	}

	// Create message bus
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Create session manager
	sessionDir := os.Getenv("HOME") + "/.goclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session manager: %v\n", err)
		os.Exit(1)
	}

	// Create memory store (memsearch)
	var searchMgr memory.MemorySearchManager
	searchMgr, err = memory.GetMemorySearchManager(cfg.Memory, workspace)
	if err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to create memory search manager: %v\n", err)
	}

	contextCfg := cfg.Memory.Memsearch.Context
	if contextCfg.Limit == 0 {
		contextCfg.Limit = 6
	}
	memoryStore := agent.NewMemoryStore(workspace, searchMgr, contextCfg.Query, contextCfg.Limit, contextCfg.Enabled)
	if err := memoryStore.EnsureBootstrapFiles(); err != nil {
		if agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create bootstrap files: %v\n", err)
		}
	}

	// Create tool registry
	toolRegistry := agent.NewToolRegistry()

	// Register memory tools
	if searchMgr != nil {
		if err := toolRegistry.RegisterExisting(tools.NewMemoryTool(searchMgr)); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register memory_search tool: %v\n", err)
		}
		if err := toolRegistry.RegisterExisting(tools.NewMemoryAddTool(searchMgr)); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register memory_add tool: %v\n", err)
		}
	}

	// Register file system tool
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths, workspace)
	for _, tool := range fsTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
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
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
	}

	// Register web tool
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
	}

	// Register smart search tool
	browserTimeout := 30
	if cfg.Tools.Browser.Timeout > 0 {
		browserTimeout = cfg.Tools.Browser.Timeout
	}
	if err := toolRegistry.RegisterExisting(tools.NewSmartSearch(webTool, true, browserTimeout).GetTool()); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to register smart_search: %v\n", err)
	}

	// Register browser tool if enabled
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
				fmt.Fprintf(os.Stderr, "Warning: Failed to register browser tool %s: %v\n", tool.Name(), err)
			}
		}
	}

	// Register use_skill tool
	if err := toolRegistry.RegisterExisting(tools.NewUseSkillTool()); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to register use_skill: %v\n", err)
	}

	agentSDKTaskStore, err := tasksdk.NewSQLiteStore(filepath.Join(workspace, "data", "agentsdk_tasks.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize agentsdk task store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = agentSDKTaskStore.Close() }()

	// Create main runtime
	mainRuntime, err := agent.NewAgentSDKMainRuntime(agent.AgentSDKMainRuntimeOptions{
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(agentTimeout)*time.Second)
	defer cancel()

	// Determine session key
	sessionKey, _ := agent.ResolveSessionKey(agent.SessionKeyOptions{
		Explicit:       agentSessionID,
		Channel:        agentChannel,
		AccountID:      "agent",
		ChatID:         "default",
		FreshOnDefault: true,
		Now:            time.Now(),
	})

	// Get or create session
	sess, err := sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get session: %v\n", err)
		os.Exit(1)
	}

	// Add user message to session
	sess.AddMessage(session.Message{
		Role:      "user",
		Content:   agentMessage,
		Timestamp: time.Now(),
	})

	runResp, err := mainRuntime.Run(ctx, agent.MainRunRequest{
		AgentID:      strings.TrimSpace(agentTo),
		SessionKey:   sessionKey,
		Prompt:       agentMessage,
		SystemPrompt: "",
		Workspace:    workspace,
		Metadata: map[string]any{
			"channel": agentChannel,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Agent execution failed: %v\n", err)
		os.Exit(1)
	}

	response := ""
	if runResp != nil {
		response = strings.TrimSpace(runResp.Output)
	}
	if response == "" {
		response = "(no output)"
	}

	// Add assistant response to session
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	// Save session
	if err := sessionMgr.Save(sess); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save session: %v\n", err)
	}

	// Output response
	if agentJSON {
		result := map[string]interface{}{
			"response": response,
			"success":  true,
			"session":  sessionKey,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		if agentThinking {
			fmt.Println("\n💡 Response:")
		}
		fmt.Println(response)
	}

	// Deliver through channel if requested
	if agentDeliver && !agentLocal {
		if err := deliverResponse(ctx, messageBus, response); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to deliver response: %v\n", err)
		}
	}
}

// deliverResponse delivers the response through the configured channel
func deliverResponse(ctx context.Context, messageBus *bus.MessageBus, content string) error {
	return messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   agentChannel,
		ChatID:    "default",
		Content:   content,
		Timestamp: time.Now(),
	})
}
