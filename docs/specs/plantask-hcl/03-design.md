# PlanTask HCL 设计文档（Design）

> 状态：草案 · 2026-07-31 · Human Owner：[待指派]
> 对应规格：`02-spec.md`。本文件定技术选型、架构、关键算法与安全考量。

## 1. 技术选型

### 1.1 状态机载体：新建 `agentcore/plantask/`，而非扩展 planmode

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| A. 扩展 `agentcore/planmode` | 复用全局 Extension 注册位 | 语义混淆：planmode 是"工具门控"，HCL 是"流程状态机"，两类职责不同；`atomic.Bool` 撑不起 8 态迁移 | ✗ |
| B. 新建 `agentcore/plantask/` | 职责单一；与 tasklist/planmode 平行；完全遵循 Extension 机制 | 需在 bootstrap 加一行注册 | ✓ 采用 |

`plantask` 通过**调用** planmode 的 `Activate()/Deactivate()` 复用门控，
通过**调用** tasklist 的 Store 复用持久化，不修改二者代码（只读依赖）。

### 1.2 中断实现：普通 Tool 返回 InterruptError（废弃 observer 方案）

`workflow_interrupt` 实现为普通 `agentcore.Tool`，其 `Execute` 返回
`agentcore.NewInterruptError("用户请求暂停")`，由用户/TUI 显式触发（非 LLM 自主
调用），走既有中断通道：executor 捕获工具结果的 `r.Err`
（`agent_run_tool.go:181,203-221`）→ `StatusInterrupted` → `Resume()` 重放恢复
（`agentcore/agent_run.go:105-118`）。

> ⚠️ **已确认事实**：`BeforeToolExecution` observer 返回 error **不会**触发中断——
> `agent_run_tool.go:95-107` 仅持久化"工具执行被生命周期钩子阻止"消息并返回
> `("", nil)`；`IsInterrupt` 只在工具**实际执行后**返回的 `r.Err` 上生效
> （`agent_run_tool.go:181`）。早期草案的"observer 返回 InterruptError"方案已废弃。

TUI 触发方式的并发安全（Agent 运行中经 `InvokeTool`（`agent.go:510`）注入工具
调用：注意 `InvokeTool` 直接走 `executor.Execute`，中断状态捕获在 Agent 循环
（`agent_run.go:286-309`）内）留待阶段三实现时验证。

### 1.3 自动触发：Gateway 回调（上一轮路径 B 的修正落地）

`agentcore/gateway.go` 新增字段 `OnHighComplexity func()`（agentcore 层
定义，不 import 外部包），由装配层（`bootstrap/setup.go`）注入闭包：

```go
// bootstrap/setup.go（示意）
gateway.OnHighComplexity = plantaskExt.AutoEnterPlanning
// plantask 内部维护连续 High 计数（默认 N=2），达到门槛且无活动会话时：
//   status: none → Planning + planmode.Activate()
```

计数规则：`ComplexityHigh` 连续 N 轮才触发；中途出现非 High 则清零。
**"轮" = 用户输入 turn**（`AgentRunContext.Turn` 递增粒度），不含 LLM 工具调用
中间轮——`Gateway.BeforeModelCall` 每轮模型调用都执行 `Decide()`，若按模型
调用计数会在单次用户输入内重复计数；计数器维护在 plantask 扩展内，按
`CaseID` 隔离（避免跨案件串扰）。`planmode` 已手动激活（TUI `/plan`）时
不重复进入。

## 2. 分层架构

```
┌─ 交互层 ──────────────────────────────────────────────┐
│ TUI（状态栏 + 审批卡片，复用 ApprovalCard 模式）        │
│ tui/agentadapter/ → EventPlanTask* 订阅                │
└────────────────────────┬──────────────────────────────┘
                         │ EventBus / 用户触发工具
┌─ 核心层 ───────────────▼──────────────────────────────┐
│ agentcore/plantask/（新增）                            │
│  ├─ extension.go    — Extension 入口 + 工具注册        │
│  ├─ session.go      — PlanTaskSession 模型 + 状态机    │
│  ├─ state_machine.go— 迁移矩阵 + Transition() 校验      │
│  ├─ store.go        — Store 接口（SQLite 实现）         │
│  ├─ tool_submit.go / tool_approve.go / tool_reject.go  │
│  │   tool_revise.go / tool_interrupt.go / tool_resume.go
│  │   tool_feedback.go                                  │
│  ├─ planner.go      — Plan→Tasklist 映射 + replan 编排  │
│  └─ auto_enter.go   — Gateway 回调 + N 轮计数           │
└──────────────┬───────────────┬──────────────┬─────────┘
               │               │              │
   调用 Activate/    调用 Store/Event    Planner/Checkpoint
   Deactivate        （只读依赖）        （只读依赖）
┌──────────────▼───────┐ ┌────────▼────────┐ ┌─────────▼─────────┐
│ agentcore/planmode   │ │ agentcore/      │ │ domains/reasoning │
│ （工具门控）           │ │ tasklist（任务） │ │ Planner/FiveStep  │
└──────────────────────┘ └─────────────────┘ └───────────────────┘
```

依赖方向严格单向：`plantask → {planmode, tasklist, reasoning, agentcore}`。
`agentcore` 层只新增 `Gateway.OnHighComplexity` 字段与事件类型，不依赖
`plantask`。

## 3. 关键流程

### 3.1 主流程时序（规划 → 批准 → 执行）

