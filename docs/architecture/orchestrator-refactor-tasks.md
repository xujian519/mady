# 编排器重构 — 可执行任务清单与验收标准

> 关联文档：[编排器重构路线图](./orchestrator-refactor-roadmap.md)
>
> 任务状态：⏳ 待开始 | 🔵 进行中 | ✅ 已完成 | ❌ 已取消

---

## Phase 0：编排基线审计（预估 1 周）

**目标**：产出统一术语表和现状全景图，消除概念混用。

### 任务 0.1 — 术语表定义

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T0.1.1** | 定义 `workflow primitive` / `graph runtime` / `pipeline stage` / `tool invocation` / `checkpoint` / `artifact` / `interrupt` / `degraded` 的精确含义 | 2h | - |
| **T0.1.2** | 在 `docs/architecture/glossary.md` 写入术语表，全文引用统一 | 1h | T0.1.1 |

**验收标准**：
- [ ] 术语表覆盖所有编排概念，每条有英文名 + 中文名 + 一句话定义 + 反例（什么不是它）
- [ ] 团队 code review 中不再出现"这个 orchestrator 是 workflow 还是 pipeline"类问题

### 任务 0.2 — 执行路径全景图

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T0.2.1** | 逐条走读 5 条执行链，画出每条的节点签名 → 状态传递 → 错误传播 → 检查点位置的时序图 | 4h | - |
| **T0.2.2** | 标注每条路径的"保留/待淘汰/待收敛"标签 | 2h | T0.2.1 |
| **T0.2.3** | 写入 `docs/architecture/execution-paths-audit.md` | 2h | T0.2.2 |

5 条执行链清单：
1. `workflow/` — Pipeline / Parallel / Router（`string → string`）
2. `graph/` — DAG CompiledGraph（`string → string`，拓扑分层并行）
3. `graph/` — Pregel CompiledPregelGraph（`PregelState → PregelState`，超步循环）
4. `agentcore/` — PipelineExecutor（`PipelineState → PipelineState`，plugin manifest 驱动）
5. `domains/reasoning/` — FiveStepRunner（FactBlackboard + PregelState 混合，5 阶段）

**验收标准**：
- [ ] 每张时序图标明节点签名、状态类型、错误传播路径、checkpoint 插入点
- [ ] 保留/淘汰/收敛标签有明确理由（一句即可）
- [ ] 文档可作为新成员 onboarding 材料

### 任务 0.3 — 设计文档补全

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T0.3.1** | 编写 `docs/architecture/unified-runtime-design.md`（目标架构的详细设计） | 4h | T0.2.3 |
| **T0.3.2** | 设计统一 Checkpoint 模型草案（合并 3 套现有抽象） | 3h | T0.2.1 |
| **T0.3.3** | 设计统一 State 模型草案（Reducer + Schema） | 3h | T0.2.1 |

**验收标准**：
- [ ] 统一 Checkpoint 模型覆盖 thread/run/step 三级粒度
- [ ] 统一 State 模型定义 Schema 类型、Reducer 类型、冲突策略
- [ ] 设计文档通过团队 review

### Phase 0 DoD

- [ ] `docs/architecture/glossary.md` 存在且被团队认可
- [ ] `docs/architecture/execution-paths-audit.md` 存在且覆盖全部 5 条路径
- [ ] `docs/architecture/unified-runtime-design.md` 存在且含状态/检查点/事件模型
- [ ] 团队能用一张图解释现状，不再混用 orchestrator/pipeline/workflow 概念

---

## Phase 1：执行内核硬化（预估 2 周）

**目标**：消除 Pregel 并发非确定性，引入 StateSchema + Reducer + 节点策略字段。

### 任务 1.1 — StateSchema + Reducer 机制

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T1.1.1** | 定义 `StateSchema` 类型：key → type + reducer + conflict policy | 2h | T0.3.3 |
| **T1.1.2** | 实现 Reducer 基础类型：`lastWriteWins` / `append` / `union` / `mergeMap` / `failOnConflict` | 4h | T1.1.1 |
| **T1.1.3** | 改造 `PregelState` 从 `map[string]any` → 带 Schema 的结构体，保持向后兼容的 `Get`/`Set` 方法 | 4h | T1.1.2 |
| **T1.1.4** | 改造 `CompiledPregelGraph.Run()` 的 merge 逻辑（L246-250）：同超步同 key 多写入走 Reducer，默认 fail-fast | 3h | T1.1.3 |
| **T1.1.5** | 编写并发冲突测试：2 节点同 key 写入 + 不同 key 写入 + 同 key 不同 reducer | 3h | T1.1.4 |

