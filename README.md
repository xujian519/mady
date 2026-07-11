# Mady（中观智能体）

[![CI](https://github.com/xujian519/mady/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/xujian519/mady/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/xujian519/mady.svg)](https://pkg.go.dev/github.com/xujian519/mady)

> 离二边，行中道 —— 恰到好处的抽象，克制的工程实践。
>
> 用 Go 实现的**中观风格智能体框架**，面向专利代理人、律师、知识产权从业者等专业人士。

措辞规范见 [docs/tone-style-guide.md](docs/tone-style-guide.md)。

## 愿景

Mady 的目标是成为专业人士的智能 Agent 平台，聚焦专业判断中"人机协作"的边界。

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

**重点节点必须进行人机协作。** 关键决策点、法律判断、风险评估不可由 AI 独立完成，必须引入人类专业人士的确认与干预。所有结论性输出附带置信度标注和免责声明，低置信度内容与高置信度内容区别呈现。

### 发展路线

- **当前**：技术交底书分析（Disclosure Pregel 管线）、专利新颖性分析、法律案例比较
- **下季度**：向量召回上线、Router LLM 分类升级、心理引擎认知扭曲检测

## 安装

### 从源码构建并安装

```bash
git clone https://github.com/xujian519/mady.git
cd mady

# 构建
go build ./...

# 一键安装到 ~/.local/bin（使其在任意目录可用，需该目录在 PATH 上）
make install

# 也可手动指定安装位置
make install PREFIX=/usr/local

# 之后可在任意目录运行
mady tui      # TUI 交互模式（多域路由，默认）
mady serve    # HTTP/SSE API 服务器
mady acp      # ACP 协议服务器（编辑器集成）
```

> mady 的 4 个内置领域 manifest（chat/assistant/patent/legal）已通过 `go:embed`
> 编进二进制，**无需额外资源文件**即可在任意目录启动。如需自定义/新增领域，
> 将 JSON 放入 `~/.mady/manifests/` 即可覆盖或扩展（无需重编译）。

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PROVIDER` | deepseek | LLM 提供商（deepseek/zhipu/kimi/generic） |
| `API_KEY` | — | LLM API 密钥 |
| `MADY_HOME` | `~/.mady` | 应用数据根目录（workspace/sessions/manifests 落在此处） |
| `MANIFEST_DIR` | `$MADY_HOME/manifests` | 外部 manifest 覆盖目录（内置 4 个始终可用） |
| `WORKSPACE_DIR` | `$MADY_HOME/workspace` | Workspace 根目录 |
| `SESSION_DIR` | `$MADY_HOME/sessions` | 会话持久化目录（仅 serve） |
| `MADY_SINGLE_AGENT` | 未设置 | 设为 `1` 强制 TUI 单 Agent 模式 |
| `WIKI_PATH` | — | Wiki 知识库路径 |

### 作为库使用

```bash
go get github.com/xujian519/mady/agentcore
```

需要 **Go 1.25+**。

> 贡献指南见 [CONTRIBUTING.md](CONTRIBUTING.md) | 变更日志见 [CHANGELOG.md](CHANGELOG.md) | API 规范见 [docs/openapi/](docs/openapi/)

## 架构概要

```
           ┌──────────────────────┐
           │  A2A │ A2UI │ Server │
           │  AGUI │ MCP │ ACP   │
           └────────┬─────────────┘
                    │
           ┌────────▼─────────────┐
           │     agentcore        │  ← 核心 Agent 运行时
           └──┬────┬──────┬───────┘
              │    │      │
    ┌─────────▼┐ ┌─▼──┐ ┌─▼──────────┐
    │ Provider │ │Tool│ │ Domain     │
    │ chatcompt│ │sys │ │ chat/assist│
    │smartroute│ │MCP │ │ patent/leg │
    └──────────┘ └────┘ └──┬─────────┘
                            │
    ┌───────────────────────▼──────────┐
    │ infrastructure                   │
    │ graph/ session/ skill/ store/    │
    │ workflow/ guardrails/ retrieval/ │
    │ knowledge/ psychological/        │
    │ disclosure/                      │
    └──────────────────────────────────┘
```

## 核心概念

### Agent 注册（Manifest）

通过 JSON 文件声明式注册 Agent，无需修改 Go 代码。

```json
{
  "name": "patent-agent",
  "domain": "patent",
  "description": "专利分析与知识产权检索",
  "guardrail_level": "strict",
  "handoff_targets": ["chat-agent", "assistant-agent"],
  "knowledge_domain": "patent"
}
```

Manifest 文件放置在 `manifests/` 目录，启动时自动加载。见 [docs/manifest-guide.md](docs/manifest-guide.md)。

### 领域路由（Router）

Router Agent（`mady-router`）通过 `HandoffDelegate` 模式将任务委派给领域 Agent：

```
mady-router
  ├── transfer_to_chat-agent      日常聊天
  ├── transfer_to_assistant-agent 工具密集型任务
  ├── transfer_to_patent-agent    专利分析
  ├── transfer_to_legal-advisor   法律查询
  └── transfer_to_project-{id}    案件专属 Agent（动态注册）
```

### 案件管理（ProjectRegistry）

每个案件在 `workspace/projects/` 下有独立元数据（`meta.json`），
通过 `BuildProjectAgent(rec, base)` 动态创建 WorkingDir 隔离的 Agent 实例。
AgentPool 管理案件 Agent 生命周期，空闲 30 分钟自动释放。

### 内置工作流

| 工作流 | 触发方式 | 说明 |
|--------|----------|------|
| **技术交底书分析** | `POST /v1/disclosure/analyze` 或 Patent Agent 工具 | 10 节点 Pregel 管线：预处理→三提取并行→合并→一致性校验(可回退)→关键词生成→新颖性初判→报告→人工复核 |
| **专利新颖性分析** | `analyze_patent_novelty` 工具 | parse→search→analyze→rule_check→conclude，含 6 条确定性规则引擎 |
| **法律案例比较** | `compare_legal_cases` 工具 | statute→case_search→compare→conclude，含 FactBlackboard + Syllogism 三段论推理 |

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

### MCP 工具

`mady` 可将外部 MCP 服务器桥接为 `agentcore.Tool`。支持 MCP `stdio` 传输和 HTTP/SSE 传输。

```go
import "github.com/xujian519/mady/mcp"

ctx := context.Background()
ext, err := mcp.NewStdioExtension(ctx, mcp.StdioConfig{
    Name:       "filesystem",
    Command:    "npx",
    Args:       []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
    ToolPrefix: "fs.",
})
```

### 多 Agent 交接

在专业 Agent 之间委派或转移任务——支持本地和远程（通过 A2A 协议）。

```go
cfg := agentcore.Config{
    Handoffs: []agentcore.HandoffConfig{
        {Name: "math-expert", Mode: agentcore.HandoffDelegate, AgentConfig: mathCfg},
        {Name: "code-expert", Mode: agentcore.HandoffTransfer, AgentConfig: codeCfg},
    },
}
```

交接时自动抽取结构化上下文（`HandoffContext`），含 UserIntent LLM 摘要（5 分钟缓存）、正则实体抽取（专利号/申请号/案件编号）和最近 N 条消息。

### 事件系统

类型安全的事件总线，用于实时可观测性。

```go
agent.On(agentcore.EventMessageDelta, func(e agentcore.Event) {
    delta := e.(*agentcore.MessageDeltaEvent)
    fmt.Print(delta.Content)
})
```

### 生命周期钩子

可拦截 Agent 执行的每个阶段。

```go
agent := agentcore.New(agentcore.Config{
    Lifecycle: agentcore.LifecycleChain{
        guardrail, auditHook, rateLimiter,
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
mgr.AppendMessage(ctx, agentcore.Message{Role: "user", Content: "你好"})
msgs := mgr.MessagesOnPath()
tree := mgr.GetTree()
stats := mgr.Stats()
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
```

### Provider（模型接入）

统一使用 OpenAI Chat Completions 兼容协议：

```go
// DeepSeek（推荐）
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
    BaseURL: "https://api.deepseek.com/v1",
})

// 智谱 GLM 编程套餐
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("ZHIPU_API_KEY"),
    BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
})

// 通义千问
p := chatcompat.New(chatcompat.Config{
    APIKey:  os.Getenv("QWEN_API_KEY"),
    BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
})
```

智能路由（`provider/smartrouter`）根据任务类型自动选择最优模型：

```go
router := smartrouter.New(smartrouter.Config{
    Priority: smartrouter.PriorityQuality,
    Profiles: []smartrouter.ModelProfile{
        {Model: "deepseek-chat", Strengths: []string{"coding", "reasoning"}},
    },
})
p := smartrouter.NewProvider(chatcompat.New(chatcompat.Config{...}), router)
```

### 结构化输出

```go
agent := agentcore.New(agentcore.Config{
    ResponseFormat: agentcore.NewJSONSchemaResponseFormat("answer", map[string]any{
        "type": "object",
        "properties": map[string]any{"answer": map[string]any{"type": "string"}},
        "required": []string{"answer"},
    }),
})
```

### 推理过程控制

```go
agent := agentcore.New(agentcore.Config{
    Model:    "deepseek-reasoner",
    Thinking: &agentcore.ThinkingConfig{
        IncludeThoughts: true,
        Display:         agentcore.ThinkingDisplaySummarized,
    },
})
```

## HTTP/SSE API 服务器

### 聊天端点

```json
POST /api/chat
{
  "message": "分析这个专利",
  "thread_id": "thread-123",   // 可选，保持对话连续
  "stream": false,
  "model": "deepseek-chat"
}
```

### 会话线程端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/threads` | 创建线程 |
| GET | `/api/threads` | 列出线程 |
| GET | `/api/threads/{key}` | 获取线程 |
| GET/PUT | `/api/threads/{key}/config` | 线程配置 |
| GET/PUT | `/api/threads/{key}/thinking` | 推理配置 |
| POST | `/api/threads/{key}/branch` | 分支 |
| DELETE | `/api/threads/{key}` | 删除线程 |

### 技能端点

GET `/api/skills` — 列出技能，POST `/api/skills/reload` — 热加载。

### Disclosure 分析端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/v1/disclosure/analyze` | 提交交底书分析，返回 `task_id` |
| GET | `/v1/disclosure/analyze/{task_id}` | 轮询分析结果 |
| GET | `/v1/disclosure/analyze/{task_id}/stream` | SSE 实时进度 |

### 状态快照端点

GET/DELETE `/api/states/{key}`。

## 心理引擎

`psychological/` 是一个基于心理学的对话分析引擎，通过 7 阶段管道分析用户情绪和认知状态。

| 阶段 | 模型 | 功能 |
|------|------|------|
| 1 | **VAD** | 三维情绪空间（Valence/Arousal/Dominance） |
| 2 | **OCC** | 14 种情绪分类评价公式 |
| 3 | **EMA** | 四维认知评价 + 应对模式检测 |
| 4 | **Beck CBT** | 13 种认知扭曲检测 |
| 5 | **SDT** | 自我决定理论跨轮次需求追踪 |
| 6 | **对话策略匹配** | 9 种策略 |
| 7 | **管道编排** | 7 阶段顺序执行 + 短路优化 |

纯 Go 标准库实现，零外部依赖。

## 三级护栏系统

| 级别 | 内容屏蔽 | 免责声明 | 审批门 |
|------|----------|----------|--------|
| **Light** | 通用风险关键词 | — | — |
| **Standard** | 专业风险关键词 | 领域免责声明 | — |
| **Strict** | 法律/专利关键词 | 法律免责声明 | 敏感结论需审批 |

措辞遵循 [docs/tone-style-guide.md](docs/tone-style-guide.md)：不使用绝对化表述（"通常"而非"绝对"），结论性输出附带置信度，拒绝类文案提供替代性帮助。

## 知识管理与检索

`knowledge/` 管理多种来源的领域知识，支持 Wiki/Obsidian、专利和法律文档。知识图谱 (`knowledge/graph/`) 支持实体关系构建和查询。检索引擎 (`retrieval/`) 支持关键词搜索（TF-IDF）、向量嵌入接口和混合检索。

## 推理引擎

`domains/reasoning/` 提供四种法律/专利领域结构化推理原语：

- **FactBlackboard** — 共享事实内存
- **Syllogism** — 三段论引擎（大前提 → 小前提 → 结论）
- **ReasoningWalker** — 知识图谱多跳遍历
- **RuleAssertion** — 规则断言校验器

## A2A / ACP / AGUI / A2UI 协议

- **A2A** — Google Agent-to-Agent 协议：Agent Card 发现、任务生命周期、SSE 流式、WebSocket
- **ACP** — JSON-RPC 的 Agent 间通信协议
- **AGUI** — SSE 事件协议，将 Agent 执行过程流式传输到 Web UI
- **A2UI** — 声明式 UI 协议，Agent 可在客户端渲染界面

## 扩展

通过单一接口插入工具、钩子、中间件和系统提示。

```go
type MyExtension struct { agentcore.BaseExtension }
func (e *MyExtension) Tools() []*agentcore.Tool { return []*agentcore.Tool{myTool} }
agent := agentcore.New(agentcore.Config{Extensions: []agentcore.Extension{&MyExtension{}}})
```

内置扩展：

| 扩展 | 模块 | 说明 |
|------|------|------|
| 工具集 | `tools/` | 40+ 内置工具（文件/Shell/搜索/浏览器/代码执行/Git） |
| MCP 桥接 | `mcp/` | 将外部 MCP 服务器桥接为 Tool |
| 心理引擎 | `psychological/` | 7 阶段心理分析管道 |
| A2A 远程 Handoff | `a2a/` | 将远程 A2A Agent 注册为 Handoff 目标 |

## 许可证

[MIT](LICENSE)
