# Mady（中观智能体）

[![CI](https://github.com/xujian519/mady/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/xujian519/mady/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/xujian519/mady.svg)](https://pkg.go.dev/github.com/xujian519/mady)

> 「中观」（Madhyamaka）的核心是 **"离二边，行中道"** ——
> 不落"有、无"两边，不执"生、灭"两端，超越一切对立概念，直达实相。
>
> 用 Go 来实现一个"中观"风格的智能体，非常契合：Go 的哲学本身就是
> **克制、中庸、去繁就简** —— 反对过度的抽象和复杂的类型体操，主张显式而非隐式。

## 指导思想

Mady 以四大思想为指引：

| 思想 | 核心要义 |
|------|---------|
| **毛泽东思想** | 实事求是，从实际出发，理论联系实际 |
| **钱学森系统工程** | 整体论与还原论的辩证统一，把 Agent 视为开放的复杂巨系统 |
| **维特根斯坦哲学** | 语言的界限即世界的界限，法律与专利的核心是语言游戏的精确运用 |
| **中论（Madhyamaka）** | 离二边，行中道，追求恰到好处的工程实践 |

## 愿景

Mady 的目标是成为 **专利代理人、专利律师、知识产权从业者、法律专业人士** 的智能 Agent 平台。

### 五步工作法

```
发现事实 → 获取规则 → 规划 → 执行 → 检查
```

1. **发现事实** — 收集用户输入、相关文档、上下文信息
2. **获取规则** — 检索相关法律法规、审查指南、判例、技术规范
3. **规划** — 基于事实和规则，制定行动方案
4. **执行** — 逐步执行，调用工具、生成文书、进行分析
5. **检查** — 对照事实和规则，验证结果，发现偏差并纠正

### 核心原则

**重点节点必须进行人机协作。** 关键决策点、法律判断、风险评估不可由 AI 独立完成，必须引入人类专业人士的确认与干预。

### 发展路线

- **前期**：聚焦聊天对话、智能助理、专利检索与分析、法律文书等核心领域
- **后期**：横向扩展至更多专业场景，纵向深化领域知识与推理能力

## 安装

### 从源码构建

```bash
git clone https://github.com/xujian519/mady.git
cd mady

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的 API Key

# 构建
go build ./...

# 运行 CLI 聊天
go run ./example/cli-chat/
```

### 作为库使用

```bash
go get github.com/xujian519/mady/agentcore
```

需要 **Go 1.25+**。

> 完整的环境变量说明见 [.env.example](.env.example) | 贡献指南见 [CONTRIBUTING.md](CONTRIBUTING.md) | 变更日志见 [CHANGELOG.md](CHANGELOG.md)

## 快速开始

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/xujian519/mady/agentcore"
    "github.com/xujian519/mady/provider/chatcompat"
)

func main() {
    agent := agentcore.New(agentcore.Config{
        Name:         "assistant",
        SystemPrompt: "你是一个有用的助手。",
        Provider:     chatcompat.New(chatcompat.Config{APIKey: "sk-..."}),
        Tools: []*agentcore.Tool{{
            Name:        "greet",
            Description: "打招呼",
            Parameters:  map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
            Func: func(ctx context.Context, args json.RawMessage) (any, error) {
                return "你好！", nil
            },
        }},
    })

    output, err := agent.Run(context.Background(), "你好")
    if err != nil {
        panic(err)
    }
    fmt.Println(output)
}
```

## 核心概念

### Agent 主循环

`Agent` 执行 LLM-工具循环，支持可配置的最大轮次、自动上下文压缩和指数退避重试。

```go
agent := agentcore.New(agentcore.Config{
    Name:         "coder",
    Provider:     provider,
    MaxTurns:     20,
    ContextWindow: &agentcore.ContextWindowConfig{MaxTokens: 128000, CompactionThreshold: 0.8},
    RetryConfig:  &agentcore.RetryConfig{MaxRetries: 3, InitialDelay: time.Second},
})
```

### 工具系统

注册带 JSON Schema 校验、单工具钩子和全局中间件的工具。

```go
tool := &agentcore.Tool{
    Name:        "read_file",
    Description: "从磁盘读取文件",
    Parameters:  map[string]any{...},
    Func: func(ctx context.Context, args json.RawMessage) (any, error) {
        // ...
    },
    Before: []agentcore.BeforeHook{authCheck},
    After:  []agentcore.AfterHook{auditLog},
}
```

### 内置工具扩展

`mady` 内置了完整的 `tools` 包，提供文件系统、Shell、搜索、浏览器和代码执行工具，以单一可插拔 `Extension` 形式提供。

```go
import "github.com/xujian519/mady/tools"

ext := tools.NewExtension(tools.ExtensionConfig{WorkingDir: "/path/to/project"})
agent := agentcore.New(agentcore.Config{
    Extensions: []agentcore.Extension{ext},
})
```

每个工具支持可插拔的操作接口，可将执行委托给远程系统（如 SSH）。

### MCP 工具

`mady` 可将外部 MCP 服务器桥接为 `agentcore.Tool`。支持 MCP `stdio` 传输和 HTTP/SSE 传输，以及 `tools/list` / `tools/call`。

```go
import "github.com/xujian519/mady/mcp"

ctx := context.Background()
ext, err := mcp.NewStdioExtension(ctx, mcp.StdioConfig{
    Name:       "filesystem",
    Command:    "npx",
    Args:       []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
    ToolPrefix: "fs.",
})
if err != nil {
    panic(err)
}

agent := agentcore.New(agentcore.Config{
    Name:       "assistant",
    Model:      "deepseek-chat",
    Provider:   provider,
    Extensions: []agentcore.Extension{ext},
})
defer agent.Close()
```

- `NewStdioExtension(...)` 立即初始化 MCP 客户端，列举远程工具，通过 `Config.Extensions` 暴露
- `ToolPrefix` 为可选参数，组合多个 MCP 服务器时建议使用，避免工具名冲突
- MCP 工具执行错误（`isError: true`）保留为工具输出而非传输失败，让模型可自行纠错

### 多 Agent 交接

在专业 Agent 之间委派或转移任务——支持本地和远程（通过 A2A 协议）。

```go
cfg := agentcore.Config{
    Handoffs: []agentcore.HandoffConfig{
        {Name: "math-expert", Agent: mathCfg, Mode: agentcore.HandoffDelegate},
        {Name: "code-expert", Agent: codeCfg, Mode: agentcore.HandoffTransfer},
    },
}
```

### 事件系统

类型安全的事件总线，用于实时可观测性。

```go
agent.On(agentcore.EventMessageDelta, func(e agentcore.Event) {
    delta := e.(*agentcore.MessageDeltaEvent)
    fmt.Print(delta.Content)
})

agent.On(agentcore.EventToolCallEnd, func(e agentcore.Event) {
    tc := e.(*agentcore.ToolCallEndEvent)
    fmt.Printf("工具 %s 执行完成\n", tc.Name)
})
```

### 生命周期钩子

可拦截 Agent 执行的每个阶段。

```go
agent := agentcore.New(agentcore.Config{
    Lifecycle: agentcore.LifecycleChain{
        &agentcore.GuardrailHook{Check: safetyCheck},
        &agentcore.AuditHook{OnEvent: logEvent},
        &agentcore.RateLimitHook{Limiter: limiter},
    },
})
```

### 图引擎

**DAG** — 独立分支并行执行：

```go
g := graph.NewGraph()
g.AddNode("parse", parseStep)
g.AddNode("validate", validateStep)
g.AddNode("transform", transformStep)
g.AddEdge("parse", "validate")
g.AddEdge("parse", "transform")

cg, _ := g.Compile(graph.CompileOptions{EntryNode: "parse"})
output, _ := cg.Run(ctx, input)
```

**Pregel** — 带超步迭代的循环状态图：

```go
pg := graph.NewPregelGraph()
pg.AddNode("agent", agentNode)
pg.AddNode("tools", toolsNode)
pg.AddEdge("tools", "agent")
pg.SetConditionalEdge("agent", func(ctx context.Context, state graph.PregelState) []string {
    if state.GetString("done") == "true" {
        return []string{graph.PregelEnd}
    }
    return []string{"tools"}
})

cpg, _ := pg.Compile("agent")
finalState, _ := cpg.Run(ctx, graph.PregelState{"input": "求解 x^2=4"})
```

### 会话管理

追加写入的 JSONL 树结构，支持分支、压缩、标签和版本迁移。

```go
store, _ := session.NewFileStore("./sessions")
mgr, _ := store.Create(ctx, session.CreateOptions{Cwd: "/project"})

// 追加消息
mgr.AppendMessage(ctx, agentcore.Message{Role: "user", Content: "你好"})
mgr.AppendMessage(ctx, agentcore.Message{Role: "assistant", Content: "你好！有什么可以帮你的？"})

// 从历史节点创建分支
mgr.Branch(earlierEntryID)
mgr.AppendMessage(ctx, agentcore.Message{Role: "user", Content: "换个话题"})

// 获取当前路径上的消息（自动处理压缩和分支摘要）
msgs := mgr.MessagesOnPath()

// 树结构检查
tree := mgr.GetTree()
stats := mgr.Stats()
leaves := mgr.Leaves()
```

### 工作流编排

```go
pipeline := &workflow.Pipeline{
    Steps: []workflow.Step{parseStep, validateStep, transformStep},
}

parallel := &workflow.Parallel{
    Steps: []workflow.Step{fetchA, fetchB, fetchC},
    Merge: func(results []string) string { return strings.Join(results, "\n") },
}

router := &workflow.Router{
    Route: func(ctx context.Context, input string) string {
        if strings.Contains(input, "code") { return "coder" }
        return "chat"
    },
    Branches: map[string]workflow.Step{"coder": coderStep, "chat": chatStep},
}
```

### Provider（模型接入）

Mady 统一使用 OpenAI Chat Completions 兼容协议，通过 `chatcompat` Provider 接入所有国产模型。

```go
// DeepSeek（推荐用于日常编码和推理）
// 模型: deepseek-chat（通用）, deepseek-reasoner（深度推理）
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
    BaseURL: "https://api.deepseek.com/v1",
})