**验收标准**：
- [ ] `PregelState` 提供 `Set(key, value)` 和 `Get(key)` 方法，内部记录写入来源
- [ ] 同超步 2 节点对同 key 的 `lastWriteWins` 写入结果可预测（后写入者胜出，按节点名排序）
- [ ] 同超步同 key 无 reducer → `ErrStateConflict`（fail-fast，非静默覆盖）
- [ ] 新增 `graph/pregel_state_test.go`，覆盖 5 种 reducer + 冲突检测 + 并发安全

### 任务 1.2 — 节点策略字段

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T1.2.1** | 在 `PregelNode` 和 DAG `Step` 上增加 `NodePolicy` 可选字段：`Retry` / `Timeout` / `IdempotencyKey` / `SideEffect` | 2h | - |
| **T1.2.2** | 实现 retry 逻辑：指数退避（base=1s, max=30s, multiplier=2）+ maxRetries | 3h | T1.2.1 |
| **T1.2.3** | 实现 timeout 逻辑：context.WithTimeout 注入，区分 timeout 错误和业务错误 | 2h | T1.2.1 |
| **T1.2.4** | 区分"副作用节点"（标记 `SideEffect: true`）和"纯计算节点"——副作用节点不可重放，必须走 checkpoint 后跳过 | 3h | T1.2.1 |
| **T1.2.5** | 编写策略测试：retry 耗尽 / timeout 触发 / 副作用节点重放跳过 | 3h | T1.2.2-4 |

**验收标准**：
- [ ] Pregel 节点支持 `Retry(3, exponential_backoff)` → 失败自动重试最多 3 次
- [ ] Pregel 节点支持 `Timeout(30s)` → 超时返回 `ErrNodeTimeout`
- [ ] 副作用节点标记后，replay 时自动跳过（返回已缓存的输出）
- [ ] DAG 节点（`graph.Graph`）同样支持策略字段

### Phase 1 DoD

- [ ] Pregel 并发 merge 变成可预测行为（按 Reducer 策略确定性合并）
- [ ] 新增 `graph/pregel_state_test.go` 覆盖冲突/并发/replay
- [ ] 新增 `graph/node_policy_test.go` 覆盖 retry/timeout/side_effect
- [ ] `go test -race ./graph/...` 通过
- [ ] 所有现有测试继续通过（向后兼容——不带 Schema 的 Pregel 按 lastWriteWins 行为降级）

---

## Phase 2：统一运行时（预估 2 周）

**目标**：PipelineExecutor + PregelGraph + FiveStepRunner 收敛到统一 runtime contract。

### 任务 2.1 — 统一 Runtime Contract

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T2.1.1** | 定义 `ExecutionPlan` 接口：`Steps []StepSpec` + `StateSchema` + `CheckpointConfig` | 2h | T0.3.1 |
| **T2.1.2** | 定义 `UnifiedRuntime` 结构体：接收 `ExecutionPlan`，产出 `RunEvent` 流 + 最终 `State` | 4h | T2.1.1 |
| **T2.1.3** | 实现 Plan → Runtime 的编译路径：DAG 编译 / Pregel 编译 / Pipeline 编译 → 统一 `ExecutionPlan` | 6h | T2.1.2 |
| **T2.1.4** | 迁移 `workflow.Pipeline` → DAG 编译路径（`Pipeline.Steps` → 线性 DAG） | 2h | T2.1.3 |
| **T2.1.5** | 迁移 `workflow.Parallel` → DAG 编译路径（`Parallel.Steps` → 同一 layer） | 2h | T2.1.3 |
| **T2.1.6** | 迁移 `workflow.Router` → Pregel 编译路径（`Router.Route` → conditional edge） | 2h | T2.1.3 |
| **T2.1.7** | 编写统一 runtime 集成测试：DAG → Pregel → DAG 链式组合 | 4h | T2.1.4-6 |

