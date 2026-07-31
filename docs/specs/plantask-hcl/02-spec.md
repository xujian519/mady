# PlanTask HCL 规格说明（Spec）

> 状态：草案 · 2026-07-31 · Human Owner：[待指派]
> 对应提案：`01-proposal.md`。本文件定义输入输出、数据模型、接口契约与验证规则。

## 1. 术语

| 术语 | 含义 |
|------|------|
| PlanTask | Plan（`domains/reasoning.Plan`）与 TaskList（`agentcore/tasklist.Task`）的绑定视图 |
| HCL 会话 | 一次"规划 → 批准 → 执行 → 反馈"闭环的运行时实例（`PlanTaskSession`） |
| 规划态 | 状态 `Planning`，此时 planmode 门控激活（只读），Agent 只允许产出 Plan |
| 反馈 | 用户在 `AwaitingFeedback` 状态下注入的改进意见（自由文本） |

## 2. 核心状态机

### 2.1 状态定义

```
Planning ──plan_submit──▶ AwaitingApproval
AwaitingApproval ──plan_approve──▶ Executing
AwaitingApproval ──plan_reject──▶ Planning          (重规划)
AwaitingApproval ──plan_revise──▶ Planning          (修改意图 → 重新生成 → plan_submit)
Executing ──workflow_interrupt──▶ AwaitingFeedback
AwaitingFeedback ──workflow_feedback──▶ Replanning
Replanning ──(replan 完成)──▶ Executing             (从变更点增量续跑)
AwaitingFeedback ──workflow_resume──▶ Executing     (无改动直接续跑)
Executing ──(执行完成)──▶ Finished
任意状态 ──plan_cancel──▶ Canceled
任意状态 ──(超时)──▶ Expired
```

### 2.2 迁移矩阵（合法迁移白名单）

| from \ to | Planning | AwaitingApproval | Executing | AwaitingFeedback | Replanning | Finished | Canceled | Expired |
|-----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| (init) | ✅ | | | | | | | |
| Planning | | ✅ | | | | | ✅ | ✅ |
| AwaitingApproval | ✅ | | ✅ | | | | ✅ | ✅ |
| Executing | | | | ✅ | | ✅ | ✅ | ✅ |
| AwaitingFeedback | | | ✅ | | ✅ | | ✅ | ✅ |
| Replanning | | | ✅ | | | | ✅ | ✅ |

- 非法迁移必须返回明确错误（`ErrInvalidTransition{from,to}`）
- 任何导致 Plan 内容变更的操作（`plan_revise` / `plan_reject`）必须回到
  `Planning` 重新生成，并经 `plan_submit` 重新提交，保证 Plan 变更经过完整门控；
  `AwaitingApproval` 内不存在停留原状态的步骤修改（`plan_revise` 的语义 =
  修改意图 → 重新生成 → 重新提交，见 §9 N2）

### 2.3 状态持久化与恢复

- 会话状态经 `PendingStore`-style 接口持久化（复用 `domains/approval.go`
  已建立的 pending 模式：活动状态写 SQLite，已决写审计表）
- 进程重启后 `ListPending()` 恢复未完成会话，回到中断时的状态
- 超时机制：`AwaitingApproval` / `AwaitingFeedback` 支持 `ExpiresAt`，
  过期迁移到 `Expired`（对应用户不响应场景）。默认超时：
  `AwaitingApproval` 24h / `AwaitingFeedback` 1h（可配，见 §9 N3）

## 3. 数据模型

### 3.1 PlanTaskSession

