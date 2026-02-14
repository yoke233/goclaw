# Bug Hunt 记录

更新时间：2026-02-13

## 已确认问题（通过测试触发）

1. `session/prune.go:282` `PruneMessages` 保留逻辑与语义不一致  
现象：传入 `preserveCount=2` 时，没有保留“最近 2 条”，而是出现保留条数错误。  
触发测试：`session/prune_test.go:29` `TestPrunerPruneMessagesKeepsMostRecent`

2. `session/prune.go:306` `PruneMessagesByTTL` 在“全部消息过期”时不会清理  
现象：当所有消息都早于 TTL 截止点，函数执行后消息仍保留。  
触发测试：`session/prune_test.go:60` `TestPrunerPruneMessagesByTTLRemovesAllExpired`

3. `session/prune.go:497` `Cleanup` 可能发生自锁死锁  
现象：`Cleanup()` 持锁后调用 `PruneSessions()`，后者重复获取同一把锁，导致阻塞。  
触发测试：`session/prune_test.go:93` `TestPrunerCleanupReturnsWithoutDeadlock`

4. `config/loader.go:282` Telegram token 校验规则错误  
现象：当前校验要求 token 以 `"bot"` 开头，但 Telegram 常规 token 格式为 `"<bot_id>:<secret>"`，不包含该前缀。  
触发测试：`config/loader_test.go:41` `TestValidateTelegramTokenWithoutBotPrefix`

5. `config/loader.go:164` `GetWorkspacePath` 未处理 `~` 路径展开  
现象：`workspace.path` 配置为 `"~/xxx"` 时返回原值，未解析为用户目录绝对路径。  
触发测试：`config/loader_test.go:52` `TestGetWorkspacePathExpandsTilde`

6. `bus/queue.go:147` `Close` 可能被阻塞中的 `ConsumeInbound` 卡住（锁顺序/粒度问题）  
现象：`ConsumeInbound` 在持有 `RLock` 时阻塞等待消息，`Close` 需要获取写锁，导致关闭过程阻塞。  
触发测试：`bus/queue_test.go:9` `TestMessageBusCloseNotBlockedByPendingConsumeInbound`

7. `config/loader.go:71` 网关默认超时单位可能错误（`time.Duration` 与整数字面量）  
现象：默认 `gateway.read_timeout` / `gateway.write_timeout` 设为 `30`，反序列化后表现为 `30ns`，与注释语义的秒级超时不一致。  
触发测试：`config/loader_test.go:76` `TestSetDefaultsGatewayTimeoutUsesSecondGranularity`

8. `bus/streaming.go:165` `StreamHandler.processChunk` 回调重入可能死锁  
现象：在持锁状态下调用 `onChunk` / `onComplete`，若回调内部调用 `GetContent` 等同锁方法会自锁。  
触发测试：`bus/streaming_test.go:9` `TestStreamHandlerOnChunkReentrantAccessShouldNotDeadlock`

9. `channels/base.go:76` `BaseChannelImpl` 重启后 `stopChan` 状态错误  
现象：执行 `Start -> Stop -> Start` 后，`WaitForStop()` 仍然是已关闭状态，重启实例会立刻收到停止信号。  
触发测试：`channels/base_test.go:10` `TestBaseChannelRestartDoesNotKeepClosedStopChan`

10. `bus/queue.go:213` 总线关闭后仍可创建活跃订阅  
现象：`Close()` 后调用 `SubscribeOutbound()` 仍返回可用订阅对象，channel 不会立即关闭，存在语义不一致/资源泄漏风险。  
触发测试：`bus/queue_test.go:53` `TestSubscribeOutboundAfterCloseShouldNotReturnActiveChannel`

11. `bus/queue.go:38` 发布空消息会触发空指针 panic  
现象：`PublishInbound(ctx, nil)` 直接解引用 `msg`，导致 panic，而非返回可处理错误。  
触发测试：`bus/queue_test.go:69` `TestPublishInboundNilMessageShouldNotPanic`

