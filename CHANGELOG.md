# Changelog

本文件遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 格式，
版本号遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。

## [0.3.0] - 2026-07-11 — 内部预览版

### Added

- **Manifest 注册表**：JSON 格式的 Agent 声明式注册，替代纯硬编码 RouterConfig
- **UserIntent v2**：LLM 驱动的意图摘要（Provider 不可用时回退 v1），5 分钟缓存
- **AgentPool**：案件专属 Agent 生命周期管理，惰性创建 / 30 分钟空闲超时 / 并发安全
- **ProjectRegistry 入口集成**：`runServer` / `runTui` 自动初始化 ProjectRegistry
- **RouterConfigWithRegistry**：动态注册 `project-{projectID}` Handoff 目标
- **Disclosure API**：`POST /v1/disclosure/analyze` 异步交底书分析 + SSE 实时进度
- **analyze_disclosure 工具**：Patent Agent 可直接触发的 10 节点 Pregel 分析流水线
- **analyze_patent_novelty 工具**：封装专利新颖性分析 Pregel 图（含规则引擎）
- **compare_legal_cases 工具**：封装法律案例比较 Pregel 图（含三段论推理）
- **错误类型体系**：RetryableError / FatalError / HandoffError / GuardrailError
- **措辞规范**：`docs/tone-style-guide.md`，禁用绝对化表述，约束向用户输出
- **e2e 测试套件**：5 条核心链路 + 4 个护栏场景 + Session 连续性
- **性能基准**：Agent 创建 / Pregel 编译执行 / Handoff 序列化延迟
- **Embed Manifest**：4 个领域 JSON（chat/assistant/patent/legal）通过 `go:embed` 编进二进制，任意目录开箱即用
- **MADY_HOME**：`util.MadyHome()` 统一路径解析（`$MADY_HOME` > `~/.mady`），workspace/sessions/manifests 均落在其下
- **Invisible Handoff**：Chat Agent 作为统一对话界面（IntegratedChatConfig），根据意图自动无缝委派给专业 Agent
- **Reasonix 扩展包（9 个 opt-in）**：
  - `agentcore/evidence/` — 工具调用证据账本（Receipt/Ledger）
  - `agentcore/filecheckpoint/` — 文件级快照与回退
  - `agentcore/permission/` — 细粒度权限门控（Allow/Ask/Deny）
  - `agentcore/planmode/` — 计划模式工具门控（/plan）
  - `agentcore/evaluate/` — 评估框架（RAGAS 风格）
  - `agentcore/tracing/` — OpenTelemetry 追踪
  - `guardrails/guardian/` — AI 安全审查子 Agent（熔断器）
  - `memory/` + `memory/compiler/` — 长期记忆系统 + 策略学习型记忆编译器
  - 四级渐进式上下文压缩（notice → snip → prune → force-fold）
- **环境变量**：新增 `MADY_HOME`、`MADY_ROUTER_MODE`
- **SQLite 知识库读取层**（`knowledge/sqlite/`）：纯 Go 无 CGO（`modernc.org/sqlite`），只读接入 knowledge.db（FTS5 trigram + BM25）/ laws-full.db（法律全文搜索）/ patent_kg.db（专利图谱批量加载）
- **RRF 混合检索**（`retrieval/hybrid.go`）：Reciprocal Rank Fusion 算法（k=60），融合 FTS 和向量搜索结果，score-agnostic 只看排名位置
- **YAML 规则引擎**（`domains/rules/`）：4 种 YAML 格式（规则文件/法条框架/事务编排/反思指示词），RulesExtension 暴露 search_rules/get_article_framework/get_orchestration 工具
- **OA 解析器**（`domains/rules/oa_parser.go`）：纯规则零 LLM，7组拒兔类型检测 + 多国专利号提取 + 权利要求范围展开
- **反 AI 套话引擎**（`domains/rules/slop_engine.go`）：三层架构（42条短语替换 + 6种结构缺陷 + 50分制5维评分 + 8项快检）
- **法律意图检测**（`domains/legal_intent.go`）：`@legal` 显式触发 + 15组关键词→CaseType 映射 + 专利语境门控
- **五步工作法 + Multi-Hypothesis Judge**：发现事实 → 获取规则 → 规划 → 执行 → 检查 的完整推理框架
- **pkg/agentconfig**：统一 Provider/Model/Thinking 配置层，所有入口共享同一环境变量约定
- **tools/browser_providers**：浏览器自动化提供商抽象（Browserbase/Firecrawl/BrowserUse）

### Changed

- `ExtractHandoffContext` 增加 LLM 摘要调用（Provider 不可用时回落 v1）
- `runTui` 支持多域路由模式（`MADY_SINGLE_AGENT=1` 回退单 Agent）
- `RouterConfigFromManifests` 支持动态 SystemPrompt 生成
- 护栏文案：`必须附带` → `附以下声明`（措辞更轻柔）
- 专利分析输出：`权利要求应确保` → `建议由代理人核实`（避免绝对化）

## [0.2.0] - 2026-07-11

### Added