// 智谱 GLM 编程套餐（推荐用于代码生成和 Agent 工作流）
// 模型: glm-5.2, glm-4.5
// 注意：编程套餐需使用专属端点，不能使用通用端点
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("ZHIPU_API_KEY"),
    BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
})

// Kimi / Moonshot 编程套餐（K2 系列，强于编程和长文本）
// 模型: kimi-k2-0905-preview（通用）, kimi-k2.7-code（编程专用）
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("KIMI_API_KEY"),
    BaseURL: "https://api.moonshot.cn/v1",
})

// 通义千问（阿里云）
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("QWEN_API_KEY"),
    BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
})

// 通用 OpenAI 兼容协议
// 可用于任何兼容 OpenAI Chat Completions API 的服务
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("API_KEY"),
    BaseURL: os.Getenv("BASE_URL"),
})
```

所有 Provider 均支持流式响应（SSE）、工具调用、结构化输出和多模态。

### 结构化输出

`agentcore.Config` 通过 `ResponseFormat` 支持结构化输出。所有 Provider 使用相同的高层请求结构。

```go
agent := agentcore.New(agentcore.Config{
    Model:    "deepseek-chat",
    Provider: chatcompat.New(chatcompat.Config{APIKey: os.Getenv("DEEPSEEK_API_KEY")}),
    ResponseFormat: agentcore.NewJSONSchemaResponseFormat("answer", map[string]any{
        "type": "object",
        "properties": map[string]any{
            "answer": map[string]any{"type": "string"},
        },
        "required": []string{"answer"},
    }),
})