12. `channels/manager.go:31` `Manager.Register(nil)` 触发 panic  
现象：未做空值保护，调用 `channel.Name()` 导致空指针。  
触发测试：`channels/manager_test.go:9` `TestManagerRegisterNilChannelShouldNotPanic`

13. `session/tree.go:75` `CreateBranch` 接受空 branch ID，可能污染树索引  
现象：传入 `branchSession.Key == ""` 仍可创建分支，节点会以空字符串为 key 写入 map。  
触发测试：`session/tree_test.go:102` `TestSessionTreeCreateBranchRejectsEmptyBranchID`

14. `session/tree.go:203` `MergeBranch` 非幂等，重复 merge 会重复追加消息  
现象：同一分支多次 merge 时会重复把分支消息追加到父节点，造成内容重复。  
触发测试：`session/tree_test.go:115` `TestSessionTreeMergeBranchIsNotAppliedTwice`

15. `cron/cron.go:63` `Cron.Stop` 非幂等，重复调用会 panic  
现象：第二次调用 `Stop()` 触发 `close of closed channel`。  
触发测试：`cron/cron_test.go:5` `TestCronStopTwiceShouldNotPanic`

16. `cron/cron.go:84` `Parse` 未校验表达式，非法 spec 也返回成功  
现象：传入明显非法字符串仍返回 `nil` error。  
触发测试：`cron/cron_test.go:19` `TestParseInvalidSpecShouldReturnError`

17. `cron/scheduler.go:87` `Scheduler.AddJob(nil)` 触发 panic  
现象：未做空值检查，直接访问 `job.ID`。  
触发测试：`cron/scheduler_test.go:10` `TestSchedulerAddJobNilShouldNotPanic`

18. `gateway/handler.go:49` `HandleRequest(nil)` 触发 panic  
现象：未判空直接访问 `req.Method`。  
触发测试：`gateway/handler_test.go:26` `TestHandleRequestNilRequestShouldNotPanic`

19. `gateway/handler.go:49` 未区分 method-not-found 错误码  
现象：未知方法统一映射为 `-32603`（内部错误），而非 JSON-RPC 的 `-32601`。  
触发测试：`gateway/handler_test.go:41` `TestHandleRequestUnknownMethodReturnsMethodNotFound`

20. `internal/workspace/workspace.go:195` `ReadMemoryFile` 存在路径穿越读取风险  
现象：传 `../secret.txt` 可读取 `memory` 目录外文件。  
触发测试：`internal/workspace/workspace_test.go:9` `TestReadMemoryFileRejectsPathTraversal`

21. `cli/input/reader.go:71` `InitReadlineHistory(nil, ...)` 触发 panic  
现象：未判空直接调用 `rl.SaveHistory`。  
触发测试：`cli/input/reader_test.go:5` `TestInitReadlineHistoryNilReaderShouldNotPanic`

22. `gateway/protocol.go:97` `MethodRegistry` 调用 nil handler 会 panic  
现象：`Register(method, nil)` 后 `Call` 直接调用函数指针导致空指针异常。  
触发测试：`gateway/protocol_test.go:5` `TestMethodRegistryCallNilHandlerShouldNotPanic`

23. `memory/qmd/manager.go:344` `truncateSnippet` 在 `maxLen < 3` 时会切片越界 panic  
现象：`snippet[:maxLen-3]` 对小长度参数产生负下标。  
触发测试：`memory/qmd/manager_helpers_test.go:10` `TestTruncateSnippetSmallMaxLenShouldNotPanic`

24. `memory/qmd/manager.go:322` `expandHomeDir` 未兼容 Windows 风格 `~\\` 前缀  
现象：输入 `"~\\docs"` 不做展开，导致路径保持原样。  
触发测试：`memory/qmd/manager_helpers_test.go:23` `TestExpandHomeDirSupportsWindowsStylePrefix`