```
用户输入(高复杂度, N轮连续)
  │ Gateway.OnHighComplexity ──▶ plantask.AutoEnterPlanning
  │                              Status=Planning, planmode.Activate()
  ▼
LLM（只读门控内）: 收集事实 → 规则检索 → Planner.GeneratePlan
  │ plan_submit ──▶ plan_submit 工具: Plan.Steps→tasklist TaskCreate
  │                  Status=AwaitingApproval, 事件 PlanTaskStatusChanged
  ▼
用户: plan_approve ──▶ Status=Executing, planmode.Deactivate()
  │                    FiveStepRunner.ContinueFromStage(3)（从规划后开始）
  ▼
执行中: 每完成一步 tasklist TaskUpdate(completed) + CheckpointID 更新
```

### 3.2 打断 → 反馈 → replan 时序

```
执行中
  │ 用户: workflow_interrupt（TUI 快捷键/命令）
  │   ──▶ 普通工具 Execute 返回 InterruptError
  │        executor 捕获 r.Err（agent_run_tool.go:181）
  │        Agent → StatusInterrupted；plantask → AwaitingFeedback
  │        FiveStepRunner.SaveCheckpoint（断点落盘）
  ▼
用户: workflow_feedback {text:"检索范围应含美国同族,重跑:step2"}
  │   ──▶ Status=Replanning
  │        反馈注入 FactBlackboard.AddFact(feedback)
  │        重新 Planner.GeneratePlan(bb, intent)
  │        合并: CompletedIDs 保持 done；未完成步骤按新 Plan 重排；
  │        反馈中显式"重跑:ID"的步骤移出 CompletedIDs
  │        tasklist 同步新增/调整任务
  ▼
  Status=Executing → FiveStepRunner.ContinueFromStage(3)（checkpoint 续跑）
```

### 3.3 replan 合并算法（`planner.go` 核心）

> 本算法同时服务 `workflow_feedback`（Replanning）与 `plan_revise`（经
> Planning 进入）两个场景，差异仅触发状态：前者反馈已执行部分的观察，
> 后者反馈批准前的修改意图。

```
输入: oldPlan(含 CompletedIDs), feedback, bb
1. completed  = oldPlan 中 CompletedIDs 对应的步骤
2. 显式重跑集 = 解析 feedback 中 "重跑:stepA,stepB"
3. keptDone   = completed - 显式重跑集
4. newPlan    = Planner.GeneratePlan(bb, intent)（feedback 已入 blackboard）
5. 若 newPlan 的步骤 ID 与 keptDone 相交 → 移除相交步骤的完成标记
   （路径变更后旧完成不再可信），仅当步骤描述哈希一致时保留 done
6. 输出: 合并后的执行序列 = keptDone(跳过) + newPlan 未完成步骤
7. tasklist 全量同步
```

描述哈希一致判定：步骤 `Order+Strategy+Description` 的 SHA-256，存于
`PlanSnapshot.Steps[].Hash`，供步骤 5 使用。

## 4. 事件与 TUI 接线

| 事件 | TUI 消费 |
|------|---------|
| `PlanTaskStatusChanged` | 状态栏显示当前 HCL 状态 + 当前步骤 |
| `PlanTaskFeedbackAdded` | 聊天流中渲染用户反馈气泡 |
| `PlanTaskInterrupted` | 复用 `EventAgentInterrupt` 的打断提示 + 审批卡片 |

TUI 快捷键映射（`tui/terminal/keymap.json`）：建议
`Ctrl+P` 打断当前执行 / `Ctrl+R` 恢复，第一版可用命令 `/interrupt` `/resume`
替代，快捷键留待阶段三。

## 5. 安全考量

1. **会话隔离**：所有用户触发工具必带 `session_id`，按 `CaseID` 前缀过滤，
   防止跨案件误操作（对齐 `domains/sqlite` 的 case 级索引模式）
2. **状态机防绕过**：`Transition()` 是唯一状态写入路径（私有），工具与
   自动触发都必须经它；迁移矩阵白名单防非法跳转
3. **敏感路径**：早期草案的 observer 位改动已废弃（见 §1.2），
   `agentcore/hooks.go` **不需要改**；仍需改动的敏感路径文件为
   `agentcore/gateway.go`（`OnHighComplexity` 字段，阶段三 3.1）。按 AGENTS.md
   敏感路径规范：提交前人工审阅 + CI 标记，变更说明随 PR（见 04-tasks 阶段零）
4. **反馈注入边界**：反馈文本写入 blackboard 前做长度截断（**2000 rune**，
   按 Unicode 码点计数而非字节，避免中文截断），防止上下文撑爆；不执行反馈
   中的任何指令（仅作事实，由 LLM 理解）
5. **门控不失效**：`plan_submit` 后到批准前，planmode 必须保持激活；
   `Deactivate` 只能由 `plan_approve`/`plan_reject` 触发的迁移执行
6. **审计留痕**：FeedbackLog 与状态迁移历史只增不改，随会话持久化
   （对齐 plantask-introduction 的 archived 而非 delete 原则）

## 6. 测试策略

| 层 | 用例 |
|----|------|
| 单元（state_machine） | 迁移矩阵全分支；非法迁移返回 ErrInvalidTransition |
| 单元（planner） | Plan→Task 映射；replan 合并（含哈希一致/不一致、显式重跑） |
| 单元（auto_enter） | N 轮计数：连续/中断清零/已激活跳过 |
| 单元（store） | Save/Load/ListPending/Delete + 恢复后状态 |
| 集成（integration/） | §3.1+§3.2 全流程；用**带执行日志的测试替身**（recorder builder：记录每次执行的步骤 ID）断言已完成步骤不重跑、反馈进入新 Plan |
| 回归 | `go test -race ./...` 四模块全绿 |

## 7. 实施顺序依赖

```
阶段一（骨架+批准门） → 阶段二（打断+反馈+replan） → 阶段三（自动触发+TUI）
     │                        │                            │
 02-spec §2/§4/§5        02-spec §3/§3.2                gateway 回调 + keymap
```