out, err := agent.Run(ctx, "请用 JSON 回复")
// out 为原始 JSON 字符串，例如 {"answer":"好的"}
```

也可以直接解码为类型化结果：

```go
type Answer struct {
    Answer string `json:"answer"`
}

result, err := agentcore.RunStructured[Answer](ctx, agent, "请用 JSON 回复")
```

### 推理过程控制

部分国产模型（如 DeepSeek reasoner）支持推理过程输出。可通过 `agentcore.Config.Thinking` 配置：

```go
agent := agentcore.New(agentcore.Config{
    Name:     "reasoner",
    Model:    "deepseek-reasoner",
    Provider: provider,
    Thinking: &agentcore.ThinkingConfig{
        IncludeThoughts: true,
        Display:         agentcore.ThinkingDisplaySummarized,
    },
})
```

- `Display` 控制是否返回推理摘要（`summarized` / `omitted`）
- `chatcompat` 通过 OpenAI 兼容协议的 `reasoning_content` 字段传递推理内容

### 富媒体消息块

`agentcore.Message` 支持图片输入以及文本和推理片段。所有 Provider 将其作为原生多模态内容发送。

```go
msg := agentcore.Message{
    Role:    agentcore.RoleUser,
    Content: "这张图片里有什么？",
}.AppendImageURLBlock("https://example.com/cat.png")
```

- `chatcompat`：将图片块作为 `image_url` 多部分内容发送（OpenAI 兼容格式）

Provider 响应通过 `ProviderResponse.Blocks` 保留块结构输出（含流式聚合），Agent 将助手块持久化到 `Message.Blocks` 中。

## A2A 协议（Agent-to-Agent）

与任何 A2A 兼容的 Agent 互操作——包括 Google ADK Agent。

```go
// 将你的 Agent 暴露为 A2A 服务器
handler := a2a.NewDefaultAgentHandler(card, agent, agent.Config())
server := a2a.NewServer(handler)
log.Fatal(server.ListenAndServe(":8080"))
```

```go
// 调用远程 A2A Agent
client := a2a.NewClient("http://remote-agent.example.com")
task, err := client.SendTask(ctx, a2a.SendTaskRequest{
    ID: "task-123",
    Message: a2a.Message{Role: "user", Parts: []a2a.Part{a2a.NewTextPart("你好")}},
})
```

```go
// 将远程 A2A Agent 注册为 Handoff 目标
ext := a2a.NewRemoteHandoffExtension([]a2a.RemoteHandoffConfig{{
    Name: "math-expert", URL: "http://math-agent.example.com",
}})
agent := agentcore.New(agentcore.Config{Extensions: []agentcore.Extension{ext}})
```

功能特性：
- Agent Card 发现（`/.well-known/agent.json`）
- 完整任务生命周期（已提交 → 执行中 → 已完成/失败/已取消）
- 同步和流式（SSE）模式
- 多模态内容：文本、文件、结构化数据部件
- 推送通知 Webhook
- WebSocket 传输

## ACP（Agent Communication Protocol）

基于 JSON-RPC 的 Agent 间通信协议，提供：
- `AgentFactory` / `AgentInstance` 接口，按会话创建和运行 Agent
- 基于会话的 Agent 生命周期管理
- 可扩展的认证 Provider 支持

## AGUI（Agent GUI 事件）

基于 SSE 的事件协议，用于将 Agent 执行过程流式传输到 Web UI：

```go
handler := agui.NewHandler(config)
http.Handle("/events", handler)
```

事件类型涵盖：运行生命周期、步骤进度、文本增量、推理块、工具调用、状态快照和自定义事件。

## A2UI（Agent-to-UI）

声明式 UI 协议，Agent 可在客户端渲染和更新界面（表单、仪表盘、实时数据流），内置数据绑定、验证，支持通过 A2A 或 AG-UI 传输。

```go
env := a2ui.NewSurface("profile", a2ui.BasicCatalogID).
    Add(a2ui.Column("root", "name", "title")).
    Add(a2ui.Text("name", a2ui.Bind("/user/name"))).
    Add(a2ui.Text("title", "数学家"))

