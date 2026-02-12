# Subagent E2E 验收说明

本文档用于验证 `sessions_spawn` 链路、`subagents.runtime` 切换以及任务管理 MVP 的可用性。

## 1. 目标

验收以下能力是否可用：

1. `sessions_spawn` 从创建到完成回灌的闭环。
2. 任务管理命令（`task create/assign/status/progress/list`）可用。
3. 子任务结果可回填任务进度（通过 `task_id` 关联）。
4. `subagents.runtime` 配置切换生效（`agentsdk|goclaw`）。

## 2. 快速执行（建议）

在仓库根目录执行：

```powershell
pwsh -NoProfile -File .\scripts\e2e_subagent.ps1
```

该脚本会完成：

1. 运行核心测试：`go test ./agent/... ./cli/... ./memory/...`
2. 使用隔离 workspace 跑任务命令 smoke 流程。
3. 输出 requirement/task ID、看板信息与进度日志。

如果需要做线上 provider 的人工验证：

```powershell
pwsh -NoProfile -File .\scripts\e2e_subagent.ps1 -WithLiveSubagent
```

## 3. 手工验证步骤

### 3.1 准备任务

```powershell
go run . task requirement --title "登录系统改造" --description "验证 subagent 回填"
go run . task create --requirement <requirement_id> --title "实现前端登录页" --role frontend
go run . task assign <task_id> --role frontend --assignee "alice"
go run . task status <task_id> --status doing --message "等待 subagent 执行"
```

### 3.2 触发 subagent

在 `goclaw` 的交互入口（TUI 或 agent 对话）中发送：

```text
请调用 sessions_spawn，task="[frontend] 生成登录页骨架并给出实现要点"，task_id="<task_id>"
```

### 3.3 验证回填

执行：

```powershell
go run . task list --requirement <requirement_id> --with-progress --progress-limit 10
```

检查点：

1. 任务状态从 `doing` 变成 `done` 或 `blocked`。
2. `Progress` 中出现 run 关联记录（包含 `run_id` 或结果摘要）。
3. 主会话收到 subagent 宣告内容。

## 4. runtime 切换验证

在配置中切换：

```json
"agents": {
  "defaults": {
    "subagents": {
      "runtime": "goclaw"
    }
  }
}
```

重启 `goclaw start` 后再次执行 3.2/3.3，确认链路仍可用。

再切回：

```json
"runtime": "agentsdk"
```

重复验证两轮，确保行为一致。

## 5. 判定标准

通过标准：

1. 自动脚本通过（退出码 0）。
2. 手工 `sessions_spawn` 能执行并回灌主会话。
3. `task list --with-progress` 能看到状态和日志更新。
4. `runtime` 两种模式至少各通过一次完整链路。

失败标准（任一项）：

1. `sessions_spawn` 失败且无明确错误日志。
2. 任务状态无法更新（命令报错或数据不变）。
3. 回填日志缺失（无 progress 记录）。
4. runtime 切换后链路中断。