```go
// agentcore/plantask/session.go

type Status string

const (
    StatusPlanning         Status = "planning"
    StatusAwaitingApproval Status = "awaiting_approval"
    StatusExecuting        Status = "executing"
    StatusAwaitingFeedback Status = "awaiting_feedback"
    StatusReplanning       Status = "replanning"
    StatusFinished         Status = "finished"
    StatusCanceled         Status = "canceled"
    StatusExpired          Status = "expired"
)

// PlanTaskSession 是一次 HCL 闭环的运行时实例。
type PlanTaskSession struct {
    ID           string            `json:"id"`            // 会话 ID（会话内唯一，按 case 前缀）
    CaseID       string            `json:"case_id"`
    CaseType     string            `json:"case_type"`
    Status       Status            `json:"status"`
    Plan         PlanSnapshot      `json:"plan"`          // 当前批准的 Plan
    TaskIDs      []string          `json:"task_ids"`      // 关联的 tasklist 任务 ID 列表
    CompletedIDs []string          `json:"completed_ids"` // 已完成的步骤 ID（replan 增量依据）
    FeedbackLog  []FeedbackEntry   `json:"feedback_log"`  // 全部反馈（审计留痕）
    CheckpointID string            `json:"checkpoint_id"` // FiveStepRunner 断点 ID
    Interrupt    *InterruptEntry   `json:"interrupt,omitempty"` // 当前中断上下文
    ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
    CreatedAt    time.Time         `json:"created_at"`
    UpdatedAt    time.Time         `json:"updated_at"`
}

type PlanSnapshot struct {
    PlanID string `json:"plan_id"`
    Steps  []StepSnapshot `json:"steps"` // Plan.Steps 的快照
    JSON   []byte `json:"json"`          // 完整 Plan 序列化（供恢复后重建）
}

type StepSnapshot struct {
    Order       int    `json:"order"`
    Strategy    string `json:"strategy"`
    Description string `json:"description"`
    Hash        string `json:"hash"` // Order+Strategy+Description 的 SHA-256
}
// Hash 由 planner 在快照时计算（03-design §3.3 步骤 5 的"已完成步骤是否可信"判定用）

type FeedbackEntry struct {
    At      time.Time `json:"at"`
    Text    string    `json:"text"`    // 用户反馈原文
    StepID  string    `json:"step_id"` // 反馈针对的步骤（可为空=全局）
}

type InterruptEntry struct {
    StepID string `json:"step_id"` // 中断时的步骤
    Reason string `json:"reason"`  // 中断原因（用户可见）
    Data   map[string]any `json:"data,omitempty"`
}
```

### 3.2 状态存储接口

```go
// agentcore/plantask/store.go

// Store 管理 PlanTaskSession 的持久化。
// 生命周期语义同 ApprovalGate 的 PendingStore：活动状态可更新/删除，已决只读。
type Store interface {
    Save(ctx context.Context, s *PlanTaskSession) error
    Load(ctx context.Context, id string) (*PlanTaskSession, error)
    ListPending(ctx context.Context) ([]*PlanTaskSession, error) // 启动恢复
    ListByCase(ctx context.Context, caseID string) ([]*PlanTaskSession, error)
    Delete(ctx context.Context, id string) error
}
```

实现：SQLite（复用 `domains/sqlite/approval_store.go` 的建表与 WAL 模式），
新增 `plantask_sessions` 表（JSON blob + 索引列，与 `pending_approvals`
同一套模式）。

## 4. 工具接口（agentcore.Tool）

| 工具 | ReadOnly | 触发方 | 语义 |
|------|:---:|------|------|
| `plan_submit` | false | LLM | 将当前 Plan + tasklist 提交为 `AwaitingApproval`（含步骤摘要供用户审阅） |
| `plan_approve` | false | 用户 | `AwaitingApproval → Executing`，解除 planmode 门控，开始执行 |
| `plan_reject` | false | 用户 | `AwaitingApproval → Planning`，保留反馈通道（reject 必带理由） |
| `plan_revise` | false | 用户 | 修改步骤列表（增/删/调序）后回到 `AwaitingApproval` |
| `workflow_interrupt` | false | 用户/TUI | 请求暂停：`Executing → AwaitingFeedback`，触发 Agent 中断 |
| `workflow_resume` | false | 用户 | `AwaitingFeedback → Executing`，从 checkpoint 续跑 |
| `workflow_feedback` | false | 用户 | `AwaitingFeedback → Replanning`，注入反馈文本 |

约定：

- **用户触发型工具（approve/reject/revise/interrupt/resume/feedback）的
  输入参数中必须含 `session_id`**，由 TUI/交互层传入，防止串会话
- `workflow_interrupt` 实现为**普通 `agentcore.Tool`**，其 `Execute` 返回
  `agentcore.NewInterruptError`，走既有中断通道（详见 03-design §1.2）。
  不作为 LifecycleHook observer 返回 error——该路径（`agent_run_tool.go:95-107`）
  只记录"被钩子阻止"消息，不会触发 `StatusInterrupted`
- 全部工具在 planmode 门控之外独立工作（plan_submit 规划态可用，
  其余按状态机校验）；planmode 的 `alwaysAllowed` 白名单中追加
  本组工具名，避免与门控互斥