enc := a2ui.NewEncoder(os.Stdout)
enc.Encode(env) // → 输出 JSONL 到客户端
```

完整的 A2UI 协议支持：界面、组件、数据模型（JSON Pointer）、验证、界面存储，以及 A2A 和 AG-UI 两种传输绑定。

## HTTP 服务器与会话线程

提供 `thread_id` 时，HTTP 服务器可使对话在多次请求间保持连续。若配置了 `Config.Store`，`/api/chat` 会自动恢复该线程的已保存状态并在运行后保存更新后的状态。若同时配置了 `Config.Checkpoint`，同一 `thread_id` 会用于自动检查点。

```json
{
  "message": "总结一下我们之前讨论的内容",
  "thread_id": "thread-123",
  "stream": false,
  "model": "deepseek-chat",
  "response_format": {
    "type": "json_object"
  },
  "thinking": {
    "display": "summarized",
    "effort": "medium",
    "budget": 2048
  }
}
```

不带 `thread_id` 的调用保持无状态。省略 `model`、`response_format` 或 `thinking` 时，服务器回退到默认的 `agentcore.Config` 值。会话线程的有效优先级为：服务器默认 < 持久化线程配置 < 请求级覆盖。

当 `Config.Store` 由 `session.NewAgentStore(...)` 支持时，`POST /api/chat` 会在客户端未提供 `thread_id` 时自动创建新线程并返回。

`Config.Store` 可使用快照文件或 JSONL 会话：

```go
// 快照存储
snapshots, _ := store.NewSnapshotStore("./states")

