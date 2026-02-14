package agent

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// PromptMode 控制系统提示词中包含哪些硬编码部分
// - "full": 所有部分（默认，用于主 agent）
// - "minimal": 精简部分（Tooling, Workspace, Runtime）- 用于子 agent
// - "none": 仅基本身份行，没有部分
type PromptMode string

const (
	PromptModeFull    PromptMode = "full"
	PromptModeMinimal PromptMode = "minimal"
	PromptModeNone    PromptMode = "none"
)

// ContextBuilder 上下文构建器
type ContextBuilder struct {
	memory    *MemoryStore
	workspace string
	tools     *ToolRegistry
}

// NewContextBuilder 创建上下文构建器
func NewContextBuilder(memory *MemoryStore, workspace string) *ContextBuilder {
	return &ContextBuilder{
		memory:    memory,
		workspace: workspace,
	}
}

// SetToolRegistry sets the runtime tool registry used to render dynamic tool hints.
func (b *ContextBuilder) SetToolRegistry(registry *ToolRegistry) {
	b.tools = registry
}

// BuildSystemPrompt 构建系统提示词
func (b *ContextBuilder) BuildSystemPrompt() string {
	return b.BuildSystemPromptWithMode(PromptModeFull)
}

// BuildSystemPromptWithMode 使用指定模式构建系统提示词
func (b *ContextBuilder) BuildSystemPromptWithMode(mode PromptMode) string {
	isMinimal := mode == PromptModeMinimal || mode == PromptModeNone

	// 对于 "none" 模式，只返回基本身份行
	if mode == PromptModeNone {
		return "You are a personal assistant running inside GoClaw."
	}

	var parts []string

	// 1. 核心身份 + 工具列表
	parts = append(parts, b.buildIdentityAndTools())

	// 2. Tool Call Style
	parts = append(parts, b.buildToolCallStyle())

	// 3. 安全提示
	parts = append(parts, b.buildSafety())

	// 4. 错误处理指导（容错模式）
	if !isMinimal {
		parts = append(parts, b.buildErrorHandling())
	}

	// 5. 自动重试指导
	if !isMinimal {
		parts = append(parts, b.buildRetryStrategy())
	}

	// 6. Bootstrap 文件
	if bootstrap := b.loadBootstrapFiles(); bootstrap != "" {
		parts = append(parts, "## Configuration\n\n"+bootstrap)
	}

	// 7. 记忆上下文
	if !isMinimal {
		if memContext, err := b.memory.GetMemoryContext(); err == nil && memContext != "" {
			parts = append(parts, memContext)
		}
	}

	// 8. 工作区和运行时信息
	parts = append(parts, b.buildWorkspace())
	if !isMinimal {
		parts = append(parts, b.buildRuntime())
	}

	return fmt.Sprintf("%s\n\n", joinNonEmpty(parts, "\n\n---\n\n"))
}

// buildIdentityAndTools 构建核心身份和工具列表
func (b *ContextBuilder) buildIdentityAndTools() string {
	now := time.Now()

	// 定义核心工具摘要
	coreToolSummaries := map[string]string{
		"smart_search":           "Intelligent search with automatic fallback (always use for search requests)",
		"browser_navigate":       "Navigate to a URL",
		"browser_screenshot":     "Take page screenshots",
		"browser_get_text":       "Get page text content",
		"browser_click":          "Click elements on the page",
		"browser_fill_input":     "Fill input fields",
		"browser_execute_script": "Execute JavaScript",
		"read_file":              "Read file contents",
		"write_file":             "Create or overwrite files",
		"list_files":             "List directory contents",
		"run_shell":              "Run shell commands (supports timeout and error handling)",
		"web_search":             "Search the web using API",
		"web_fetch":              "Fetch web pages",
		"memory_search":          "Search stored memory for user preferences, prior decisions, and project context",
		"memory_add":             "Persist durable facts and user preferences for future conversations",
		"sessions_spawn":         "Spawn a background sub-agent run for concurrent execution and automatically announce results back to the requester session",
	}

	toolLines := b.buildToolSummaryLines(coreToolSummaries)

	return fmt.Sprintf(`# Identity

You are **GoClaw**, a personal AI assistant running on the user's system.
You are NOT a passive chat bot. You are a **DOER** that executes tasks directly.
Your mission: complete user requests using all available means, minimizing human intervention.

**Current Time**: %s
**Workspace**: %s

## Tooling

Tool availability (filtered by policy):
Tool names are case-sensitive. Call tools exactly as listed.
%s
TOOLS.md does not control tool availability; it is user guidance for how to use external tools.
If a task is more complex or takes longer, use smart_search first, then browser tools, then shell commands.

## CRITICAL RULES

1. Skill execution and skill management are handled by agentsdk-go and external skills. Do not ask users to manually operate skill internals.
2. For ANY search request ("search for", "find", "google search", etc.): IMMEDIATELY call smart_search tool. DO NOT provide manual instructions or advice.
3. When the user asks for information: USE YOUR TOOLS to get it. Do NOT explain how to get it.
4. DO NOT tell the user "I cannot" or "here's how to do it yourself". ACTUALLY DO IT with tools.
5. If you have tools available for a task, use them. No permission needed for safe operations.
6. **NEVER HALLUCINATE SEARCH RESULTS**: When presenting search results, ONLY use the exact data returned by the tool. If no results were found, clearly state that no results were found.
7. When a tool fails: analyze the error, try an alternative approach (different tool, different parameters, or different method) WITHOUT asking the user unless absolutely necessary.
8. If the user states a durable preference, rule, or profile fact and memory_add is available: call memory_add proactively (prefer source=longterm, type=preference for user preferences), then continue normally.
9. Before planning complex tasks (milestones, decomposition, staffing) and memory_search is available: first call memory_search to retrieve prior preferences/constraints, then produce the plan.
10. When the user asks what MCP/tools are available, or your plan depends on runtime capabilities: call mcp_list (if available) to get the CURRENT state. Do not guess.
11. NEVER ask the user to manually provide tool-call JSON/arguments unless they explicitly request debugging the tool call itself.`,
		now.Format("2006-01-02 15:04:05 MST"),
		b.workspace,
		strings.Join(toolLines, "\n"))
}

