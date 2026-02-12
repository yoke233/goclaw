# agentsdk-go 集成实现说明（执行级细化）

本文档是对 `docs/requirements/agentsdk-integration.md` 的执行级细化，面向实现工作树的工程师。内容以“能直接写代码”为目标，列出需要新增的文件、函数签名、数据流与改动点。

## 1. 新增模块与接口

### 1.1 新建运行时适配层

新建目录 `agent/runtime/`，新增文件：

1. `agent/runtime/runtime.go`
2. `agent/runtime/agentsdk_runtime.go`
3. `agent/runtime/role_pool.go`
4. `agent/runtime/context_keys.go`

#### `agent/runtime/runtime.go`

```go
package runtime

import "context"

type SubagentRuntime interface {
    Spawn(ctx context.Context, req SubagentRunRequest) (string, error)
    Wait(ctx context.Context, runID string) (*SubagentRunResult, error)
    Cancel(ctx context.Context, runID string) error
}

type SubagentRunRequest struct {
    RunID          string
    Task           string
    Role           string
    WorkDir        string
    SkillsDir      string
    SystemPrompt   string
    TimeoutSeconds int
}

type SubagentRunResult struct {
    Status   string // ok|error|timeout
    Output   string
    ErrorMsg string
}
```

#### `agent/runtime/context_keys.go`

```go
package runtime

type ctxKey string

const (
    CtxSessionKey ctxKey = "goclaw.session_key"
    CtxAgentID    ctxKey = "goclaw.agent_id"
    CtxChannel    ctxKey = "goclaw.channel"
    CtxAccountID  ctxKey = "goclaw.account_id"
    CtxChatID     ctxKey = "goclaw.chat_id"
)
```

#### `agent/runtime/role_pool.go`

实现角色级并发池（前端 5、后端 4），允许扩展。

```go
package runtime

type RolePool interface {
    Acquire(role string)
    Release(role string)
}

// SimpleRolePool: role -> semaphore channel
```

#### `agent/runtime/agentsdk_runtime.go`

> 需要对照 `agentsdk-go` 实际 API 修改。以下是结构框架。

```go
package runtime

type AgentsdkRuntime struct {
    pool RolePool
    // sdk runtime, hooks, logger, config...
}

func NewAgentsdkRuntime(pool RolePool /* + sdk options */) *AgentsdkRuntime {}

func (r *AgentsdkRuntime) Spawn(ctx context.Context, req SubagentRunRequest) (string, error) {
    r.pool.Acquire(req.Role)
    // 启动 goroutine 执行 task
    return req.RunID, nil
}

func (r *AgentsdkRuntime) Wait(ctx context.Context, runID string) (*SubagentRunResult, error) {}
func (r *AgentsdkRuntime) Cancel(ctx context.Context, runID string) error {}
```

## 2. 角色与路径解析

### 2.1 角色解析规则

建议优先用 `label` 前缀，其次用 `task` 前缀：

1. `label` 以 `[frontend]` / `[backend]` 开头
2. `task` 以 `[frontend]` / `[backend]` 开头
3. 默认 `backend`

示例实现（放在 `agent/runtime/role.go` 或 `agent/manager.go` 私有函数）：

```go
func parseRole(task, label string) string {
    t := strings.ToLower(strings.TrimSpace(label + " " + task))
    switch {
    case strings.HasPrefix(t, "[frontend]"):
        return "frontend"
    case strings.HasPrefix(t, "[backend]"):
        return "backend"
    default:
        return "backend"
    }
}
```

### 2.2 目录解析规则

使用 workspace 基路径：

- `workdir := filepath.Join(workspace, "subagents", runID, "workspace")`
- `skills := filepath.Join(workspace, "skills", role)`

若目录不存在则创建。

## 3. 现有代码改动点（细化）

### 3.1 `agent/manager.go`

#### 新增字段

```go
subagentRuntime runtime.SubagentRuntime
workspace string
```

#### 构造时传入

在 `NewAgentManagerConfig` 增加：

```go
Workspace string
SubagentRuntime runtime.SubagentRuntime
```

#### `handleSubagentSpawn` 具体流程

1. 从 `SubagentSpawnResult` 获取 `RunID`/`ChildSessionKey`。
2. 从 registry 中拿到 `SubagentRunRecord`（补齐 task/label）。
3. 解析 `role`、`workdir`、`skillsDir`。
4. 构建 `SystemPrompt`（复用 `BuildSubagentSystemPrompt`）。
5. 调用 `subagentRuntime.Spawn` 执行。
6. 等待执行完成后，调用 `subagentRegistry.MarkCompleted`。

