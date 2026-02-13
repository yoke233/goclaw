# Subagent 三层加载（goclawdir / roledir / repodir）与 `.agents/` 约定

## 背景

我们希望 goclaw 的 subagent（`sessions_spawn`）既能：

- 共享一套“公共能力”（全局/默认的 MCP 与 skills）
- 又能按角色（frontend/backend 等）加载角色包（role pack）
- 同时在具体项目目录（repo root）上执行任务，并允许项目目录覆盖角色包/公共能力

并且：

- **当 roledir 存在时，必须隔离主 agent 的 goclawdir**，避免主 agent 的能力/配置干扰角色执行。
- **repodir 永远是最高优先级 overlay**（项目配置 > 角色配置/公共配置）。

为此定义 subagent 的三层路径输入：

- `goclawdir`：主 agent 的加载目录（公共层，fallback）。
- `roledir`：角色包目录（角色层，存在则使用，并且完全不加载 goclawdir）。
- `repodir`：项目目录（repo root，overlay 层，永远叠加在 base 上）。

## 目录约定：统一使用 `.agents/`

在任意一个 layer root 目录下，如果需要提供能力与配置，约定使用：

- MCP/运行时配置：`<root>/.agents/config.toml`
- Skills：`<root>/.agents/skills/<skill_name>/SKILL.md`
- 禁用 skill：`<root>/.agents/skills/<skill_name>/.disabled`

说明：

- `.agents/` 是一个“面向 agent 的配置与能力目录”，可放在 `goclawdir`、`roledir`、或 `repodir`。
- goclaw 自身的运行数据（sessions/logs 等）仍可保留在 `~/.goclaw/`，二者不冲突。

## Layer 合并规则（核心）

### 1) baseRoot 选择

```text
baseRoot = roledir（有效） ? roledir : goclawdir
```

其中 “roledir 有效” 的判定建议为：

- `roledir/.agents/config.toml` 存在，或
- `roledir/.agents/skills/` 目录存在

一旦 roledir 有效：

- **只加载 roledir**
- **完全不加载 goclawdir**

### 2) overlayRoot（项目层）

```text
overlayRoot = repodir（若非空）
```

overlayRoot 永远叠加在 baseRoot 之上，且覆盖 baseRoot。

特例：

- 如果 `repodir == baseRoot`（即项目目录与角色包目录合并），则等价于只有一层。

### 3) Skills 合并语义（repodir 覆盖 baseRoot）

- Skills 搜索路径：
  - `baseRoot/.agents/skills/`
  - `repodir/.agents/skills/`（若存在）

- 合并规则：
  - skill key = `<skill_name>`（目录名）
  - 同名 skill：repodir 覆盖 baseRoot
  - `.disabled` 存在则视为禁用（覆盖规则同上，repodir 的禁用优先）

### 4) MCP 合并语义（repodir 覆盖 baseRoot）

- MCP 配置路径：
  - `baseRoot/.agents/config.toml`
  - `repodir/.agents/config.toml`（若存在）

- 合并规则：
  - server key = `<name>`（`[mcp_servers.<name>]`）
  - 同名 server：repodir 覆盖 baseRoot
  - `enabled=false` 表示该 server 在最终配置中禁用

## `sessions_spawn`（subagent）行为约定

### 1) repo_dir 参数（关键）

`sessions_spawn` 允许指定 subagent 的项目目录：

- `repo_dir`（可选）：repodir。允许传入任意已存在的目录（推荐绝对路径）。
- 若未提供 `repo_dir`：默认使用 goclaw 为该次 run 创建的隔离工作目录（安全默认）。

### 2) roledir 的来源

roledir 由 host 根据 role 决定（例如通过 task 前缀 `[frontend]` / `[backend]` 解析），并从配置的 role pack 基目录拼接得到。

### 3) goclawdir 的来源

goclawdir 通常为当前 Agent 的 workspace root（由 `workspace.path` 或运行时默认值决定）。

## `.agents/config.toml`（MCP）示例

```toml
[mcp_servers.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]
enabled = true
startup_timeout_sec = 20
tool_timeout_sec = 45

[mcp_servers.context7.env]
MY_ENV_VAR = "MY_ENV_VALUE"

[mcp_servers.figma]
url = "https://mcp.figma.com/mcp"
bearer_token_env_var = "FIGMA_OAUTH_TOKEN"
http_headers = { "X-Figma-Region" = "us-east-1" }
enabled = true

[mcp_servers.chrome_devtools]
url = "http://localhost:3000/mcp"
enabled_tools = ["open", "screenshot"]
disabled_tools = ["screenshot"]
startup_timeout_sec = 20
tool_timeout_sec = 45
enabled = true
```

备注：

- `command/args` 代表 stdio 类型；`url` 代表 http/sse 类型（具体类型可由 host 推断，或额外字段显式指定）。
- `bearer_token_env_var` 表示从环境变量读取 token 并生成 `Authorization: Bearer ...` 头（避免 token 落盘）。

## 迁移说明（从旧结构到新结构）

旧结构（历史实现）：

- workspace MCP：`<workspace>/.goclaw/mcp.json`
- workspace skills（按 role）：`<workspace>/<skills_role_dir>/<role>/<skill>/SKILL.md`
- subagent 私有覆盖：`<workdir>/.goclaw/mcp.json`、`<workdir>/.goclaw/skills/`

新结构（本需求）：

- MCP：`<layer_root>/.agents/config.toml`
- Skills：`<layer_root>/.agents/skills/<skill>/SKILL.md`
- subagent：通过三层 `goclawdir / roledir / repodir` 自动加载并合并