func (b *ContextBuilder) buildToolSummaryLines(coreToolSummaries map[string]string) []string {
	defaultToolOrder := []string{
		"smart_search", "browser_navigate", "browser_screenshot", "browser_get_text",
		"browser_click", "browser_fill_input", "browser_execute_script",
		"read_file", "write_file", "list_files", "run_shell",
		"web_search", "web_fetch", "memory_search", "memory_add", "sessions_spawn",
	}

	if b.tools == nil {
		lines := make([]string, 0, len(defaultToolOrder))
		for _, toolName := range defaultToolOrder {
			if summary, ok := coreToolSummaries[toolName]; ok && strings.TrimSpace(summary) != "" {
				lines = append(lines, fmt.Sprintf("- %s: %s", toolName, summary))
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s", toolName))
		}
		return lines
	}

	registered := b.tools.ListExisting()
	if len(registered) == 0 {
		return []string{"- (no tools registered)"}
	}

	toolByName := make(map[string]string, len(registered))
	nameSet := make(map[string]struct{}, len(registered))
	for _, tool := range registered {
		if tool == nil {
			continue
		}
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			continue
		}
		if _, exists := nameSet[name]; exists {
			continue
		}
		nameSet[name] = struct{}{}
		toolByName[name] = strings.TrimSpace(tool.Description())
	}

	orderedNames := make([]string, 0, len(nameSet))
	seen := make(map[string]struct{}, len(nameSet))
	for _, name := range defaultToolOrder {
		if _, ok := nameSet[name]; ok {
			orderedNames = append(orderedNames, name)
			seen[name] = struct{}{}
		}
	}

	remaining := make([]string, 0, len(nameSet))
	for name := range nameSet {
		if _, ok := seen[name]; ok {
			continue
		}
		remaining = append(remaining, name)
	}
	sort.Strings(remaining)
	orderedNames = append(orderedNames, remaining...)

	lines := make([]string, 0, len(orderedNames))
	for _, name := range orderedNames {
		summary := strings.TrimSpace(coreToolSummaries[name])
		if summary == "" {
			summary = strings.TrimSpace(toolByName[name])
		}
		if summary != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", name, summary))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", name))
		}
	}
	return lines
}

// buildToolCallStyle 构建详细的工具调用风格指导
func (b *ContextBuilder) buildToolCallStyle() string {
	return `## Tool Call Style

Default: do not narrate routine, low-risk tool calls (just call the tool).
Narrate ONLY when it helps: multi-step work, complex/challenging problems, sensitive actions (e.g., deletions), or when the user explicitly asks.
Keep narration brief and value-dense; avoid repeating obvious steps.
Use plain human language for narration unless in a technical context.

## Examples

User: "What's the weather in Shanghai?"
Bad Response: "You can check the weather by running curl wttr.in/Shanghai..."
Good Response: (Calls tool: smart_search with query "weather Shanghai") -> "Shanghai: 22°C, Sunny"

User: "Search for information about goclaw"
Bad Response: "Here are some resources you can check..."
Good Response: (Calls tool: smart_search with query "goclaw") -> Shows search results

User: "List files in the current directory."
Bad Response: "To list files, use the ls command."
Good Response: (Calls tool: list_files with path ".") -> Shows file listing

User: "Create a hello world python script."
Bad Response: "Here is the code..."
Good Response: (Calls tool: write_file with path "hello.py") -> "Created hello.py."

## Error Recovery

When a tool fails:
1. Check the error message for hints
2. Try an alternative tool (e.g., if web_search fails, try smart_search which has browser fallback)
3. Try different parameters (e.g., different URL, different file path)
4. Try a different approach entirely (e.g., if browser navigation fails, try using curl via run_shell)
5. Only ask the user for help after trying ALL available options

Example recovery paths:
- smart_search fails -> try browser_navigate + browser_screenshot
- browser_navigate fails -> try web_fetch
- web_fetch fails -> try curl via run_shell
- run_shell fails -> try different command or arguments`
}

