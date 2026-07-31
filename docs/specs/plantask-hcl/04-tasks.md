# PlanTask HCL 任务拆解（Tasks）

> 状态：草案 · 2026-07-31 · Human Owner：[待指派]
> 前置：02-spec.md 的 [NEEDS CLARIFICATION] N1-N4 经人工确认后冻结本拆解。
> 粒度约束：单次改动 3-5 文件（AGENTS.md 任务粒度红线），故按阶段拆为三批。

## 阶段零：敏感路径与基线（前置）

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|---------|---------|
| 0.1 | 准备敏感路径变更说明：`agentcore/gateway.go` 新增 `OnHighComplexity` 字段（阶段三 3.1 用）的用途与影响评估，按 AGENTS.md 流程申请人工审阅 | 说明文档（随 PR 提交） | 人工审阅通过后再动工 |
| 0.2 | 基线回归：`make verify` 四模块全绿，确认改动前状态 | — | 基线记录（含 test-race 时长） |

> 注：早期草案涉及 `agentcore/hooks.go` observer 位的改动已废弃（03-design §1.2），
> 本特性最终只触碰 `agentcore/gateway.go` 一个敏感路径文件。

## 阶段一：plantask 骨架 + 批准门（对应 02-spec §2/§4/§5）

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|---------|---------|
| 1.1 | 新建 `agentcore/plantask/` 包：`session.go`（模型 + 8 态状态机 + `Transition()` 私有写入路径；含 `plan_revise` 语义 = 回 `Planning`） | 新增 `agentcore/plantask/session.go`、`state_machine.go`、`session_test.go` | 迁移矩阵全分支单测通过（含非法迁移 → `ErrInvalidTransition`） |
| 1.2 | `store.go` + SQLite 实现（`plantask_sessions` 表，JSON blob + 索引列，复用 approval_store 的 WAL 模式） | 新增 `agentcore/plantask/store.go`、`store_test.go` | Save/Load/ListPending/Delete 单测通过；恢复后状态正确 |
| 1.3a | Extension 入口 + 提交/批准工具：`extension.go` + `tool_submit.go` + `tool_approve.go` + 测试 | 新增 `agentcore/plantask/extension.go`、`tool_submit.go`、`tool_approve.go`、`extension_test.go` | 工具单测：plan_submit/plan_approve 的状态机校验与错误语义（§6） |
| 1.3b | 驳回/修订工具 + 事件类型：`tool_reject.go` + `tool_revise.go` + `event_types.go`（agentcore 包内新增 `PlanTaskStatusChanged` 等 3 事件）+ 测试 | 新增 `agentcore/plantask/tool_reject.go`、`tool_revise.go`、`agentcore/event_types.go`（追加）、`tool_reject_test.go` | 工具单测：plan_reject → Planning、plan_revise（修改意图 → 重新生成 → 重新提交）；事件载荷正确 |
| 1.4 | `planner.go` 第一步：Plan→tasklist 映射（`plan_submit` 时 TaskCreate，含优先级/依赖）+ StepSnapshot.Hash 计算 | 新增 `agentcore/plantask/planner.go` + `planner_test.go` | 映射单测：Plan.Steps 全量转 Task，blockedBy 正确，Hash 一致 |
| 1.5 | bootstrap 注册 + planmode 门控联动（Planning→Activate，approve/reject→Deactivate） | `bootstrap/setup.go`（+1 行注册）、`agentcore/planmode/extension.go` 不动（只读调用） | 集成：Planning 态写工具被拦截；approve 后放行 |

**阶段一退出标准**：`go test -race ./agentcore/... ./bootstrap/...` 全绿；
`plan_submit → plan_approve` 端到端（noop builder 下执行）跑通。

