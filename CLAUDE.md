# Mady（中观智能体）

> AI 协作规范见 [AGENTS.md](AGENTS.md) — 跨工具 AI 编码助手标准指令

面向专利代理人、专利律师、知识产权从业者、法律专业人士的智能 Agent 平台。
用 Go 实现的 **中观风格智能体框架** —— 克制、中庸、去繁就简。

## 技术栈

- **Go 1.26**：多模块项目（go.work 包含根模块 + `./tools` + `./tui` + `./desktop` 四个子模块）
- 核心依赖极少（`gorilla/websocket` + `modernc.org/sqlite` + `gopkg.in/yaml.v3`）
- 1500+ 个 Go 源文件（~1001 非测试 + ~530 测试，不含 vendor），~281K 行代码

## 构建与测试

```bash
# 提交前标准（推荐）：lint + build + race 测试，覆盖全部四个模块
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
├── agentcore/        # 核心 Agent 运行时（113 源 + 69 测试，含子目录）
│   ├── concurrency/  #   并发原语
│   ├── evidence/     #   工具调用证据账本（Receipt/Ledger/Claim Binding）
│   ├── filecheckpoint/ # 文件级快照与回退
│   ├── iface/        #   接口抽象层（Agent/Extension/Lifecycle/Provider/Event 契约）
│   ├── permission/   #   细粒度权限门控（Allow/Ask/Deny）
│   ├── planmode/     #   计划模式工具门控
│   ├── tasklist/     #   结构化任务管理（4 工具 + FileStore 持久化）
│   ├── worker/       #   工作协程（目录/注册/监控）
│   ├── skill_extension.go # SKILL.md 技能扩展（加载与执行）
│   ├── reasoning_strategy.go  # 推理策略编排（6 种策略）
│   ├── reasoning_router.go    # 推理策略路由（三档复杂度分类）
│   └── manifests/    #   内置领域 manifest JSON（go:embed）
├── a2a/              # Google Agent-to-Agent 协议
├── a2ui/             # Agent-to-UI 声明式协议 v0.9.1
├── acp/              # Agent 通信协议（JSON-RPC + 认证）
├── agui/             # Agent GUI 事件协议（SSE）
├── disclosure/       # 技术交底书分析管线（13 节点 Pregel，含 review_gate 主动中断）
├── doomloop/         # 死循环检测器（6 种探测器，LifecycleHook 实现）
├── domains/          # 领域 Agent 配置 + 推理引擎 + 专利分析模块
│   ├── claimdrafting/#   权利要求书撰写（LLM 增强撰写 + 6 类规则引擎 + 评分器）
│   ├── config/       #   领域配置（项目注册 registry.json + 文档风格）
│   ├── case_*.go     #   案件管理（生命周期/索引/工作区，domains 根包）
│   ├── evidence/     #   专利证据判断规则引擎（三性/类型/举证责任/证明标准/日期/可信度）
│   ├── doctmpl/      #   文档模板库（Markdown + YAML frontmatter）
│   ├── enablement/   #   26.3 充分公开判断（图引擎 + 领域规则 + 知识库联动）
│   ├── inventiveness/#   创造性判断图引擎（四轮迭代优化 + 审查模拟）
│   ├── novelty/      #   新颖性分析（图引擎 + 评分器）
│   ├── infringement/ #   侵权比对（图引擎 + 规则）
│   ├── ipc/          #   IPC 分类
│   ├── checker/      #   撰写检查器（catalog/dispatch/verdict）
│   ├── provisions/   #   法条条款域 Agent 装配
│   ├── reasoning/    #   事实黑板、三段论、多跳遍历、五步工作法、规划编译器、拓扑驱动泛化
│   │   ├── sqlite/   #     推理持久化
│   │   └── wiring/   #     装配层（vector/skill/rule 三路适配）
│   ├── rules/        #   YAML 规则引擎 + OA 解析 + 反套话引擎
│   ├── specdrafting/ #   说明书撰写（12 节点 Pregel 图 + 规则引擎 + 评分器）
│   ├── sqlite/       #   领域持久化（approval_store / case_index）
│   ├── workflows/    #   领域工作流（legal/patent/design）
│   └── writing/      #   撰写质量评估、模式存储、技能编译器
├── graph/            # 图引擎（DAG + Pregel，含 StateSchema/Reducer、NodePolicy、DegradationMark）
├── guardrails/       # 三级护栏系统（含引用核验 Gate）
│   ├── citation_gate.go  # 引用核验门（双级核验 S1 静态表 + S2 知识源）
│   ├── citation_table.go # S1 静态主题表（专利法 82 条精校）
│   ├── citation_source.go# 知识源抽象 + Composite 复合源
│   └── guardian/     #   AI 安全审查子 Agent（熔断器）
├── knowledge/        # 知识管理（知识图谱 + 文档加载器 + 风险扫描）
│   ├── fileindex/    #   文件索引（MD 文件扫描与缓存）
│   ├── graph/        #   图谱存储/查询/缓存/增量
│   ├── knowledgeinit/#   知识库初始化
│   ├── loader/       #   Wiki/Patent/Legal 加载器 + 法条索引构建
│   ├── risk/         #   风险扫描器（侵权/合规关键词）
│   ├── sqlite/       #   SQLite 只读层（FTS5 全文 + 向量余弦）
│   └── standards/    #   IPC 标准（ipc-standards.yaml）
├── mcp/              # MCP 客户端（stdio + HTTP/SSE）
├── memory/           # 长期记忆系统（三层模型）
│   └── compiler/     #   策略学习型记忆编译器（时间衰减置信度、质量加权、持久化）
├── psychological/    # 心理引擎（VAD 情绪空间模型）
├── provider/         # LLM 接入层
│   ├── adapter/      #   Agent 适配器模式（Claude Code / Codex CLI）
│   ├── chatcompat/   #   OpenAI Chat Completions 兼容
│   └── sanitizer/    #   PII 脱敏 Provider 包装（请求脱敏 + 响应还原）
├── retrieval/        # 检索引擎（关键词/BM25/向量/RRF 混合）
│   ├── domain/       #   检索域基础抽象（DomainRetriever 接口）
│   │   ├── sqlite/   #     SQLite 域存储（本地 FTS5 语料）
│   │   └── browser/  #     ego-browser 在线检索器（Google Patents/CNIPA/Espacenet
│   │                 #     + CompositeRetriever 组合降级）
│   └── model_rerank.go # cross-encoder 重排
├── server/           # HTTP/SSE API 服务器
├── session/          # 会话管理（JSONL 树）
├── skill/            # SKILL.md 解析器（含 MadyExtension 扩展字段）
├── skills/           # 内置技能定义（chat/patent/legal/disclosure/ego-browser）
│   └── ego-browser/  #   浏览器自动化技能（ego lite，macOS only）+ 专利数据库 learnings
│                     #   （patent-google / patent-cnipa / patent-espacenet）
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块，74 源 + 23 测试）
│   ├── desktop/          # 桌面控制（computer_use*.go：macOS/Linux/Windows 三平台 + SOM）
│   ├── browser_*.go      # 浏览器自动化（stealth/session/recorder/supervisor）
│   └── browserproviders/ # 浏览器提供商抽象
├── tracing/          # OpenTelemetry 追踪（分布式 span 注入）
├── tui/              # 终端 UI（分层 Elm 架构，Layer 0-7）
│   ├── core/         #   Layer 0: 基础类型与组件接口（含 Cell 级渲染模型）
│   ├── layout/       #   Layer 0 扩展：布局原语（仅依赖 core）
│   ├── terminal/     #   Layer 1: 终端 I/O（含 keymap.json 配置文件）
│   ├── theme/        #   Layer 2: 主题系统（品牌主题 + 颜色模式）
│   ├── tui.go        #   Layer 3: 引擎层
│   ├── component/    #   Layer 4: UI 组件（含 ToolCard / StatusBar / Markdown 块缓存）
│   ├── chat/         #   Layer 5: 聊天应用（含 AppState 显式状态机）
│   └── agentadapter/ #   Layer 7: Agent 适配器（agentcore → chat 事件转换）
├── bootstrap/        # 启动装配（disclosure/infringement 适配器 + 知识初始化）
├── evaluate/         # 评估框架（RAGAS 风格，含 benchmark 跑批 + CLI 引擎 + 校准）
├── integration/      # 端到端集成测试（含 doomloop/chain/drafting/guardrails/handoff）
├── intent/           # 意图分类（keyword/LLM 多分类器路由，domains.ClassifyIntent 委托使用）
├── cmd/mady/         # 统一入口（14 个子命令：tui / serve / acp / eval / evidence / ocr / mcp-install / trust-mcp / trust-knowledge / util / patent / start-embeddings / stop-embeddings / status-embeddings）
├── example/          # 示例应用（10 个）
├── docs/             # 文档（ADRs、OpenAPI 规范、设计文档、评审报告）
├── fuzzy/            # 模糊匹配
├── prompt/           # 提示词模板加载器 + 内置模板库（prompt/templates/）
├── plugins/          # 专利工作流插件（novelty-analysis / infringement-check / oa-response）
├── desktop/          # Wails 桌面应用（独立子模块：Go 后端 + React 前端）
├── styles/           # 文档风格指南 YAML（patent-standard / legal-standard / chat-friendly / assistant-neutral）
├── doc-templates/    # 文档模板库（claims / specification / oa-response / disclosure / legal）
├── manifests/        # 外部 manifest 示例
├── pkg/
│   ├── agentconfig/  #   统一 Provider/Model 配置层
│   ├── csync/        #   并发同步原语
│   ├── framework/    #   框架装配（DeferredInit 并发原语等）
│   ├── i18n/         #   国际化（zh-CN / en-US，护栏与通用文案翻译）
│   ├── lawcite/      #   法条引用解析与归一化（中文数字+条/款/项/之N）
│   ├── ocr/          #   OCR 识别（ONNX Runtime，多平台）
│   ├── omlx/         #   oMLX 嵌入服务管理（start/stop/status-embeddings）
│   ├── util/         #   路径解析、沙箱配置等通用工具
│   └── vecbytes/     #   向量字节编码
```

