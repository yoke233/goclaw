# agentsdk-go 接入计划（不迁移 session 真源）

**目标与结论**
保留现有 `session` 作为产品层真源存储，不将其迁移到 agentsdk-go。agentsdk-go 的 `history` 仅作为可观测、对比、回放辅助，避免状态漂移与兼容风险。

**范围与原则**
- 新能力必须带配置开关，默认关闭或仅对比；关闭后行为与当前一致。
- `session` 写入逻辑保持不变，agentsdk-go 仅做双写/对比，不反向回写。
- 任何能力接入都需有回滚路径与兼容性说明。

**建议接入能力与优先级**
1. 事件流（RunStream）与 Gateway 推送：提升实时性与可观测性。
2. History 双写与对比：验证 agentsdk-go 行为一致性，仅记录日志。
3. 权限审批统一：减少分叉的审批路径与风险。
4. Token 统计：补齐成本/性能指标。
5. AutoCompact：控制上下文增长。
6. Prompt Cache：降低成本，提高稳定性。
7. OTEL tracing：全链路可观测性与问题定位。

**哪些模块可以用 agentsdk-go 实现**
- `agent` 运行时：支持 `RunStream`、统计、审批、AutoCompact、Prompt Cache。
- `gateway`：流式事件推送（`agent.stream.event` / `agent.stream.end`）。
- `cli` / `tui`：实时输出 token/增量内容。
- 监控/日志：token 统计、trace 透传与异常观测。

**实施步骤（建议顺序）**
1. 配置与开关定义。
说明: 定义 history 模式、对比开关、cleanup 天数、流式开关等，默认不改变行为。
涉及文件: `config/schema.go`, `config/loader.go`。

2. 事件流 RunStream 能力。
说明: 增加 streaming 接口与收集器；gateway 推送事件；CLI/TUI 实时输出，非流式回退 `Run`。
涉及文件: `agent/stream_types.go`, `agent/stream_collect.go`, `agent/main_runtime_agentsdk.go`, `agent/manager.go`, `gateway/handler.go`, `gateway/server.go`, `gateway/notifier.go`, `cli/commands/tui.go`, `cli/root.go`。

3. History 双写与对比。
说明: 写入 `.claude/history` 并与 `session` 最近两轮对话做对比；仅写日志，不改 `session`。
涉及文件: `agent/history_compare.go`, `agent/agentsdk_settings_overrides.go`, `agent/manager.go`, `cli/agent.go`, `cli/commands/tui.go`。

4. 权限审批统一。
说明: 统一 subagent/tool 的审批到 agentsdk-go approval，`session` 继续记录审批结果。
涉及文件: `agent/subagent_approvals.go`, `agent/runtime/agentsdk_runtime.go`, `gateway/handler.go`。

5. Token 统计接入。
说明: 从 agentsdk-go `stats` 获取 token usage，写入 `session` 或指标上报。
涉及文件: `agent/main_runtime_agentsdk.go`, `agent/manager.go`, `metrics/*`（若已有）。

6. AutoCompact 与 Prompt Cache。
说明: 按配置启用；先小范围试点，再扩大。
涉及文件: `agent/agentsdk_settings_overrides.go`, `config/schema.go`, `config/loader.go`。

7. OTEL tracing。
说明: 将 trace 上下文贯穿 gateway → agent → subagent；必要时透传到外部。
涉及文件: `gateway/server.go`, `agent/runtime/agentsdk_runtime.go`, `metrics/*`。

8. 兼容性与回滚验证。
说明: 逐项验证开关关闭时与当前行为一致，记录对比日志；确保故障时可一键回退。

**配置项草案**
- `agents.defaults.history.mode`: `session_only` | `agentsdk_only` | `dual`（默认 `session_only`）。
- `agents.defaults.history.compare`: `true|false`（默认 `false`）。
- `agents.defaults.history.agentsdk_cleanup_days`: `int`（默认 `7`）。
- `agents.defaults.stream.enable`: `true|false`（默认 `false`）。
- `agents.defaults.approvals.use_agentsdk`: `true|false`（默认 `false`）。
- `agents.defaults.stats.enable`: `true|false`（默认 `false`）。
- `agents.defaults.autocompact.enable`: `true|false`（默认 `false`）。
- `agents.defaults.prompt_cache.enable`: `true|false`（默认 `false`）。

**测试与验证**
- 重点回归: `go test ./agent ./gateway ./cli/...`。
- 手动验证: 流式输出、history 对比日志、审批链路、token 统计。
- 记录已知旧测失败列表，不在本分支修复。

**回滚策略**
- 所有新能力必须受配置开关保护。
- 关闭开关后必须与现有行为一致，不写入/不读 agentsdk-go 状态。

**开放问题（实现前确认）**
- 是否需要对 stream 事件协议做兼容层（旧客户端是否存在）？
- stats/trace 数据应该落在 `session`、metrics 还是两者？
- 是否需要为部分模型/渠道做灰度开关？
