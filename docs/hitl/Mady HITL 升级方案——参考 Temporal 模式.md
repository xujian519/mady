# Mady HITL 升级方案——参考 Temporal 模式

> 基于对 Mady 项目结构和 Temporal HITL 模式的深度分析，提出渐进式改造方案。

---

## 一、现状评估：Mady 已有的 HITL 能力

Mady 已经在核心层面植入了 HITL 意识，这是显著优势：

| 已有组件 | 作用 | 所在位置 |
|----------|------|----------|
| `ApprovalGate` (LifecycleHook) | 拦截 Agent 关键决策点，暂停等待确认 | `domains/approval/` |
| `ApprovalState` 状态机 | `drafted → pending_approval → approved/modified/rejected/canceled/expired` | `domains/approval/` |
| `ApprovalStore` (接口 + SQLite) | 持久化审批记录 | `domains/approval/` |
| `Disclosure ReviewGate` | Pregel 图中断等待人工决策 | `disclosure/graph/review_gate.go` |
| `EventBus` | 解耦事件驱动 | `graph/` |
| Pregel 图引擎 | 已有 DAG + 超步迭代执行 | `graph/pregel/` |

**不足**（与 Temporal 对比）：

| 维度 | Temporal | Mady 现状 |
|------|----------|-----------|
| **暂停持久化** | Workflow 状态全持久化，进程重启恢复 | Approval 在内存中，进程重启 pending 消失 |
| **Event History** | 完整事件日志，支持 Replay | EventBus 事件无持久化 |
| **Signal 原语** | `waitForSignal()` 原生暂停/恢复 | 轮询 + channel 阻塞，无原生挂起 |
| **长等待** | 数天-数年原生支持 | 无超时/过期/升级机制 |
| **工作流暂停** | 整个 Workflow 可暂停 | 仅单个 Gate/Hook 点暂停 |
| **通知层** | 需自建 | 无统一通知接口 |
| **可观测性** | Web UI 可视化所有等待状态 | 无集中式等待审批仪表盘 |

---

## 二、升级方案（分阶段）

### 第一阶段：持久化 Workflow State（基础，< 1 周）

将 Pregel 图的执行状态持久化到 SQLite，使工作流可跨进程恢复。

**新增表结构**：

```sql
-- 工作流实例
CREATE TABLE workflow_instances (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,         -- 'disclosure', 'patent_analysis', 'legal_research'
    status      TEXT NOT NULL DEFAULT 'running',  -- running/paused/completed/aborted
    state_json  TEXT NOT NULL,         -- 完整的图执行状态（节点结果、当前节点、上下文）
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT                  -- 超时时间
);

-- 审批挂起点
CREATE TABLE workflow_suspend_points (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflow_instances(id),
    node_id         TEXT NOT NULL,         -- Pregel 节点 ID
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending/approved/rejected/expired
    request_payload TEXT NOT NULL,         -- 提交给人类的信息
    response_payload TEXT,                 -- 人类决策结果
    assigned_to     TEXT,                  -- 分配给谁审批
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    responded_at    TEXT,
    expires_at      TEXT                   -- 超时时间，空=不超时
);

-- 事件日志（Event History）
CREATE TABLE workflow_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_id TEXT NOT NULL REFERENCES workflow_instances(id),
    event_type  TEXT NOT NULL,         -- 'node_enter', 'node_exit', 'llm_call', 'human_signal', etc.
    payload     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_workflow_events_wf ON workflow_events(workflow_id, created_at);
```

**关键代码变更**：

