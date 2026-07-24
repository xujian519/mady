# Mady HITL 增强方案（代码库驱动版）

> 基于对 Mady 实际代码库的逐文件分析，提出渐进式 HITL 增强方案。
> 与 `参考 Temporal 模式` 方案不同，本方案所有文件路径、接口和实现
> 均从现有代码中提炼，不做架构假设。

---

## 一、现有 HITL 基础设施清单（代码验证版）

### 1.1 已存在且可复用的组件

| 组件 | 文件 | 接口/能力 | 备注 |
|------|------|-----------|------|
| `ApprovalGate` | `domains/approval.go:35` | `LifecycleHook.AfterModelCall` — 关键词检测→Steer注入→Event发射 | 软中断 |
| `ApprovalState` | `domains/approval_state.go:28` | 状态机：`drafted→pending_approval→approved/modified/rejected/canceled/expired` | 含 `TransitionAllowed` 校验 |
| `ApprovalStore` | `domains/approval.go:208` | 接口：`Save/List/ListByCase` | 领域层不依赖具体实现 |
| `MemoryApprovalStore` | `domains/approval.go:220` | 内存实现 | 测试/回退用 |
| `SQLiteApprovalStore` | `domains/sqlite/approval_store.go:20` | SQLite 持久化（JSON blob），WAL 模式 | `approval_records` 表 |
| `InterruptError` | `agentcore/interrupt.go:15` | Tool/Node 返回→Agent 暂停→`Resume()` 恢复 | 硬中断 |
| `InterruptReason` | `agentcore/interrupt.go:60` | 中断上下文：ToolCallID, ToolName, Reason, Data | |
| `Agent.Interrupt()` / `Resume()` | `agentcore/agent.go` | Agent 状态管理：`Interrupted()` 检查 | 内存状态 |
| `InterruptableGraph` | `graph/checkpoint.go:39` | DAG 图中断/恢复包装器 | `MemoryCheckpointStore` |
| `PregelCheckpointer` | `graph/pregel.go` | Pregel 超步级 checkpoint 包装器 | `MemoryCheckpointStore` |
| `CheckpointStore` | `graph/checkpoint.go:22` | 接口：`Save/Load/List/Delete` | 仅有内存实现 |
| `EventBus` | `agentcore/event.go:15` | 异步 pub/sub，`Emit` + `EmitMustDeliver` | 纯内存，无持久化 |
| `ApprovalPromptEvent` | `agentcore/event_types.go:303` | TUI 审批卡片渲染信号 | 已有 `EventApprovalPrompt` |
| `AgentInterruptEvent` | `agentcore/event_types.go:92` | 中断事件 | 已有 `EventAgentInterrupt` |
| `review_gate` | `disclosure/report.go` | Pregel 节点→返回 `InterruptError` | 硬中断（Hard-interrupt） |
| `DeferredPersistQueue` | `guardrails/deferred_persist.go:20` | Strict 模式暂存队列 | **未接入消费端** |
| `PushNotifier` | `a2a/push.go:92` | Webhook 推送（指数退避+SSRF防护） | A2A 协议层 |
| `ApprovalCard` | `tui/component/` | TUI 审批卡片组件 | 已有渲染 |
| `store.CaseStore` / `store.Closer` | `store/contract.go` | 统一存储接口契约 | 所有 SQLite 存储遵循 |

### 1.2 关键缺口

| # | 缺口 | 影响 | 设计方案章节 |
|---|------|------|------------|
| G1 | `ApprovalGate.lastTriggeredOutput` 是内存字段 | 进程重启后 pending 审批丢失 | §2 |
| G2 | `CheckpointStore` 仅有内存实现 | 图执行状态不能跨进程恢复 | §3 |
| G3 | `DeferredPersistQueue` 未接入消费端 | Strict 模式 + ApprovalGate 联动不可用 | §4 |
| G4 | 无 Event History 持久化 | 无法审计 Agent 运行轨迹 | §5 |
| G5 | 无超时/过期机制 | pending 审批可能永久等待 | §6 |
| G6 | 无统一通知层 | 用户需主动轮询 TUI 查看待审项 | §7 |
| G7 | 无工作流仪表盘 | 无法一览所有待审批事项 | §8 |