// 会话存储
sessions, _ := session.NewFileStore("./sessions")
threadStore := session.NewAgentStore(sessions, "/project")

srv := server.New(agentcore.Config{
    Provider: provider,
    Store:    threadStore,
})
```

当 `Config.Store` 由 `session.NewAgentStore(...)` 支持时，HTTP 服务器还暴露线程相关端点：

- `POST /api/threads` — 创建空线程
- `GET /api/threads` — 列出已持久化的线程
- `GET /api/threads/{key}` — 获取线程元数据和对话记录（含每条消息的 `entry_id`）
- `GET /api/threads/{key}/config` — 获取线程级调用配置
- `PUT /api/threads/{key}/config` — 持久化或清除线程级配置
- `GET /api/threads/{key}/thinking` — 获取线程级推理配置
- `PUT /api/threads/{key}/thinking` — 持久化或清除线程级推理配置
- `POST /api/threads/{key}/branch` — 从当前叶子节点或指定历史节点创建分支
- `DELETE /api/threads/{key}` — 删除线程

## TUI（终端 UI）

全分层终端 UI，Elm 风格架构，差异渲染。

```
tui/
├── core/              基础层：Component 接口、rune 工具、模糊匹配
├── terminal/          终端 I/O、按键解析（Kitty 协议）、termios（macOS/Linux）
├── theme/             ANSI 样式、语义调色板、JSON 热重载
├── component/         UI 组件：Editor、Markdown、Input、SelectList、Loader、Box 等
├── chat/              聊天应用，带可滚动对话记录
├── stdio/             过程式 stdout/stdin 工具（Spinner、Renderer、ProgressBar）
├── agentadapter/      Agentcore → Chat 事件桥接
└── tui.go             TUI 引擎：事件循环、覆盖层系统、差异渲染器
```

核心设计原则：
- **层隔离**：上层导入下层，反之不行
- **Agent 解耦的 Chat**：`tui/chat` 使用自有事件类型和 `Subscriber` 接口——不直接依赖 `agentcore`
- **双渲染模式**：TUI 引擎（差异渲染，Elm 架构）+ stdio 层（过程式 `\r` 覆写）

```go
app := tui.NewChatApp(tui.ChatAppConfig{
    Chat: chatConfig,
    TUI:  tuiConfig,
})
app.Run(ctx)
```

## 扩展

通过单一接口插入工具、钩子、中间件和系统提示。

```go
type MyExtension struct {
    agentcore.BaseExtension
}

func (e *MyExtension) Init(ctx context.Context, agent *agentcore.Agent) error { return nil }

func (e *MyExtension) Tools() []*agentcore.Tool {
    return []*agentcore.Tool{myTool}
}

agent := agentcore.New(agentcore.Config{
    Extensions: []agentcore.Extension{&MyExtension{}},
})
```

## 许可证

[MIT](LICENSE)
