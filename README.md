# goclaw

Go 语言版本的 openclaw。

## 功能特性

- 🛠️ **完整的工具系统**：FileSystem、Shell、Web、Message、Spawn，支持安全沙箱与权限控制。
- 📚 **技能系统 (Skills)**：兼容 [OpenClaw](https://github.com/openclaw/openclaw) 和 [AgentSkills](https://agentskills.io) 规范，支持自动发现与环境准入控制 (Gating)。
- 💾 **持久化会话**：基于 JSONL 的会话存储，支持完整的工具调用链 (Tool Calls) 记录与恢复。
- 📢 **多渠道支持**：Telegram、WhatsApp、飞书 、QQ。
- 🔧 **灵活配置**：支持 YAML/JSON 配置，热加载。
- 🎯 **多 LLM 提供商**：OpenAI (兼容接口)、Anthropic、OpenRouter。

## 技能系统 (New!)

goclaw 引入了先进的技能系统，允许用户通过编写 Markdown 文档 (`SKILL.md`) 来扩展 Agent 的能力。

### 特性
*   **Prompt-Driven**: 技能本质上是注入到 System Prompt 中的指令集，指导 LLM 使用现有工具 (exec, read_file 等) 完成任务。
*   **OpenClaw 兼容**: 完全兼容 OpenClaw 的技能生态。您可以直接将 `openclaw/skills` 目录下的技能复制过来使用。
*   **自动准入 (Gating)**: 智能检测系统环境。例如，只有当系统安装了 `curl` 时，`weather` 技能才会生效；只有安装了 `git` 时，`git-helper` 才会加载。

### 使用方法

1.  **列出可用技能**
    ```bash
    ./goclaw skills list
    ```

2.  **安装技能**
    将技能文件夹放入以下任一位置：
    *   `./skills/` (随附目录，最高优先级)
    *   `${WORKSPACE}/skills/` (工作区目录)
    *   `~/.goclaw/skills/` (用户全局目录)

3.  **编写技能**
    创建一个目录 `my-skill`，并在其中创建 `SKILL.md`：
    ```yaml
    ---
    name: my-skill
    description: A custom skill description.
    metadata:
      openclaw:
        requires:
          bins: ["python3"] # 仅当 python3 存在时加载
    ---
    # My Skill Instructions
    When the user asks for X, use `exec` to run `python3 script.py`.
    ```

## 项目结构

```
goclaw/
├── agent/              # Agent 核心逻辑
│   ├── loop.go         # Agent 循环
│   ├── context.go      # 上下文构建器
│   ├── memory.go       # 记忆系统
│   ├── skills.go       # 技能加载器
│   ├── subagent.go     # 子代理管理器
│   └── tools/          # 工具系统
├── channels/           # 消息通道
│   ├── base.go         # 通道接口
│   ├── telegram.go     # Telegram 实现
│   ├── whatsapp.go     # WhatsApp 实现
│   └── feishu.go       # 飞书实现
├── bus/                # 消息总线
│   ├── events.go       # 消息事件
│   └── queue.go        # 消息队列
├── config/             # 配置管理
│   ├── schema.go       # 配置结构
│   └── loader.go       # 配置加载器
├── providers/          # LLM 提供商
│   ├── base.go         # 提供商接口
│   ├── openai.go       # OpenAI 实现
│   ├── anthropic.go    # Anthropic 实现
│   └── openrouter.go   # OpenRouter 实现
├── session/            # 会话管理
│   └── manager.go      # 会话管理器
├── cli/                # 命令行界面
│   └── root.go         # CLI 命令
├── internal/           # 内部包
│   ├── logger/         # 日志
│   └── utils/          # 工具函数
└── main.go             # 主入口
```

## 快速开始

### 安装

```bash
go mod tidy
go build -o goclaw .
```

### 配置

创建 `config.json`:

```json
{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 0.7
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "your-openrouter-key"
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "your-telegram-bot-token"
    }
  }
}
```

### 运行

```bash
# 启动 Agent
./goclaw start

# 交互模式
./goclaw chat

# 查看配置
./goclaw config show
```

## 开发

### 添加新工具

在 `agent/tools/` 目录下创建新工具文件，实现 `Tool` 接口：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

### 添加新通道

在 `channels/` 目录下创建新通道，实现 `BaseChannel` 接口：

```go
type BaseChannel interface {
    Name() string
    Start(ctx context.Context) error
    Send(msg OutboundMessage) error
    IsAllowed(senderID string) bool
}
```

## 常见问题

### Q: 如何切换不同的 LLM 提供商？

A: 修改配置文件中的 `model` 字段：
- `gpt-4` - OpenAI
- `claude-3-opus-20240229` - Anthropic
- `openrouter:anthropic/claude-opus-4-5` - OpenRouter

### Q: 工具调用失败怎么办？

A: 检查工具配置，确保 `enabled: true`，且没有权限限制。

### Q: 如何限制 Shell 工具的权限？

A: 在配置中设置 `denied_cmds` 列表，添加危险的命令。


## 许可证

MIT
