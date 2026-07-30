# YunXi vs Mady 核心模块深度对比报告

> 生成日期：2026-07-30
> 范围：YunXi（Rust 原生，`/Users/xujian/projects/YunXi`） vs Mady（Go 1.26，`/Users/xujian/projects/Mady`）
> 聚焦：提示词工程、上下文工程、工具调用引擎

---

## 全景概览

| 维度 | YunXi | Mady |
|------|-------|------|
| **语言** | Rust（核心）+ Python（桥接层） | Go 1.26 |
| **代码量（三大模块）** | ~150K 行 | ~40K 行估算 |
| **架构风格** | 单态化编译 + 宏驱动注册 + 文件驱动热加载 | 接口类型断言 + HashMap 注册表 + 扩展插件 |
| **专利领域深度** | 极高：50+ 意图类型、9 专业角色、80+ 工具 | 高：6 领域、graph/Pregel 图引擎、handoff 交接 |
| **多语言定位** | 单一项目（专利专业化） | 框架（多领域 Agent 运行时） |

---

## 一、提示词工程（Prompt Engineering）

### 1.1 YunXi 方案

**架构层次**：三层递进体系

```
L1: SystemPromptBuilder (runtime/src/prompt.rs)
  └─ Builder 模式构造提示词：Intro → Output Style → System → Doing Tasks → Actions → 动态边界 → Environment → Project Context → Config → Append Sections

L2: Agent Role XML (assets/agents/*.xml)
  └─ 9 个 XML 定义：identity + methodology + tools + constraints
  └─ 通过 agent_roles.rs agent_role_from_str() → 读取 XML → 注入 SystemPromptBuilder.append_section()

L3: Skill XML 体系 (assets/skills/)
  └─ 能力层 (cap-*)：80+ 行能力描述 + 伪代码流程
  └─ 任务层 (task-*)：结构化步骤指导
  └─ 共享层 (_shared/)：通过 <include ref="_shared/module"/> 在运行时由 skill.rs 展开（递归 3 层）
  └─ 基础层 (technique/, foundation-hitl/): 通用策略
```

**关键特征**：
- **宪法规则引擎**（constitutional-engine）：18 种检查类型（StructuralAnalysis / KeywordBlocklist / CategoryDetection / PatternAnalysis / SpecificationAnalysis / SectionStructure / ClaimClarityAnalysis 等），可配置 severity + action + phase 挂载到工具调用前/后
- **Include 展开**：SKILL.xml 中的 `<include ref="_shared/module_name" />` 在运行时递归展开，免于重复
- **双元输出风格**：`OutputStyle` 支持不同响应风格（Concise / Detail）
- **Token 预算硬约束**：单指令文件 ≤4K chars，总指令 ≤12K chars，超量截断 + 标记

### 1.2 Mady 方案

**架构层次**：

```
L1: PromptStore (prompt/store.go)
  └─ go:embed 内嵌模板 + 用户目录覆盖 → HashMap[string]int 索引

L2: PromptTemplate (prompt/loader.go)
  └─ JSON 序列化的 system + user 提示对，{{variable}} 手柄替换

L3: DocumentStyle (domains/style.go)
  └─ YAML 定义风格指南 → SystemPrompt() 方法渲染为注入块
  └─ 4 套预置风格：patent-standard / legal-standard / chat-friendly / assistant-neutral

L4: Extension 注入
  └─ Extension 接口 (SystemPromptProvider) 通过类型断言注入额外提示段落
```

### 1.3 差距分析

| 对比项 | YunXi 优势 | Mady 优势 |
|--------|-----------|-----------|
| **提示词粒度** | 三层（系统/角色/技能）+ 宪法规则 | 两层（模板/风格）+ 扩展注入 |
| **技能展开** | **XML Include 递归展开** — 天然模块复用 | 无类似机制，需手动组合 |
| **宪法规则** | **18 种规则检查**，与工具调用绑定 | `guardrails/` 三级护栏 + `citation_gate` 引用核验，规则类型较少但有 LLM 深度分析 |
| **角色定义** | **XML 文件驱动** — 非侵入式热加载 | 代码内 AgentConfig / Handoff 配置 |
| **Token 预算控制** | **硬约束截断** | ContextBuilder 的 LayerConfig.MaxTokens |
| **国际化** | 无内置 | **i18n 模块**（zh-CN/en-US） |

