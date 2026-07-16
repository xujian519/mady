# Mady（中观智能体）

> AI 协作规范见 [AGENTS.md](AGENTS.md) — 跨工具 AI 编码助手标准指令

面向专利代理人、专利律师、知识产权从业者、法律专业人士的智能 Agent 平台。
用 Go 实现的 **中观风格智能体框架** —— 克制、中庸、去繁就简。

## 技术栈

- **Go 1.25**：多模块项目（go.work 包含根模块 + `./tools` 子模块）
- 核心依赖极少（`gorilla/websocket` + `modernc.org/sqlite` + `gopkg.in/yaml.v3`）
- 737 个 Go 源文件（490 非测试 + 247 测试），~134K 行代码

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
├── agentcore/        # 核心 Agent 运行时（75 源 + 40 测试）
│   ├── evidence/     #   工具调用证据账本（Receipt/Ledger）
│   ├── filecheckpoint/ # 文件级快照与回退
│   ├── permission/   #   细粒度权限门控（Allow/Ask/Deny）
│   ├── planmode/     #   计划模式工具门控
│   ├── evaluate/     #   评估框架（RAGAS 风格）
│   ├── tracing/      #   OpenTelemetry 追踪
│   └── manifests/    #   内置领域 manifest JSON（go:embed）
├── a2a/              # Google Agent-to-Agent 协议
├── a2ui/             # Agent-to-UI 声明式协议 v0.9.1
├── acp/              # Agent 通信协议（JSON-RPC）
├── agui/             # Agent GUI 事件协议（SSE）
├── disclosure/       # 技术交底书分析管线（10 节点 Pregel）
├── domains/          # 领域 Agent 配置 + 推理引擎
│   ├── reasoning/    #   事实黑板、三段论、多跳遍历
│   └── rules/        #   YAML 规则引擎 + OA 解析 + 反套话引擎
├── graph/            # 图引擎（DAG + Pregel）
├── guardrails/       # 三级护栏系统
│   └── guardian/     #   AI 安全审查子 Agent（熔断器）
├── knowledge/        # 知识管理（知识图谱 + 文档加载器）
│   ├── graph/        #   图谱存储/查询/缓存/增量
│   ├── loader/       #   Wiki/Patent/Legal 加载器
│   └── sqlite/       #   SQLite 只读层（FTS5 全文 + 向量余弦）
├── mcp/              # MCP 客户端（stdio + HTTP/SSE）
├── memory/           # 长期记忆系统（三层模型）
│   └── compiler/     #   策略学习型记忆编译器
├── psychological/    # 心理引擎（VAD/OCC/EMA/SDT/CBT）
├── provider/         # LLM 接入层
│   ├── chatcompat/   #   OpenAI Chat Completions 兼容
│   └── smartrouter/  #   智能模型路由
├── retrieval/        # 检索引擎（关键词/BM25/向量/RRF 混合）
│   └── domain/       #   检索域基础抽象
├── server/           # HTTP/SSE API 服务器
├── session/          # 会话管理（JSONL 树）
├── skill/            # SKILL.md 解析器
├── skills/           # 内置技能定义
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块，35 工具）
│   └── browser_providers/ # 浏览器提供商抽象（Browserbase/Firecrawl/BrowserUse）
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
├── benchmark/        # 性能基准测试
├── integration/      # 端到端集成测试（5 条核心链路）
├── cmd/mady/         # 统一入口（mady tui | mady serve | mady acp）
├── example/          # 示例应用（7 个）
├── docs/             # 文档（ADRs、OpenAPI 规范）
├── filequeue/        # 文件队列
├── fuzzy/            # 模糊匹配
├── prompt/           # 提示词模板
├── protocol/         # JSON-RPC 协议原语
└── pkg/
    ├── agentconfig/  #   统一 Provider/Model 配置层
    └── util/         #   路径解析等通用工具
```

## 架构概要

8 层分层架构，上层依赖下层，反之不行：

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
        提供者层   工具层(35)    扩展层    领域扩展层
                 \      |       /         /
         基础设施层：graph/ session/ skill/ prompt/ store/
                     disclosure/ memory/ filequeue/ fuzzy/
                     knowledge/ retrieval/ benchmark/ integration/
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
- **措辞规范**：面向用户的文案（护栏/免责声明/错误提示/报告结论）遵循 `docs/tone-style-guide.md`：
  - 不使用绝对化表述（绝对/一定/百分百→通常/大概率）
  - 结论性表述附带置信度标注
  - 拒绝类文案提供替代性帮助而不是单纯说"不行"
  - 日常对话中不提及中观/佛教出处

## Handoff 交接机制

Router Agent（`mady-router`）通过 `HandoffDelegate` 模式将任务委派给领域 Agent：

```
Router (mady-router)
  ├── transfer_to_chat       → Chat Agent      (日常聊天)
  ├── transfer_to_assistant  → Assistant Agent (任务执行)
  ├── transfer_to_patent     → Patent Agent    (专利分析)
  └── transfer_to_legal      → Legal Agent     (法律分析)
```

**Invisible Handoff（v0.3.0 新增）：** `IntegratedChatConfig` 将 Chat Agent 作为统一对话界面，
内部通过 Invisible Handoff 无缝委派专业任务——不向用户显示 `transfer_to_*` 工具调用和交接公告。
入口 `cmd/mady/main.go` 默认使用集成模式（`MADY_ROUTER_MODE=1` 回退传统 Router，`MADY_SINGLE_AGENT=1` 回退单 Agent）。

**核心组件：**

| 组件 | 文件 | 说明 |
|------|------|------|
| HandoffConfig | `agentcore/handoff.go` | 交接目标配置（名称/模式/来源白名单/兜底文案/Invisible 标志） |
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

## 人机协助开发规范

本项目遵循 [AGENTS.md](AGENTS.md) 定义的人机协助开发规范。AI 参与开发时请注意：

1. **AGENTS.md** — 跨平台 AI 指令标准，非 Claude 的 AI 助手读取此文件
2. **AI_CHANGELOG.md** — 每次 AI 参与的功能变更须在 `docs/decisions/AI_CHANGELOG.md` 记录决策
3. **敏感路径** — 编辑 `agentcore/handoff.go`、`guardrails/levels.go`、`tools/bash.go` 等
   涉及安全红线的文件后，`scripts/check-sensitive-paths.sh` 和 CI 将自动标记
4. **Spec-Driven** — 新功能按 proposal → spec → design → tasks 四阶段文档进行（详见 `docs/specs/`）
5. **PR 检查** — 在 PR 模板中勾选 AI 参与级别和涉红线变更
6. **Code Review 分级** — 按 L1-L4 四级审查要求（详见 `CONTRIBUTING.md`）

### 敏感路径快速参考

| 路径 | 安全边界 |
|------|---------|
| `agentcore/handoff.go` | 交接白名单校验 (isHandoffAllowed) |
| `guardrails/levels.go` | 护栏等级枚举 (Light/Standard/Strict) |
| `domains/router.go` | 路由白名单 AllowedSources |
| `domains/patent.go` | BuildProjectAgent 动态 WorkingDir |
| `domains/approval.go` | ApprovalGate 生命周期钩子 |
| `tools/path.go` | 文件系统沙箱隔离 |
| `tools/tools.go` | 工具能力门控 (ExtensionConfig) |
| `agentcore/manifest.go` | Manifest 校验规则 |
| `domains/project.go` | ValidateProjectPath 路径校验 |
| `tools/bash.go` | Bash 工具 (非沙箱模式) |
