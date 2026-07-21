# 编排器重构 — 可验证检查清单

> 关联文档：[路线图](./orchestrator-refactor-roadmap.md) | [任务清单](./orchestrator-refactor-tasks.md)
>
> 此清单用于 PR review 和里程碑验收，每个检查项可独立验证。

---

## Phase 0 检查清单：编排基线审计

### 文档产出
- [ ] `docs/architecture/glossary.md` 存在，定义 ≥10 个核心术语
- [ ] `docs/architecture/execution-paths-audit.md` 存在，覆盖全部 5 条执行链
- [ ] `docs/architecture/unified-runtime-design.md` 存在，含状态/检查点/事件模型设计
- [ ] 执行路径审计文档中，每条路径标注了"保留/待淘汰/待收敛"

### 概念统一
- [ ] 代码库中不再出现 `orchestrator` 和 `pipeline` 混用（grep 验证）
- [ ] PR review 中至少 2 名团队成员能正确使用术语表

---

## Phase 1 检查清单：执行内核硬化

### StateSchema + Reducer

- [ ] `PregelState` 不再暴露裸 `map[string]any`，改为带 Schema 的结构体
- [ ] 运行以下测试并通过：
  ```bash
  go test -race -run TestPregelState ./graph/...
  ```
- [ ] 以下场景有独立测试用例：

| 场景 | 测试函数名 | 通过 |
|------|-----------|------|
| 2 节点同 key 写入（lastWriteWins） | `TestPregelState_ConcurrentWrite_LastWriteWins` | - [ ] |
| 2 节点同 key 写入（append） | `TestPregelState_ConcurrentWrite_Append` | - [ ] |
| 2 节点同 key 写入（union） | `TestPregelState_ConcurrentWrite_Union` | - [ ] |
| 2 节点同 key 写入（mergeMap） | `TestPregelState_ConcurrentWrite_MergeMap` | - [ ] |
| 2 节点同 key 写入（无 reducer → fail） | `TestPregelState_ConcurrentWrite_Conflict` | - [ ] |
| 不同 key 并发写入（无冲突） | `TestPregelState_ConcurrentWrite_NoConflict` | - [ ] |
| 100 goroutine 并发安全性 | `TestPregelState_Race` | - [ ] |
| Clone 后修改不影响原 state | `TestPregelState_CloneIsolation` | - [ ] |

### 节点策略

- [ ] `PregelNode` 支持 `Retry(maxAttempts=3, backoff=exponential)`
- [ ] `PregelNode` 支持 `Timeout(duration=30s)`
- [ ] `PregelNode` 支持 `SideEffect: true` — replay 时跳过
- [ ] 运行以下测试并通过：
  ```bash
  go test -race -run TestNodePolicy ./graph/...
  ```

| 场景 | 测试函数名 | 通过 |
|------|-----------|------|
| 失败 → 重试 3 次后成功 | `TestNodePolicy_RetrySuccess` | - [ ] |
| 失败 → 重试耗尽 → 返回 error | `TestNodePolicy_RetryExhausted` | - [ ] |
| 超时 → ErrNodeTimeout | `TestNodePolicy_Timeout` | - [ ] |
| 副作用节点 → replay 时自动跳过 | `TestNodePolicy_SideEffectReplay` | - [ ] |

### 向后兼容

- [ ] 运行全量测试：
  ```bash
  go test -race ./graph/... ./agentcore/... ./domains/reasoning/...
  ```
- [ ] 运行集成测试：
  ```bash
  go test -race ./integration/...
  ```
- [ ] 现有 283 个测试全部通过（无退化）

---

## Phase 2 检查清单：统一运行时

### ExecutionPlan 编译

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| `workflow.Pipeline` → DAG 编译 | `go test -run TestPipelineToDAG ./workflow/...` | - [ ] |
| `workflow.Parallel` → 同一 layer 编译 | `go test -run TestParallelToDAG ./workflow/...` | - [ ] |
| `workflow.Router` → Pregel conditional edge 编译 | `go test -run TestRouterToPregel ./workflow/...` | - [ ] |
| DAG → Pregel → DAG 链式组合 | `go test -run TestChainedExecutionPlan ./agentcore/...` | - [ ] |
| 现有 `workflow/` 公开 API 测试不变 | `go test ./workflow/...` | - [ ] |

### ToolAdapter

- [ ] `agentcore/pipeline_executor.go` 中不再存在 `"tool-based execution not yet implemented"` 字符串
- [ ] `FailOnUnknown` 默认值为 `true`（新默认值）

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| search tool stage 真实检索 | `go test -run TestToolAdapter_Search ./agentcore/...` | - [ ] |
| extract tool stage 真实提取 | `go test -run TestToolAdapter_Extract ./agentcore/...` | - [ ] |
| 未知 tool → 返回 error（非静默跳过） | `go test -run TestToolAdapter_Unknown ./agentcore/...` | - [ ] |
| tool 执行失败 → 错误传播到 RunEvent | `go test -run TestToolAdapter_Error ./agentcore/...` | - [ ] |

### 插件系统

- [ ] 现有 3 个插件的测试通过：
  ```bash
  go test -run TestPlugin ./agentcore/...
  ```
- [ ] 插件执行不再跳过任何 tool stage

---

## Phase 3 检查清单：耐久执行与恢复

### 持久化

- [ ] `tui_session_config.go` 中 review mode 使用 `SQLiteCheckpointStore`（grep 确认 `NewMemoryCheckpointStore` 不出现在 tui_session_config.go）
- [ ] Server 模式使用 `SQLiteCheckpointStore`

