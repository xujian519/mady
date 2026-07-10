# ADR-0002: 图引擎采用 DAG + Pregel 双模式设计

- **状态**：Accepted
- **日期**：2026-01-01
- **决策者**：Mady Authors
- **影响范围**：graph/ 模块, 工作流执行

## 上下文

Agent 工作流需要两种执行模式：

1. **有向无环图（DAG）执行**：用于确定性工作流，如 Pipeline（A → B → C）、Parallel（A, B, C 并行执行后合并）。这类工作流没有循环依赖，执行路径是确定的。

2. **带循环的迭代执行**：用于 Agent 主循环（LLM → 工具 → LLM → …），需要在特定条件下继续循环或在满足条件时退出。这类似 Google Pregel 的超步模型（superstep），每个节点在每个超步中可能被激活多次。

需要决定是用单一图模型覆盖所有场景，还是采用两种不同的图模型。

## 决策

采用双模式设计：

- **`graph.Graph`**：静态 DAG 图模型，用于确定性工作流。编译时拓扑排序，运行时按依赖关系调度节点。节点是纯函数（`func(ctx, input) (output, error)`）。

- **`graph.PregelGraph`**：循环图模型，用于迭代工作流。支持条件边（conditional edge），每个超步根据当前状态决定下一步激活哪些节点。节点签名是 `func(ctx, PregelState) (PregelState, error)`，状态在超步间传递。

两种图共享底层的检查点机制（`graph.CheckpointSaver`）。

## 备选方案

| 方案 | 优点 | 缺点 |
|------|------|------|
| **双模式 DAG + Pregel（已选）** | 各取所长，语义清晰 | 两套 API，学习曲线略高 |
| 统一为单一循环图 | API 统一 | 简单工作流（Pipeline）需要处理不必要的循环语义 |
| 使用外部工作流引擎（Temporal、Cadence） | 功能强大 | 引入重依赖，不符合项目理念 |

## 后果

### 正面影响
- DAG 模型天然适合 `workflow.Pipeline` 和 `workflow.Parallel`
- Pregel 模型天然适合 Agent 主循环和复杂的条件图
- 两种图共享检查点接口，持久化和恢复统一
- 编译期可验证 DAG 无环

### 负面影响
- 开发者需要理解两种图模型及其适用场景
- `graph/` 包的 API 表面积较大
- 不能在同一图中混合 DAG 和循环语义（需要外层编排）

### 迁移策略
无需迁移——从项目初期就确定了此设计。