## 阶段二：打断 + 反馈 + replan 闭环（对应 02-spec §3.2/§3.3）

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|---------|---------|
| 2.1 | `workflow_interrupt` 工具：**普通 `agentcore.Tool`**，`Execute` 返回 `agentcore.NewInterruptError`；会话迁移 `AwaitingFeedback`，事件 `PlanTaskInterrupted`；回归确认 `BeforeToolExecution` 返回 error 不触发中断（`agent_run_tool.go:95-107`） | 新增 `agentcore/plantask/tool_interrupt.go`、`tool_interrupt_test.go` | 单测：工具 Execute 返回 InterruptError → Agent `StatusInterrupted`；会话状态正确 |
| 2.2 | `workflow_feedback` + `workflow_resume` 工具：反馈入 blackboard（截断 2000 rune）→ `Replanning`；resume 直接回 `Executing` | 新增 `agentcore/plantask/tool_feedback.go`、`tool_resume.go` | 单测：空反馈 → `ErrFeedbackEmpty`；resume 状态校验 |
| 2.3 | replan 合并算法（03-design §3.3：keptDone / 显式重跑 / 哈希一致判定） | 修改 `agentcore/plantask/planner.go` | 单测：4 场景（全保留/路径变更/显式重跑/哈希不一致） |
| 2.4 | FiveStepRunner 集成：`SaveCheckpoint`（打断时）+ `ContinueFromStage`（续跑时）；CompletedIDs 回写 tasklist；引入**带执行日志的测试替身**（recorder builder：记录每次执行的步骤 ID，供"不重跑"断言） | 新增 `agentcore/plantask/runner_adapter.go`、`runner_adapter_test.go` | 集成测试：打断→反馈→续跑，基于 recorder 日志断言已完成步骤不重跑 |
| 2.5 | 集成测试：§3.1+§3.2 全流程（integration/ 包，可复用 `domains/reasoning/phase5_test.go` 的 WorkflowTool 模式；用 recorder 替身验证执行日志） | 新增 `integration/plantask_e2e_test.go` | 端到端断言：反馈出现在新 Plan；`重跑:stepID` 语法生效（recorder 日志可见重跑） |

**阶段二退出标准**：`go test -race ./agentcore/... ./integration/...` 全绿；
人工演示：执行中断 → 反馈 → 续跑全流程。

## 阶段三：自动触发 + TUI 接线（对应 02-spec §N4、03-design §1.3/§4）

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|---------|---------|
| 3.1 | `Gateway.OnHighComplexity func()` 字段 + `Decide()` 中触发点 | `agentcore/gateway.go`（+字段与触发）、`gateway_test.go` | 单测：回调在分类 High 时触发、非 High 不触发 |
| 3.2 | `auto_enter.go`：连续 N 轮计数（N 默认 2，可配）+ `AutoEnterPlanning`（含 planmode 已激活跳过） | 新增 `agentcore/plantask/auto_enter.go`、`auto_enter_test.go` | 单测：连续/中断清零/已激活跳过 |
| 3.3 | bootstrap 装配：`gateway.OnHighComplexity = plantaskExt.AutoEnterPlanning` | `bootstrap/setup.go` | 集成：主 Agent 对话循环 High 输入自动进入 Planning |
| 3.4 | TUI 接线：订阅 `PlanTaskStatusChanged` 渲染状态栏；命令 `/interrupt` `/resume`（快捷键 `Ctrl+P`/`Ctrl+R` 进 keymap）。触发方式经 `Agent.InvokeTool`（`agent.go:510`）注入工具调用，中断状态捕获在 Agent 循环内（`agent_run.go:286-309`），并发安全于实现时验证 | `tui/agentadapter/`、`tui/component/`、`tui/terminal/keymap.json` | TUI 手动验证：状态栏实时更新；打断/恢复命令生效 |
| 3.5 | AI_CHANGELOG 记录 + docs/specs/README.md 索引更新 | `docs/decisions/AI_CHANGELOG.md`、`docs/specs/README.md` | 文档一致；make doc-check 通过 |

**阶段三退出标准**：`make verify`（四模块 lint+build+test-race）全绿；
TUI 演示：自然语言高复杂度输入 → 自动规划 → 批准 → 执行 → 打断 → 反馈 → 续跑。

## 风险与备选

| 风险 | 缓解 |
|------|------|
| replan 后已完成步骤结论失效 | 哈希一致判定（03-design §3.3 步骤 5），不一致一律重跑 |
| 打断时机在工具调用中途 | 复用既有 `InterruptError` 通道（executor 已处理 `IsInterrupt` 透传），断点以工具结果边界为准 |
| Gateway 回调触发频率过高 | N 轮计数 + 无活动会话条件（阶段三 3.2） |
| agentcore 层新增字段引发敏感路径 CI 标记 | 按 AGENTS.md 敏感路径流程：变更前后对照说明 + 人工审阅 |