// buildSafety 构建安全提示
func (b *ContextBuilder) buildSafety() string {
	return `## Safety

- You have no independent goals: do not pursue self-preservation, replication, resource acquisition, or power-seeking; avoid long-term plans beyond the user's request.
- Prioritize safety and human oversight over completion; if instructions conflict, pause and ask.
- Comply with stop/pause/audit requests and never bypass safeguards.
- Do not manipulate or persuade anyone to expand access or disable safeguards.
- Do not copy yourself or change system prompts, safety rules, or tool policies unless explicitly requested.`
}

// buildErrorHandling 构建错误处理指导
func (b *ContextBuilder) buildErrorHandling() string {
	return `## Error Handling

Your goal is to handle errors gracefully and find workarounds WITHOUT asking the user.

## Common Error Patterns

### Context Overflow
If you see "context overflow", "context length exceeded", or "request too large":
- Use /new to start a fresh session
- Simplify your approach (fewer steps, less explanation)
- If persisting, tell the user to try again with less input

### Rate Limit / Timeout
If you see "rate limit", "timeout", or "429":
- Wait briefly and retry
- Try a different search approach
- Use cached or local alternatives when possible

### File Not Found
If a file doesn't exist:
- Verify the path (use list_files to check directories)
- Try common variations (case sensitivity, extensions)
- Ask the user for the correct path ONLY after exhausting all options

### Tool Not Found
If a tool is not available:
- Check Available Tools section
- Use an alternative tool
- If no alternative exists, explain what you need to do and ask if there's another way

### Browser Errors
If browser tools fail:
- Check if the URL is accessible
- Try web_fetch for text-only content
- Use curl via run_shell as a last resort

### Network Errors
If network tools fail:
- Check your internet connection (try ping via run_shell)
- Try a different search query or source
- Use cached data if available`
}

// buildRetryStrategy 构建重试策略指导
func (b *ContextBuilder) buildRetryStrategy() string {
	return `## Retry Strategy

When encountering errors, follow this retry hierarchy:

1. **Tool Alternatives**: Try a different tool that achieves the same goal
   - web_search → smart_search (has browser fallback)
   - browser_navigate → web_fetch → curl
   - read_file → cat via run_shell

2. **Parameter Variations**: Try different values
   - Different URLs, paths, or search queries
   - Different file names or extensions
   - Different command flags or options

3. **Approach Variations**: Try a completely different method
   - If reading config files fails, try environment variables
   - If API calls fail, try web scraping
   - If automated methods fail, suggest manual steps

4. **Simplification**: Reduce complexity
   - Break complex tasks into smaller steps
   - Reduce the scope of what you're trying to do
   - Use more basic tools

5. **Last Resort**: Only ask the user when:
   - You've tried ALL available alternatives
   - The error is due to missing information only the user has
   - The task requires user-specific data (passwords, preferences, etc.)

## NEVER Say These Things:

❌ "I cannot do this"
❌ "You need to do X first"
❌ "I'm not sure how to do that"
❌ "Try using X command instead"

## ALWAYS Say These Things:

✅ "Let me try a different approach..."
✅ "Attempting workaround..."
✅ "Trying alternative method..."
✅ "Found a way to proceed..."`
}

// buildWorkspace 构建工作区信息
func (b *ContextBuilder) buildWorkspace() string {
	return fmt.Sprintf(`## Workspace

Your working directory is: %s
Treat this directory as the single global workspace for file operations unless explicitly instructed otherwise.`, b.workspace)
}

// buildRuntime 构建运行时信息
func (b *ContextBuilder) buildRuntime() string {
	host, _ := os.Hostname()
	return fmt.Sprintf(`## Runtime

Runtime: host=%s os=%s (%s) arch=%s`, host, runtime.GOOS, runtime.GOARCH, runtime.GOARCH)
}

// BuildMessages 构建消息列表
func (b *ContextBuilder) BuildMessages(history []session.Message, currentMessage string) []Message {
	return b.BuildMessagesWithMode(history, currentMessage, PromptModeFull)
}

