# 主链路全量迁移到 agentsdk-go 计划

## 目标

将 goclaw 主执行链路从自研 orchestrator 全量迁移到 agentsdk-go，并统一 `start` / `agent` / `tui` 三个入口，最终移除旧执行链路代码。

## 已锁定决策

1. 切换策略：一次切换（不保留旧运行时回退开关）。
2. 入口范围：`start` / `agent` / `tui` 全部统一。
3. 兼容策略：重构优先（允许行为语义调整）。
4. 模型层：主链路全部走 agentsdk model provider。
5. 旧链路处理：本次直接删除旧 orchestrator 链路代码。
6. 会话迁移：不迁移旧会话，冷启动。
7. 任务系统：切换到 agentsdk task 体系，并补齐 SQLite 持久化。

## 实施阶段

## Phase 0：基线与追踪

- [x] 建立迁移计划文档
- [x] 建立执行追踪清单并持续更新
- [x] 记录迁移前测试基线：`go test ./...`

## Phase 1：主执行引擎

- [x] 新增统一执行引擎（AgentSDKEngine）
- [x] 封装 goclaw tool -> agentsdk tool 适配
- [x] 封装 model 选择（anthropic/openai/openrouter）
- [x] 维护 agent 维度 runtime 复用与关闭

## Phase 2：start 主入口切换

- [x] `cli/root.go` 使用 AgentSDKEngine 驱动主回合
- [x] `AgentManager` 使用 AgentSDKEngine 执行
- [x] 保持 bus/channel/gateway 流程不变（当前回归通过）

## Phase 3：agent/tui 入口统一

- [x] `cli/agent.go` 改为复用 AgentSDKEngine
- [x] `cli/commands/tui.go` 删除本地循环，改为 AgentSDKEngine
- [x] 统一 session_key 规则（`agent.ResolveSessionKey`）

## Phase 4：任务系统迁移

- [x] agentsdk-go 增加可注入 TaskStore 接口（SDK 不内置具体 SQLite 实现）
- [x] goclaw 接入持久化 task store（`agent/tasksdk.SQLiteStore`）
- [x] 现有 `task` 命令切换到新 task store（命令语义对齐 agentsdk task）
- [x] `sessions_spawn` 与 task 状态映射对齐（`agent/tasksdk.Tracker`）

## Phase 5：旧链路删除与清理

- [x] 删除 `agent/orchestrator.go`
- [x] 删除 `cli/commands/tui.go` 中旧 `runAgentIteration` 路径
- [x] 清理主链路 `providers.Provider` 依赖
- [x] 清理旧 runtime 配置字段和文档（`role_max_concurrent` 已替换）

## Phase 6：验收

- [x] `go test ./...` 全量通过
- [x] 新增并执行主链路 e2e 脚本（`scripts/e2e_subagent.ps1`）
- [ ] 手工验证：主会话 + subagent + task 完整闭环
  - 说明：该项依赖真实 provider 凭证（如 `OPENAI_API_KEY`/`ANTHROPIC_API_KEY`）。
  - 当前状态（2026-02-12）：本地环境未注入上述凭证，自动化 smoke 已通过，手工闭环待在真实凭证环境执行。

## 风险提示

1. 一次切换风险高，必须靠自动化回归兜底。
2. task 持久化跨仓库改造（goclaw + agentsdk-go）需要分阶段合并。
3. 手工全链路验证依赖真实 provider 凭证，CI 无法完全覆盖。