```go
// graph/executor.go — 持久化执行器包装

type PersistentExecutor struct {
    inner   graph.Executor
    store   WorkflowStore       // SQLite 实现
    eventLog EventLogStore
}

func (e *PersistentExecutor) Execute(ctx context.Context, wf *WorkflowInstance) error {
    // 1. 保存初始状态到 SQLite
    e.store.Save(wf)

    // 2. 执行每个节点后保存 checkpoint
    result, err := e.inner.ExecuteNode(ctx, wf.CurrentNode, wf.State)
    wf.State = result.NewState
    wf.CurrentNode = result.NextNode
    e.store.Save(wf)             // checkpoint
    e.eventLog.Append(wf.ID, "node_exit", result)

    // 3. 遇到 SuspendPoint 则暂停并持久化
    if result.IsSuspend {
        wf.Status = "paused"
        e.store.Save(wf)
        return ErrWorkflowSuspended  // 外部处理
    }
}
```

**改动范围**：
- 新增 `store/` 或复用 `domains/approval/` 的 SQLite 能力
- `graph/` 中新增 `persistent.go`
- 修改 Pregel 执行器，在节点切换时 checkpoint

---

### 第二阶段：Signal/Suspend 原语（核心 HITL，1-2 周）

引入 Temporal 风格的 Signal 模式：工作流在指定点自动暂停，等待外部信号恢复。

**1. SuspendNode — Pregel 图中的暂停节点**

```go
// graph/nodes/suspend_node.go

type SuspendNode struct {
    ID          string
    RequestFn   func(ctx context.Context, state *State) (*SuspendRequest, error)
    ResumeFn    func(ctx context.Context, state *State, signal *Signal) (*State, error)
    Timeout     time.Duration  // 超时后自动处理
    OnTimeout   Signal         // 超时后的默认信号
}

type SuspendRequest struct {
    Title       string
    Message     string
    Options     []string          // 给人类的选项
    Assignee    string            // 谁应该审批
    Metadata    map[string]any    // 附加上下文
    ExpiresAt   time.Time
}

type Signal struct {
    Action  string            // "approved", "rejected", "modified"
    Payload map[string]any    // 人类提供的数据
    By      string            // 操作人
    At      time.Time
}
```

**2. 在 Pregel 图中的用法**

```go
// disclosure/graph/main.go
graph.AddNode(&SuspendNode{
    ID: "review_gate",
    RequestFn: func(ctx, state) {
        report := state.Get("draft_report").(string)
        return &SuspendRequest{
            Title:   "复核技术交底书分析结果",
            Message: report,
            Options: []string{"adopted", "modified", "rejected"},
            Metadata: map[string]any{
                "patent_id": state.Get("patent_id"),
            },
        }
    },
    Timeout: 48 * time.Hour,
    OnTimeout: Signal{Action: "adopted"},  // 超时默认通过
})
```

**3. Signal 投递入口**

```go
// server/handlers/workflow.go

// POST /api/v1/workflows/{workflow_id}/signal
func (h *WorkflowHandler) Signal(w http.ResponseWriter, r *http.Request) {
    var req SignalRequest
    json.NewDecoder(r.Body).Decode(&req)

    // 1. 查找挂起的工作流
    wfi := h.store.Get(workflowID)

    // 2. 记录人类决策
    h.eventLog.Append(workflowID, "human_signal", req)

    // 3. 恢复工作流
    wfi.Status = "running"
    wfi.CurrentNode = wfi.SuspendPoint
    h.store.Save(wfi)

    // 4. 发送信号给等待的 goroutine
    h.signalBus.Send(workflowID, Signal{
        Action:  req.Action,
        Payload: req.Payload,
        By:      req.UserID,
    })

    // 5. 异步继续执行
    go h.resumeWorkflow(ctx, wfi)
}
```

**4. 与现有 ApprovalGate 的整合**

`ApprovalGate` 目前是`LifecycleHook`。可以改为统一的 `WorkflowHook`，同时支持流式 Agent 和 Pregel 图：

