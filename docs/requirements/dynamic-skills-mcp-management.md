# Skills + MCP 动态管理（自我进化能力）需求与实现清单

## 背景

我们希望主 agent 不靠“编译期插件”，而是通过**可编辑的 skills** 与**可动态配置的 MCP 服务器**来扩展能力。

目标是让系统在运行中做到：

- skills 可以由 agent/用户在对话中创建、修改、启用/禁用，并可刷新生效。
- MCP 服务器可以由 agent/用户在对话中增删改查、开关，并可刷新生效。
- 主 agent 能在对话中查询当前 skills 与 MCP 配置状态（列表/详情）。

这套机制在效果上等价于“动态扩展/动态插件”，但实现上更轻量、可审计、可版本化（文件即真相）。

## 术语与约定

- **workspace**：goclaw 配置中 `workspace.path` 指向的目录（通常是主 agent 的工作目录）。
- **.agents**：面向 agent 的能力与配置目录（文件即真相），统一使用：
  - skills：`<root>/.agents/skills/<skill_name>/SKILL.md`
  - config：`<root>/.agents/config.toml`
- **skill 目录结构**：`<root>/.agents/skills/<skill_name>/SKILL.md`（允许 `skill.md` 兼容，但写入统一用 `SKILL.md`）。
- **禁用 skill**：在 skill 目录下存在 `.disabled` 哨兵文件则视为禁用。
- **MCP 配置文件**：`<root>/.agents/config.toml`（由 goclaw 管理，供运行时注入 agentsdk-go 的 settings overrides）。
- **subagent 三层输入**：`goclawdir / roledir / repodir`，用于实现“角色隔离 + 项目覆盖”。详见：`docs/requirements/subagent-layering-and-agents-dir.md`。

## 设计原则

- **配置与内容即真相**：skills 与 MCP 配置以文件形式保存，可被 Git 管理。
- **最小侵入**：不引入 Go plugin，不引入编译期扩展机制。
- **可回滚**：文件修改可通过 Git 或备份快速回退。
- **安全边界清晰**：写入/删除仅允许发生在 workspace 指定目录中（防止路径穿越）。
- **刷新可控**：变更不要求每次自动热加载；提供显式 `reload`（以及写操作后自动触发 reload）。

## 数据模型

### 1) Skills（.agents 目录）

目录：

- `<root>/.agents/skills/<skill>/SKILL.md`

启用/禁用：

- 启用：不存在 `<skill_dir>/.disabled`
- 禁用：存在 `<skill_dir>/.disabled`（文件内容可为空）

### 2) MCP（.agents/config.toml）

文件：`<root>/.agents/config.toml`

建议 schema（v1，节选 MCP 部分）：

```toml
[mcp_servers.time]
enabled = true
command = "uvx"
args = ["mcp-server-time"]
startup_timeout_sec = 10

[mcp_servers.time.env]
TZ = "UTC"
```

运行时注入策略：

- goclaw 读取 `config.toml`，仅将 `enabled=true` 的 servers 转换为 agentsdk-go `SettingsOverrides.MCP.Servers` 并在 runtime 初始化时注入。
- 变更后通过 `reload` 触发 runtime 重建，以便 MCP 工具重新注册。

### 3) Subagent MCP（base + overlay）

默认行为：

- subagent runtime 使用三层合并：`baseRoot + repodir overlay`，并遵循：repodir 覆盖 baseRoot。
- baseRoot 选择：`roledir 有效 ? roledir : goclawdir`（roledir 有效时不加载 goclawdir）。

可选覆盖：

- `sessions_spawn` 支持可选参数 `repo_dir` 指定 repodir（项目目录）。
- （可选）保留显式配置文件路径覆盖（例如 `mcp_config_path`），用于一次性指定配置源。

限制：

- subagent 的 MCP 配置仅在该 subagent runtime 初始化时加载；运行中不做热加载（需要重新 spawn 才能生效）。

### 4) Subagent Skills（base + overlay）

默认行为：

- subagent skills 采用三层合并：`baseRoot/.agents/skills + repodir/.agents/skills`（repodir 覆盖 baseRoot）。

## 对话工具（Tooling）

> 这些工具是给主 agent “自我管理”的管理面能力，不是最终业务能力。

### Skills 管理工具

- `skills_list`
  - 入参：`role`（可选，默认 `main`），`include_disabled`（可选）
  - 出参：skills 列表（name、enabled、path、has_skill_md 等）

- `skills_get`
  - 入参：`role`（可选，默认 `main`），`skill_name`，`include_content`（可选，默认 true）
  - 出参：skill 元信息 + SKILL.md 内容

- `skills_put`
  - 入参：`role`（可选，默认 `main`），`skill_name`，`skill_md`（SKILL.md 全文），`enabled`（可选），`overwrite`（可选）
  - 行为：写入/更新 `<workspace>/<skills_role_dir>/<role>/.agents/skills/<skill_name>/SKILL.md`，并按 enabled 写 `.disabled`；成功后请求 runtime reload

- `skills_delete`
  - 入参：`role`（可选，默认 `main`），`skill_name`
  - 行为：删除 `<workspace>/<skills_role_dir>/<role>/.agents/skills/<skill_name>`；成功后请求 runtime reload

- `skills_set_enabled`
  - 入参：`role`（可选，默认 `main`），`skill_name`，`enabled`
  - 行为：增删 `.disabled`；成功后请求 runtime reload

### MCP 管理工具

- `mcp_list`
  - 出参：servers 列表（name、enabled、type、command/url、timeoutSeconds）

- `mcp_put_server`
  - 入参：`name`，`enabled`，`type`（可选），`command`/`args`/`url`，`env`，`headers`，`timeoutSeconds` 等
  - 行为：写入/更新 `<root>/.agents/config.toml`；成功后请求 runtime reload

- `mcp_delete_server`
  - 入参：`name`
  - 行为：删除 server；成功后请求 runtime reload

- `mcp_set_enabled`
  - 入参：`name`，`enabled`
  - 行为：切换 enabled；成功后请求 runtime reload

- `runtime_reload`
  - 入参：可选 `agent_id`（默认从 ctx 取当前 agent）
  - 行为：标记主 runtime 在当前回合结束后重建（不打断当前回合）

## 运行时刷新策略（关键）

约束：管理工具会在**当前回合运行中**被调用，不能在 tool 执行时直接关闭正在运行的 runtime（会引发竞态/中断）。

解决方案：

- 主 runtime 增加 `Invalidate(agentID)`：
  - 若当前 runtime 正在被使用：仅标记为 `invalidated=true`，等回合结束后由 `Run()` 的 defer 释放逻辑安全关闭。
  - 若未被使用：可立即关闭并删除缓存。
- skills/MCP 写操作默认触发 `Invalidate(currentAgentID)`，保证下一回合以新配置启动并重建工具注册（含 MCP）。

## 验收标准

- 能在对话中 `skills_list` / `mcp_list` 查询状态。
- 能在对话中创建/修改 skills（写入 `SKILL.md`）并在下一回合生效。
- 能在对话中新增 MCP server，并在下一回合看到对应 MCP 工具可被调用（至少 tool registry 注册成功）。
- 启用/禁用 skills 与 MCP server 均可工作。
- `go test ./...` 通过（新增单测覆盖基础读写与路径校验）。

## 非目标（本阶段不做）

- 自动文件 watcher 的“实时热加载”（后续可加）。
- 复杂的权限审批 UI（沿用现有 shell/filesystem policy）。
- 插件二进制装载（Go plugin / wasm / node plugin 系统）。