**验收标准**：
- [ ] `ExecutionPlan` 是 DAG/Pregel/Pipeline 的统一中间表示
- [ ] `UnifiedRuntime.Run(ctx, plan)` 返回 `(<-chan RunEvent, error)`
- [ ] `workflow.Pipeline.Run()` 内部编译为 DAG ExecutionPlan 并走 UnifiedRuntime
- [ ] `workflow.Parallel.Run()` 同上
- [ ] `workflow.Router.Run()` 编译为 Pregel ExecutionPlan（conditional edge）
- [ ] 现有 `workflow/` 包的公开 API 不变（向后兼容）

### 任务 2.2 — Tool Adapter（tool stage 真执行）

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T2.2.1** | 定义 `ToolAdapter` 接口：`Execute(ctx, ToolCall, State) (ToolResult, error)` | 2h | - |
| **T2.2.2** | 实现 `SearchToolAdapter` — 桥接 retriever 到 tool stage | 3h | T2.2.1 |
| **T2.2.3** | 实现 `ExtractToolAdapter` — 桥接 LLM 提取到 tool stage | 3h | T2.2.1 |
| **T2.2.4** | 改造 `PipelineExecutor.executeStage()` — tool stage 不再跳过，走 ToolAdapter | 3h | T2.2.2-3 |
| **T2.2.5** | 设置 `FailOnUnknown: true` 作为新默认值（未知 stage 硬失败，不再静默跳过） | 1h | T2.2.4 |
| **T2.2.6** | 在 PluginManager 中注册 ToolAdapter 注册表 | 2h | T2.2.4 |

**验收标准**：
- [ ] `pipeline_executor.go:124-129` 的 `continue` 逻辑被替换为 `ToolAdapter.Execute()`
- [ ] 未知 tool 的 stage 默认返回 error（`FailOnUnknown: true`）
- [ ] `search` tool stage 能通过 retriever 真实检索
- [ ] 新增 `agentcore/pipeline_executor_tool_test.go`

### 任务 2.3 — 插件系统执行路径统一

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T2.3.1** | 移除 `agentcore/plugin_manager.go` 中的 `executor *PipelineExecutor` 直接依赖，改为 `runtime *UnifiedRuntime` | 3h | T2.1.2 |
| **T2.3.2** | 插件 manifest 的 `pipeline.stages` 编译为 `ExecutionPlan` | 3h | T2.1.3 |
| **T2.3.3** | 验证所有现有插件（novelty-analysis / infringement-check / oa-response）在新 runtime 下行为一致 | 3h | T2.3.2 |

**验收标准**：
- [ ] 插件执行路径唯一：`plugin spec → compile → UnifiedRuntime`
- [ ] 现有 3 个插件的集成测试通过
- [ ] 删除 `PipelineExecutor` 中的遗留跳过逻辑

### Phase 2 DoD

- [ ] `atom` 和 `tool` 都能走同一执行器
- [ ] 插件不再是"半实现"——tool stage 真实执行
- [ ] `workflow/` 包的公开 API 不变，内部编译到统一 runtime
- [ ] 所有现有测试继续通过

---

## Phase 3：耐久执行与恢复（预估 2 周）

**目标**：中断后重启进程仍可续跑，统一 TUI/Server/ACP 的恢复语义。

### 任务 3.1 — Checkpoint 统一

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T3.1.1** | 定义统一 `Checkpoint` 模型：`RunID + ThreadID + CheckpointID` 三元组 + `StepIndex` + `State` + `Metadata` | 2h | T0.3.2 |
| **T3.1.2** | 合并 3 套 CheckpointStore 接口为一个 `UnifiedCheckpointStore` | 3h | T3.1.1 |
| **T3.1.3** | 以 `domains/reasoning/sqlite/checkpoint_store.go` 为基线，扩展为 `graph/sqlite/unified_checkpoint_store.go` | 4h | T3.1.2 |
| **T3.1.4** | 迁移 `agentcore/checkpoint.go` → 统一接口（MemoryCheckpointSaver 保留用于测试） | 3h | T3.1.2 |
| **T3.1.5** | 迁移 `graph/checkpoint.go` → 统一接口 | 2h | T3.1.2 |