```go
// agentcore/hooks/approval_hook.go

// BeforeResponse 中判断是否需要暂停
func (h *ApprovalHook) BeforeResponse(ctx Context, resp *Response) error {
    if h.needsApproval(resp) {
        // 创建 SuspendPoint
        sp := &SuspendPoint{
            WorkflowID:  ctx.WorkflowID(),
            Message:     resp.Content,
            Options:     h.getOptions(ctx),
        }
        // 持久化
        h.store.CreateSuspendPoint(sp)
        // 通知外部
        h.notifier.Notify(ctx, sp)
        // 阻塞等待信号
        signal := h.waitForSignal(ctx, sp.ID)
        // 应用人类决策
        h.applySignal(resp, signal)
    }
    return nil
}
```

**改动范围**：
- 新增 `pkg/workflow/suspend.go` — SuspendNode 和 Signal 类型定义
- 新增 `pkg/workflow/store.go` — 工作流持久化接口 + SQLite 实现
- 修改 `graph/pregel/executor.go` — 支持 SuspendNode 暂停逻辑
- 修改 `server/` — 新增 workflow signal API
- 修改 `domains/approval/` — 对齐到新的信号系统

---

### 第三阶段：Event History + Replay（鲁棒性，< 1 周）

**1. 事件持久化**

所有关键事件写入 `workflow_events` 表：

```go
// 在 PersistentExecutor 中自动记录
func (e *PersistentExecutor) logEvent(wfID, eventType string, payload any) {
    e.eventLog.Append(wfID, eventType, payload)
}

// 事件类型枚举
const (
    EventNodeEnter      = "node_enter"
    EventNodeExit       = "node_exit"
    EventLLMCall        = "llm_call"
    EventLLMResponse    = "llm_response"
    EventHumanSignal    = "human_signal"
    EventCheckpoint     = "checkpoint"
    EventError          = "error"
    EventRecovery       = "recovery"
)
```

**2. 故障恢复**

```go
// server/recovery.go — 启动时恢复未完成的工作流

func (s *Server) recoverWorkflows(ctx context.Context) {
    pending := s.store.ListByStatus("running", "paused")
    for _, wf := range pending {
        // 从事件日志恢复最后状态
        events := s.eventLog.List(wf.ID)
        state := replay(events)     // Replay 重建状态
        wf.State = state
        s.store.Save(wf)

        if wf.Status == "paused" {
            // 重新挂起等待信号
            s.signalBus.Register(wf.ID, wf.SuspendPoint)
        } else {
            // 继续执行
            go s.executor.Execute(ctx, wf)
        }
    }
}

func replay(events []Event) *WorkflowState {
    state := &WorkflowState{}
    for _, e := range events {
        switch e.Type {
        case EventCheckpoint:
            state = e.Payload.(*WorkflowState)
        case EventHumanSignal:
            state.LastSignal = e.Payload.(*Signal)
        }
    }
    return state
}
```

**3. Crash-Only 设计**

Mady 目前是单体应用，进程崩溃会丢失所有内存中的任务状态。加上持久化后，可以实现：

```
正常:    Execute → checkpoint → Execute → checkpoint → Suspend
崩溃:    checkpoint 已持久化 → 恢复工作流 → 从 checkpoint 继续
```

**改动范围**：
- 新增 `pkg/workflow/replay.go`
- 修改 `cmd/mady/serve.go` — 启动时调用 `recoverWorkflows()`
- EventBus 事件可选持久化

---

### 第四阶段：通知层抽象 + 多渠道（1 周）

Temporal 没有内置通知，需要自建。Mady 当前缺乏这一层。

**1. 通知接口**

```go
// pkg/notifier/interface.go

type ApprovalNotification struct {
    SuspendPointID string
    Title          string
    Message        string
    Options        []string
    Metadata       map[string]any
    ExpiresAt      time.Time
    WorkflowID     string
    WorkflowType   string
}

type Notifier interface {
    Notify(ctx context.Context, req *ApprovalNotification) error
    Name() string
}
```

**2. 内置实现**

