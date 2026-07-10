# Mady（中观智能体）

面向专利代理人、专利律师、知识产权从业者、法律专业人士的智能 Agent 平台。
用 Go 实现的 **中观风格智能体框架** —— 克制、中庸、去繁就简。

## 技术栈

- **Go 1.25+**：多模块项目（go.work 包含根模块 + `./tools` 子模块）
- 核心依赖极少（仅 `gorilla/websocket` 一个直接依赖）
- 419 个 Go 源文件（283 非测试 + 136 测试），~108K 行代码

## 构建与测试

```bash
# 构建所有包
go build ./...

# 运行所有测试
go test ./...

# 运行 tools 子模块测试
cd tools && go test ./...
```

## 目录结构

```
mady/
├── agentcore/        # 核心 Agent 运行时（50 源 + 27 测试）
├── a2a/              # Google Agent-to-Agent 协议
├── a2ui/             # Agent-to-UI 声明式协议 v0.9.1
├── acp/              # Agent 通信协议（JSON-RPC）
├── agui/             # Agent GUI 事件协议（SSE）
├── domains/          # 领域 Agent 配置 + 推理引擎
│   └── reasoning/    #   事实黑板、三段论、多跳遍历
├── graph/            # 图引擎（DAG + Pregel）
├── guardrails/       # 三级护栏系统
├── knowledge/        # 知识管理（知识图谱 + 文档加载器）
│   ├── graph/        #   图谱存储/查询/缓存/增量
│   └── loader/       #   Wiki/Patent/Legal 加载器
├── mcp/              # MCP 客户端（stdio + HTTP/SSE）
├── psychological/    # 心理引擎（VAD/OCC/EMA/SDT/CBT）
├── provider/         # LLM 接入层
│   ├── chatcompat/   #   OpenAI Chat Completions 兼容
│   └── smartrouter/  #   智能模型路由
├── retrieval/        # 检索引擎（关键词/BM25/向量）
├── server/           # HTTP/SSE API 服务器
├── session/          # 会话管理（JSONL 树）
├── skill/            # SKILL.md 解析器
├── skills/           # 内置技能定义
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块）
├── tui/              # 终端 UI（8 层 Elm 架构）
│   ├── core/         #   Layer 0: Component 接口
│   ├── terminal/     #   Layer 1: 终端 I/O
│   ├── theme/        #   Layer 2: 主题系统
│   ├── tui.go        #   Layer 3: 引擎层
│   ├── component/    #   Layer 4: UI 组件
│   ├── chat/         #   Layer 5: 聊天应用
│   ├── stdio/        #   Layer 6: 过程式 I/O
│   └── agentadapter/ #   Layer 7: Agent 适配器
├── workflow/         # 工作流原语（Pipeline/Parallel/Router）
├── workflows/        # 领域工作流（legal/patent）
├── cmd/mady/         # 统一入口（mady tui | mady acp）
├── example/          # 示例应用（9 个）
├── docs/             # 文档（ADRs、OpenAPI 规范）
├── filequeue/        # 文件队列
├── fuzzy/            # 模糊匹配
├── prompt/           # 提示词模板
├── protocol/         # JSON-RPC 协议原语
├── components/       # 共享组件（已废弃，将迁移）
└── pkg/              # 通用工具
```

## 架构概要

8 层分层架构，上层依赖下层，反之不行：

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
        提供者层   工具层(10+)   扩展层    领域扩展层
                 \      |       /         /
         基础设施层：graph/ session/ skill/ prompt/ store/ mcp/
                                   |
                    TUI 层：8-layer Elm 架构
                                   |
                    应用入口：cmd/mady  server/  example/
```

## 设计约定

- **分层隔离**：严格单向依赖，上层可导入下层，反向禁止
- **Extension 接口**：工具、钩子、中间件均通过 extension 机制注入
- **Lifecycle Hook**：Agent 执行每个阶段可拦截（BeforeToolCall、AfterModelCall 等）
- **EventBus**：类型安全事件总线，支持实时可观测性
- **Conventional Commits**：提交信息格式 `类型: 描述`（feat/fix/docs/test/refactor/chore）
- **中文文档**：文档和注释使用中文，代码和标识符使用英文

## Handoff 交接机制

Router Agent（`mady-router`）通过 `HandoffDelegate` 模式将任务委派给领域 Agent：

```
Router (mady-router)
  ├── transfer_to_chat       → Chat Agent      (日常聊天)
  ├── transfer_to_assistant  → Assistant Agent (任务执行)
  ├── transfer_to_patent     → Patent Agent    (专利分析)
  └── transfer_to_legal      → Legal Agent     (法律分析)
```

**核心组件：**

| 组件 | 文件 | 说明 |
|------|------|------|
| HandoffConfig | `agentcore/handoff.go` | 交接目标配置（名称/模式/来源白名单/兜底文案） |
| HandoffContext | `agentcore/handoff_context.go` | 交接时抽取的结构化上下文（意图/实体/最近消息） |
| HandoffResult | `agentcore/handoff_result.go` | 子 Agent 返回的结构化结果（Action/Result/Success） |
| SafeHandoff | `agentcore/handoff.go` | 基于 AllowedSources 白名单的运行时交接校验 |

**交接流程：**
1. Router 分类用户意图 → 调用 `transfer_to_<domain>` 工具
2. `createHandoffTool` 先校验 `AllowedSources` 白名单
3. `executeDelegate` 构建 `HandoffContext`（含实体抽取），传给子 Agent
4. 子 Agent 处理后返回 `HandoffResult` JSON（或纯文本回退）
5. 失败时返回含 `FallbackMsg` 的 `HandoffResult`，不暴露裸错误

## 测试约定

- 模块级测试：`go test ./<module>/...`
- 集成测试：`go test ./integration/...`
- 竞态检测：`go test -race ./...`
- 覆盖率：`go test -coverprofile=coverage.out ./...`

## 常见开发流程

| 场景 | 步骤 |
|------|------|
| 添加新工具 | `tools/` 下创建文件 → `tools/tools.go` 注册 → 编写测试 |
| 添加新领域 | `domains/` 下创建配置 → 实现 System Prompt → `domains/router.go` 注册 → `skills/` 添加 SKILL.md |
| 添加新技能 | `skills/<domain>/` 下创建 `SKILL.md` → YAML frontmatter → 使用说明 |
| 运行入口程序 | `go run ./cmd/mady/ tui`（或 `acp`） |