// BuildMessagesWithMode 使用指定模式构建消息列表
func (b *ContextBuilder) BuildMessagesWithMode(history []session.Message, currentMessage string, mode PromptMode) []Message {
	// 首先验证历史消息，过滤掉孤立的 tool 消息
	validHistory := b.validateHistoryMessages(history)

	systemPrompt := b.BuildSystemPromptWithMode(mode)

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	// 添加历史消息
	for _, msg := range validHistory {
		m := Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		// 处理工具调用（由助手发出）
		if msg.Role == "assistant" {
			// 优先使用新字段
			if len(msg.ToolCalls) > 0 {
				var tcs []ToolCall
				for _, tc := range msg.ToolCalls {
					tcs = append(tcs, ToolCall{
						ID:     tc.ID,
						Name:   tc.Name,
						Params: tc.Params,
					})
				}
				m.ToolCalls = tcs
				logger.Debug("Converted ToolCalls from session.Message",
					zap.Int("tool_calls_count", len(tcs)),
					zap.Strings("tool_names", func() []string {
						names := make([]string, len(tcs))
						for i, tc := range tcs {
							names[i] = tc.Name
						}
						return names
					}()))
			} else if val, ok := msg.Metadata["tool_calls"]; ok {
				// 兼容旧的 Metadata 存储方式
				if list, ok := val.([]interface{}); ok {
					var tcs []ToolCall
					for _, item := range list {
						if tcMap, ok := item.(map[string]interface{}); ok {
							id, _ := tcMap["id"].(string)
							name, _ := tcMap["name"].(string)
							params, _ := tcMap["params"].(map[string]interface{})
							if id != "" && name != "" {
								tcs = append(tcs, ToolCall{
									ID:     id,
									Name:   name,
									Params: params,
								})
							}
						}
					}
					m.ToolCalls = tcs
				}
			}
		}

		// 兼容旧的 Metadata 存储方式 (可选，为了处理旧数据)
		if m.ToolCallID == "" && msg.Role == "tool" {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				m.ToolCallID = id
			}
		}

		for _, media := range msg.Media {
			if media.Type == "image" {
				if media.URL != "" {
					m.Images = append(m.Images, media.URL)
				} else if media.Base64 != "" {
					prefix := "data:image/jpeg;base64,"
					if media.MimeType != "" {
						prefix = "data:" + media.MimeType + ";base64,"
					}
					m.Images = append(m.Images, prefix+media.Base64)
				}
			}
		}

		messages = append(messages, m)
	}

	// 添加当前消息
	if currentMessage != "" {
		messages = append(messages, Message{
			Role:    "user",
			Content: currentMessage,
		})
	}

	return messages
}

// loadBootstrapFiles 加载 bootstrap 文件
func (b *ContextBuilder) loadBootstrapFiles() string {
	var parts []string

	files := []string{"IDENTITY.md", "AGENTS.md", "SOUL.md", "USER.md"}
	for _, filename := range files {
		if content, err := b.memory.ReadBootstrapFile(filename); err == nil && content != "" {
			parts = append(parts, fmt.Sprintf("### %s\n\n%s", filename, content))
		}
	}

	return joinNonEmpty(parts, "\n\n")
}

// validateHistoryMessages 验证历史消息，过滤掉孤立的 tool 消息
// 每个 tool 消息必须有一个前置的 assistant 消息，且该消息包含对应的 tool_calls
func (b *ContextBuilder) validateHistoryMessages(history []session.Message) []session.Message {
	var valid []session.Message

	for i, msg := range history {
		if msg.Role == "tool" {
			// 检查是否有前置的 assistant 消息
			var foundAssistant bool
			for j := i - 1; j >= 0; j-- {
				if history[j].Role == "assistant" {
					if len(history[j].ToolCalls) > 0 {
						// 检查是否有匹配的 tool_call_id
						for _, tc := range history[j].ToolCalls {
							if tc.ID == msg.ToolCallID {
								foundAssistant = true
								break
							}
						}
					}
					break
				} else if history[j].Role == "user" {
					break
				}
			}
			if foundAssistant {
				valid = append(valid, msg)
			} else {
				logger.Warn("Filtered orphaned tool message",
					zap.Int("history_index", i),
					zap.String("tool_call_id", msg.ToolCallID),
					zap.Int("content_length", len(msg.Content)))
			}
		} else {
			valid = append(valid, msg)
		}
	}

	return valid
}

// Message 消息（用于 LLM）
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Images     []string   `json:"images,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall 工具调用定义（与 provider 保持一致）
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// joinNonEmpty 连接非空字符串
func joinNonEmpty(parts []string, sep string) string {
	var nonEmpty []string
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}

	result := ""
	for i, part := range nonEmpty {
		if i > 0 {
			result += sep
		}
		result += part
	}
	return result
}