**关键差距**：
1. **YunXi 的 XML Include 技能展开机制**是独特优势 — Mady 的 skill 系统（skills/ 目录 + SKILL.md）缺少运行时递归展开和共享模块引用
2. **YunXi 的宪法规则引擎**规则种类（18 种）远多于 Mady 的 guardrails 模块，且完全可配置（YAML + 运行时 JSON）
3. **Mady 的 DocumentStyle + i18n** 组合在面向用户的文案风格控制和国际化上领先

---

## 二、上下文工程（Context Engineering）

### 2.1 YunXi 方案

**四层记忆系统（memory crate）**：

```
UnifiedMemory (门面)
  ├── MemoryStore (文件记忆 ~/.yunxi/memory/)
  │   └─ 基于文件系统的持久化
  ├── TieredMemoryStore (SQLite 分层)
  │   ├── HOT:   当前对话（RAM）
  │   ├── WARM:  近期会话摘要（SQLite）
  │   ├── COLD:  长期归档（SQLite + 压缩）
  │   └── ETERNAL: 不可变核心知识
  ├── SemanticSearch (可选 BGE-M3 嵌入)
  │   └─ 混合检索：关键词 + 语义
  └── Hebbian 学习 (hebbian.rs)
       └─ 神经网络风格的关联强化
```

**对话运行时**（runtime/src/conversation.rs）：
- `ConversationRuntime<C, T>` 泛型：`C: ApiClient` + `T: ToolExecutor`
- 自动压缩：200K tokens 阈值触发增量摘要
- `TurnSummary` + `AutoCompactionEvent` 追踪每次交互
- `HookRunner`：PreToolUse / PostToolUse 两个事件

**意图分类**（intent/classifier.rs）：
- 50+ 领域特定意图类型
- 三路分类：关键词匹配（≥0.9 直接返回）→ 嵌入增强（BGE-M3）→ 偏好提升（记忆系统）
- 偏好学习：用户修正时自动记录 → 下次倾向该意图

**复杂推理**（reasoning/strategy.rs）：
- 7 种推理策略：StepByStep / StructuredAnalysis / Debate / TreeOfThoughts / VerifiedThinking / FirstPrinciples
- 复杂度分类（Low/Medium/High）→ 策略选择 + Token 预算

### 2.2 Mady 方案

**三层会话记忆（memory/）**：
```
MemoryStore 接口
  ├── InMemoryStore（开发/测试）
  ├── SQLiteStore（FTS5 + 向量 RRF 融合）
  └── 复合评分：score = 0.5×语义 + 0.3×新鲜度 + 0.2×重要性
```

**分层上下文**（agentcore/context_builder.go）：
- 5 层 Layer：System / Tools / Knowledge / Memory / History
- 每层：LayerConfig（Enabled / InjectMode / MaxTokens / Priority / Position）
- 4 种注入策略：always / per_turn / on_demand / by_trigger
- `LayerProvider` 接口 + `EngineRegistry` 工厂 Map

**会话管理**（session/）：
- JSONL 追加日志 + 树结构（Entry 节点）
- 分支/压缩支持
- `pathToLeaf()` + `atomic.Pointer` 缓存

**推理策略**（agentcore/reasoning_strategy.go）：
- 6 种策略：ReAct / PlanAndSolve / TreeOfThoughts / Debate / SelfAsk / StepByStep
- `reasoning_router.go`：三档复杂度分类 → 路由

### 2.3 差距分析