**验收标准**：
- [ ] `UnifiedCheckpointStore` 提供 `Save/Load/List/Delete` + `ListByThread` + `Latest`
- [ ] SQLite 实现通过 WAL 模式支持并发读写
- [ ] 旧接口标记为 `Deprecated`，内部委托到新接口
- [ ] 迁移期间旧测试不中断

### 任务 3.2 — 默认持久化

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T3.2.1** | 修改 `tui_session_config.go:188`：review mode 时使用 SQLiteCheckpointStore 替代 MemoryCheckpointStore | 2h | T3.1.3 |
| **T3.2.2** | 为 Server 模式和 ACP 模式同样接入 SQLite checkpoint | 3h | T3.2.1 |
| **T3.2.3** | 实现 `Resume(ctx, runID)` API，支持从任意 checkpoint 恢复执行 | 4h | T3.2.1 |
| **T3.2.4** | 编写端到端恢复测试：模拟进程崩溃 → 重启 → 从 checkpoint 续跑 | 4h | T3.2.3 |

**验收标准**：
- [ ] review mode 下中断后重启进程，执行从上次 checkpoint 继续，不重复已完成阶段
- [ ] `UnifiedRuntime.Resume(ctx, runID)` 返回 `(<-chan RunEvent, error)`
- [ ] 端到端测试覆盖：中断 → 重启 → 恢复 → 完成 全流程

### 任务 3.3 — Human Gate 运行时原语

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T3.3.1** | 定义 `HumanGate` 接口：`Pause(reason) → WaitForDecision(ctx) → Resume/Skip/Abort` | 2h | - |
| **T3.3.2** | 把 `approvalGateHandler`（`pipeline_stage_handlers.go:600`）和 FiveStepRunner 的 confirmation gate 统一为 `HumanGate` | 3h | T3.3.1 |
| **T3.3.3** | 把散落在领域代码里的"人工确认"分支迁移到 HumanGate | 3h | T3.3.2 |

**验收标准**：
- [ ] 所有"人工确认"场景走同一 `HumanGate` 原语
- [ ] `HumanGate` 决策被记录到 checkpoint（可追溯谁在什么时间做了什么决定）
- [ ] `HumanGate` 支持 timeout → 默认决策（如"超时自动跳过"）

### Phase 3 DoD

- [ ] 中断后重启进程仍可续跑
- [ ] 能查询每个 run 的状态与检查点链（`ListByThread`）
- [ ] review mode 默认持久化（不再是内存）
- [ ] HumanGate 统一所有人工确认场景

---

## Phase 4：可观测与治理（预估 2 周）

**目标**：能回答"这次为什么慢、为什么停、为什么降级、为什么重试"。

### 任务 4.1 — RunEvent 总线

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T4.1.1** | 定义 `RunEvent` 枚举：`started / node_started / node_finished / retried / interrupted / resumed / degraded / failed / completed` | 1h | - |
| **T4.1.2** | 在 UnifiedRuntime 中埋入事件发布点（每个状态变迁发布一个 RunEvent） | 4h | T2.1.2 |
| **T4.1.3** | 实现 `EventSubscriber` 接口：支持 `chan` 订阅 + `callback` 订阅 | 2h | T4.1.2 |
| **T4.1.4** | 为 TUI 的 StatusBar 接入 RunEvent 流，实时显示当前执行阶段 | 3h | T4.1.3 |

**验收标准**：
- [ ] `UnifiedRuntime.Run()` 返回的 channel 包含所有状态变迁事件
- [ ] 事件携带：`timestamp` / `runID` / `nodeName` / `stepIndex` / `duration` / `error`
- [ ] TUI 的 StatusBar 实时更新（如"Stage 3/5: 规则检索中…"）

### 任务 4.2 — Trace 和 Metrics

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T4.2.1** | 为每个 node 执行记录：`start_time` / `end_time` / `duration_ms` / `token_usage` / `retry_count` / `error_type` | 3h | T4.1.2 |
| **T4.2.2** | 接入 OpenTelemetry tracing（项目已有 `tracing/`，扩展 span 到 node 级别） | 3h | T4.2.1 |
| **T4.2.3** | 实现 `RunSummary` 聚合：总耗时、各 node 耗时分布、token 消耗、重试次数、降级次数 | 3h | T4.2.1 |
| **T4.2.4** | 在 GRAPH_REPORT.md 风格的审计报告中输出 RunSummary | 2h | T4.2.3 |