25. `config/loader.go:274` 通道校验未兼容“仅多账号配置”模式  
现象：`channels.<type>.enabled=true` 且顶层字段为空、但 `accounts` 内配置完整时，仍被判定无效。  
触发测试：`config/loader_test.go:93` `TestValidateChannelsMultiAccountMode`

26. `cron/cron.go:84` 未实现 `every N minutes` 语义  
现象：`Parse("every 5 minutes")`、`Parse("every 30 minutes")` 的 `Next()` 均返回固定 +1 分钟。  
触发测试：`cron/cron_test.go:27` `TestParseEveryNMinutes`

27. `channels/manager.go:109` 状态接口返回名称与注册别名不一致  
现象：通过 `RegisterWithName(channel, "telegram:acc1")` 注册后，`Status("telegram:acc1")` 返回 `name=telegram`，丢失账号维度。  
触发测试：`channels/manager_test.go:44` `TestManagerStatusUsesRegisteredAliasName`

28. `bus/events.go:29` 会话键未包含 `AccountID`，多账号会话会冲突  
现象：同一通道、同一 chat_id、不同账号生成相同 `SessionKey()`。  
触发测试：`bus/events_test.go:5` `TestInboundMessageSessionKeyIncludesAccountDimension`

29. `bus/streaming.go:165` 完成回调忽略 `IsFinal` 内容，返回空结果  
现象：仅发送 `IsFinal && IsComplete` chunk 时，`OnComplete` 收到的字符串为空。  
触发测试：`bus/streaming_test.go:38` `TestStreamHandlerCompleteCallbackIncludesFinalContent`

30. `internal/workspace/workspace.go:208` `ListMemoryFiles` 对未初始化目录不友好  
现象：在尚未执行 `Ensure()` 的新工作区上调用，返回目录不存在错误而不是空列表。  
触发测试：`internal/workspace/workspace_test.go:31` `TestListMemoryFilesWithoutEnsureReturnsEmpty`

31. `bus/queue.go:119` 出站消息在无订阅者时被丢弃，`ConsumeOutbound` 无法补偿读取  
现象：先 `PublishOutbound` 再 `ConsumeOutbound`（期间无订阅者）会超时，消息已在 fanout 阶段被丢弃。  
触发测试：`bus/queue_test.go:84` `TestConsumeOutboundCanReadPreviouslyPublishedMessage`

32. `internal/workspace/workspace.go:138` `ReadBootstrapFile` 存在路径穿越读取风险  
现象：传入 `../outside.txt` 可读取工作区目录外文件。  
触发测试：`internal/workspace/workspace_test.go:42` `TestReadBootstrapFileRejectsPathTraversal`

33. `gateway/handler.go:95` `logs.get` 未约束负数行数参数  
现象：传入 `lines=-5` 会原样返回负值，不符合“读取日志行数”语义。  
触发测试：`gateway/handler_test.go:82` `TestHandleRequestLogsGetRejectsNegativeLines`

34. `memory/search.go:110` `AddMemoryBatch` 未校验 embedding 数量一致性  
现象：Provider 返回 embedding 数量少于输入条数时发生数组越界 panic。  
触发测试：`memory/search_manager_test.go:50` `TestAddMemoryBatchEmbeddingCountMismatchShouldReturnError`

35. `agent/stream_collect.go:11` 流式文本拼接会丢失空白 token  
现象：中间空格增量（`" "`）被过滤，`"Hello" + " " + "World"` 变为 `"HelloWorld"`。  
触发测试：`agent/stream_collect_test.go:9` `TestCollectStreamOutputPreservesWhitespaceSemantics`

36. `agent/stream_collect.go:58` 结果统一 `TrimSpace`，破坏前后空格语义  
现象：仅一个 delta `"  hello  "` 最终输出变成 `"hello"`。  
触发测试：`agent/stream_collect_test.go:9` `TestCollectStreamOutputPreservesWhitespaceSemantics`

37. `agent/session_key.go:38` Fresh key 生成策略存在高碰撞风险  
现象：`FreshOnDefault=true` 时 key 基于 `Unix()` 秒级时间戳，多次快速创建会话得到相同 key。  
触发测试：`agent/session_key_test.go:5` `TestResolveSessionKeyFreshOnDefaultGeneratesUniqueKeys`

