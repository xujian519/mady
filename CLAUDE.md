# Mady（中观智能体）

> AI 协作规范见 [AGENTS.md](AGENTS.md) — 跨工具 AI 编码助手标准指令

面向专利代理人、专利律师、知识产权从业者、法律专业人士的智能 Agent 平台。
用 Go 实现的 **中观风格智能体框架** —— 克制、中庸、去繁就简。

## 技术栈

- **Go 1.26**：多模块项目（go.work 包含根模块 + `./tools` 子模块）
- 核心依赖极少（`gorilla/websocket` + `modernc.org/sqlite` + `gopkg.in/yaml.v3`）
- 841 个 Go 源文件（558 非测试 + 283 测试），~185K 行代码

## 构建与测试

```bash
# 提交前标准（推荐）：lint + build + race 测试，覆盖根模块 + tools 子模块
make verify

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
│   ├── cache/        #   缓存抽象
│   ├── concurrency/  #   并发原语
│   ├── doomloop/     #   死循环检测器（6 种探测器）
│   ├── evidence/     #   工具调用证据账本（Receipt/Ledger）
│   ├── evaluate/     #   评估框架（RAGAS 风格）
│   │   ├── benchmark/#     跑批基准（P2A 31 题 / 无效决定 100 例）
│   │   └── cli/      #     CLI 评估引擎（static/live 模式）
│   ├── filecheckpoint/ # 文件级快照与回退
│   ├── permission/   #   细粒度权限门控（Allow/Ask/Deny）
│   ├── planmode/     #   计划模式工具门控
│   ├── tracing/      #   OpenTelemetry 追踪
│   ├── atom.go       #   Pipeline Atoms（可组合原子操作）
│   ├── plugin.go     #   插件系统（plugin.json + SKILL.md）
│   ├── reasoning_strategy.go  # 推理策略编排（6 种策略）
│   ├── reasoning_router.go    # 推理策略路由（三档复杂度分类）
│   └── manifests/    #   内置领域 manifest JSON（go:embed）
├── a2a/              # Google Agent-to-Agent 协议
├── a2ui/             # Agent-to-UI 声明式协议 v0.9.1
├── acp/              # Agent 通信协议（JSON-RPC + 认证）
├── agui/             # Agent GUI 事件协议（SSE）
├── disclosure/       # 技术交底书分析管线（11 节点 Pregel，含 review_gate 主动中断）
├── domains/          # 领域 Agent 配置 + 推理引擎
│   ├── doctmpl/      #   文档模板库（Markdown + YAML frontmatter）
│   ├── reasoning/    #   事实黑板、三段论、多跳遍历、五步工作法、规划编译器
│   │   ├── collector/#     上下文收集与路由
│   │   ├── sqlite/   #     推理持久化
│   │   └── wiring/   #     装配层（vector/skill/rule 三路适配）
│   ├── rules/        #   YAML 规则引擎 + OA 解析 + 反套话引擎
│   └── sqlite/       #     领域持久化（approval_store）
├── graph/            # 图引擎（DAG + Pregel）
├── guardrails/       # 三级护栏系统（含引用核验 Gate）
│   ├── citation_gate.go  # 引用核验门（双级核验 S1 静态表 + S2 知识源）
│   ├── citation_table.go # S1 静态主题表（专利法 82 条精校）
│   ├── citation_source.go# 知识源抽象 + Composite 复合源
│   └── guardian/     #   AI 安全审查子 Agent（熔断器）
├── knowledge/        # 知识管理（知识图谱 + 文档加载器）
│   ├── fileindex/    #   文件索引（MD 文件扫描与缓存）
│   ├── graph/        #   图谱存储/查询/缓存/增量
│   ├── loader/       #   Wiki/Patent/Legal 加载器 + 法条索引构建
│   └── sqlite/       #   SQLite 只读层（FTS5 全文 + 向量余弦）
├── mcp/              # MCP 客户端（stdio + HTTP/SSE）
├── memory/           # 长期记忆系统（三层模型）
│   └── compiler/     #   策略学习型记忆编译器
├── psychological/    # 心理引擎（VAD/OCC/EMA/SDT/CBT）
├── provider/         # LLM 接入层
│   ├── adapter/      #   Agent 适配器模式（Claude Code / Codex CLI）
│   ├── chatcompat/   #   OpenAI Chat Completions 兼容
│   └── smartrouter/  #   智能模型路由
├── retrieval/        # 检索引擎（关键词/BM25/向量/RRF 混合）
│   ├── domain/       #   检索域基础抽象
│   │   └── sqlite/   #     SQLite 域存储
│   └── model_rerank.go # cross-encoder 重排
├── server/           # HTTP/SSE API 服务器
├── session/          # 会话管理（JSONL 树）
├── skill/            # SKILL.md 解析器（含 MadyExtension 扩展字段）
├── skills/           # 内置技能定义（chat/patent/legal/disclosure）
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块 ~60 源文件）
│   ├── computer_use*.go  # 桌面控制（macOS/Linux/Windows 三平台 + SOM）
│   ├── browser_*.go      # 浏览器自动化（stealth/session/recorder/supervisor）
│   └── browser_providers/# 浏览器提供商抽象（Browserbase/Firecrawl/BrowserUse）
├── tui/              # 终端 UI（8 层 Elm 架构）
│   ├── core/         #   Layer 0: Component 接口
│   ├── terminal/     #   Layer 1: 终端 I/O（含 keymap.json 配置文件）
│   ├── theme/        #   Layer 2: 主题系统（品牌主题 + 颜色模式）
│   ├── tui.go        #   Layer 3: 引擎层
│   ├── component/    #   Layer 4: UI 组件（含 ToolCard / StatusBar / Markdown 块缓存）
│   ├── chat/         #   Layer 5: 聊天应用（含 AppState 显式状态机）
│   ├── stdio/        #   Layer 6: 过程式 I/O
│   ├── agentadapter/ #   Layer 7: Agent 适配器
│   └── layout/       #   Layer 0 扩展：布局原语（仅依赖 core）
├── workflow/         # 工作流原语（Pipeline/Parallel/Router）
├── workflows/        # 领域工作流（legal/patent/autoresearch）
├── benchmark/        # 性能基准测试
├── integration/      # 端到端集成测试（含 doomloop/chain/drafting/guardrails/handoff）
├── cmd/mady/         # 统一入口（mady tui | mady serve | mady acp | mady eval | mady mcp-install | mady trust-mcp）
├── example/          # 示例应用（7 个）
├── docs/             # 文档（ADRs、OpenAPI 规范、设计文档、评审报告）
├── filequeue/        # 文件队列
├── fuzzy/            # 模糊匹配
├── prompt/           # 提示词模板加载器
├── prompt-templates/ # 提示词模板库（16 个 curated 模板）
├── protocol/         # JSON-RPC 协议原语
├── plugins/          # 专利工作流插件（novelty-analysis / infringement-check / oa-response）
├── styles/           # 文档风格指南 YAML（patent-standard / legal-standard / chat-friendly / assistant-neutral）
├── doc-templates/    # 文档模板库（claims / specification / oa-response / disclosure）
├── manifests/        # 外部 manifest 示例
├── pkg/
│   ├── agentconfig/  #   统一 Provider/Model 配置层
│   ├── lawcite/      #   法条引用解析与归一化（中文数字+条/款/项/之N）
│   ├── csync/        #   并发同步原语
│   └── util/         #   路径解析等通用工具
```

