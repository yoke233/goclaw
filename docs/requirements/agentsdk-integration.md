# agentsdk-go 集成需求说明

> 状态说明：本文档为阶段 1 需求草案。主链路迁移已完成，实际落地请以 `docs/requirements/main-agentsdk-full-migration-plan.md` 为准。

## 背景
目标是引入 `agentsdk-go` 作为执行运行时，以获得完整的 agent 流程、skills 与 hooks 能力，并在不破坏现有通道/消息总线的前提下接管主/分身执行。

## 目标
1. **进程内集成** `agentsdk-go`，用于执行 subagent（分身）任务。
2. **按角色共享技能目录**：`workspace/skills/<role>`。
3. **每个 subagent 独立工作目录**：`workspace/subagents/<run_id>/workspace`。
4. **分身状态可追踪**：run 进度与结果写入 `subagent_registry`，并可回传主会话。
5. **并发控制**：按角色配置并发上限（`role_max_concurrent`，可扩展）。

## 非目标（阶段 1）
1. 不改现有通道与 bus 结构。
2. 不引入外部服务部署（仅进程内 SDK）。

## 设计决策
- 集成方式：进程内 SDK。
- Skills 布局：按角色共享目录（`workspace/skills/frontend`、`workspace/skills/backend`）。
- 模型提供者：优先用 agentsdk-go 自带 Anthropic provider 跑通流程（后续可扩展）。

## 目录与资源约定
- **分身工作目录**：`<workspace>/subagents/<run_id>/workspace`
- **分身技能目录**：`<workspace>/skills/<role>`
- **分身运行时缓存**：`<workspace>/subagents/<run_id>/.claude`（若 agentsdk-go 默认需要）
- **分身运行记录**：复用 `agent/subagent_registry.go`

## 新增/变更配置（建议）
> 这些是建议字段，需在 `config/schema.go` 中落地

```json
{
  "agents": {
    "defaults": {
      "subagents": {
        "max_concurrent": 8,
        "role_max_concurrent": {
          "frontend": 5,
          "backend": 4
        },
        "archive_after_minutes": 60,
        "timeout_seconds": 900,
        "skills_role_dir": "skills",      // 基于 workspace 的相对路径
        "workdir_base": "subagents"        // 基于 workspace 的相对路径
      }
    }
  }
}
```

## 核心执行链路（阶段 1）
1. 用户请求触发主 Agent 的 `sessions_spawn` 工具调用。
2. `SubagentSpawnTool.Execute` 生成 `run_id`、`child_session_key` 并登记到 registry。
3. `AgentManager.handleSubagentSpawn` 启动 agentsdk-go 执行：
   - 计算工作目录 `workspace/subagents/<run_id>/workspace`
   - 选择技能目录 `workspace/skills/<role>`
   - 设置系统提示词（使用现有 BuildSubagentSystemPrompt）
4. 执行完成后：
   - 写入 `subagent_registry`（状态/错误/结束时间）
   - 触发 `subagentAnnouncer` 通知主会话

## 角色与并发模型
- 支持角色：开放集合，默认可使用 `frontend`、`backend`，并可扩展 `qa`/`devops` 等。
- 并发池按 `role_max_concurrent` 限制，未配置角色走 `max_concurrent` 默认上限。
- 角色来源：
  - `sessions_spawn` 参数 `label` 前缀或 `task` 约定标记，例如：`[frontend]`、`[qa]`
  - 若无标记，默认 `backend`

## agentsdk-go 接入抽象（建议接口）
在 `agent/runtime/` 新增一层适配，避免直接侵入现有逻辑。

```go
type SubagentRuntime interface {
    Spawn(ctx context.Context, req SubagentRunRequest) (runID string, err error)
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

## Hook 与进度
利用 agentsdk-go 的 Hook 机制写入 registry，并可选向 bus 发送进度消息。

- `BeforeRun`：记录 started_at
- `AfterRun`：记录 ended_at + outcome
- `OnError`：记录 error
- `BeforeToolUse/AfterToolUse`：可用于审计或进度提示

## 关键代码改动位置（最小集合）
1. `agent/manager.go`
   - 完成 `handleSubagentSpawn`（执行 agentsdk-go）
   - 完成 `sendToSession`（通知主会话）
2. `agent/tools/subagent_spawn_tool.go`
   - 从 `context.Context` 读取请求者会话/来源
   - 把系统提示词传入 subagent 运行时
3. `cli/root.go`
   - 初始化 runtime 适配器（如果放在 AgentManager 中则只需传入配置）
4. `config/schema.go`
   - 添加 role 并发与目录配置字段
5. `agent/runtime/agentsdk_runtime.go`（新增）
   - 封装 agentsdk-go 执行

## 测试与验收
### 功能验收
1. `sessions_spawn` 触发分身执行，run 状态写入 registry。
2. 分身工作目录为 `workspace/subagents/<run_id>/workspace`。
3. 分身读取 `workspace/skills/<role>` 的技能。
4. 主会话收到分身结果回传。
5. 并发限制生效（frontend<=5, backend<=4）。

### 失败场景
1. agentsdk-go 初始化失败：记录错误并回传主会话。
2. 任务超时：状态为 `timeout`，并记录结束时间。
3. 技能目录不存在：仍能执行，但记录 warning。

## 风险与注意事项
1. agentsdk-go API 需要确认：具体构造与 Hook 接口可能调整，需要在实现时对照 SDK 实际代码。
2. Windows 环境：确保工作目录与技能路径处理正确（路径分隔符）。
3. 现有 goclaw shell 工具使用 `sh -c`，分身执行时建议避免使用该工具或单独适配。

## 后续阶段（可选）
1. 主 Agent 也迁移到 agentsdk-go Runtime。
2. 引入任务表/需求表持久化（SQLite/Postgres），与 registry 合并。
3. 支持外部 SDK（HTTP 服务）以实现隔离部署。