38. `skills/eligibility.go:193` 路径匹配使用 `Contains`，会误匹配兄弟目录前缀  
现象：force-include `/.../skill` 会错误匹配 `/.../skill-extra`。  
触发测试：`skills/pattern_filter_test.go:8` `TestPatternFilterForceIncludeShouldNotMatchSiblingPrefixPath`

39. `gateway/protocol.go:118` 方法名校验未过滤空白字符  
现象：`"method":"   "` 会被当作合法请求通过解析。  
触发测试：`gateway/protocol_test.go:20` `TestParseRequestRejectsWhitespaceMethod`

40. `agent/subagent_announce.go:322` 默认超时仅在外层生效，未传递给底层等待函数  
现象：`timeoutSeconds<=0` 时外层使用 5 分钟，但传入 `waitFunc` 的仍是 0/负数。  
触发测试：`agent/subagent_wait_test.go:5` `TestWaitForSubagentCompletionPassesEffectiveTimeoutToWaitFunc`

41. `extensions/claude_plugins.go:856` 工具选择器标准化未去除首尾空白  
现象：matcher 配置值如 `"  Bash  "` 未被 trim，可能导致规则匹配失败。  
触发测试：`extensions/claude_plugins_helpers_test.go:5` `TestNormalizeToolSelectorPatternTrimsWhitespace`

42. `providers/streaming.go:61` 流缓冲上限校验未计入“本次新增 chunk”长度  
现象：内容已满时继续追加仍成功，导致缓冲超过 `maxSize`。  
触发测试：`providers/stream_buffer_test.go:5` `TestStreamBufferEnforcesMaxSizeWithIncomingChunk`

43. `agent/tools/subagent_spawn_tool.go:360` `run_timeout_seconds` 允许负值透传  
现象：传入负超时时间仍返回 accepted，并将负值写入运行参数。  
触发测试：`agent/tools/subagent_spawn_tool_test.go:114` `TestSubagentSpawnToolExecuteRejectsNegativeTimeout`

44. `agent/tools/subagent_spawn_tool.go:301` cleanup 策略未做 trim/lower 规范化  
现象：传入 `"  DELETE  "` 被当作非法值回退到 `"keep"`，与用户意图不一致。  
触发测试：`agent/tools/subagent_spawn_tool_test.go:134` `TestSubagentSpawnToolExecuteNormalizesCleanupPolicy`

45. `agent/tools/subagent_spawn_tool.go:321` `agent_id` 未去除首尾空白导致误判跨 Agent  
现象：`"  assistant-main  "` 本应视为同一 agent，却触发权限校验并返回 forbidden。  
触发测试：`agent/tools/subagent_spawn_tool_test.go:157` `TestSubagentSpawnToolExecuteTrimsAgentIDBeforePermissionCheck`

46. `memory/store.go:502`/`memory/store.go:909` 过滤占位符与模板拼接产生双括号 SQL  
现象：`sourcePlaceholders/typePlaceholders` 已返回带括号片段，外层 `IN (...)` 再包一层后形成 `IN ((?,?))`。  
触发测试：`memory/store_helpers_test.go:8` `TestSearchVectorFilterSQLShouldNotContainDoubleParentheses`

47. `gateway/handler.go:70` 未区分“方法不存在”与“内部错误”  
现象：调用未注册方法时返回 `ErrorInternalError(-32603)`，而非 JSON-RPC 语义更准确的 `ErrorMethodNotFound(-32601)`。  
触发测试：`gateway/handler_test.go:42` `TestHandleRequestUnknownMethodReturnsMethodNotFound`

48. `agent/tools/mcp_manage.go:292` 传输配置歧义未拒绝（`command` 与 `url` 同时给出）  
现象：未指定 `type` 且同时提供 `command`+`url` 时仍返回成功并推断为 `stdio`，导致配置语义含混。  
触发测试：`agent/tools/manage_tools_test.go:688` `TestMCPPutServerRejectsAmbiguousTransportConfig`

