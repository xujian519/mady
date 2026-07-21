# Mady 编排器重构路线图

- **状态**：Proposed
- **日期**：2026-07-21
- **作者**：Mady Authors + AI 协作
- **影响范围**：`graph/`, `workflow/`, `agentcore/`, `domains/reasoning/`, `plugins/`

## 一、动机

Mady 的编排层已积累 5 条独立执行路径（`workflow/` Pipeline/Parallel/Router、`graph/` DAG、`graph/` Pregel、`agentcore/` PipelineExecutor、`domains/reasoning/` FiveStepRunner），三套互不兼容的 checkpoint 抽象，以及 Pregel 并发 merge 的非确定性问题。

本路线图的目标不是换掉现有编排器，而是：
1. 吸收 Temporal、LangGraph、Prefect、Argo 四者的设计原则
2. 收敛多套编排语义到统一内核
3. 分阶段硬化执行器、持久化、可观测性
4. 最终迁移领域工作流到新内核

## 二、现状诊断

### 2.1 已验证的代码级问题

| # | 位置 | 问题 | 严重度 |
|---|------|------|--------|
| 1 | `graph/pregel.go:246-250` | 同超步并发写 PregelState 采用裸 map merge，最终值受 Go map 迭代顺序影响 | **P0** |
| 2 | `agentcore/pipeline_executor.go:124-129` | tool stage 直接跳过（`continue`），且默认 `FailOnUnknown: false` 不报错 | **P0** |
| 3 | `workflows/patent/analysis.go:149-170` | 检索器为 nil 或检索失败时返回占位文本 + `nil` error，调用方无法区分降级 | **P1** |
| 4 | `workflows/legal/comparison.go:82-87` | 判例检索为硬编码占位字符串 | **P1** |
| 5 | `cmd/mady/tui_session_config.go:188` | review mode 默认走 MemoryCheckpointStore，SQLite 实现已存在但未接入 | **P1** |

### 2.2 结构性问题

| # | 问题 | 影响 |
|---|------|------|
| 6 | 5 条执行路径的状态模型互不兼容（`string→string` vs `PregelState` vs `PipelineState` vs `FactBlackboard`） | 无法跨层组合 |
| 7 | 3 套 checkpoint 抽象并存（`agentcore.CheckpointSaver` / `reasoning.CheckpointStore` / `graph.CheckpointStore`），语义不统一 | 无法统一恢复 |
| 8 | `workflow/Parallel` 无 timeout、retry、concurrency limit | 缺少运行时治理 |
| 9 | 降级行为不透明：占位数据、跳过执行、错误吞没均对调用方不可见 | 可信度受损 |

### 2.3 外部对标

| 项目 | 核心启发 | Mady 对应缺口 |
|------|---------|---------------|
| **Temporal** | 确定性重放、副作用隔离（workflow/activity 边界）、版本演进 | Pregel 非确定性 merge、无副作用节点标记 |
| **LangGraph** | StateSchema + Reducer、Checkpointer、Interrupt | PregelState 裸 map、checkpoint 不统一 |
| **Prefect** | 代码优先、运行态一等公民、失败恢复、实时 UI | 无 RunEvent 总线、无可观测性 |
| **Argo** | 并发治理（Mutex/Semaphore）、挂起/恢复、工件 | 无 workflow-level 背压 |

## 三、设计原则

1. **不换赛道**：继续保留 DAG + Pregel 双模式，不引入 Temporal/Argo 作为运行时依赖
2. **先统一语义，再补功能**：先定义统一的状态/节点/检查点模型，再迁移领域工作流
3. **默认安全而非默认降级**：证据缺失、工具未执行、检索失败必须显式暴露
4. **持久化默认开启**：支持人工确认、长运行、跨入口恢复的工作流默认接持久化 store
5. **运行时先于 DSL**：先把执行内核打稳，再谈 plugin DSL 扩展

## 四、目标架构

```
WorkflowSpec (Graph / Pipeline / FiveStep)
        │
        ▼
    Compiler
        │
        ▼
  Execution Plan
        │
        ▼
  Unified Runtime ─────────────────────────┐
   ├── Node Adapter    (纯计算 / 副作用)   │
   ├── Tool Adapter    (真实执行)          │
   ├── Human Gate      (中断/确认/恢复)    │
   ├── Reducer Engine  (确定性合并)        │
   ├── Retry/Timeout   (策略引擎)          │
   ├── Event Store     (RunEvent 总线)     │
   ├── Checkpoint Store (SQLite 持久化)    │
   └── Trace/Metrics   (可观测性)          │
```

## 五、社区对照

- [Temporal Workflow Versioning](https://docs.temporal.io/develop/go/workflows/versioning)
- [LangGraph Overview](https://docs.langchain.com/oss/python/langgraph/overview)
- [LangGraph Durable Execution](https://docs.langchain.com/oss/python/langgraph/durable-execution)
- [LangGraph Graph API](https://docs.langchain.com/oss/python/langgraph/use-graph-api)
- [Prefect v3 Introduction](https://docs.prefect.io/v3/get-started)
- [Argo Synchronization](https://argo-workflows.readthedocs.io/en/latest/synchronization/)
- [ADR-0002: DAG + Pregel 双模式设计](../adr/0002-graph-engine-design.md)