---

## 二、Phase 1：Pending 状态持久化（~3 天）

### 2.1 设计目标

解决 G1：`ApprovalGate` 触发审批后，`lastTriggeredOutput` 和 Agent `Interrupted()` 状态
在内存中，进程重启丢失。需要将"待审批请求"本身写入 SQLite，启动时 reload。

### 2.2 数据模型

在 `domains/sqlite/approval_store.go` 中新增表（与已有 `approval_records` 共存）：

```sql
-- 待审批请求：ApprovalGate 触发后、人工响应前的活动状态
CREATE TABLE IF NOT EXISTS pending_approvals (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    case_id         TEXT NOT NULL DEFAULT '',
    agent_run_id    TEXT NOT NULL DEFAULT '',
    trigger_keyword TEXT NOT NULL,
    original_output TEXT NOT NULL,        -- 审批门触发时保存的 AI 输出
    tool_calls_json TEXT NOT NULL DEFAULT '[]',  -- 关联的工具调用
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending/approved/rejected/expired
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at      TEXT,                 -- 超时时间（Phase 5 用）
    responded_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_pending_session ON pending_approvals(session_id);
CREATE INDEX IF NOT EXISTS idx_pending_status  ON pending_approvals(status);
```

设计原则：
- **不做新表设计中的过度抽象**：`pending_approvals` 不是通用的 workflow_instances，
  而是精确对应 ApprovalGate 的 HITL 挂起。通用化留到 Phase 5。
- **与 `approval_records` 的关系**：`pending_approvals` 存储活动状态，
  `approval_records` 存储已决历史。一条 pending 记录被响应后，要么转为
  `approval_records` 中的记录（approved/modified/rejected），要么被删除（canceled）。
- **JSON blob + 索引列模式**：与现有 `approval_records` 一致，metadata 列保留扩展性。

### 2.3 接口扩展

```go
// domains/approval.go — 新增接口

// PendingStore 管理活动审批请求的持久化。
// 独立于 ApprovalStore（审计日志），因为生命周期不同：
// PendingStore 写入→等待→读取/删除；ApprovalStore 只增不删。
type PendingStore interface {
    // Save 创建或更新待审批请求。
    Save(ctx context.Context, p PendingApproval) error
    // Load 加载一个待审批请求。
    Load(ctx context.Context, id string) (*PendingApproval, error)
    // ListPending 列出所有状态为 pending 的请求（启动时恢复用）。
    ListPending(ctx context.Context) ([]PendingApproval, error)
    // ListBySession 列出某会话的待审批请求。
    ListBySession(ctx context.Context, sessionID string) ([]PendingApproval, error)
    // Delete 删除（已响应或取消的）待审批请求。
    Delete(ctx context.Context, id string) error
    // Respond 原子地将待审批标记为已响应并写入审批记录。
    // 即: pending_approvals.status = 'responded' + approval_records INSERT
    Respond(ctx context.Context, id string, record ApprovalRecord) error
}

type PendingApproval struct {
    ID             string
    SessionID      string
    CaseID         string
    AgentRunID     string
    TriggerKeyword string
    OriginalOutput string
    ToolCallsJSON  string    // 序列化的 []ToolCall
    Status         string    // pending / responded / expired
    CreatedAt      time.Time
    ExpiresAt      *time.Time
    RespondedAt    *time.Time
}
```

### 2.3a `PendingStore` 为何独立于 `ApprovalStore`

两种 Store 的生命周期和访问模式截然不同：

| 维度 | `ApprovalStore`（审计日志） | `PendingStore`（活动状态） |
|------|---------------------------|--------------------------|
| 写入次数 | 每个审批决策写一次 | 创建时写一次，响应时再更新/删除 |
| 读取模式 | 按 session/case 离线查询 | 启动时全量 reload + 按状态过滤 |
| 删除 | 不允许（审计日志不可删除） | 响应后必须删除/标记 |
| 一致性要求 | 最终一致即可 | 需要原子性（Respond 操作必须确保"审批记录写入"和"待审批标记完成"不分裂） |

两个 Store 操作同一个 SQLite 数据库文件，只是表不同。`Respond` 方法用事务保证原子性。

