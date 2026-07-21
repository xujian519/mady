# ADR-0003: 统一运行时（Unified Runtime）设计

- **状态**：Proposed
- **日期**：2026-07-21
- **决策者**：Mady Authors + AI 协作
- **影响范围**：`graph/`, `workflow/`, `agentcore/`, `domains/reasoning/`
- **前置**：[ADR-0002: DAG + Pregel 双模式设计](./0002-graph-engine-design.md)
- **关联**：[编排器重构路线图](../architecture/orchestrator-refactor-roadmap.md)

## 上下文

### 现状

Mady 的编排层目前有 3 种执行模式在运行时独立运作：

| 执行模式 | 状态类型 | 节点签名 | 并发模型 | 检查点 |
|---------|---------|---------|---------|--------|
| `workflow.Pipeline/Parallel/Router` | `string → string` | `Step.Run(ctx, string) (string, error)` | 串行/并行（无治理） | 无 |
| `graph.CompiledGraph`（DAG） | `string → string` | `Step.Run(ctx, string) (string, error)` | 分层并行 | `graph.CheckpointStore` |
| `graph.CompiledPregelGraph` | `PregelState` | `PregelNode(ctx, PregelState) (PregelState, error)` | 超步内并行 | `graph.CheckpointStore`（仅 PregelCheckpointer） |

此外，还有两个使用上述模式的"复合执行器"：

| 复合执行器 | 使用的底层模式 | 状态管理 |
|-----------|-------------|---------|
| `agentcore.PipelineExecutor` | 顺序 stage 分派 | `PipelineState`（独立 map） |
| `domains/reasoning.FiveStepRunner` | Pregel 图（Stage ④）+ 自定义状态机 | `FactBlackboard` + `PregelState` |

### 问题

1. **状态模型不兼容**：`string → string` 和 `PregelState` 之间无法直接组合。一个 DAG 的输出不能直接作为 Pregel 的输入，反之亦然。
2. **检查点不统一**：三套 CheckpointStore 接口（`agentcore.CheckpointSaver`、`reasoning.CheckpointStore`、`graph.CheckpointStore`）语义不同，无法跨层恢复。
3. **执行能力不对齐**：PipelineExecutor 的 tool stage 跳过执行、workflow.Parallel 无超时/重试/并发限制、FiveStepRunner 的 checkpoint 默认内存。
4. **领域代码直接依赖特定运行时**：专利/法律工作流硬编码使用 Pregel，迁移成本高。

这些问题在 [编排器重构路线图](../architecture/orchestrator-refactor-roadmap.md) 中有详细诊断。

### 约束

Mady 是**专利/法律专业 Agent 平台**，不是通用工作流引擎。统一运行时的设计目标不是成为 Temporal/LangGraph 的 Go 移植，而是：

1. **收敛现有模式**：DAG、Pregel、Pipeline 三种执行语义不变，统一它们的运行时契约
2. **领域优先**：复杂度不超过专利审查和案例比对工作流的需求
3. **渐进迁移**：现有 `workflow/` 包的公开 API 不变，内部编译为统一 Plan
4. **不做的事**：不定义 DSL、不引入子图嵌套（v1）、不实现分布式执行、不替换现有 API

## 决策

引入 **ExecutionPlan** 作为 DAG/Pregel/Pipeline 的统一中间表示，**UnifiedRuntime** 作为唯一执行入口。

### 核心接口

```go
// =============================================================================
// ExecutionPlan — 统一中间表示
// =============================================================================

// StepKind 分类节点的执行类型。
type StepKind string

const (
    StepCompute    StepKind = "compute"     // 纯计算节点（PregelNode）
    StepTool       StepKind = "tool"        // 工具调用节点
    StepHumanGate  StepKind = "human_gate"  // 人工确认节点
)

// StepSpec 描述 ExecutionPlan 中的一个步骤。
type StepSpec struct {
    Name     string          // 节点唯一名称
    Kind     StepKind        // 执行类型
    Node     PregelNode      // compute 节点的执行函数
    Tool     ToolCallSpec    // tool 节点的调用参数（Phase 2.2 实现）
    Gate     HumanGateSpec   // human_gate 节点的参数（Phase 3.3 实现）
    Policy   NodePolicy      // 重试/超时/副作用策略（Phase 1.2 已实现）
    Edges    []string        // 静态后继节点名
    CondEdge *CondRouterSpec // 条件边（Pregel 风格路由）
}

// ToolCallSpec 描述工具调用。
type ToolCallSpec struct {
    ToolName string
    InputKey string // 从 state 中读取输入的 key
}

// HumanGateSpec 描述人工确认点。
type HumanGateSpec struct {
    ReviewContextKey string // state 中存储审查上下文的 key
    Timeout          time.Duration
    DefaultDecision  string // "approve" | "skip" | "abort"
}

// CondRouterSpec 条件边路由。
type CondRouterSpec struct {
    Router PregelEdgeRouter
}

// ExecutionPlan 是完整的执行计划。
type ExecutionPlan struct {
    Steps       []StepSpec
    EntryNode   string
    StateSchema *StateSchema // Phase 1.1 已实现
    MaxSteps    int64
    RunID       string // 用于检查点恢复的唯一标识
}

// =============================================================================
// RunEvent — 运行时事件（Phase 4 实现发布）
// =============================================================================

type RunEventType string

const (
    EventStarted       RunEventType = "started"
    EventNodeStarted   RunEventType = "node_started"
    EventNodeFinished  RunEventType = "node_finished"
    EventRetried       RunEventType = "retried"
    EventInterrupted   RunEventType = "interrupted"
    EventResumed       RunEventType = "resumed"
    EventDegraded      RunEventType = "degraded"
    EventFailed        RunEventType = "failed"
    EventCompleted     RunEventType = "completed"
)

type RunEvent struct {
    Type      RunEventType
    Timestamp time.Time
    RunID     string
    NodeName  string
    StepIndex int64
    Duration  time.Duration
    Error     error
    Metadata  map[string]any
}

// =============================================================================
// UnifiedRuntime — 统一执行器
// =============================================================================

// ToolAdapter 将工具调用桥接到统一运行时。
type ToolAdapter interface {
    Execute(ctx context.Context, spec ToolCallSpec, state PregelState) (PregelState, error)
}

// UnifiedRuntime 执行 ExecutionPlan。
type UnifiedRuntime struct {
    checkpoint graph.CheckpointStore // 统一后的检查点存储
    tools      map[string]ToolAdapter
    subscribers []chan<- RunEvent
}

func NewUnifiedRuntime(opts ...RuntimeOption) *UnifiedRuntime

// Run 执行完整的 ExecutionPlan，返回事件通道。
func (r *UnifiedRuntime) Run(ctx context.Context, plan ExecutionPlan, initial PregelState) (<-chan RunEvent, error)

// Resume 从检查点恢复执行。
func (r *UnifiedRuntime) Resume(ctx context.Context, runID string) (<-chan RunEvent, error)

// Subscribe 注册事件订阅者。
func (r *UnifiedRuntime) Subscribe(ch chan<- RunEvent)
```