## 架构概要

8 层分层架构，上层依赖下层，反之不行：

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
        提供者层   工具层(60源)    扩展层    领域扩展层
                 \      |       /         /
         基础设施层：graph/ session/ skill/ prompt/ store/
                     disclosure/ memory/ filequeue/ fuzzy/
                     knowledge/ retrieval/ benchmark/ integration/
                     filequeue/
                                   |
                    TUI 层：8-layer Elm 架构（含 layout 层）
                                   |
                    应用入口：cmd/mady（6 子命令） server/  example/
```

## 设计约定

- **分层隔离**：严格单向依赖，上层可导入下层，反向禁止
- **Extension 接口**：工具、钩子、中间件均通过 extension 机制注入
- **Lifecycle Hook**：Agent 执行每个阶段可拦截（BeforeToolCall、AfterModelCall 等），DoomLoop / ReasoningStrategyRouter / CitationGate / Guardrails 均为 LifecycleHook 实现
- **EventBus**：类型安全事件总线，支持实时可观测性
- **Conventional Commits**：提交信息格式 `类型: 描述`（feat/fix/docs/test/refactor/chore）
- **中文文档**：文档和注释使用中文，代码和标识符使用英文
- **措辞规范**：面向用户的文案（护栏/免责声明/错误提示/报告结论）遵循 `docs/tone-style-guide.md`：
  - 不使用绝对化表述（绝对/一定/百分百→通常/大概率）
  - 结论性表述附带置信度标注
  - 拒绝类文案提供替代性帮助而不是单纯说"不行"
  - 日常对话中不提及中观/佛教出处
- **DocumentStyle**：`domains/style.go` 以 YAML 定义领域风格指南（tone/voice/anti_patterns/citation/disclaimers），`styles/` 目录 4 套默认风格
- **Spec-Driven**：新功能按 proposal → spec → design → tasks 四阶段文档进行（详见 `docs/specs/`）

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
| 添加新技能 | `skills/<domain>/` 下创建 `SKILL.md` → YAML frontmatter → `mady:` 扩展段 |
| 注册新插件 | `plugins/<name>/` 下创建 `plugin.json` + `SKILL.md` |
| 添加文档模板 | `doc-templates/<category>/` 下创建 Markdown 文件（`{{variable}}` 语法） |
| 运行入口程序 | `mady tui`（或 `mady serve`、`mady acp`、`mady eval`） |

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

完整且唯一权威的敏感路径清单见 `scripts/check-sensitive-paths.sh` 的 `SENSITIVE_PATHS` 数组。
此处为快速参考（以脚本为准）：

`agentcore/handoff.go` · `guardrails/levels.go` · `domains/router.go` · `domains/patent.go` · `domains/approval.go` · `tools/path.go` · `tools/tools.go` · `agentcore/manifest.go` · `domains/project.go` · `tools/bash.go` · `agentcore/hooks.go` · `disclosure/report.go` · `guardrails/citation_gate.go` · `guardrails/citation_table.go` · `mcp/config_trust.go` · `acp/auth.go` · `server/server.go` · `tools/vision.go` · `agentcore/permission/` · `guardrails/guardian/`