### 2.4 ApprovalGate 改造

```go
// domains/approval.go

type ApprovalGate struct {
    agentcore.BaseLifecycleHook
    config ApprovalConfig
    store  ApprovalStore    // 保留，写审计日志
    pstore PendingStore     // 新增，持久化 pending 状态

    // lastTriggeredOutput 保留作为运行时缓存（避免每次读 SQLite）
    lastTriggeredOutput string
    lastPendingID       string   // 当前 pending 的 ID，用于重入检测
}

func (g *ApprovalGate) AfterModelCall(ctx context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
    // ... 现有关键词检测 ...

    g.lastTriggeredOutput = mcc.Response.Content

    // 新增：持久化 pending 状态
    if g.pstore != nil {
        pending := PendingApproval{
            ID:             fmt.Sprintf("pend_%d", time.Now().UnixNano()),
            SessionID:      arc.SessionID,  // 需要 AgentRunContext 暴露 SessionID
            CaseID:         arc.CaseID,
            TriggerKeyword: matchedKeyword,
            OriginalOutput: mcc.Response.Content,
            Status:         "pending",
            CreatedAt:      time.Now(),
        }
        _ = g.pstore.Save(ctx, pending)
        g.lastPendingID = pending.ID
    }

    // ... 现有 Steer + Emit ...
}

// RecordDecision 改造：响应后自动清理 pending
func (g *ApprovalGate) RecordDecision(ctx context.Context, ...) error {
    if originalOutput == "" {
        originalOutput = g.lastTriggeredOutput
    }
    g.lastTriggeredOutput = ""

    // 创建审批记录
    record := buildApprovalRecord(...)

    // 原子操作：写入审批记录 + 删除 pending
    if g.pstore != nil && g.lastPendingID != "" {
        if err := g.pstore.Respond(ctx, g.lastPendingID, record); err != nil {
            // 写审计日志失败不应阻塞审批
            log.Printf("[WARN] pending respond failed: %v", err)
        }
        g.lastPendingID = ""
    } else if g.store != nil {
        _ = g.store.Save(ctx, record)
    }
    return nil
}
```

### 2.5 `AgentRunContext` 暴露 SessionID

现有 `AgentRunContext`（`agentcore/agent.go`）已包含 session 信息，但未在
`AfterModelCall` 的上下文中暴露。需要添加 `SessionID()` 和 `CaseID()` 访问器：

```go
// agentcore/agent.go — AgentRunContext 新增方法

func (arc *AgentRunContext) SessionID() string {
    if arc.Agent != nil {
        return arc.Agent.SessionID()
    }
    return ""
}
func (arc *AgentRunContext) CaseID() string {
    if arc.Agent != nil {
        return arc.Agent.CaseID()
    }
    return ""
}
```

### 2.6 启动恢复

```go
// cmd/mady/serve.go 或 tui_session_config.go

func (s *Server) recoverPendingApprovals(ctx context.Context, agent *agentcore.Agent) {
    if s.pstore == nil {
        return
    }
    pendings, err := s.pstore.ListPending(ctx)
    if err != nil || len(pendings) == 0 {
        return
    }
    // 找到属于当前 agent run 的 pending 请求，重建 lastTriggeredOutput
    for _, p := range pendings {
        if p.SessionID == agent.SessionID() {
            s.approvalGate.RestorePending(p)
            break
        }
    }
}

// domains/approval.go
func (g *ApprovalGate) RestorePending(p PendingApproval) {
    g.lastTriggeredOutput = p.OriginalOutput
    g.lastPendingID = p.ID
}
```

### 2.7 改动范围

| 文件 | 改动 |
|------|------|
| `domains/approval.go` | 新增 `PendingStore` 接口、`PendingApproval` 结构体；`ApprovalGate` 新增 `pstore` 字段及持久化逻辑；`RecordDecision` 改造 |
| `domains/approval_state.go` | 无改动（状态机已完整） |
| `domains/sqlite/approval_store.go` | 新增 `pending_approvals` 表 schema；新增 `SQLitePendingStore` 实现（`Save/Load/ListPending/ListBySession/Delete/Respond`）；`Respond` 用事务保证原子性 |
| `agentcore/agent.go` | `AgentRunContext` 新增 `SessionID()` / `CaseID()` 方法 |
| `cmd/mady/tui_session_config.go` | 初始化 `PendingStore` 并注入 `ApprovalGate`；启动时调用 `recoverPendingApprovals` |
| `cmd/mady/serve.go` | 同上（serve 模式） |