49. `gateway/handler.go:164` 小数秒超时被截断为 0 秒  
现象：`agent.wait` 传入 `timeout=0.5` 时被 `time.Duration(t)` 截断为 `0`，请求立即超时。  
触发测试：`gateway/handler_test.go:107` `TestHandleRequestAgentWaitSupportsFractionalTimeoutSeconds`

50. `agent/runtime/mcp_settings_overrides.go:201` Bearer token 注入未按 HTTP 头大小写不敏感语义处理  
现象：已有 `authorization` 头时仍新增 `Authorization`，导致同义头重复并可能触发服务端认证歧义。  
触发测试：`agent/runtime/mcp_settings_overrides_test.go:220` `TestBuildSDKMCPOverridesFromAgentsConfigAuthorizationHeaderCaseInsensitive`

51. `agent/runtime/mcp_settings_overrides.go:224` MCP 工具过滤列表未做标准化清洗  
现象：`enabled_tools/disabled_tools` 中带空白或空字符串会原样透传，导致工具匹配失败或出现无效条目。  
触发测试：`agent/runtime/mcp_settings_overrides_test.go:256` `TestBuildSDKMCPOverridesFromAgentsConfigNormalizesToolFilters`

52. `agent/runtime/agentsdk_runtime.go:100` `Spawn` 忽略调用方 context 取消信号  
现象：`Spawn(ctx, req)` 在 `ctx` 已取消时仍成功创建运行，可能导致请求取消后后台任务继续执行。  
触发测试：`agent/runtime/agentsdk_runtime_spawn_test.go:13` `TestAgentsdkRuntimeSpawnHonorsCanceledContext`

53. `agent/runtime/mcp_settings_overrides.go:58` 显式 MCP 配置路径不存在时无告警（静默退为空配置）  
现象：`MCPConfigPath` 指向不存在文件时 `LoadAgentsConfig` 返回空配置且无错误，最终 MCP 覆盖为空且无 warning。  
触发测试：`agent/runtime/mcp_settings_overrides_test.go:289` `TestBuildSubagentSDKSettingsOverrides_ExplicitMissingPathShouldWarn`

54. `memory/store.go:318`/`memory/store.go:394` 单连接 SQLite 下 `Add/AddBatch` 存在自锁死风险  
现象：事务中调用 `s.isVectorEnabled()/s.isFTSEnabled()` 走 `db.QueryRow` 需要新连接，但连接池限制 `MaxOpenConns=1`，导致写入阻塞。  
触发测试：`memory/store_deadlock_test.go:8` `TestSQLiteStoreAddShouldNotBlockWhenVectorAndFTSDisabled`

55. `agent/runtime/skills_loader.go:168` `repo` 层 `.disabled` 无法屏蔽继承的同名 `base` skill  
现象：`repo` 中同名 skill 标记 `.disabled` 后只会被本层跳过，`base` 层同名技能仍被加载，违背“repo overrides base”语义。  
触发测试：`agent/runtime/skills_loader_test.go:11` `TestBuildSubagentSkillRegistrationsRepoDisabledShouldOverrideBaseSkill`

56. `extensions/agents_config.go:215` 分层合并时无法用空 `http_headers` 清空继承配置  
现象：higher 显式 `http_headers={}` 时，lower 头部仍保留，导致无法移除继承的认证/路由头。  
触发测试：`extensions/agents_config_merge_test.go:5` `TestMergeAgentsConfigHigherEmptyHeadersShouldClearLowerHeaders`

57. `extensions/agents_config.go:207` 分层合并时无法用空 `env` 清空继承环境变量  
现象：higher 显式 `env={}` 时，lower 环境变量仍保留，导致无法撤销继承的敏感变量。  
触发测试：`extensions/agents_config_merge_test.go:33` `TestMergeAgentsConfigHigherEmptyEnvShouldClearLowerEnv`