| 对比项 | YunXi 优势 | Mady 优势 |
|--------|-----------|-----------|
| **记忆分级** | **4 层（HOT/WARM/COLD/ETERNAL）** + 自动层级迁移 | 3 层（User/Session/LongTerm）+ 复合评分 |
| **对话压缩** | **自动增量压缩**（200K 阈值） | session 支持压缩但无自动触发 |
| **意图分类** | **50+ 专业意图** + 偏好学习 + 嵌入增强 | `reasoning_router.go` 三档分类，无专门意图模块 |
| **上下文组装** | 线性系统提示构建 | **5 层可配置 + 4 种注入策略 + EngineRegistry 工厂** |
| **Hebbian 学习** | 神经网络风格关联记忆强化 | 无类似机制 |
| **记忆编译器** | 无 | **memory/compiler/** — 策略学习型记忆编译器（时间衰减+质量加权） |
| **推理策略提示** | 7 种策略提示注入 | 6 种策略 + 图拓扑映射 |

**关键差距**：
1. **YunXi 的意图分类器**是碾压性优势：50+ 专利领域意图类型，三路分类 + 偏好学习，Mady 完全没有等效模块
2. **YunXi 的四层分级记忆 + 自动层级迁移**比 Mady 的三层更精细，但 Mady 的编译器有独特的学习能力
3. **Mady 的 ContextBuilder 5 层可配置架构**在灵活性和扩展性上领先 YunXi 的线性提示构建
4. **YunXi 的 Hebbian 学习**是独特的神经网络风格关联记忆，Mady 无等效

---

## 三、工具调用引擎（Tool Invocation Engine）

### 3.1 YunXi 方案

**双注册表体系**：

```rust
// 1. 全局 LazyLock 注册表 (tool_registry.rs)
pub static GLOBAL_REGISTRY: LazyLock<ToolRegistry> = LazyLock::new(init_global_registry);
// HashMap<&'static str, ToolRunner> where ToolRunner = Arc<dyn Fn(&Value) -> Result<String, String> + Send + Sync>

// 2. Trait 体系 (tool_trait.rs / tool_exec.rs)
pub trait Tool { fn spec() -> ToolSpec; fn execute(&self, input: &Value) -> Result<String, String>; }
pub trait ToolExecutable: Send + Sync { fn execute(&self, input: &Value) -> Result<String, String>; fn name() -> &'static str; }

// 3. ToolOutput 结构化结果
pub struct ToolOutput { content, data, duration_ms, success, metadata }
```

**80+ 工具分类**（tool_registry.rs）：
- Core（bash/read/write/edit/glob/grep/search/fetch）
- 专利搜索（SynonymSearch / SearchQueryBuilder / PatentSearch / IterativeSearch / HybridRetrieval / GooglePatentsFetch 等 7 工具）
- 专利分析（NoveltyAnalysis / InventivenessAnalysis / InfringementAnalysis / ExaminerSimulate / SemanticCompare 等 6 工具）
- 专利撰写（ClaimGenerator / SpecificationDrafter / AbstractDrafter / InnovationEvaluator 等 4 工具）
- 专利质量（QualityScorer / QualityChecker / ClaimFormalityCheck / SubjectMatterCheck / UnityCheck 等 6 工具）
- 审查意见（OaParse / ResponseTemplate / SuccessPredictor 等 3 工具）
- 知识库（KnowledgeSearch / LegalReasoning / LawQuery / KnowledgeCard / SuperReasoningPlan 等 5 工具）
- 文档处理（DocumentRead / PdfParse / DocxParse / ExcelParse / MarkdownParse 等 5 工具）

**子 Agent 系统**（agent.rs + agent_roles.rs）：
- 9 种专业角色（Retriever/Analyzer/Writer/NoveltyChecker/CreativityChecker/InfringementChecker/InvalidityChecker/Reviewer/QualityChecker）
- 每种角色：XML 定义 + 独立 SystemPrompt + 工具白名单
- `AgentJob` 结构：manifest + prompt + system_prompt + allowed_tools
- 完整生命周期：agent_id → output_dir → manifest_file → spawn → completion/error

**宪法合规检查**（constitutional_check.rs）：
- `ConstitutionalCheckTool` 在工具执行前后自动检查
- 可配置检查器配置文件
- 输出 `CheckReport` 含通过/失败/警告

**DAG 图编排**（workflow/graph.rs + workflow/orchestrator.rs）：
- `FlowGraph` 结构：nodes + edges + conditions（Always/OnSuccess/OnFailure）
- 拓扑排序执行 + 条件分支
- `Orchestrator`：PlanGenerator → GraphExecutor → CheckpointStore → 重试/恢复

### 3.2 Mady 方案

**注册表 + 中间件链**：

```go
// agentcore/Registry (tool.go)
type Registry struct { tools map[string]*Tool }
type Tool struct {
    Name, Description, Parameters // JSON Schema
    Func ToolFunc
    Before, After []HookFunc
    DynamicParameters, DynamicReadOnly func() []byte
}

// agentcore/Executor (executor.go)
type Executor struct { registry *Registry; globalBefore, globalAfter []HookFunc }
// 中间件链：tool.Before → globalBefore → middlewareChain → coreExecute → globalAfter → tool.After
// 支持串行(executeSerial)和并行(executeParallel + concurrency.Pool)
```

**Extension 系统**（agentcore/extension.go）：
```go
type Extension interface { Name(); Init(); Dispose() }
// 可选子接口：ToolProvider / HookProvider / MiddlewareProvider
//            SystemPromptProvider / TransformContextProvider / LifecycleProvider
```

**工具定义**（tools/tools.go）：
- 30+ 内置工具：read / edit / bash / grep / glob / browser / git / process / vision / code_exec 等
- WorkingDirSandbox 通过 `propagateSandbox()` 传播

**Handoff 交接**（agentcore/handoff.go）：
- `delegate` 模式（子 Agent 结果作为工具结果）
- `transfer` 模式（完全转移 + 继承运行时状态）
- 白名单安全控制（default-deny / "*" 通配 / 显式名称）
- 敏感工具在 transfer 时跳过

### 3.3 差距分析

| 对比项 | YunXi 优势 | Mady 优势 |
|--------|-----------|-----------|
| **工具数量** | **80+ 工具**（60+ 专利专业工具） | 30+ 工具（通用为主） |
| **注册机制** | 宏驱动 `reg!` 编译期注册 | 运行时 `map[string]*Tool` 注册 |
| **执行模型** | 函数式 `Arc<dyn Fn>` | **中间件洋葱模型** + 串/并行支持 |
| **Extension 体系** | 无（编译时确定） | **5 种扩展接口 + 类型断言** |
| **子 Agent** | **9 专业角色 + XML 文件定义 + 白名单** | Handoff delegate/transfer 两种模式 |
| **合规检查** | **宪法引擎 18 种规则检查** | guardrails 三级护栏 + citation_gate |
| **DAG 编排** | **FlowGraph 图编排 + Orchestrator** | graph/ Pregel 图引擎（更有表达能力） |
| **结构化输出** | `ToolOutput` 含 duration/metadata | `DualToolOutput`（LLM + 用户分开） |
| **MCP 集成** | 5 个 MCP crate（pool/reconnect/stdio/remote） | mcp/ 客户端（stdio + HTTP/SSE） |
| **沙箱** | runtime/sandbox.rs (12794 行) | tools/path.go + WorkingDirSandbox |

**关键差距**：
1. **YunXi 的专利专业工具数量（60+）是 Mady 的 2 倍以上**，覆盖检索/分析/撰写/质量全流程
2. **YunXi 的子 Agent 系统比 Mady 的 Handoff 更丰富**：9 专业角色 + XML 定义 + 独立生命周期管理
3. **YunXi 的宪法引擎规则检查**比 Mady 的 guardrails 更系统化、可配置
4. **Mady 的中间件链 + Extension 体系**在架构扩展性上领先 YunXi 的硬编码注册表
5. **Mady 的 Pregel 图引擎**比 YunXi 的 FlowGraph 更有表达能力（StateSchema/Reducer/NodePolicy）
6. **YunXi 的 DAG 编排 + Orchestrator + Checkpoint** 形成完整的规划-执行-恢复闭环，Mady 缺少等效的编排器

---

## 综合结论与建议

### 可以借鉴到 Mady 的 YunXi 亮点

| 优先级 | 特性 | 预估工作量 | 说明 |
|--------|------|-----------|------|
| **P0** | **意图分类器**（50+ 领域意图 + 偏好学习） | 2-3 周 | Mady 依赖 reasoning_router 的三档分类，可新建 `intent/` 模块，复用现有 knowledge/ + memory/ |
| **P0** | **技能 Include 展开** | 1 周 | 在 `skill/` 解析器中添加 `<include>` 标签支持，类似 XML Include |
| **P1** | **宪法规则引擎** | 2-3 周 | 扩充 guardrails/，添加 YAML 配置的规则检查器，与现有 citation_gate 整合 |
| **P1** | **子 Agent 角色 XML 定义** | 1-2 周 | 将 Handoff 目标改用 XML 文件定义 + 工具白名单 |
| **P2** | **四层分级记忆** | 2 周 | memory/ 增加 TieredMemoryStore + 自动迁移 |
| **P2** | **工具结构化输出 ToolOutput** | 1 周 | tools/ 中的函数包装为含 duration/metadata 的结构体 |
| **P3** | **自动对话压缩** | 1 周 | session/ 增加增量摘要触发 |
| **P3** | **DAG 编排器** | 3-4 周 | 在 graph/ Pregel 之上包装 Orchestrator + Checkpoint |

### 可以借鉴到 YunXi 的 Mady 亮点

| 特性 | 说明 |
|------|------|
| **中间件洋葱模型** | Executor 的工具执行链比 YunXi 的函数式 Arc 调用更灵活 |
| **Extension 接口体系** | 类型安全的插件系统，YunXi 可借鉴替代硬编码 Registry |
| **Pregel 图引擎** | 比 FlowGraph 更强大的消息传递 + StateSchema 模式 |
| **i18n 国际化** | guardrails 文案多语言支持 |
| **DocumentStyle** | 风格指南独立 YAML 定义 + 注入 |
| **记忆编译器** | 质量加权 + 衰减的时间衰减学习 |
| **DualToolOutput** | LLM 与用户的分开输出通道 |

---

## 文件清单

| 文件 | 行数 | 模块 |
|------|------|------|
| `runtime/src/prompt.rs` | 845 | 提示词构建器 |
| `runtime/src/conversation.rs` | 38K | 对话运行时 |
| `runtime/src/hooks.rs` | 10K | 钩子系统 |
| `tools/src/tool_registry.rs` | 14.5K | 全局工具注册表 |
| `tools/src/tool_trait.rs` | 8.6K | Tool trait 定义 |
| `tools/src/tool_exec.rs` | 5.1K | ToolExecutable trait 注册表 |
| `tools/src/agent.rs` | 37.5K | 子 Agent 系统 |
| `tools/src/agent_roles.rs` | 10.9K | 9 专业角色 |
| `tools/src/skill.rs` | 6K | 技能注入引擎 |
| `tools/src/system_prompt.rs` | 4.2K | Athena 能力注入 |
| `tools/src/constitutional_check.rs` | 14.7K | 宪法合规检查 |
| `memory/src/tier.rs` | 19.9K | 四层分级记忆 |
| `memory/src/unified.rs` | 13.3K | 统一记忆接口 |
| `memory/src/hebbian.rs` | 18.8K | Hebbian 学习 |
| `intent/src/classifier.rs` | 16.9K | 意图分类器 |
| `intent/src/intent_types.rs` | 14.1K | 50+ 意图类型 |
| `reasoning/src/strategy.rs` | 8.2K | 7 种推理策略 |
| `constitutional-engine/src/engine.rs` | 15K | 宪法规则引擎 |
| `constitutional-engine/src/model.rs` | 11.6K | 规则数据模型 |
| `workflow/src/orchestrator.rs` | 13K | 工作流编排器 |
| `workflow/src/graph.rs` | 14.7K | DAG 图引擎 |

---

*报告完毕。所有数据基于 2026-07-30 代码快照。*