---

## 三、Phase 2：CheckpointStore SQLite 实现（~2 天）

### 3.1 设计目标

解决 G2：`graph/checkpoint.go` 的 `CheckpointStore` 接口已有抽象，但唯一实现
是 `MemoryCheckpointStore`。新增 `SQLiteCheckpointStore` 使图执行状态可跨进程恢复。

### 3.2 实现定位

**不新建文件，不修改`CheckpointStore`接口**。在 `domains/sqlite/` 下新增
`checkpoint_store.go`，与 `approval_store.go` 同级：

```sql
CREATE TABLE IF NOT EXISTS graph_checkpoints (
    id          TEXT PRIMARY KEY,
    graph_id    TEXT NOT NULL,
    node_name   TEXT NOT NULL,
    step_index  INTEGER NOT NULL DEFAULT 0,
    state_json  TEXT NOT NULL,       -- Checkpoint.State (json.RawMessage)
    metadata    TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_gc_graph ON graph_checkpoints(graph_id, created_at);
```

### 3.3 与 `domains/reasoning/sqlite/checkpoint_store.go` 的关系

推理层已有 `SQLiteCheckpointStore`（保存 FactBlackboard + Plan），但它是
**领域专属的**（实现 `reasoning.CheckpointStore`，而非 `graph.CheckpointStore`）。

本次新增的是 **通用图级别的** CheckpointStore，实现 `graph.CheckpointStore` 接口。
两者互不替代，分别服务于不同的抽象层级。

### 3.4 接入点

```go
// graph/checkpoint.go — 无需修改接口

// 已有 InterruptableGraph 和 PregelCheckpointer 的构造器接受 CheckpointStore，
// 现在传入 SQLiteCheckpointStore 即可自动获得持久化能力：

store := sqlite.NewGraphCheckpointStore(dbPath)
cg := graph.NewCompiledGraph(...)
ig := graph.NewInterruptableGraph(cg, store)  // 只需换入参
```

### 3.5 改动范围

| 文件 | 改动 |
|------|------|
| `domains/sqlite/checkpoint_store.go` | 新增文件：`SQLiteGraphCheckpointStore` 实现 `graph.CheckpointStore` |
| `graph/` | 无改动（接口不变） |
| `cmd/mady/serve.go` | 初始化时创建 `SQLiteGraphCheckpointStore` 替代 `MemoryCheckpointStore` |

---

## 四、Phase 3：接入 DeferredPersistQueue（~2 天）

### 4.1 设计目标

解决 G3：`guardrails/deferred_persist.go` 的 `DeferredPersistQueue` 已实现但
头注释明确写着"尚未接入 production 消费端"。接入后，Strict guardrail 模式 +
ApprovalGate 的联动流程才能完整工作。

### 4.2 当前状态

```
Strict guardrail 触发: SuppressPersist=true → 消息标记暂存
                                               ↓
                                        DeferredPersistQueue.Store()
                                               ↓
                                        ❌ 实际无消费端读取队列
                                               ↓
                                        消息最终被丢弃
```

### 4.3 接入点

**接入点 A**（`guardrails/levels.go` AfterModelCall）：当 `SuppressPersist=true` 时，
调用 `queue.Store(msgIndex, msg)` 暂存消息。

```go
// guardrails/levels.go — 在 AfterModelCall 中

func (g *GuardrailHook) AfterModelCall(ctx context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
    // ... 现有等级检查逻辑 ...

    if level == LevelStrict && result.SuppressPersist {
        // 暂存消息，等待审批决定
        for i, msg := range mcc.Response.Messages {
            g.deferredQueue.Store(i, msg)
        }
        // 不再立即丢弃
    }
}
```

**接入点 B**（`domains/approval.go` RecordDecision）：审批通过后 `Commit`/
拒绝后 `Discard`。