58. `extensions/claude_plugins.go:749` 空 `mcpServers` 包装对象被误解析为伪服务器  
现象：`{"mcpServers": {}}` 会回退到 direct map 解析并生成名为 `mcpServers` 的空 server。  
触发测试：`extensions/claude_plugins_mcp_parse_test.go:5` `TestParsePluginMCPServersEmptyWrapperShouldReturnNoServers`

59. `session/tree.go:234` 合并默认分支会重复追加父历史消息  
现象：`CreateBranch(..., nil, ...)` 默认复制父消息，`MergeBranch` 又将整段 branch 消息回灌父会话，导致历史重复。  
触发测试：`session/tree_test.go:155` `TestSessionTreeMergeDefaultBranchShouldNotDuplicateParentHistory`

60. `config/loader.go:333` 多账号校验采用“任一账号有效即通过”，会放过其他已启用坏账号  
现象：`telegram.accounts` 中存在一个有效账号时，其他 `Enabled=true` 且 token 非法的账号不会触发校验错误。  
触发测试：`config/loader_test.go:170` `TestValidateRejectsWhenAnyEnabledTelegramAccountIsInvalid`

61. `channels/manager.go:320` Feishu 多账号未继承顶层 webhook 安全配置  
现象：多账号模式仅透传 `AppID/AppSecret/AllowedIDs`，丢失顶层 `verification_token/encrypt_key/webhook_port`，导致 challenge 校验与签名能力异常。  
触发测试：`channels/manager_test.go:70` `TestManagerSetupFromConfigFeishuAccountsShouldInheritGlobalWebhookSettings`

62. `channels/manager.go:405` WeWork 多账号未继承顶层 webhook 安全配置  
现象：多账号模式仅透传 `CorpID/AgentID/Secret/AllowedIDs`，丢失顶层 `token/encoding_aes_key/webhook_port`，导致回调验签/解密配置缺失。  
触发测试：`channels/manager_test.go:111` `TestManagerSetupFromConfigWeWorkAccountsShouldInheritGlobalWebhookSettings`

63. `config/loader.go:390` Feishu 多账号校验仍强制顶层 `app_id/app_secret`  
现象：`feishu.accounts` 已提供账号级 `app_id/app_secret`，且设置共享 `verification_token`，校验仍因顶层 `app_id/app_secret` 为空而失败。  
触发测试：`config/loader_test.go:93` `TestValidateChannelsMultiAccountMode/feishu_with_account_app_credentials_and_shared_verification_token`

64. `config/loader.go:402` QQ 多账号校验仍强制顶层 `app_id/app_secret`  
现象：`qq.accounts` 已提供账号级 `app_id/app_secret`，校验仍要求顶层字段，导致多账号配置被误拒绝。  
触发测试：`config/loader_test.go:93` `TestValidateChannelsMultiAccountMode/qq_with_account_app_credentials_only`

65. `config/loader.go:413` WeWork 多账号校验仍强制顶层 `corp_id/secret/agent_id`  
现象：`wework.accounts` 已提供账号级 `corp_id/agent_id/app_secret`，校验仍要求顶层字段，导致多账号配置被误拒绝。  
触发测试：`config/loader_test.go:93` `TestValidateChannelsMultiAccountMode/wework_with_account_credentials_only`

66. `gateway/handler.go:71` 参数校验错误被映射为内部错误码  
现象：`agent` 缺少必填 `content` 时返回 `ErrorInternalError(-32603)`，而非客户端可修复的 `ErrorInvalidParams(-32602)`。  
触发测试：`gateway/handler_test.go:157` `TestHandleRequestAgentMissingContentReturnsInvalidParams`

67. `gateway/protocol.go:27` JSON-RPC `id` 被固定为字符串，无法兼容数字 ID  
现象：请求 `{"id":1,...}` 解析直接失败（Go 反序列化报类型不匹配），与常见 JSON-RPC 客户端默认数字 ID 不兼容。  
触发测试：`gateway/protocol_test.go:28` `TestParseRequestAcceptsNumericID`