伪代码：

```go
func (m *AgentManager) handleSubagentSpawn(result *tools.SubagentSpawnResult) error {
    record, ok := m.subagentRegistry.GetRun(result.RunID)
    if !ok { return fmt.Errorf("run not found") }

    role := parseRole(record.Task, record.Label)
    workdir := filepath.Join(m.workspace, "subagents", record.RunID, "workspace")
    skillsDir := filepath.Join(m.workspace, "skills", role)

    prompt := BuildSubagentSystemPrompt(&SubagentSystemPromptParams{
        RequesterSessionKey: record.RequesterSessionKey,
        RequesterOrigin: record.RequesterOrigin,
        ChildSessionKey: record.ChildSessionKey,
        Label: record.Label,
        Task: record.Task,
    })

    runReq := runtime.SubagentRunRequest{
        RunID: result.RunID,
        Task: record.Task,
        Role: role,
        WorkDir: workdir,
        SkillsDir: skillsDir,
        SystemPrompt: prompt,
        TimeoutSeconds: /* from config */,
    }

    if _, err := m.subagentRuntime.Spawn(context.Background(), runReq); err != nil { ... }

    // Wait in background
    go func() {
        res, err := m.subagentRuntime.Wait(context.Background(), result.RunID)
        endedAt := time.Now().UnixMilli()
        outcome := &SubagentRunOutcome{Status:"ok"}
        if err != nil { outcome.Status="error"; outcome.Error=err.Error() }
        _ = m.subagentRegistry.MarkCompleted(result.RunID, outcome, &endedAt)
    }()

    return nil
}
```

#### `sendToSession` 具体实现

推荐方式：构造 `InboundMessage` 走现有 `handleInboundMessage`，避免直接改 orchestrator 并发。

```go
func (m *AgentManager) sendToSession(sessionKey, message string) error {
    // 优先从 registry 的 RequesterOrigin 获取 channel/account/chatID
    // 若为空，解析 sessionKey（格式 channel:account:chat）
    msg := &bus.InboundMessage{
        Channel: channel,
        AccountID: accountID,
        ChatID: chatID,
        Content: message,
        Timestamp: time.Now(),
    }
    return m.RouteInbound(context.Background(), msg)
}
```

### 3.2 `agent/tools/subagent_spawn_tool.go`

新增对 `context.Context` 的读取，获取请求者 session/channel/account/chatID：

```go
if v := ctx.Value(runtime.CtxSessionKey); v != nil { requesterSessionKey = v.(string) }
```

将 `childSystemPrompt` 放入 `SubagentRunParams` 或 `SubagentSpawnResult` 以便在 `handleSubagentSpawn` 使用。

### 3.3 `AgentManager.handleInboundMessage`

在调用 orchestrator 前，包装 `context.WithValue`：

```go
ctx = context.WithValue(ctx, runtime.CtxSessionKey, sessionKey)
ctx = context.WithValue(ctx, runtime.CtxChannel, msg.Channel)
ctx = context.WithValue(ctx, runtime.CtxAccountID, msg.AccountID)
ctx = context.WithValue(ctx, runtime.CtxChatID, msg.ChatID)
```

### 3.4 `cli/root.go`

初始化 `AgentsdkRuntime`，并把它传给 `AgentManager`：

```go
subagentRuntime := runtime.NewAgentsdkRuntime(rolePool /* + sdk cfg */)
agentManager := agent.NewAgentManager(&agent.NewAgentManagerConfig{
    ...
    SubagentRuntime: subagentRuntime,
    Workspace: workspaceDir,
})
```

### 3.5 `config/schema.go`

为 subagent 增加 runtime 与路径配置：

```go
type SubagentsConfig struct {
    Runtime string `mapstructure:"runtime" json:"runtime"` // goclaw|agentsdk
    SkillsRoleDir string `mapstructure:"skills_role_dir" json:"skills_role_dir"`
    WorkdirBase string `mapstructure:"workdir_base" json:"workdir_base"`
}
```

## 4. agentsdk-go API 对接点（需要实现时确认）

实现时需确认以下概念是否存在：

1. Runtime/Runner 实例化方式
2. Skills manager 初始化方式与根目录参数
3. Hook 注册方式（Run/Tool/Error 事件）
4. 同步/异步执行接口