```go
// domains/approval.go — 在 RecordDecision 中

func (g *ApprovalGate) RecordDecision(ctx context.Context, ...) error {
    // ... 现有逻辑 ...

    // 处理暂存消息
    if g.deferredQueue != nil {
        switch decision {
        case DecisionAdopted, DecisionModified:
            g.deferredQueue.Commit()   // 写入持久化
        case DecisionRejected:
            g.deferredQueue.Discard()  // 丢弃
        }
    }
    return nil
}
```

### 4.4 改动范围

| 文件 | 改动 |
|------|------|
| `guardrails/levels.go` | AfterModelCall 中调用 `queue.Store(msgIndex, msg)` |
| `guardrails/deferred_persist.go` | 可能需要暴露 `Store(idx int, msg Message)` 方法（已实现） |
| `domains/approval.go` | `RecordDecision` 中 Commit/Discard |
| `cmd/mady/tui_session_config.go` | 创建 `DeferredPersistQueue` 实例并注入 GuardrailHook 和 ApprovalGate |

---

## 五、Phase 4：Event History 轻量持久化（~2 天）

### 5.1 设计目标

解决 G4：EventBus 的事件当前是纯内存的，发出即丢弃。需要将关键事件写入 SQLite
以实现审计追踪和可观测性。

### 5.2 设计决策：不做完整 Replay

Temporal 的 Event History Replay 对 Mady 不适用，因为：
1. **LLM 调用不可重放**：LLM 返回是非确定性的，即使记录输入，re-run 不能保证相同输出
2. **Mady 不需要精确恢复**：客户端场景是"重启后让用户重新触发"或"从 checkpoint 继续"，
   而非"精确重放历史得到相同状态"
3. **存事件日志的开销与收益不成正比**：一次 Agent Run 可能产生数百个事件

### 5.3 实现方案：EventBus Handler → SQLite

注册一个 EventBus 全局 handler，将事件异步写入 `workflow_events` 表：

```sql
CREATE TABLE IF NOT EXISTS workflow_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type  TEXT NOT NULL,
    session_id  TEXT NOT NULL DEFAULT '',
    agent_name  TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL,        -- JSON 序列化的事件体
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_we_session ON workflow_events(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_we_type     ON workflow_events(event_type);
```

```go
// NewEventLogger 创建一个 EventBus handler 将事件持久化到 SQLite。
// 为了不影响 EventBus 的核心吞吐，写 SQLite 是异步的（goroutine + channel buffer）。
func NewEventLogger(db *sql.DB) *EventLogger {
    el := &EventLogger{
        db:   db,
        ch:   make(chan Event, 256),
        done: make(chan struct{}),
    }
    go el.loop()
    return el
}

func (el *EventLogger) Handle(ctx context.Context, e agentcore.Event) {
    select {
    case el.ch <- e:
    default:
        // channel 满时丢弃，不阻塞 EventBus 主循环
    }
}

func (el *EventLogger) loop() {
    for e := range el.ch {
        payload, _ := json.Marshal(e)
        _, _ = el.db.Exec(
            `INSERT INTO workflow_events (event_type, payload) VALUES (?, ?)`,
            string(e.EventKind()), string(payload),
        )
    }
}
```

### 5.4 哪些事件需要记录

| 事件类型 | 记录理由 |
|----------|---------|
| `EventAgentStart` | 会话开始 |
| `EventAgentEnd` | 会话结束 |
| `EventApprovalPrompt` | 审批门触发 |
| `EventAgentInterrupt` | Agent 中断 |
| `EventToolCallStart` | 工具调用 |
| `EventToolCallEnd` | 工具调用结果 |
| `EventError` | 错误记录 |

高频事件（`EventMessageDelta`）不记录，避免日志膨胀。

### 5.5 改动范围

| 文件 | 改动 |
|------|------|
| `agentcore/event_logger.go` | 新增文件：`EventLogger` |
| `domains/sqlite/event_store.go` | 新增文件：schema + CRUD |
| `cmd/mady/tui_session_config.go` | 创建 `EventLogger`，注册到 EventBus |
| `cmd/mady/serve.go` | 同上 |

---

## 六、Phase 5：超时/过期机制（~1 天）

### 6.1 设计目标