68. `config/loader.go:331` `DingTalk` 启用状态缺少必填凭证校验  
现象：`dingtalk.enabled=true` 且未配置任何凭证（无顶层、无可用账号）时，`Validate` 仍返回成功，运行期才暴露不可用。  
触发测试：`config/loader_test.go:236` `TestValidateRejectsEnabledDingTalkWithoutCredentials`

69. `config/loader.go:331` `DingTalk` 多账号启用时未校验账号级 `client_secret`  
现象：`dingtalk.accounts.acc1.enabled=true` 且仅有 `client_id` 时，`Validate` 仍通过，`SetupFromConfig` 阶段才在创建通道时报错并静默跳过。  
触发测试：`config/loader_test.go:245` `TestValidateRejectsEnabledDingTalkAccountWithoutClientSecret`

70. `session/manager.go:234` `List()` 返回文件名映射 key，导致特殊字符 key 失真  
现象：保存 key 为 `user/1:alpha` 后，`List()` 返回 `user_1_alpha`，调用方无法拿到真实业务会话 key。  
触发测试：`session/manager_test.go:111` `TestManagerListShouldReturnOriginalKeyForSpecialCharacters`

71. `session/manager.go:310` 文件名清洗多对一映射会导致会话持久化串线  
现象：`team/a:b` 与 `team_a/b` 清洗后同名文件，保存第二个会把第一个会话历史混入同一文件，重启后读取出现跨会话消息污染。  
触发测试：`session/manager_test.go:149` `TestManagerSaveShouldNotCollideWhenKeysSanitizeToSameFilename`

72. `session/cache.go:261` `Contains()` 未检查 TTL，过期会话仍被视为存在  
现象：缓存项已过期但未被主动清理时，`Contains(key)` 仍返回 `true`，导致上层误判“可直接复用会话”。  
触发测试：`session/cache_test.go:92` `TestCacheContainsShouldReturnFalseForExpiredEntry`

73. `session/cache.go:270` `Keys()` 会返回已过期 key，导致缓存索引脏读  
现象：`Keys()` 仅遍历 map，不做 TTL 过滤；调用方会拿到已过期 key 列表并继续尝试使用。  
触发测试：`session/cache_test.go:108` `TestCacheKeysShouldNotIncludeExpiredEntry`

74. `session/prune.go:310` `PruneMessagesByTTL` 仅裁剪“前缀过期消息”，乱序场景漏删过期项  
现象：当消息序列为“新-旧-新”时，函数因首条是新消息直接返回，导致中间已过期消息长期保留。  
触发测试：`session/prune_test.go:120` `TestPrunerPruneMessagesByTTLShouldRemoveExpiredMessagesEvenWhenOutOfOrder`

75. `providers/streaming.go:146` `ThinkingParser` 不支持跨 chunk 的起始标签拼接  
现象：当模型分片输出 `<thin` 与 `king>...` 时，前半标签会被当正文输出，导致思考内容泄漏到普通文本流。  
触发测试：`providers/thinking_parser_test.go:5` `TestThinkingParserHandlesSplitThinkingStartTagAcrossChunks`

76. `providers/streaming.go:146` `ThinkingParser` 不支持跨 chunk 的结束标签拼接  
现象：当输出 `</fi` + `nal>` 时，结束标签碎片会污染 final 文本内容，并额外产生普通文本噪声。  
触发测试：`providers/thinking_parser_test.go:25` `TestThinkingParserHandlesSplitFinalClosingTagAcrossChunks`

## 备注

- 当前阶段按“先发现问题，不急于修复”的目标执行。  
- 后续新增发现会持续追加到本文件。

## 复测状态更新（2026-02-13）

- 已复测通过（当前不复现）：`#60` `#61` `#62` `#63` `#64` `#65` `#66` `#67` `#68` `#69`  
  说明：这些条目在本轮开始时曾复现，但代码已有后续改动；对应测试当前转绿。为保留排查历史，不删除原记录。