**验收标准**：
- [ ] 每个 run 可查询：哪个 node 最慢、哪个 node 重试最多、token 消耗分布
- [ ] OpenTelemetry span 包含 node 级别的 attribute（node_name / step_index / retry_count）
- [ ] `RunSummary` JSON 可被外部监控系统消费

### 任务 4.3 — 并发治理

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T4.3.1** | 实现 `WorkflowSemaphore`：限制同时执行的 workflow 数量 | 2h | - |
| **T4.3.2** | 实现 `ToolConcurrencyLimit`：限制单个 tool 的并发调用数 | 2h | - |
| **T4.3.3** | 实现 `KeyedMutex`：按 key（如 caseID）串行化执行 | 2h | - |
| **T4.3.4** | 在 `UnifiedRuntime` 中集成并发治理配置 | 2h | T4.3.1-3 |

**验收标准**：
- [ ] 可配置 `max_concurrent_workflows: 3`
- [ ] 可配置 `search_tool_max_concurrency: 2`
- [ ] 同一 `caseID` 的两个请求自动串行化（KeyedMutex）

### 任务 4.4 — 降级显式化

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T4.4.1** | 定义 `DegradationMark` 结构：`reason` / `severity` / `affected_fields` / `manual_action_required` | 1h | - |
| **T4.4.2** | 改造 `analysis.go:149-170` 的降级逻辑：返回 `DegradationMark` 而非静默占位 | 2h | T4.4.1 |
| **T4.4.3** | 改造 `comparison.go:82-87` 的占位逻辑：返回 `DegradationMark` | 1h | T4.4.1 |
| **T4.4.4** | 在 RunSummary 中汇总降级信息，确保调用方可见 | 2h | T4.4.2-3 |

**验收标准**：
- [ ] 检索失败时状态中包含 `_degraded: {reason: "retriever_unavailable", severity: "warning"}`
- [ ] RunSummary 中降级次数 > 0 时，状态标记为 `degraded`（不是 `completed`）
- [ ] 用户可见降级原因和影响范围

### Phase 4 DoD

- [ ] 能回答"这次为什么慢"——RunSummary 含各 node 耗时
- [ ] 能回答"这次为什么停"——中断事件含中断原因和等待的人
- [ ] 能回答"这次为什么降级"——降级标记在状态和报告中可见
- [ ] 能回答"这次为什么重试"——重试事件含 node 名和 attempt 序号

---

## Phase 5：领域工作流迁移（预估 2-3 周）

**目标**：至少 2 条核心业务流不再依赖 placeholder 路径，且可用同一运行态观察与恢复。

### 任务 5.1 — 专利新颖性工作流迁移

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T5.1.1** | 实现 `strict/degraded` 双模式：`strict` 模式下检索失败 → 中断并等待 retriever 恢复；`degraded` 模式下检索失败 → 标记降级但继续 | 3h | T4.4.1 |
| **T5.1.2** | 改造 `analysis.go` 的 `noveltyNode`：接入真实 retriever，移除 nil-retriever 占位 | 3h | T5.1.1 |
| **T5.1.3** | 将 `workflows/patent/analysis.go` 的 Pregel 图编译为 UnifiedRuntime 的 ExecutionPlan | 3h | T2.1.3 |
| **T5.1.4** | 编写专利工作流迁移测试：正常流程 + 检索失败降级 + 中断恢复 | 4h | T5.1.3 |

**验收标准**：
- [ ] `analysis.go:149-170` 的"无检索器占位"和"检索失败占位"不再返回 `nil` error
- [ ] `strict` 模式下检索失败 → `ErrInterrupt(reason: "retriever_unavailable")`
- [ ] `degraded` 模式下检索失败 → 继续但标记 `_degraded`
- [ ] 工作流可在中断后从 checkpoint 恢复