解决 G5：pending 审批可能永久等待。需要：
1. 审批请求可设超时
2. 超时后自动执行默认操作
3. 后台协程定期扫描过期 pending

### 6.2 实现

```go
// PendingStore 扩展

func (s *SQLitePendingStore) ExpirePending(ctx context.Context) (int64, error) {
    res, err := s.db.ExecContext(ctx, `
        UPDATE pending_approvals
        SET status = 'expired', responded_at = datetime('now')
        WHERE status = 'pending'
          AND expires_at IS NOT NULL
          AND expires_at < datetime('now')
    `)
    if err != nil {
        return 0, err
    }
    return res.RowsAffected()
}

// ApprovalGate 配置扩展
type ApprovalConfig struct {
    DefaultExpiry time.Duration  // 审批请求默认超时（0=永不超时）
    OnExpire      ExpireAction   // timeout/auto_approve
}

type ExpireAction string
const (
    ExpireReject     ExpireAction = "reject"        // 超时视为拒绝
    ExpireAutoApprove ExpireAction = "auto_approve" // 超时视为通过
)
```

后台扫描协程：

```go
// cmd/mady/serve.go 或独立的 PendingExpirer

func StartPendingExpirer(ctx context.Context, pstore PendingStore, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            expired, _ := pstore.ExpirePending(ctx)
            if expired > 0 {
                log.Printf("[HITL] 过期 %d 个待审批请求", expired)
            }
        }
    }
}
```

### 6.3 改动范围

| 文件 | 改动 |
|------|------|
| `domains/approval.go` | `ApprovalConfig` 新增 `DefaultExpiry` / `OnExpire` |
| `domains/sqlite/approval_store.go` | `SQLitePendingStore.ExpirePending()` |
| `cmd/mady/serve.go` | 启动 `StartPendingExpirer` |

---

## 七、Phase 6：通知层（~2 天）

### 7.1 设计目标

解决 G6：用户需主动轮询 TUI 查看待审项。需要审批事件触发时主动通知用户。

### 7.2 设计：不新建 `pkg/notifier/`

Mady 已有 `a2a/push.go` 的 `PushNotifier`（Webhook 推送），以及 `agentcore.EventBus`
（进程内事件分发）。通知层应**复用**这些基础设施，而非新建包。

### 7.3 通知通道

```
审批事件 → EventBus.EventApprovalPrompt
               │
               ├──→ TUI Notifier（进程内）→ 状态栏闪烁 + 通知音
               │
               ├──→ A2A PushNotifier（HTTP Webhook）→ 对接外部系统
               │
               └──→ (预留) Loopgate / Telegram / Email
```

```go
// tui/notification/notifier.go — TUI 通知
type TUINotifier struct {
    bus    *agentcore.EventBus
    status *StatusBar
}

func (n *TUINotifier) Start() {
    cancel := n.bus.On(agentcore.EventApprovalPrompt, func(e agentcore.Event) {
        n.status.Flash("🔔 新审批请求 — 请查看")
    })
    // cancel 用于清理
}
```

```go
// server/notifier.go — 如果是 Server 模式，走 Webhook
type WebhookNotifier struct {
    pushNotifier *a2a.PushNotifier
    config       PushNotificationConfig
}

func (n *WebhookNotifier) OnApprovalPrompt(ctx context.Context, pending PendingApproval) {
    _ = n.pushNotifier.Notify(ctx, &n.config, &a2a.Task{
        ID:          pending.ID,
        Title:       fmt.Sprintf("审批请求: %s", pending.TriggerKeyword),
        Description: pending.OriginalOutput[:min(len(pending.OriginalOutput), 500)],
        Status:      a2a.TaskStatusAwaiting,
    })
}
```

### 7.4 通知路由配置

```yaml
# ~/.mady/config.yaml
notifications:
  approval:
    channels: [tui]
    # channels: [tui, webhook]
  webhook:
    url: "https://hooks.example.com/mady-approval"
    secret: "..."     # HMAC 签名
```

### 7.5 改动范围

| 文件 | 改动 |
|------|------|
| `tui/notification/` | 新建目录：`TUINotifier` |
| `server/notifier.go` | 新增文件：`WebhookNotifier`（复用 `a2a.PushNotifier`） |
| `cmd/mady/tui_session_config.go` | 注入通知通道 |
| `config/` | 支持通知路由配置 |