## 架构概要

8 层分层架构，上层依赖下层，反之不行：

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
        提供者层   工具层(74源)    扩展层    领域扩展层
                 \      |       /         /
         基础设施层：graph/ session/ skill/ prompt/ store/
                     disclosure/ memory/ fuzzy/ intent/
                     knowledge/ retrieval/ evaluate/ integration/
                                   |
                    TUI 层：分层 Elm 架构（Layer 0-7，含 layout 层）
                                   |
                    应用入口：cmd/mady（14 子命令） server/  desktop/  example/
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
- **Go 编码规范**：所有 Go 代码必须遵守 `docs/GO-DEVELOPMENT-STANDARDS.md`，AI 编码前必读第 0 章硬约束清单

## Handoff 交接机制

统一 Agent（`mady-agent`，由 `UnifiedAgentConfig` 构建）根据用户意图，通过 `HandoffDelegate` 模式将专业任务委派给子 Agent：

```
mady-agent (UnifiedAgentConfig)
  ├── transfer_to_patent     → Patent Agent    (专利分析)
  └── transfer_to_legal      → Legal Agent     (法律分析)
```

**Invisible Handoff（v0.3.0 引入，v0.4.0 简化）：** `UnifiedAgentConfig`（`domains/unified.go`）
是当前唯一入口，融合原 Chat/Assistant/Router 三者为单一 `mady-agent`。统一 Agent 根据用户
意图自动调用 `transfer_to_patent` / `transfer_to_legal`，通过 Invisible Handoff 无缝委派专业
任务——不向用户显示 `transfer_to_*` 工具调用和交接公告。