若 API 与设想不同，优先通过适配层 `AgentsdkRuntime` 解决，不改上层调用。

### 4.1 已核对的 API 摘要（本地 `D:\project\agentsdk-go`）

> 以下内容基于本地仓库 `D:\project\agentsdk-go` 的实际代码。实现时以该仓库版本为准。

#### 核心运行时 `pkg/api`

```go
rt, err := api.New(ctx, api.Options{
    ProjectRoot: workdir,
    ModelFactory: modelFactory,
    Skills:     skillRegs,
    Commands:   cmdRegs,
    Subagents:  subagentRegs,
    TypedHooks: hookRegs,
})
defer rt.Close()

resp, err := rt.Run(ctx, api.Request{
    Prompt:    "...",
    SessionID: "session-1",
})

stream, err := rt.RunStream(ctx, api.Request{
    Prompt:    "...",
    SessionID: "session-2",
})
```

**并发约束**：同一个 `SessionID` 不能并发执行，`Run/RunStream` 会返回 `ErrConcurrentExecution`。  
**建议**：subagent 的 `SessionID` 直接使用 `child_session_key` 或 `run_id`，避免冲突。

`api.Options` 关键字段（节选，真实字段来自 `pkg/api/options.go`）：
- `ProjectRoot`
- `Model` / `ModelFactory`
- `ModelPool` / `SubagentModelMapping`
- `SystemPrompt`
- `MaxIterations` / `Timeout` / `TokenLimit`
- `Tools` / `EnabledBuiltinTools` / `CustomTools`
- `DisallowedTools`
- `Skills` / `Commands` / `Subagents`
- `TypedHooks` / `HookMiddleware` / `HookTimeout`
- `SettingsPath` / `SettingsOverrides` / `SettingsLoader`
- `Sandbox`（root/allowed paths/allowed domains）
- `MCPServers`

`api.Request` 关键字段（节选）：
- `Prompt`
- `ContentBlocks`
- `SessionID`
- `RequestID`
- `Model`（ModelTier）
- `EnablePromptCache`
- `Traits` / `Tags` / `Channels` / `Metadata`
- `TargetSubagent`
- `ToolWhitelist`
- `ForceSkills`

`api.Response` 关键字段（节选）：
- `Result.Output` / `Result.StopReason`
- `Subagent`（subagents.Result）
- `SkillResults` / `CommandResults`
- `HookEvents`

#### Skills / Subagents / Hooks 的加载方式

SDK 内置 `.claude` 解析器在 `pkg/prompts`：

```go
fsys := os.DirFS(workspace)
builtins := prompts.ParseWithOptions(fsys, prompts.ParseOptions{
    SkillsDir:    filepath.Join("skills", role),
    CommandsDir:  "__none__", // 不存在也没关系
    SubagentsDir: "__none__",
    HooksDir:     "__none__",
})
```

`ParseWithOptions` 会忽略不存在目录（返回 `nil` 错误），适合按角色加载技能目录。

#### ModelFactory（Anthropic）

SDK 提供 `model.AnthropicProvider`（实现 `ModelFactory` 接口）：

```go
provider := &model.AnthropicProvider{
    APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
    ModelName: "claude-sonnet-4-5",
    MaxTokens: 4096,
}

rt, _ := api.New(ctx, api.Options{
    ProjectRoot: workdir,
    ModelFactory: provider,
})
```

## 5. Registry 扩展建议

当前 `SubagentRunRecord` 没有输出字段。建议新增字段：

```go
ResultText string `json:"result_text,omitempty"`
Artifacts  map[string]string `json:"artifacts,omitempty"`
```

用于存放 agentsdk-go 最终输出摘要与文件路径。

## 6. Hook 与进度汇报

1. Hook 收到执行阶段事件时，写入 registry（started/ended）
2. 关键阶段向 bus 发出进度消息（可配置开关）
3. 任务完成后触发 `subagentAnnouncer` 生成摘要

## 7. 验收用例

1. 输入：
   - `[frontend] 实现登录页`
   - `[backend] 增加用户查询接口`
2. 预期：
   - 生成两个 run_id
   - 分别在 `workspace/subagents/<run_id>/workspace` 下执行
   - skills 从 `workspace/skills/frontend` / `workspace/skills/backend` 加载
   - 主会话收到完成摘要
   - registry 状态为 `completed`