| 实现 | 场景 | 优先级 |
|------|------|--------|
| **TUI Notifier** | 终端内实时通知 | P0 — 已有 TUI，直接加 |
| **WebSocket Push** | 浏览器/TUI 实时推送 | P0 — 已有 WebSocket |
| **HTTP Webhook** | 对接外部审批系统 | P1 |
| **Loopgate** | Telegram 推送审批 | P1 — 直接复用 Loopgate |
| **Email** | 低优先级通知 | P2 |

**3. 通知路由配置**

```yaml
# config/notifications.yaml
notifications:
  disclosure_review:
    channels:
      - tui
      - websocket
    timeout: 48h
    escalation:
      after: 24h
      to: email

  patent_conclusion:
    channels:
      - loopgate  # Telegram 审批
    timeout: 72h
```

**改动范围**：
- 新增 `pkg/notifier/` 包
- 实现 WebSocket 推送（复用已有 `gorilla/websocket`）
- TUI 集成：在状态栏显示 pending 数量，回车查看详情

---

### 第五阶段：审批仪表盘 + 批量管理（P2，1 周）

Temporal Web UI 是工作流可视化关键入口。Mady 需要类似能力。

**1. TUI 仪表盘**

```
┌─────────────────────────────────────────────────────┐
│  Mady — 待审批工作流                         3 pending │
├─────────────────────────────────────────────────────┤
│ □ ID          类型         等待时间   assigned      │
│ ● wf_abc123   交底书分析    2h 15m    你            │
│ ● wf_def456   新颖性评估    45m       张三          │
│ ○ wf_ghi789   创造性三步法  5m        未分配        │
├─────────────────────────────────────────────────────┤
│                                                     │
│  详情面板 (选中 wf_abc123)                          │
│  ┌─────────────────────────────────────────────┐   │
│  │ 请求：请复核技术交底书的独立权利要求分析结论   │   │
│  │ 附件：2024-xxxxxx_技术交底书.pdf              │   │
│  │                                                 │   │
│  │ [✅ 通过]  [✏️ 修改]  [❌ 驳回]  [📋 退回补正] │   │
│  └─────────────────────────────────────────────┘   │
│                                                     │
│  [1-9] 切换选中  [Enter] 详情  [a] 全选  [q] 退出   │
└─────────────────────────────────────────────────────┘
```

**2. HTTP API**

```
GET    /api/v1/workflows                    — 列出工作流
GET    /api/v1/workflows/pending            — 列出待审批
GET    /api/v1/workflows/{id}               — 工作流详情
POST   /api/v1/workflows/{id}/signal        — 提交人类决策
GET    /api/v1/workflows/{id}/events        — 事件历史
DELETE /api/v1/workflows/{id}               — 取消工作流
```

**改动范围**：
- TUI 新增审批仪表盘页面（复用 Elm 架构模式）
- `server/` 新增 workflow REST 路由
- 复用 `domains/approval/` 数据模型

---

## 三、与现有架构的融合（关键设计决策）

### 3.1 Pregel 图与 Workflow 的关系

Mady 已有 Pregel 图引擎，这是天然的 Workflow 基础。改造策略：

```
当前：Pregel Graph → Execute → 全部在内存中完成 → 结果

改造后：
Pregel Graph → PersistentExecutor 包装
                ├── 每步 checkpoint 到 SQLite
                ├── SuspendNode 自动暂停
                ├── Event Log 自动记录
                └── 故障后从 checkpoint Replay
```

**不改 Pregel 核心**，只在 Executor 层加持久化包装。

### 3.2 ApprovalGate 的演化路径

```
当前（Phase 0）:
  Agent.BeforeResponse → ApprovalGate → 阻塞等待 → 继续

Phase 1（本方案）:
  Agent.BeforeResponse → ApprovalGate → 持久化 SuspendPoint → 通知
  → goroutine 阻塞等待 Signal Bus → 恢复

Phase 2（未来）:
  Agent.BeforeResponse → 触发 Workflow 暂停 → WorkflowInstance.Status = paused
  → 事件持久化 → 进程可安全关闭 → 重启后恢复
```

