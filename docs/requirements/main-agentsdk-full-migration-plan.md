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
- [x] `AgentManager` 使用 AgentSDKEngine 执行（保留旧链路后备分支，待删除）
- [x] 保持 bus/channel/gateway 流程不变（当前回归通过）

## Phase 3：agent/tui 入口统一

- [x] `cli/agent.go` 改为复用 AgentSDKEngine
- [x] `cli/commands/tui.go` 删除本地循环，改为 AgentSDKEngine
- [ ] 统一 session_key 规则

## Phase 4：任务系统迁移

- [ ] agentsdk-go 增加可注入、可持久化 TaskStore（SQLite）
- [ ] goclaw 接入持久化 task store
- [ ] 现有 `task` 命令切换到新 task store
- [ ] `sessions_spawn` 与 task 状态映射对齐

## Phase 5：旧链路删除与清理

- [ ] 删除 `agent/orchestrator.go`
- [ ] 删除 `cli/commands/tui.go` 中旧 `runAgentIteration` 路径
- [ ] 清理主链路 `providers.Provider` 依赖
- [ ] 清理旧 runtime 配置字段和文档

## Phase 6：验收

- [ ] `go test ./...` 全量通过
- [ ] 新增并执行主链路 e2e 脚本
- [ ] 手工验证：主会话 + subagent + task 完整闭环

## 风险提示

1. 一次切换风险高，必须靠自动化回归兜底。
2. task 持久化跨仓库改造（goclaw + agentsdk-go）需要分阶段合并。
3. `start` / `agent` / `tui` 当前执行路径不一致，重构时需统一抽象后再替换。