> 历史说明：v0.3.0 曾用 `IntegratedChatConfig` + `MADY_ROUTER_MODE` / `MADY_SINGLE_AGENT`
> 环境变量切换三种模式。v0.4.0 起这些已被删除，统一为 `UnifiedAgentConfig` 单一路径
> （见 `docs/decisions/ai-changelog/archive-AI_CHANGELOG.md` 2026-07-23「Agent 三合一」章节，第 3814 行起）。

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
| 添加新证据规则 | `domains/evidence/` 下创建规则 → `extension.go` 注册 → 编写测试 |
| 添加文档模板 | `doc-templates/<category>/` 下创建 Markdown 文件（`{{variable}}` 语法） |
| 运行入口程序 | `mady tui`（或 `mady serve`、`mady acp`、`mady eval`） |

## 人机协助开发规范

本项目遵循 [AGENTS.md](AGENTS.md) 定义的人机协助开发规范。AI 参与开发时请注意：

1. **AGENTS.md** — 跨平台 AI 指令标准，非 Claude 的 AI 助手读取此文件
2. **AI_CHANGELOG** — 每次 AI 参与的功能变更须通过脚本追加到 `docs/decisions/ai-changelog/`（先读 `INDEX.json`，再用 `scripts/changelog/main.go` 追加）
3. **敏感路径** — 编辑 `agentcore/handoff.go`、`guardrails/levels.go`、`tools/bash.go` 等
   涉及安全红线的文件后，`scripts/check-sensitive-paths.sh` 和 CI 将自动标记
4. **Spec-Driven** — 新功能按 proposal → spec → design → tasks 四阶段文档进行（详见 `docs/specs/`）
5. **PR 检查** — 在 PR 模板中勾选 AI 参与级别和涉红线变更
6. **Code Review 分级** — 按 L1-L4 四级审查要求（详见 `CONTRIBUTING.md`）

### 敏感路径快速参考

完整且唯一权威的敏感路径清单见 `scripts/check-sensitive-paths.sh` 的 `SENSITIVE_PATHS` 数组。
此处为快速参考（以脚本为准）：

`agentcore/handoff.go` · `guardrails/levels.go` · `domains/router.go` · `domains/patent.go` · `domains/approval.go` · `tools/path.go` · `tools/tools.go` · `agentcore/manifest.go` · `domains/project.go` · `tools/bash.go` · `agentcore/hooks.go` · `disclosure/report.go` · `guardrails/citation_gate.go` · `guardrails/citation_table.go` · `mcp/config_trust.go` · `acp/auth.go` · `server/server.go` · `tools/vision.go` · `agentcore/permission/` · `guardrails/guardian/`