- **心理引擎 (psychological/)**: 7-stage pipeline with VAD emotional space, OCC emotional model, EMA cognitive appraisal, Beck cognitive distortion detection, SDT self-determination theory tracking. LLM-based distortion verification mode. Persistent SDT state per session.
- **智能路由 Provider (provider/smartrouter/)**: Task-type classification (coding/reasoning/legal/patent/creative/analysis/general), priority-based profile ranking (quality/cost/balanced/latency), ModelProfile registry.
- **知识管理 (knowledge/)**: Wiki/Patent/Legal document loaders, multi-source loader pipeline, KnowledgeStore with domain-organized collections, retrieval hook integration with guardrails.
- **知识图谱 (knowledge/graph/)**: Graph store with adapter, builder, query engine, incremental updates, cache layer, retrieval enhancer integration.
- **推理引擎 (domains/reasoning/)**: FactBlackboard shared memory, categorical Syllogism engine (major premise → minor premise → conclusion), multi-hop ReasoningWalker, RuleAssertion validator with reference tracing.
- **三级护栏系统 (guardrails/)**: Light/Standard/Strict guardrail levels, domain-specific disclaimers, keyword-based content safety checks, approval gating for critical conclusions.
- **检索引擎 (retrieval/)**: Paragraph/section chunker, keyword searcher with TF-IDF scoring, BM25-inspired reranker with position bias, vector embedding support, domain-specific retrieval base.
- **领域工作流 (workflows/)**: Legal and patent domain workflow steps with human-approval gates.
- **MCP 客户端 (mcp/)**: stdio transport, HTTP/SSE transport, tool hot-refresh, capability discovery.
- **HTTP/SSE 服务器 (server/)**: `/api/chat` endpoint, thread CRUD, skill management, state snapshots, AG-UI event streaming.
- **示例应用**: cli-chat, a2a-server, a2a-client, acp-server, tui-demo/2/3, wiki-import, provider-compat (9 examples)
- **统一入口 (cmd/mady/)**: `mady tui` and `mady acp` subcommands

### Changed

- **Provider 层**: Simplified to `provider/chatcompat/` (OpenAI Chat Completions compatible) and `provider/smartrouter/` (intelligent routing). Removed standalone per-provider packages.
- **TUI**: 8-layer Elm architecture (up from 7), with explicit layer numbering (0-7) documented in LAYERS.md.

### Removed

- **Runnable interface**: Removed from agentcore; replaced by direct agent execution model.

## [0.1.0] - 2026-07-10

### Added

- **核心 Agent 运行时**：LLM-工具循环、流式响应、多 Agent 交接（委托/转移/转发）、自动上下文压缩、指数退避重试、检查点机制
- **事件系统**：类型安全的事件总线，支持实时可观测性（消息增量、工具调用、状态变更等）
- **生命周期钩子**：Guardrail 钩子、审计钩子、限流钩子，可拦截 Agent 执行的每个阶段
- **工具系统**：JSON Schema 校验、单工具钩子、全局中间件
- **内置工具扩展**（tools/）：文件系统（read/edit/write/delete/move/patch）、Shell（bash/process）、搜索（ls/grep/find/glob）、浏览器自动化（多提供商）、Web 搜索、代码执行、Git 操作、MCP 桥接、macOS 桌面控制
- **三层 Provider**：统一 OpenAI Chat Completions 兼容协议，支持 DeepSeek / 智谱 GLM / Kimi / 通义千问 / 通用兼容 API
- **7 层 TUI 架构**：Elm 风格消息传递、差异渲染、Kitty/Sixel 图像支持、模糊搜索自动补全、Markdown 渲染、主题系统
- **HTTP/SSE 服务器**：`/api/chat` 端点、线程 CRUD、技能管理、状态快照、AG-UI 事件流
- **A2A 协议**：完整的服务器/客户端实现，符合 Google Agent2Agent 规范，支持 Agent Card 发现、任务生命周期、WebSocket 传输
- **A2UI 协议**：声明式 UI 流式传输 v0.9.1，数据绑定、验证、A2A/AG-UI 传输绑定
- **ACP 协议**：基于 JSON-RPC 的 Agent 间通信，会话级 Agent 生命周期管理
- **AG-UI 协议**：SSE 事件流，运行生命周期/步骤进度/文本增量/推理块/工具调用/状态快照
- **MCP 客户端**：stdio 和 HTTP/SSE 传输，工具热刷新
- **领域路由**：Router Agent + Chat/Patent/Legal 子 Agent，基于关键词的意图分类器
- **护栏系统**：三级（Light/Standard/Strict）、领域免责声明、审批门控
- **心理引擎**：VAD 三维情绪空间、OCC 情绪模型、EMA 认知评价、Beck 认知扭曲、SDT 自我决定理论
- **知识管理**：Wiki/Patent/Legal 文档加载器、分块器、关键词搜索、BM25 重排序
- **图引擎**：静态 DAG 并行执行 + 循环 Pregel 图超步迭代
- **工作流原语**：Pipeline、Parallel、Router、Map 步骤
- **会话管理**：JSONL 追加写入树结构，支持分支、压缩、标签和版本迁移
- **SKILL 系统**：Markdown 格式的 AI 技能定义，YAML 前置元数据，热重载
- **示例应用**：cli-chat（TUI 聊天）、tui-demo、a2a-client/server、wiki-import、provider-compat
- **GitHub Actions CI**：go vet + build + test

[0.3.0]: https://github.com/xujian519/mady/releases/tag/v0.3.0
[0.2.0]: https://github.com/xujian519/mady/releases/tag/v0.2.0
[0.1.0]: https://github.com/xujian519/mady/releases/tag/v0.1.0
