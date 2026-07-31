# PlanTask HCL（Human-Collaborative Loop）提案

> 状态：草案 · 2026-07-31 · Human Owner：[待指派]
> 上游参考：`docs/specs/plantask-introduction/plan.md`（tasklist 已落地）、
> `docs/hitl/` 三份 HITL 方案（ApprovalGate/PendingStore 已落地，SuspendError 未落地）

## 1. 背景与动机

### 1.1 现状：专业工作流是"一键跑完"

Mady 面向专利/法律场景的工作流执行（`FiveStepRunner` 五步工作法、
`workflows.WorkflowOrchestrator`）当前是**线性一次跑完**的模式：

```
用户输入 → 事实收集 → 规则检索 → 规划 → 执行 → 校验 → 输出
```

用户在这条链路上只有两个干预点：
1. **规划前**：无。规划（Stage ③ `Planner.GeneratePlan`）直接执行，产物不可见
2. **执行中**：无。只能等整体结束；`workflows` 的 `StepHumanApproval` 只是
   步骤间插桩，没有"用户主动打断 → 反馈 → 改路径"的闭环

对于 OA 答复、无效宣告、侵权比对这类**步骤数运行时才能确定、且专业判断
直接影响路径选择**的任务，这带来三个实际问题：

- **信任缺失**：用户不知道 Agent 将要执行什么，只能事后审查产物
- **路径锁定**：执行到一半发现方向错了（如检索范围过窄、比对维度缺失），
  无法中途纠正，只能推倒重来
- **协作缺失**：专利代理的价值在于专业判断，现在的 Agent 是"替代者"
  而非"协作者"

### 1.2 现有资产盘点（代码验证版）

| 资产 | 位置 | 状态 | 在 HCL 中的角色 |
|------|------|------|-----------------|
| `PlanModeExtension` 工具门控（Activate/Deactivate） | `agentcore/planmode/extension.go:46-52` | ✅ 已实现 | 规划态禁止写操作 |
| `tasklist`（4 工具 + FileStore + 事件） | `agentcore/tasklist/` | ✅ 已实现 | 任务清单可视化/持久化 |
| `Planner.GeneratePlan`（模板/KG/LLM 三路） | `domains/reasoning/planner.go:97` | ✅ 已实现 | 规划产物来源 |
| `FiveStepRunner` checkpoint（Save/ContinueFromStage） | `domains/reasoning/checkpoint.go:69-118` | ✅ 已实现 | 断点续跑 |
| `InterruptError` → `StatusInterrupted` → `Resume()` | `agentcore/interrupt.go`、`agentcore/agent_run.go:105-118` | ✅ 已实现 | 执行中打断/恢复 |
| `ApprovalGate` + `ApprovalState` + `PendingStore` | `domains/approval.go`、`domains/approval_state.go` | ✅ 已实现 | 批准语义与持久化的既有先例 |
| `Gateway` 单次复杂度分类 | `agentcore/gateway.go:131` | ✅ 已实现 | 自动进入规划态的触发点 |
| `StepHumanApproval` / `WorkflowOrchestrator` | `workflows/workflow.go:42,299` | ✅ 已实现 | 步骤级审批的既有能力 |

### 1.3 缺口（HCL 需要新建的部分）

| # | 缺口 | 说明 |
|---|------|------|
| H1 | 流程状态机载体 | planmode 只有 `atomic.Bool`，tasklist 只管任务，没有"Planning → AwaitingApproval → Executing → AwaitingFeedback"的单一载体 |
| H2 | 批准/驳回/修改工具 | 无 `plan_approve` / `plan_reject` / `plan_revise` 显式工具 |
| H3 | 执行中打断入口 | 无公开 `Agent.Interrupt()`；打断只能由工具返回 `InterruptError` 触发 |
| H4 | 反馈 → re-plan 闭环 | 用户想法无注入 blackboard 的通道，Planner 无法基于反馈重规划 |
| H5 | 全局自动触发 | 复杂任务不会自动进入规划态（上一轮确认的挂载点是 Gateway） |

### 1.4 借鉴与不借鉴

- **借鉴**：eino `adk/middlewares/plantask` 的工具面设计（已通过
  `plantask-introduction/plan.md` 落地为 Mady 的 tasklist）；Temporal 的
  Signal/Suspend 思想（见 `docs/hitl/Mady HITL 集成方案_参考Loopgate与Temporal.md` 第三阶段）
- **不借鉴**：不做完整 Temporal 移植（无分布式确定性重放需求）；不做
  Loopgate 直接集成（决策理由见 HITL 文档附录 §为什么不用 Loopgate）

## 2. 目标

### 2.1 用户视角目标

| # | 目标 | 用户可见效果 |
|---|------|-------------|
| G1 | 执行前可见规划 | 复杂任务先产出 Plan + 任务清单（tasklist），用户批准前不执行任何写操作 |
| G2 | 批准门 | 用户可 approve / reject（重规划）/ revise（改步骤后继续） |
| G3 | 执行中可打断 | 任意时刻用户可暂停，Agent 停在 `StatusInterrupted`，进度不丢 |
| G4 | 反馈改进路径 | 打断后用户提想法 → 注入 blackboard → 重新规划 → 从变更点增量续跑 |
| G5 | 全局自动触发 | 高复杂度任务自动进入规划态（Gateway 分类 + N 轮确认），无需手动命令 |

### 2.2 成功标准（可衡量）

| 标准 | 衡量方式 |
|------|---------|
| 状态机完整 | `plantask` 包单元测试覆盖全部合法/非法迁移（迁移矩阵 100% 分支覆盖） |
| 批准门有效 | 规划态下所有写工具被门控（复用 planmode Policy 测试）；批准前零写副作用 |
| 打断/续跑不丢状态 | 集成测试：执行中打断 → 注入反馈 → 续跑 → 已完成任务不重跑、blackboard 事实不丢 |
| 零功能退化 | 现有 `go test -race ./...` 全绿；tasklist/planmode/FiveStepRunner 行为不变 |
| 覆盖全局 | 主 Agent 对话循环（不走 FiveStepRunner 工具）也能自动进入规划态 |

### 2.3 非目标（本版本明确不做）

- 不做完整 Temporal 移植（workflow instance / 确定性重放）
- 不做审批仪表盘（`docs/hitl/HITL-ENHANCEMENT-PLAN.md` §8 的 G7 留待后续）
- 不做多用户并发审批（单用户本地 TUI 场景）
- 不改造 `workflows.WorkflowOrchestrator` 的 Pregel 执行路径（HCL 第一版
  与 `FiveStepRunner` 集成；workflows 集成作为阶段三之后的可选项）

## 3. 影响范围预判

- 新增：`agentcore/plantask/`（新包，Extension + 状态机 + 工具 + 事件）
- 修改：`bootstrap/setup.go`（注册一行）、`cmd/mady/tui_session_config.go`
  （状态展示接线）、`agentcore/gateway.go`（`OnHighComplexity` 回调字段）
- 不改：`agentcore/planmode/`、`agentcore/tasklist/`、`domains/reasoning/`
  （均为只读依赖）