### 恢复能力

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| 中断 → 保存 checkpoint | `go test -run TestCheckpoint_Save ./graph/...` | - [ ] |
| checkpoint → 恢复 → 继续执行 | `go test -run TestCheckpoint_Resume ./graph/...` | - [ ] |
| 恢复后不重复已完成阶段 | `go test -run TestCheckpoint_NoDuplicateStage ./graph/...` | - [ ] |
| 进程重启后从 SQLite 恢复 | `go test -run TestCheckpoint_SQLitePersist ./graph/...` | - [ ] |
| 端到端：中断 → 重启 → 恢复 → 完成 | `go test -run TestE2E_ResumeAfterCrash ./integration/...` | - [ ] |

### HumanGate

- [ ] 所有"人工确认"场景走 `HumanGate` 接口（grep `"人工确认"` → 全部通过 HumanGate）
- [ ] HumanGate 决策被记录到 checkpoint（`metadata.human_decisions` 非空）
- [ ] HumanGate 支持 timeout → 默认决策

---

## Phase 4 检查清单：可观测与治理

### RunEvent

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| 所有 9 种事件类型可触发 | `go test -run TestRunEvent_AllTypes ./agentcore/...` | - [ ] |
| 事件携带 timestamp | `go test -run TestRunEvent_Timestamp ./agentcore/...` | - [ ] |
| 事件 channel 不阻塞（buffer ≥ 100） | `go test -run TestRunEvent_NonBlocking ./agentcore/...` | - [ ] |
| TUI StatusBar 实时更新 | 手动启动 `mady tui`，观察 StatusBar | - [ ] |

### Trace/Metrics

- [ ] 每个 node 执行记录包含 `start_time` / `end_time` / `duration_ms` / `token_usage`
- [ ] `RunSummary` JSON 可被外部消费（含 schema 定义）
- [ ] OpenTelemetry span 包含 `node_name` / `step_index` / `retry_count` attribute

### 降级显式化

- [ ] `analysis.go` 检索失败时状态中包含 `_degraded` 字段
- [ ] `comparison.go` 占位被 `_degraded` 替换
- [ ] 降级时 RunSummary 的 status 为 `degraded`（不是 `completed`）

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| retriever 为 nil → degraded（非占位） | `go test -run TestDegradation_RetrieverNil ./workflows/patent/...` | - [ ] |
| retriever 错误 → degraded（非占位） | `go test -run TestDegradation_RetrieverError ./workflows/patent/...` | - [ ] |
| case search 占位 → degraded | `go test -run TestDegradation_CaseSearch ./workflows/legal/...` | - [ ] |

### 并发治理

- [ ] `max_concurrent_workflows: 3` 配置生效
- [ ] `search_tool_max_concurrency: 2` 配置生效
- [ ] 同一 `caseID` 的两个请求自动串行

---

## Phase 5 检查清单：领域工作流迁移

### 专利工作流

- [ ] `analysis.go` 中不再存在"检索结果将在此处展示"字符串
- [ ] `analysis.go` 中不再存在"检索暂时不可用"字符串
- [ ] `strict` 模式下检索失败 → `ErrInterrupt`
- [ ] `degraded` 模式下检索失败 → 继续但含 `_degraded` 标记

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| 正常流程（检索成功） | `go test -run TestPatentWorkflow_Normal ./workflows/patent/...` | - [ ] |
| 降级流程（检索失败） | `go test -run TestPatentWorkflow_Degraded ./workflows/patent/...` | - [ ] |
| 严格流程（检索失败 → 中断） | `go test -run TestPatentWorkflow_Strict ./workflows/patent/...` | - [ ] |
| 中断 → 恢复 → 完成 | `go test -run TestPatentWorkflow_Resume ./workflows/patent/...` | - [ ] |

### 法律工作流

- [ ] `comparison.go` 中不再存在"类似判例检索（查询："字符串
- [ ] `comparison.go` 中不再存在"类似判例检索结果"字符串

| 验证项 | 命令/方式 | 通过 |
|--------|----------|------|
| 正常流程（FTS5 检索成功） | `go test -run TestLegalWorkflow_Normal ./workflows/legal/...` | - [ ] |
| 降级流程（检索失败） | `go test -run TestLegalWorkflow_Degraded ./workflows/legal/...` | - [ ] |
| 中断 → 恢复 → 完成 | `go test -run TestLegalWorkflow_Resume ./workflows/legal/...` | - [ ] |

### FiveStepRunner 统一

- [ ] `FiveStepRunner.runFrom()` 使用 UnifiedRuntime
- [ ] FiveStepRunner checkpoint 走 SQLite
- [ ] 现有 `domains/reasoning/` 测试全部通过

---

## 全局回归检查

每个 Phase 完成后执行：

```bash
# 全量 race 测试
go test -race ./...

# 集成测试
go test -race ./integration/...

# 编译验证
go build ./...

# Lint
make verify
```

- [ ] Phase 0 全局回归通过
- [ ] Phase 1 全局回归通过
- [ ] Phase 2 全局回归通过
- [ ] Phase 3 全局回归通过
- [ ] Phase 4 全局回归通过
- [ ] Phase 5 全局回归通过

---

## 代码审查关注点

每个 PR 的 reviewer 额外检查：

- [ ] 新增的 state key 是否在 Schema 中声明
- [ ] 新增的 node 是否有对应的 RunEvent 发布
- [ ] 副作用节点是否标记 `SideEffect: true`
- [ ] 降级行为是否显式标记 `_degraded`
- [ ] 错误传播是否走统一 `NodeError` 而非裸 `fmt.Errorf`
- [ ] checkpoint 是否在合适的边界保存（不过频也不遗漏）