---

## 八、Phase 7：TUI 审批仪表盘 + REST API（~3 天）

### 8.1 设计目标

解决 G7：当前审批卡片（`tui/component/approval_card.go` 等）是聊天内的单条审批，
无法一览所有待审批事项。需要新增仪表盘页面。

### 8.2 TUI 仪表盘

复用已有 Elm 架构（`tui/layout/` + `tui/chat/`）：

```
┌─ Mady ─────────────────────────────────────────────┐
│  📋 待审批工作流                          3 项待处理 │
├───────────────────────────────────────────────────┤
│                                                    │
│  ● 交底书分析复核        patent_1     ⏱ 2h 15m   │
│    关键词: "新颖性判断"                               │
│                                                    │
│  ● OA 答复策略审批       patent_2     ⏱ 45m      │
│    关键词: "法律意见"                                 │
│                                                    │
│  ○ 创造性三步法分析      patent_3     ⏱ 5m        │
│    关键词: "最终建议"                                 │
│                                                    │
├───────────────────────────────────────────────────┤
│  详情面板 (选中 #1)                                 │
│  ┌───────────────────────────────────────────┐   │
│  │ ⚠️ 需要人工审核确认。请检查以下内容后回复...│   │
│  │                                            │   │
│  │ ...新颖性判断：对比文件D1公开了特征A...     │   │
│  │                                            │   │
│  │ [✅ 通过] [✏️ 修改] [❌ 驳回] [⏰ 催办]      │   │
│  └───────────────────────────────────────────┘   │
│                                                    │
│  [↑↓] 切换选中  [Enter] 详情  [a] 全选  [q] 退出   │
└───────────────────────────────────────────────────┘
```

实现路径：
- 新增 `tui/approval/` 组件（参考 `tui/component/approval_card.go` 的渲染逻辑）
- 注册到 TUI 的路由（如来 `/approvals` 或快捷键 `Ctrl+A`）
- 数据源：`PendingStore.ListPending()`

### 8.3 REST API

```go
// server/workflow_handler.go

GET    /api/v1/approvals/pending           → 列出待审批
GET    /api/v1/approvals/pending/{id}      → 待审批详情
POST   /api/v1/approvals/pending/{id}/respond → 提交决策

// 响应格式
type PendingResponse struct {
    ID             string `json:"id"`
    SessionID      string `json:"session_id"`
    CaseID         string `json:"case_id"`
    TriggerKeyword string `json:"trigger_keyword"`
    OriginalOutput string `json:"original_output"`
    Status         string `json:"status"`
    CreatedAt      string `json:"created_at"`
    ExpiresAt      *string `json:"expires_at,omitempty"`
}

type RespondRequest struct {
    Action   string `json:"action"`   // "approved" / "modified" / "rejected"
    Feedback string `json:"feedback,omitempty"`
    Modified string `json:"modified_output,omitempty"`
}
```

### 8.4 改动范围

| 文件 | 改动 |
|------|------|
| `tui/approval/dashboard.go` | 新增文件：仪表盘组件 |
| `tui/chat/` | 注册仪表盘路由 |
| `server/workflow_handler.go` | 新增文件：审批 REST API |
| `server/router.go` | 注册新路由 |

---

## 九、完整实施路线图

```
Phase 1: Pending 持久化     ████████████░░░░░░  3 天  P0
Phase 2: Checkpoint SQLite  ████████░░░░░░░░░░  2 天  P0
Phase 3: DeferredPersist    ████████░░░░░░░░░░  2 天  P1
Phase 4: Event History      ████████░░░░░░░░░░  2 天  P1
Phase 5: 超时/过期           ████░░░░░░░░░░░░░░  1 天  P1
Phase 6: 通知层              ████████░░░░░░░░░░  2 天  P2
Phase 7: 仪表盘 + API       ████████████░░░░░░  3 天  P2
                              ────────────────
                              总计约 15 天
```

### 优先级排序