### 任务 5.2 — 法律比较工作流迁移

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T5.2.1** | 实现真实 case/statute retriever（接入 `knowledge/` 的 FTS5 全文搜索） | 4h | - |
| **T5.2.2** | 改造 `comparison.go:82-87` 的 `caseSearchNode`：移除硬编码占位，接入 retriever | 2h | T5.2.1 |
| **T5.2.3** | 将 `workflows/legal/comparison.go` 编译为 UnifiedRuntime 的 ExecutionPlan | 3h | T2.1.3 |
| **T5.2.4** | 编写法律工作流迁移测试 | 3h | T5.2.3 |

**验收标准**：
- [ ] `comparison.go:82-87` 的硬编码字符串被真实 FTS5 检索结果替换
- [ ] 检索失败时走 degraded 模式（同专利工作流）
- [ ] 工作流可在中断后从 checkpoint 恢复

### 任务 5.3 — FiveStepRunner 统一

| ID | 任务 | 预估 | 依赖 |
|----|------|------|------|
| **T5.3.1** | 将 FiveStepRunner 的 5 个阶段编译为 ExecutionPlan（Stage ①-⑤ → Pregel 节点） | 4h | T2.1.3 |
| **T5.3.2** | 让 Stage ④ 的工具编排走 UnifiedRuntime 的 ToolAdapter（不再直连 Pregel） | 3h | T2.2.4 |
| **T5.3.3** | 统一 FiveStepRunner 的 checkpoint 到 UnifiedCheckpointStore | 3h | T3.1.3 |
| **T5.3.4** | 编写 FiveStepRunner 迁移测试（含中断恢复） | 3h | T5.3.1-3 |

**验收标准**：
- [ ] FiveStepRunner 的 `runFrom()` 方法内部使用 UnifiedRuntime
- [ ] FiveStepRunner 的 checkpoint 走 SQLite（不再是内存）
- [ ] 现有 FiveStepRunner 测试继续通过

### Phase 5 DoD

- [ ] 专利新颖性工作流无 placeholder 路径
- [ ] 法律比较工作流无 placeholder 路径
- [ ] 两条核心业务流可用 `UnifiedRuntime.Resume()` 恢复
- [ ] 两条核心业务流产出 RunSummary（含降级信息）

---

## 优先级总览

### P0（必须先行，阻塞后续阶段）

| 任务 | 说明 |
|------|------|
| T1.1.1-5 | StateSchema + Reducer + 冲突检测 |
| T2.2.1-6 | ToolAdapter + tool stage 真执行 |
| T3.2.1-4 | 持久化 checkpoint 默认接入 |

### P1（核心价值，紧随 P0）

| 任务 | 说明 |
|------|------|
| T2.1.1-7 | 统一 Runtime Contract |
| T4.1.1-4 | RunEvent 总线 |
| T4.2.1-4 | Trace + Metrics |
| T4.4.1-4 | 降级显式化 |
| T5.1.1-4 | 专利工作流迁移 |
| T5.2.1-4 | 法律工作流迁移 |

### P2（增强能力，最后完成）

| 任务 | 说明 |
|------|------|
| T2.3.1-3 | 插件 DSL 收敛 |
| T4.3.1-4 | 并发治理（Semaphore/KeyedMutex） |
| T5.3.1-4 | FiveStepRunner 统一 |

---

## 不做清单

- ❌ 不引入 Temporal/Argo 作为运行时依赖
- ❌ 不走 BPMN/YAML-first 大而全 DSL
- ❌ 不新增工作流类型（在 5 条路径收敛前）
- ❌ 不修改 `workflow/` 包的公开 API（内部重构，外部不变）

---

## 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| Phase 1 的 StateSchema 改动破坏现有 Pregel 工作流 | 中 | 高 | 向后兼容 Get/Set 方法 + 默认 lastWriteWins 降级 |
| ToolAdapter 的 retriever 接入引入新依赖问题 | 低 | 中 | 复用现有 `knowledge/` 模块的 FTS5，不引入新外部依赖 |
| Phase 5 领域迁移引入行为变化 | 中 | 中 | strict/degraded 双模式默认走 degraded（行为不变），strict 显式 opt-in |
| 5 阶段路线图周期过长，团队中途被打断 | 中 | 高 | 每个 Phase 独立交付价值，Phase 1 完成即可获得确定性执行收益 |

---

## 一句话路线

**先修内核确定性 → 再统一执行器 → 再补持久化与可观测 → 最后迁移领域工作流。**