### 编译器 — 将现有模式映射到 ExecutionPlan

```go
// CompilePipeline 将 workflow.Pipeline 编译为线性 DAG ExecutionPlan。
// Pipeline 的每个 Step 映射为一个 StepSpec{Kind: StepCompute, Node: adaptStep(step)}。
func CompilePipeline(pipeline *workflow.Pipeline) (ExecutionPlan, error)

// CompileDAG 将 graph.CompiledGraph 编译为 ExecutionPlan。
// 保留拓扑分层结构，每层节点可并行执行。
func CompileDAG(g *graph.CompiledGraph) (ExecutionPlan, error)

// CompilePregel 将 graph.CompiledPregelGraph 编译为 ExecutionPlan。
// 保留超步循环语义，条件边映射为 CondRouterSpec。
func CompilePregel(g *graph.CompiledPregelGraph) (ExecutionPlan, error)
```

### 执行模型

UnifiedRuntime 内部复用现有的 Pregel 超步模型（`CompiledPregelGraph.Run()` 的核心逻辑），对三种编译来源统一处理：

1. **Pipeline → 线性 DAG**：每个 Step 独立一个超步，无并行
2. **DAG → 分层并行**：每层节点在同一超步内并行执行
3. **Pregel → 超步循环**：保持现有超步语义，含条件边

关键行为：
- 所有节点共享 `PregelState`（统一状态模型）
- 节点执行策略（重试/超时/副作用）通过 `NodePolicy` 应用
- 状态合并通过 `StateSchema.ReducerFor()` 确定性执行（Phase 1.1）
- 每个状态变迁发布 `RunEvent`
- 检查点在每个超步后自动保存（当 `checkpoint != nil` 时）

## 备选方案

| 方案 | 优点 | 缺点 |
|------|------|------|
| **ExecutionPlan + UnifiedRuntime（已选）** | 统一契约，渐进迁移，不破坏现有 API | 需要编译器层，初期有适配成本 |
| 直接统一为 Pregel（消除 DAG 和 Pipeline） | API 最简 | DAG 的编译时验证（拓扑排序）丢失；string→string 工作流需要全部改写 |
| 引入 Temporal/Cadence 作为运行时 | 耐久执行开箱即用 | 引入重依赖，与项目"去繁就简"理念冲突；专利审查不需要分布式 workflow |
| 保持现状，只修 bug | 零迁移成本 | 维护成本指数增长，新功能无法跨模式复用 |

## 后果

### 正面影响
- 所有工作流走同一执行器，工具调用、人工确认、错误处理统一
- 检查点、事件、降级标记在统一层实现，领域代码无需关心
- 现有 `workflow/` 包 API 不变，内部编译为 ExecutionPlan
- 新增工作流类型只需实现 `→ ExecutionPlan` 编译器

### 负面影响
- 引入 ExecutionPlan 中间表示，增加一层抽象
- `string → string` 到 `PregelState` 的适配需要 adapter 函数（`adaptStep`）
- 编译器层需要维护与底层运行时的同步

### 迁移策略

分阶段执行（详见 [路线图](../architecture/orchestrator-refactor-roadmap.md)）：

1. **Phase 1（已完成）**：StateSchema + Reducer + NodePolicy — 确保执行内核确定性
2. **Phase 2（本 ADR）**：定义接口 + 实现 UnifiedRuntime + 3 个编译器
3. **Phase 3**：统一检查点 + 默认 SQLite 持久化 + HumanGate 原语
4. **Phase 4**：RunEvent 总线 + Trace/Metrics + 降级显式化
5. **Phase 5**：领域工作流迁移到 UnifiedRuntime

### 不做清单

- ❌ 不设计 BPMN/YAML DSL——当前问题不在"描述力不够"
- ❌ 不引入子图嵌套（subgraph）——Phase 2 只做线性收敛
- ❌ 不实现分布式执行——Mady 是单进程 Agent 平台
- ❌ 不替换 `workflow.Pipeline` 等公开 API——内部编译，外部不变
- ❌ 不引入 Temporal/Argo 作为运行时依赖