| 优先级 | 阶段 | 理由 |
|--------|------|------|
| **P0** | Phase 1 (Pending) | 最核心缺口：进程重启丢状态直接破坏 HITL 信任 |
| **P0** | Phase 2 (Checkpoint) | 同样解决进程崩溃丢状态，但影响面更广（图引擎） |
| **P1** | Phase 3 (DeferredPersist) | Strict 模式联动不可用，但可通过配置回避 |
| **P1** | Phase 4 (Event History) | 审计追踪，无此功能不影响核心流程 |
| **P1** | Phase 5 (Timeout) | 用户体验问题，pending 不会无限阻塞但有风险 |
| **P2** | Phase 6 (Notification) | 增强体验，无此功能可通过轮询 TUI 替代 |
| **P2** | Phase 7 (Dashboard) | 增强体验，现有 TUI 单个审批卡片可工作 |

### 建议执行顺序

1. **Phase 1 + Phase 2 并行**（3-4 天）— 两者互不依赖，分别解决 Agent 层和图引擎层的持久化
2. **Phase 3**（2 天）— 依赖 Phase 1 的 `RecordDecision` 改造
3. **Phase 4**（2 天）— 独立，可在任意时间插入
4. **Phase 5**（1 天）— 依赖 Phase 1 的 `PendingStore`，改动很小
5. **Phase 6 + Phase 7 并行**（3-4 天）— 互不依赖，分别处理通知和 UI

---

## 十、与原有方案的差异总结

| 维度 | 原方案（Temporal 参考） | 本方案（代码库驱动） |
|------|-----------------------|-------------------|
| 持久化层定位 | 新建 `graph/executor.go` + `PersistentExecutor` | 复用已有 `CheckpointStore` 接口，在 `domains/sqlite/` 下新增实现 |
| Signal 机制 | 新建 `signalBus` + `SuspendNode` | 复用 `EventBus` + 已有 `InterruptError` 机制 |
| 通知层 | 新建 `pkg/notifier/` | 复用 `a2a.PushNotifier` |
| 审批存储 | 新建 `workflow_instances` 通用表 | 在现有 `approval_records` 旁新增 `pending_approvals`（专注 HITL） |
| Replay | 设计完整 Replay 机制 | 不做 Replay，用 checkpoint + 启动恢复代替 |
| DoomLoop 融合 | DoomLoop → SuspendPoint | DoomLoop 保持异常检测职责，不混入审批流程 |
| 工作流抽象 | Temporal 风格通用 Workflow | Mady 既有 Agent Run + Pregel 图 + Workflow 工具三路径 |
| DeferredPersist | 未提及 | 纳入 Phase 3，补充缺口 |
| 目录结构 | 多处与实际不符 | 精确到每个文件路径 |

---

## 十一、附录：关键接口变更总览

### 新增接口

```go
// domains/approval.go
type PendingStore interface { ... }
type PendingApproval struct { ... }

// server/workflow_handler.go（可选 Phase 7 做）
type ApprovalHandler struct { ... }
```

### 新增文件

```
domains/sqlite/
├── approval_store.go     ← 修改（新增 pending_approvals 表 + SQLitePendingStore）
├── checkpoint_store.go   ← 新增（SQLiteGraphCheckpointStore）
├── event_store.go        ← 新增（SQLiteEventStore）

agentcore/
├── event_logger.go       ← 新增（EventBus → SQLite handler）

tui/approval/
├── dashboard.go          ← 新增（审批仪表盘组件）

server/
├── workflow_handler.go   ← 新增（审批 REST API）
├── notifier.go           ← 新增（Webhook 通知）
```

### 修改文件

```
domains/approval.go        — ApprovalGate + pstore + RecordDecision 改造
guardrails/levels.go       — 接入 DeferredPersistQueue.Store
guardrails/deferred_persist.go — 暴露 Commit/Discard（如需）
agentcore/agent.go         — AgentRunContext.SessionID() / CaseID()
cmd/mady/tui_session_config.go — 初始化所有新组建
cmd/mady/serve.go          — 启动恢复 + 后台协程
```

---

> **核心原则**：本方案的所有设计决策都基于"渐进增强"而非"新建替换"。
> 每阶段都能独立交付价值，不要求所有阶段完成才能使用。
> 每阶段改动范围控制在 3-5 个文件内。
