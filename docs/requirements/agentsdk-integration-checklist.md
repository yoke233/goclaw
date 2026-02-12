# agentsdk-go 集成执行清单（给实施 worktree）

## 0. 代码与依赖准备
- [ ] 在 `go.mod` 引入 `github.com/cexll/agentsdk-go`（版本待确认）
- [ ] 执行 `go mod tidy`

## 1. 运行时适配层
- [ ] 新建 `agent/runtime/` 目录
- [ ] 定义 `SubagentRuntime` 接口与请求/结果结构体
- [ ] 实现 `AgentsdkRuntime`（进程内）
- [ ] 集成 Hook：Run/Tool 事件写入 registry

## 2. sessions_spawn 闭环
- [ ] `agent/manager.go` 完成 `handleSubagentSpawn`
- [ ] `agent/manager.go` 完成 `sendToSession`
- [ ] 分身执行完成后调用 `MarkCompleted` 并触发 `subagentAnnouncer`

## 3. 请求上下文透传
- [ ] 在 `AgentManager.handleInboundMessage` 创建 `context.WithValue`（sessionKey/agentID/channel/chatID）
- [ ] `SubagentSpawnTool.Execute` 从 ctx 读取请求者信息
- [ ] 分身 `SystemPrompt` 注入执行链路

## 4. 目录与技能隔离
- [ ] 约定目录：
  - `workspace/subagents/<run_id>/workspace`
  - `workspace/skills/<role>`
- [ ] 角色来源解析（task/label 前缀）
- [ ] 不存在目录时自动创建

## 5. 并发限制
- [ ] 角色级并发池（frontend=5, backend=4）
- [ ] 超出并发时排队

## 6. 配置扩展
- [ ] `config/schema.go` 增加 subagent runtime/目录字段
- [ ] 默认值落在 `config/loader.go`（如需要）

## 7. 验收
- [ ] 手动验证：frontend/ backend 任务并发执行
- [ ] registry 状态正确更新
- [ ] 主会话能看到分身结果

