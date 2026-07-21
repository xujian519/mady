# Mady 项目架构全面审阅与长期发展建议

> **审阅日期**：2026-07-21
> **审阅范围**：全部 940 个 Go 源文件（~207K 行代码）、8 层分层架构
> **审阅方法**：代码库静态分析 + 依赖图追踪 + 子系统深潜（3 个并行探索 Agent）

## 目录

1. [架构总体评估](#一架构总体评估)
2. [关键子系统评估](#二关键子系统评估)
3. [长期发展建议](#三长期发展建议按优先级排列)
4. [技术债务清单](#四技术债务清单)
5. [验证方式](#五验证方式)

---

## 一、架构总体评估

### 1.1 分层结构评分：★★★★☆（良好）

项目在 CLAUDE.md 中声明的 8 层分层架构在实践中得到了较好遵守。

**架构全景图：**

```
cmd/mady/         ← 应用入口（8 个子命令：tui | serve | acp | eval | patent | ...）
    │
    ├── server/   ← HTTP/SSE API（Agent 池、线程管理、SSE 流、交底书端点）
    ├── tui/      ← 8 层 Elm 架构（90 源文件，独立 LAYERS.md）
    │   └── agentadapter/  ← agentcore → chat 事件转换
    │
    └── domains/  ← 领域 Agent 工厂 + 路由 + 推理引擎
            │
    ┌───────┘
    ▼
 agentcore/       ← 核心运行时（Extension、Hook、Middleware、Handoff、EventBus）
    │
    ├── tools/    ← 独立子模块（browser、bash、file I/O、git）
    ├── graph/    ← DAG + Pregel 图引擎（15 源文件）
    ├── guardrails/  ← 三级护栏（Light/Standard/Strict）
    ├── knowledge/   ← SQLite 知识库 + 知识图谱（13 节点类型、11 关系类型）
    ├── memory/      ← 三层记忆系统（User/Session/LongTerm）
    ├── retrieval/   ← 关键词→BM25→向量→RRF→重排检索管道
    ├── provider/    ← LLM 接入层（ChatCompat、SmartRouter、AgentAdapter）
    ├── mcp/         ← MCP 协议客户端（stdio + HTTP/SSE）
    ├── workflow/    ← Pregel 工作流编排
    ├── session/     ← JSONL 树会话管理
    ├── disclosure/  ← 11 节点 Pregel 交底书分析管线
    ├── skill/       ← SKILL.md 技能系统
    ├── prompt/      ← 提示词模板
    └── pkg/         ← 共享工具包（agentconfig、lawcite、util）
```

**优点：**
- TUI 模块有独立的 8 层 Elm 架构（`tui/LAYERS.md`），设计决策均有文档记录
- 核心引擎 `agentcore` 是一个设计良好的运行时，提供了统一的 Extension、Hook、Middleware 机制
- 基础设施层各模块职责清晰，互不重叠

**问题：**
- 分层声明与实际代码存在偏差。例如 `domains/` 被声明为"领域扩展层"，但实际上它严重依赖 `agentcore` 和 `workflow`，更像是一个"胶水层"
- 缺少自动化的分层边界检查工具

### 1.2 模块划分评分：★★★☆☆（中等）

**优点：**
- 模块粒度适中，大多数模块 10-35 个源文件，便于理解
- TUI 组件的多文件拆分（Editor 5 文件按职责分组、ChatApp 14 文件拆分）是良好的工程实践
- `go.work` 多模块结构将 heavy dependencies（browser/chromedp）隔离到 `tools/` 子模块

**问题：**

**1. agentcore 过于庞大。** 包含以下所有子包和概念，接近单点故障：

| 职责 | 文件 |
|------|------|
| Agent 运行时 | `agent.go`, `run.go` |
| 配置 | `config.go`（~30+ 字段） |
| 插件系统 | `plugin.go` |
| 交接机制 | `handoff.go`, `handoff_context.go`, `handoff_result.go` |
| 生命周期钩子 | `hooks.go`, `lifecycle.go`, `observer.go` |
| 扩展系统 | `extension.go` |
| Pipeline Atoms | `atom.go` |
| 推理策略 | `reasoning_strategy.go`, `reasoning_router.go` |
| 事件总线 | `event.go` |
| 权限门控 | `permission/` 子包 |
| 追踪 | `tracing/` 子包 |
| 死循环检测 | `doomloop/` 子包 |
| 评估框架 | `evaluate/` 子包 |
| 清单系统 | `manifest.go` |

建议将 `evaluate/`、`doomloop/`、`tracing/` 等横切关注点提升为独立顶层模块。

**2. 命名冲突风险。** `graph/`（图引擎，DAG + Pregel）与 `knowledge/graph/`（知识图谱，13 节点类型 + 11 关系类型）容易混淆。

**3. `domains/` 目录职责不清。** 它既是领域 Agent 配置中心，又包含 reasoning 引擎、规则引擎、文档模板、写作评估、创造性评估等多个子系统。

### 1.3 依赖关系评分：★★★☆☆（中等）

**优点：**
- TUI 的依赖方向严格受控（Layer 7 → Layer 0，参见 `tui/LAYERS.md`）
- `tui/chat` 不导入 `agentcore`，通过 `agentadapter` 解耦（`AppHost` 接口模式）
- 接口抽象使用得当（`AppHost`、`Subscriber`、`KnowledgeBackend`、`WritableBackend`）

**问题：**

**agentcore 是事实上的上帝包**。几乎所有模块都依赖它：

```
domains/         → agentcore
guardrails/      → agentcore
knowledge/       → agentcore
memory/          → agentcore
psychological/   → agentcore
server/          → agentcore
provider/        → agentcore
workflow/        → agentcore (间接)
tui/agentadapter → agentcore
```

**缺少接口隔离。** 各模块直接依赖 `agentcore.Config`、`agentcore.Agent`、`agentcore.LifecycleHook` 等具体类型，而非通过接口。如果 agentcore 的 API 发生变化，影响面极大。

**三角依赖风险。** `domains/router.go` 导入 `workflow` 包，而 `workflow` 可能间接依赖 `agentcore`，形成了事实上的 `agentcore → domains → workflow → agentcore` 三角。

### 1.4 接口设计评分：★★★★☆（良好）

**优点：**
- **Extension 接口**统一了工具、钩子、中间件的注入，支持 `ToolProvider`、`HookProvider`、`MiddlewareProvider` 等 8 个可选子接口
- **`LifecycleHook`** 提供 10 个拦截点（BeforeAgentRun/BeforeTurn/BeforeModelCall/AfterModelCall/AfterTurn/AfterAgentRun 等），Hook 链按注册顺序执行，可组合性强
- **`Step` 接口**（`Run(ctx, input) (string, error)`）是图引擎的通用抽象，DAG、PregelStep、Router 都实现此接口
- **`Atom` 接口**将声明式工作流定义与实现解耦，插件作者按名引用标准操作

**可改进：**
- `agentcore.Config` 结构体字段过多（~30+），已拆分为 4 个内嵌结构（ModelConfig/SkillConfig/ExecutionConfig/CompactionConfig），但仍然偏大
- 缺少 `context.Context` 在接口中的一致性使用——部分接口用，部分不用
- PregelState 使用 `map[string]any` 无类型安全

### 1.5 数据流向评分：★★★☆☆（中等）

主要数据流路径清晰：

```
用户输入
  → Router（关键词意图分类或 LLM 分类）
    → Handoff（Delegate 或 Transfer，白名单校验 + 深度限制）
      → 领域 Agent（chat/assistant/patent/legal）
        → BeforeModelCall 链（记忆注入、护栏检查、引用核验、RAG 检索）
          → LLM 调用（Provider → SmartRouter 路由）
        → AfterModelCall 链（内容审核、记忆提取、引用标注）
        → 工具执行（Middleware 链：限流、超时、重试）
      → 输出传递（HandoffResult JSON 或纯文本）
    → 事件通知（EventBus → TUI/SSE）
  → 用户可见输出
```

但存在以下问题：

1. **Handoff 上下文传递依赖 JSON 序列化**（`handoff.go:128-130`），`marshalErr` 后静默降级为 `{}`
2. **Pregel 状态传递使用 `map[string]any`**，StateSchema 的引入是向正确方向的一步，但尚未在 disclosure 管线中全面应用
3. **事件总线传递的事件使用字符串类型判断**（`EventType`），缺少泛型 / 强类型约束
4. **`inheritRuntime` 复制全部工具到目标 Agent**，缺乏细粒度的权限过滤

---

## 二、关键子系统评估

### 2.1 Agent 运行时（agentcore）★★★★☆

**设计亮点：**

- **Handoff 机制**的白名单校验（default-deny）设计正确；委托深度限制（`DefaultMaxDelegationDepth`）防止无限递归；Invisible 标志让 Chat Agent 内部路由体验无缝
- **10 个拦截点的 LifecycleHook 链**，支持组合式扩展（GuardrailHook + RetrievalHook + MemoryHook + CitationGate 同时注册）
- **Observers 模式**（5 个细粒度接口：`AgentRunObserver`、`TurnObserver`、`ModelCallObserver`、`ToolCallObserver`、`MessagePersistObserver`），比单体 Hook 更关注点分离
- **6 种推理策略**（default/step_by_step/structured_analysis/debate/tree_of_thoughts/verified_thinking/first_principles），三档复杂度分类（low/medium/high），由 `ReasoningRouter` 动态调整 thinking budget
- **插件式上下文引擎**（compressor/chunked/tiered/truncate），支持结构化压缩（JSON summaries）
- **EventBus 双模式投递**（Emit 非阻塞丢失 vs EmitMustDeliver 有界阻塞）
- **Extension 事务性注册**（初始化失败时反向回滚已初始化的 Extension）

**关注点：**

| 关注点 | 风险等级 | 说明 |
|--------|----------|------|
| `inheritRuntime` 复制全部工具 | 安全 | 缺少细粒度权限过滤（虽然安全注释已标注） |
| AgentPool 引用计数 | 低 | 缺少泄漏检测，server `Close` 时 evict 所有 |
| `agentcore.Config` 字段膨胀 | 中 | 30+ 字段，缺少配置校验的集中化 |
| `deprecatedHookAdapter` | 低 | 遗留适配器，v0.6.0 应移除 |

### 2.2 TUI 模块 ★★★★★

**这是项目中设计最精良的子系统。** 90 个源文件 / 8 层 Elm 架构，生产级质量。

**关键设计决策：**

| 决策 | 说明 |
|------|------|
| 无重导出 | 根 `tui` 包不重导出子包类型 |
| `chat` 不依赖 `agentcore` | 通过 `agentadapter` + `AppHost` 接口解耦 |
| Cell-level 渲染 | `Cell`/`Row`/`CellGrid` + `DiffRows` 单元格级差分，消除宽字符截断和 SGR 编码歧义 |
| 双渲染模型 | TUI Engine（Elm 架构，差分渲染） + stdio（过程式 stdout/spinners） |
| 显式 FSM | `chat/state.go` 249 行显式状态机解耦交互状态与事件处理 |
| 编辑器 5 文件拆分 | 核心/编辑/渲染/历史/kill-ring，按职责分组 |
| `core.Every` 移除 | 改为 `TUI.Every()` 生命周期感知替代方案 |
| `AppHost` 接口 | 打破 `chat → root tui → chat` 循环依赖 |

**建议：**
- TUI 模块可以独立发布为 Go library（`github.com/xujian519/mady-tui`）
- 考虑将 TUI 从主仓库中拆分为独立模块（go.work 已支持多模块）

### 2.3 图引擎（graph/）★★★★☆

**同时支持 DAG + Pregel 两种执行模型：**

| 特性 | DAG (graph.go) | Pregel (pregel.go) |
|------|---------------|-------------------|
| 编排方式 | 拓扑排序 → 同层并行 | SuperStep 迭代 |
| 状态管理 | `GraphState`（Mutex） | `PregelState`（map[string]any，深拷贝隔离） |
| 边类型 | 静态 + 条件 | 静态 + 条件路由 |
| 合并策略 | 无 | StateSchema（5 种 Reducer） |
| 节点策略 | 无 | NodePolicy（重试/超时/副作用标记/panic 恢复） |
| 检查点 | `InterruptableGraph` | `PregelCheckpointer` |
| 降级 | 无 | `DegradationMark` + `DegradationSummary` |

**设计亮点：**
- StateSchema 的 Reducer 模式（`ReducerLastWriteWins`/`ReducerAppend`/`ReducerUnion`/`ReducerMergeMap`/`ReducerFailOnConflict`），合并总是确定性（按节点名排序）
- NodePolicy 支持指数退避重试、上下文超时、SideEffect 标记（I/O 节点不写入共享状态）
- DegradationMark 提供优雅降级追踪（`retriever_unavailable`、`search_failed` 等）
- `PregelStep` 适配 Pregel 图到 DAG 的 `Step` 接口，实现图嵌套

**关注点：**
- 并行节点执行未限制并发数，过多 goroutine 可能引发调度开销
- StateSchema 仅在编译时配置，运行时无法动态修改合并策略

### 2.4 推理引擎（domains/reasoning/）★★★★☆

**五阶段工作流设计完整，由 YAML 驱动的 WorkflowManifest 配置：**

```
Stage ① Collect Facts          ─┤ 4 种收集器并行（用户输入/文档/知识库/推导）
Stage ② Retrieve Rules         ─┤ 4 源并行检索（知识图谱/向量库/SKILL.md/确定性规则）
Stage ③ Plan                   ─┤ 模板路径（预设 PlanTemplate）+ LLM 路径
Stage ④ Execute                ─┤ 多假设辩论 Pregel 子图（正反方 Agent → 三段论法官 → 证据法官）
Stage ⑤ Check                  ─┤ 核查 + 合规评分
```

**设计亮点：**
- `FactBlackboard`（共享内存黑板），RWMutex 保证并发安全，阶段间数据传递
- `MultiSourceRetriever` 4 源并行检索 + 优先权排序 + 去重
- **三段论引擎**（`syllogism.go`）实现形式化逻辑校验：大前提（规则）→ 小前提（事实）→ 结论
- **多假设辩论子图**（`multi_hypothesis.go`）：正反方 ReAct Agent 并行 → `SyllogismJudge` 过滤 → `EvidenceJudge` 按权威度加权 → 条件边支持未解决恢复
- `StageCheckpoint` 支持阶段级暂停恢复
- `RequireRuleConfirmation` 中断在 Stage ② → ③ 之间，等待人工确认检索到的规则

**4 个内置 WorkflowManifest：**

| Manifest | 用途 | 启用多假设 |
|----------|------|-----------|
| `patent_novelty_default` | 新颖性检索 | 否 |
| `patent_patentability_default` | 创造性评估（三步法） | 是 |
| `patent_drafting_default` | 权利要求撰写 | 否 |
| `patent_invalidation_default` | 无效分析 | 否 |

**关注点：**
- `WorkflowManifest` 以 Go 代码硬编码而非从 YAML 文件加载，与声明的"YAML 驱动"不一致
- 阶段间数据传递依赖 `FactBlackboard` 具体类型（非接口），测试和扩展受限
- 多假设子图在 Stage ④ 执行，但 "Execute" 阶段名过于通用

### 2.5 技术交底书管线（disclosure/）★★★★☆

**11 节点 Pregel 图，实现完整的中国专利交底书分析：**

```
preprocess
  → [extract_problem ‖ extract_features ‖ extract_effects]   ← 三个提取 Agent 并行
    → merge_extractions
      → check_consistency   ← PFE 三元组闭包检查
        ├── [retry: extract_*]  ← 最多 2 次，带 LLM 反馈
        └── [continue: generate_keywords]
            → retrieve_prior_art
              → check_novelty
                → generate_report
                  → review_gate   ← human-in-the-loop 暂停点
                    → draft_claims
                      → __end__
```

**设计亮点：**
- 三个提取 Agent 并行运行，JSON Schema 约束 LLM 输出格式，各写独立 PregelState Key
- `check_consistency` 实现 PFE 三元组闭包检查，失败时带反馈自动重试，最终 fail-open
- `review_gate` 使用 `NewInterruptErrorWithData()` 实现人工复核暂停点
- 支持 `DomainRetriever` 注入（未配置时降级为 `evidence_coverage=none`）
- 关联 `inventiveness/` 创造性三步法子图，通过 EventBus 在 disclosure 完成后自动触发
- 启发式回退路径（`assessNoveltyFromState`）确保 LLM 失败时仍有基础的评估

**关注点：**
- 关键词生成（`generateKeywordsNode`）使用确定性规则提取，代码标注"Phase 2 将替换为 LLM 混合模式"
- PregelState key 使用字符串常量（`StateKeyExtraction`、`StateKeySearchKeywords`），缺少编译时类型检查
- `DisclosureDoc` 的 9 段式解析假设较强的输入格式一致性

### 2.7 护栏系统（guardrails/）★★★★☆

**三层护栏 + 双级引用核验 + AI guardian 熔断器，组成完整的 AI 安全体系：**

| 层级 | 拦截时机 | 触发条件 | 行为 |
|------|----------|----------|------|
| 🟢 Level Light | `AfterModelCall` | BlockedPhrases | 整条回复拦截，替换为安全文案 |
| 🟡 Level Standard | `AfterModelCall` | RiskKeywords | 追加不确定性声明 + 免责声明 |
| 🔴 Level Strict | `AfterModelCall` | ApprovalKeywords | SuppressPersist + 等待人工审核 |
| 📖 CitationGate | `AfterModelCall` | 引用内容 | S1 静态表核验 + S2 知识源核验 |
| 👁️ Guardian | ToolsCalled | 高风险操作 | LLM 裁判拒绝或熔断器打开 |

**设计亮点：**
- 双级引用核验（R1 存在性 + R2 上下文相关性）是领域内创新
- 三源知识架构（S1 静态表 / S2 知识库索引 / Composite 复合源）
- false-positive 防御：有工具调用时跳过、不可核验引用跳过、干扰词过滤、枚举检测
- `SuppressPersist` 机制确保未经审批的输出不被写入会话存储
- `ApprovalRecord` 持久化留痕（SQLite），决策类型（Adopted/Modified/Rejected）反馈 Golden Benchmark
- `RegisterLevel()` 开放 API 允许外部注册自定义护栏等级

**建议：**
- `citation_table.go` 中硬编码的 82 条专利法精校数据应外置为 YAML/JSON，便于维护和社区贡献
- Guardian 的 `CircuitBreaker` 缺少自动恢复机制（连续拒绝后一直熔断）

### 2.8 检索引擎（retrieval/）★★★★☆

**完整的混合检索管道：**

```
查询输入
  → 关键词搜索（TF-IDF 启发式，中文 CJK 分词 + 英文 term 提取）
  → 向量搜索（余弦相似度，base64 float32）
  → 混合融合（加权线性组合，默认 0.7 向量权重）
  → RRF 融合（与 FTS BM25 融合，k=60）
  → Rerank 链（PositionRerank → DedupRerank → LegalRerank/PatentRerank）
  → Model Rerank（Cross-encoder，最多 20 文档，Qwen3-Reranker 兼容）
  → Citation 格式化（按法律层级分组排序）
```

**设计亮点：**
- `HybridSearcher` + `RRFFuser` 两条融合路径分别应对不同场景
- `DeduplicatingReranker` 基于内容前缀重叠去重
- `PatentReranker` 按专利文献权威度 + 申请日时效性加权
- `LegalReranker` 按中国法律渊源层级加权（宪法 100 → 法律 90 → 司法解释 85 → ...）
- `RetrievalHook` 作为 LifecycleHook 自动检索，支持 `always`/`smart`/`first_n`/`on_demand` 四策略

**关注点：**
- `knowledge/` 的 SQLite 实现（FTS+向量 RRF）与 `memory/` 各有一个 RRF 融合器，功能重复
- 缺少检索结果缓存层，高频同查询重复计算
- Cross-encoder rerank 的 20 文档上限可能遗漏相关结果

### 2.9 知识管理与记忆系统 ★★★☆☆

**两个系统存在功能重叠和基础设施分裂：**

| 维度 | `knowledge/` | `memory/` |
|------|-------------|-----------|
| 存储 | SQLite（FTS+向量） + 内存图谱 | SQLite + 内存 双实现 |
| 检索 | 关键词 + 向量 RRF 混合 | BM25 + 向量 RRF 融合 |
| 图谱 | 13 节点类型、11 关系类型、两阶段构建器 | 无 |
| 嵌入 | `APIEmbedder`（OpenAI 兼容） | `providerExtractor`（同一 Provider） |
| 范围 | 文档/法条/判例/专利 | 用户偏好/会话/长期事实 |
| 缓存 | TTL 图谱缓存（5min） | 无 |

**暴露的问题：**
1. **存储重复** — 两套独立的 SQLite 管理，相同文档可能被重复索引
2. **向量空间不一致** — 即使使用同一 Embedding API，不同库的表结构和管理方式导致向量无法直接共享
3. **RRF 融合重复** — `retrieval/hybrid.go` 和 `memory/rrf.go` 各自实现了 RRF 融合
4. **命名混淆** — `knowledge/graph/`（知识图谱）与 `graph/`（图引擎）容易误认

**建议：** 创建 `store/` 顶层包，提供统一的 `VectorStore`、`DocumentStore`、`GraphStore` 接口，knowledge 和 memory 共享底层存储。

### 2.10 P0 — 插件与技能系统（skill/ + plugins/ + agentcore/plugin.go）★★★☆☆

**三个相关系统但职责分散：**

| 系统 | 定义位置 | 格式 | 注册方式 |
|------|----------|------|----------|
| Skill | `skill/` 目录下的 `SKILL.md` | YAML frontmatter + Markdown | 扫描发现，LLM 选择 |
| Plugin | `plugins/*/plugin.json` + `SKILL.md` | JSON Manifest + Markdown | `ScanPlugins()` 发现 |
| Manifest | `agentcore/manifests/` | JSON（go:embed） | 编译时嵌入 + 外部覆盖 |

**问题：**
- Plugin 系统深嵌在 `agentcore` 中（`plugin.go`、`ScanPlugins`、`ValidatePlugin`），`Atom` 也定义在 `agentcore`
- Skill 和 Plugin 共享 SKILL.md 格式，但完全独立管理
- Plugin 的 `available_sources`/`handoff_targets` 白名单与 agentcore 的 `HandoffConfig.AllowedSources` 存在概念重复

---

## 三、长期发展建议（按优先级排列）

### 🔴 P0 — 立即处理（1-2 个月内）

#### 1. agentcore API 接口化

**问题**：几乎所有模块都直接依赖 `agentcore` 的具体类型（`Config`、`Agent`、`LifecycleHook` 等），导致 agentcore 的变更影响面极大。agentcore 目前 ~60+ 源文件，包含 10+ 个职责不同的子系统。

**建议**：
1. 在 `agentcore/` 中定义核心接口（`AgentRunner`、`ConfigProvider`、`HookRegistrar`、`EventBus`）
2. 内层模块（guardrails/knowledge/memory/psychological）依赖接口而非具体类型
3. 将 `evaluate/`、`doomloop/`、`tracing/` 提升为独立顶层模块

**预期收益**：减少变更影响范围、便于 mock 测试、为插件化做准备
**风险**：需要仔细设计接口粒度，避免过度工程

#### 2. 意图分类升级

**问题**：`domains/router.go` 的 `ClassifyIntent` 使用硬编码关键词匹配（中英文共 ~40 个关键词），覆盖面有限。

**建议**：实现 `LLMClassifier`（已有一个字段但未实现），与 `KeywordClassifier` 并行运行。复杂场景（关键词匹配失败或冲突时）回退到 LLM 分类。

**预期收益**：显著提高意图分类准确率
**工作量**：3-5 天

#### 3. 分层边界自动化检查

**问题**：声明的分层架构缺少自动验证机制。

**建议**：引入 `depaware` 或自定义 `go vet` 分析器，CI 中强制执行。关键约束：
- `graph/` 不得导入 `agentcore`
- `knowledge/` 不得导入 `server/`
- `tui/chat` 不得导入 `agentcore`
- `agentcore` 不得导入 `domains/`

**预期收益**：防止架构退化
**工作量**：1-2 天

---

### 🟡 P1 — 短期规划（3-6 个月内）

#### 4. 插件系统独立化

**问题**：Plugin 系统深嵌在 `agentcore` 中。

**建议**：将插件系统提升为顶层 `pluginsys/` 包，提供 `PluginLoader`、`PluginValidator`、`PluginRegistry` 接口。agentcore 通过接口消费插件。

**预期收益**：
- 插件可以独立于 agentcore 版本演进
- 社区可以开发不依赖完整 agentcore 的轻量插件

#### 5. 统一配置管理

**问题**：
- `agentcore.Config` 字段 30+，`memory.ExtensionConfig`、`guardrails.Config` 等各自独立
- 缺少统一的配置校验和 env 映射

**建议**：
- 将 Config 拆分为 4 个更专注的子配置
- 基于 `pkg/agentconfig/` 扩展统一加载器（支持 JSON/YAML/环境变量）
- 配置文件热加载（感谢 `tui/theme/watch.go` 已有文件监控模式可复用）

#### 6. 知识/记忆存储统一

**问题**：`knowledge/sqlite/` 和 `memory/sqlite_store.go` 各自管理。

**建议**：
1. 创建 `store/` 顶层包，提供 `VectorStore`、`DocumentStore`、`GraphStore` 接口
2. knowledge 和 memory 共享同一套接口和底层连接
3. 统一 Embedding Provider，确保向量空间一致
4. 合并 `retrieval/` 和 `memory/` 的 RRF 实现

**预期收益**：消除重复索引、向量空间一致、减少 SQLite 连接数、减少代码重复

#### 7. Disclosure 管线 Phase 2 实现

**建议**：完成 `generate_keywords` 的 LLM 混合模式（已标注 Phase 2）。将 PregelState key 从字符串常量迁移为类型安全 key（利用 StateSchema 的 Reducer 模式）。

---

### 🟢 P2 — 中期规划（6-12 个月内）

#### 8. 公开发布 TUI 库

TUI 模块是项目中质量最高的子系统，90 源文件、完整 8 层架构、生产级组件库。建议：
- 独立发布为 `github.com/xujian519/mady-tui`
- 添加 `pkg.go.dev` 文档和 demo
- 提供独立示例（`example/tui-demo/`）

#### 9. 可观测性增强

当前已有 OpenTelemetry tracing（`agentcore/tracing/`），建议扩展：
- Prometheus metrics：Agent 延迟分布、工具调用计数、护栏触发率、Token 消耗、RRF 延迟
- 结构化日志：统一 slog 级别和字段规范（当前部分用 `slog`、部分 `log/slog`、部分 `fmt.Printf`）
- 健康检查端点：`/health`、`/ready`、`/metrics`

#### 10. 领域 Agent 声明式配置

当前领域 Agent 通过 Go 代码工厂函数（`PatentAgentConfig()`、`LegalAgentConfig()`）配置。建议支持 YAML/JSON 声明式配置（类似 Manifest 但更丰富），实现热加载。

#### 11. API 版本化

`server/` 的 HTTP API 部分有版本化（`/v1/disclosure/`），部分没有（`/api/chat/`、`/api/threads/`）。建议：
- 统一使用 `/v1/` 前缀
- 定义 API 稳定性契约（GA 端点保证向后兼容，Beta 端点标注）
- 添加 OpenAPI 规范文档

---

### 🔵 P3 — 长期愿景（12 个月以上）

#### 12. 评估体系持续化

`agentcore/evaluate/` 已有 RAGAS 风格评估框架（P2A 31 题 + 无效决定 100 例）。建议：
- 每次 agentcore 变更后自动运行回归评估
- 扩展评估覆盖范围到 TUI 渲染和 Guardrails 行为
- 将 `ApprovalRecord.AdoptionRate` 接入评估管道作为质量信号

#### 13. 联邦式 Agent 网络

基于 A2A 协议（已有实现），支持跨实例 Agent 协作。适合专利事务所多团队协同场景。

#### 14. 多语言支持

领域 Agent 的 System Prompt、规则引擎、护栏文案以中文为主。考虑 i18n 框架和英语支持。

---

## 四、技术债务清单

| 项目 | 严重度 | 估算工作量 | 文件 |
|------|--------|-----------|------|
| 关键词分类器脆弱 | 🔴 中 | 3-5 天 | `domains/router.go` |
| `agentcore.Config` 字段膨胀 | 🔴 中 | 2-3 天 | `agentcore/config.go` |
| Pregel 状态 `map[string]any` 无类型安全 | 🔴 中 | 3-5 天 | `graph/pregel.go` |
| 缺少架构边界自动化检查 | 🔴 中 | 1-2 天 | CI 配置 |
| Handoff JSON 序列化失败静默降级 | 🔴 中 | 1-2 天 | `agentcore/handoff.go` |
| knowledge/memory SQLite 存储重复 | 🟡 低 | 5-7 天 | `knowledge/sqlite/` + `memory/` |
| retrieval/memory 独立 RRF 实现（重复） | 🟡 低 | 1-2 天 | `retrieval/hybrid.go` + `memory/rrf.go` |
| citation_table.go 硬编码法条数据 | 🟡 低 | 2-3 天 | `guardrails/citation_table.go` |
| Pregel 并行节点无并发限制 | 🟡 低 | 0.5-1 天 | `graph/pregel.go` |
| Server Agent 池缺少泄漏检测 | 🟡 低 | 1-2 天 | `server/server.go` |
| disclosure 关键词生成 Phase 2 未实现 | 🟢 低 | 2-3 天 | `disclosure/report.go` |
| WorkflowManifest Go 硬编码 | 🟢 低 | 1-2 天 | `domains/reasoning/manifest.go` |
| `knowledge/graph/` vs `graph/` 命名混淆 | 🟢 低 | 0.5 天 | 重命名 |

---

## 五、验证方式

本审阅为分析性报告，验证方法：

1. **代码库静态分析** — 使用 `go vet`、`go build`、依赖图分析验证架构约束声明
2. **与设计文档交叉验证** — 与以下文档对比确认一致性：
   - `docs/chat-assistant-architecture.md`
   - `docs/specs/` 目录下的设计说明
   - `tui/LAYERS.md`
3. **增量采纳验证** — 每项 P0 建议实施后独立验证其效果

---

## 附录：关键接口与依赖关系

### A. agentcore 核心接口

| 接口 | 方法 | 用途 |
|------|------|------|
| `Extension` | `Init()`, `Dispose()` | 插件注入基础 |
| `ToolProvider` | `Tools()` | 注册工具 |
| `HookProvider` | `BeforeHooks()`, `AfterHooks()` | 注册钩子 |
| `LifecycleProvider` | `LifecycleHook()` | 注册生命周期 |
| `LifecycleHook` | `BeforeAgentRun`, `AfterAgentRun`, `BeforeTurn`, `AfterTurn`, `BeforeModelCall`, `AfterModelCall`, `BeforeToolExecution`, `AfterToolExecution`, `BeforeMessagePersist`, `AfterMessagePersist` | 10 点拦截 |
| `Provider` | `Complete()`, `Stream()` | LLM 调用 |
| `ContextEngine` | `Compress()` | 上下文窗口管理 |

### B. 主要模块间依赖关系

```
cmd/mady ──→ server ──→ agentcore
cmd/mady ──→ tui ────→ agentadapter ──→ agentcore
cmd/mady ──→ domains ──→ agentcore, workflow
workflow ──→ graph (DAG + Pregel)
guardrails ──→ agentcore (LifecycleHook)
knowledge ──→ agentcore (Extension), retrieval
memory ──→ agentcore (Extension), retrieval
disclosure ──→ agentcore, graph (Pregel)
provider ──→ agentcore (Provider 接口)
mcp ──→ (外部 MCP 服务器)
tools ──→ agentcore (通过 replace 主模块)，main module ──→ tools (版本化 import)
```

### C. 关键设计模式总结

| 模式 | 示例位置 | 说明 |
|------|----------|------|
| Extension | `agentcore/extension.go` | 插件化功能注入 |
| LifecycleHook | `agentcore/lifecycle.go` | 运行时拦截与横切关注点 |
| Middleware | `agentcore/hooks.go` | 工具执行链包裹（洋葱模型） |
| Elm Architecture | `tui/`（8 层） | 组件树 + Msg 驱动 + Cmd 异步 |
| Pregel | `graph/pregel.go` | SuperStep 并行图计算 |
| Step Interface | `graph/graph.go` | 统一节点执行抽象 |
| Handoff | `agentcore/handoff.go` | 白名单 Agent 委派/转移 |
| EventBus | `agentcore/event.go` | 类型安全发布/订阅 |
| RRF Fusion | `retrieval/hybrid.go` | 异构评分系统融合 |
| Observer Pattern | `agentcore/observer.go` | 细粒度运行时观察 |
| FactBlackboard | `domains/reasoning/fact_blackboard.go` | 共享内存推理工作区 |

---

> **审阅完成**：2026-07-21 | 后续版本跟踪：`docs/decisions/AI_CHANGELOG.md`