### 3.3 EventBus 与 Event History 的关系

**不重复造轮子**：EventBus 是运行时解耦，Event History 是持久化追踪。

```go
// EventBus 事件可选择性持久化
eventBus.Subscribe("node_exit", func(e Event) {
    if wfID := e.Metadata["workflow_id"]; wfID != "" {
        eventLog.Append(wfID, "node_exit", e.Payload)
    }
})
```

### 3.4 与现有 DoomLoop 的关系

Mady 的 DoomLoop（死循环检测器）是 LifecycleHook，检测到死循环后中断恢复。融合后：

```go
// DoomLoop 发现死循环 → 创建 SuspendPoint 而不是直接恢复
func (d *DoomLoop) BeforeResponse(ctx, resp) error {
    if d.detectLoop(ctx) {
        sp := &SuspendPoint{
            Reason:  "deadlock_detected",
            Message: "Agent 出现重复模式，需要人工介入",
        }
        return ctx.Suspend(sp)  // 暂停工作流，等待人类
    }
    return nil
}
```

---

## 四、实施路线图

| 阶段 | 内容 | 工作量 | 优先级 |
|------|------|--------|--------|
| **P0** | 第一阶：SQLite 持久化 Workflow State | 3-5 天 | 最高 — 基础能力 |
| **P0** | 第二阶：SuspendNode + Signal 原语 | 5-7 天 | 最高 — 核心 HITL |
| **P1** | 第三阶：Event History + Replay | 2-3 天 | 高 — 鲁棒性 |
| **P1** | TUI 审批仪表盘 + REST API | 3-5 天 | 高 — 可用性 |
| **P2** | 第四阶：通知层（WebSocket Push + Loopgate） | 3-5 天 | 中 — 触达 |
| **P2** | 超时/升级策略 | 2 天 | 中 — 可靠性 |
| **P3** | 多工作流并发控制 + 限流 | 3 天 | 低 — 规模化 |

**建议 P0 先行，做通核心链路后再扩展。** 核心是：

1. Workflow State 持久化（解决进程崩溃丢状态问题）
2. SuspendNode + Signal（解决"怎么暂停，怎么恢复"问题）
3. TUI 仪表盘（解决"人类怎么看怎么批"问题）

三者打通后，Mady 就具备了 Temporal 风格的核心 HITL 能力，后续通知渠道和超时策略可迭代添加。

---

## 五、与 Temporal 的对比（改造后预期效果）

| 维度 | Temporal | Mady（改造后） |
|------|----------|----------------|
| 暂停持久化 | Event History | SQLite WorkflowInstance |
| Signal | `waitForSignal()` | `SuspendNode` + Signal Bus |
| Replay | Event History Replay | workflow_events Replay |
| 通知 | 无内置 | TUI + WebSocket + Loopgate |
| 可观测性 | Temporal Web UI | TUI 仪表盘 + REST API |
| 部署模式 | Temporal Server + Worker | 单体二进制（简化） |
| SDK 语言 | 多语言 | Go（原生集成） |

Mady 的优势在于 **深度集成**——Temporal 是通用引擎，Mady 的 HITL 直接嵌入 Agent 运行时和图引擎，语义更自然。

---

## 六、不推荐的做法

1. **直接嵌入 Temporal SDK** — 引入外部 Server 依赖，增加部署复杂度，与 Mady 单体哲学不符
2. **用 Redis 做状态存储** — Mady 已选择 SQLite，额外引入 Redis 增加 Ops 成本
3. **完全重写 Pregel** — 现有图引擎可用，只在执行层加持久化包装即可
4. **Signal 用 Webhook 回调** — 增加网络依赖和安全性问题；Signal Bus 在进程内更可靠