- 工具错误返回 `plan_task.ErrXxx` 语义错误（见 §6），供 LLM 向用户解释

## 5. 事件类型（agentcore/event_types.go 新增）

| 事件 | 载荷要点 | 消费方 |
|------|---------|--------|
| `EventPlanTaskStatusChanged` | `session_id`、`from_status`、`to_status`、`plan` | TUI 状态栏、SSE |
| `EventPlanTaskFeedbackAdded` | `session_id`、`entry` | TUI 反馈气泡、审计 |
| `EventPlanTaskInterrupted` | `session_id`、`InterruptEntry` | TUI 打断提示（复用 `EventAgentInterrupt` 语义） |

事件在每次状态迁移时经 `Agent.EmitEvent` 发出（与 `ApprovalPromptEvent`
同模式）。

## 6. 错误语义

| 错误 | 触发场景 |
|------|---------|
| `ErrNoActiveSession` | 工具调用时无匹配 session（ID 错误或已 Finished） |
| `ErrInvalidTransition{from,to}` | 状态迁移不在 §2.2 白名单内 |
| `ErrSessionExpired` | 会话已超时（`Expired`） |
| `ErrPlanNotApproved` | `Executing` 状态下尝试 approve 类操作 |
| `ErrFeedbackEmpty` | `workflow_feedback` 文本为空 |

## 7. 与既有模块的边界契约

| 模块 | 依赖方向 | 契约 |
|------|---------|------|
| `agentcore/planmode` | plantask → planmode | plantask 在 Planning 态调 `Activate()`、批准后调 `Deactivate()`；不改 planmode 代码 |
| `agentcore/tasklist` | plantask → tasklist | plantask 将 Plan.Steps 映射为 Task（`TaskCreate`），执行进度回写 `TaskUpdate`；不改 tasklist 代码 |
| `domains/reasoning` | plantask → reasoning | 规划调 `Planner.GeneratePlan`；反馈注入 `FactBlackboard.AddFact`（`fact_blackboard.go`）；续跑调 `FiveStepRunner.SaveCheckpoint/ContinueFromStage`；不改 reasoning 代码 |
| `agentcore` | plantask → agentcore | 状态机实现为 `agentcore.Extension` + 工具；中断经既有 `InterruptError` 通道 |
| `agentcore/gateway` | gateway → plantask（回调） | `Gateway.OnHighComplexity func()` 由装配层注入 `plantask.AutoEnterPlanning` |

## 8. 验证规则

1. 状态机：迁移矩阵全部合法/非法组合的单元测试
2. 门控：Planning 态下写工具被 planmode 拦截（复用 `planmode.Policy.Decide` 测试），批准后放行
3. 持久化：Save/Load/ListPending/恢复后状态正确；重启后未决会话可恢复
4. 端到端（integration）：`plan_submit → plan_approve → 执行 → workflow_interrupt →
   workflow_feedback → replan → 续跑 → Finished`，断言已完成步骤不重跑、
   反馈出现在新 Plan 中
5. 零退化：`go test -race ./...`（根模块 + tools/tui/desktop 子模块）全绿

## 9. [NEEDS CLARIFICATION]

以下为**建议结论**（评审采纳），仍需人工 Sign-off 后冻结：

- **N1**：replan 时已完成步骤默认保持 done，仅未完成步骤重新规划；用户可在
  反馈文本中用 `重跑:步骤ID` 语法显式要求重跑（算法见 03-design §3.3）
- **N2**：`plan_revise` 采用 **LLM 辅助**：revise 参数 = 用户修改意图，LLM 生成
  新 Plan，语义为 `AwaitingApproval → Planning → 重新生成 → plan_submit →
  AwaitingApproval`（§2.1 已按此修订，与 replan 共用 03-design §3.3 算法，
  仅触发状态不同）
- **N3**：`ExpiresAt` 默认值 AwaitingApproval 24h / AwaitingFeedback 1h（已写入 §2.3），**待确认**
- **N4**：自动进入规划态门槛：**轮 = 用户输入 turn**（`AgentRunContext.Turn`
  粒度，不含 LLM 工具调用中间轮）；计数器维护在 plantask 扩展内，按
  `CaseID` 隔离；连续 High N=2 触发；`planmode` 已手动激活时跳过（已写入 03-design §1.3）
