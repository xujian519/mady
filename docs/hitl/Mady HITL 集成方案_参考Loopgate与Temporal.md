# Mady HITL 集成方案

> 基于 Loopgate / Temporal 调研 + 现有代码分析

---

## 现状摘要

Mady 已有较完善的 HITL 基础设施：

| 组件 | 现有实现 |
|------|----------|
| **审批触发** | `ApprovalGate`（LifecycleHook）关键词触发，`review_gate`（Pregel InterruptError）管线中断 |
| **状态机** | `ApprovalState`：`drafted→pending_approval→approved/modified/rejected/canceled/expired` |
| **持久化** | `SQLiteApprovalStore`：`approval_records` 表，JSON 列 + 索引 |
| **事件总线** | `agentcore.EventBus`：Broker 模式，best-effort + bounded-blocking 两种发射 |
| **留痕入口** | TUI `/approve`/`/reject`、Server HTTP `POST /review`、ACP `recordPermissionDecision` |
| **决策记录** | `RecordApprovalDecision` 统一函数，含 `TriggerKeyword` 溯源 |

### 现有缺口

1. **无持久化等待** — 审批请求在进程内存中，Server 重启后丢失
2. **无 Signal/Suspend 原语** — review_gate 用 `InterruptError` 是 one-shot 退出而非可恢复暂停
3. **无 Event History** — 长流程崩溃后无法恢复
4. **无远程仪表盘** — 无法在手机上查看和审批
5. **无外部通知接口** — 审批仅在 TUI 渲染，未来接入微信/飞书/Telegram 等需先抽象接口

---

## 设计原则

1. **渐进改造，不推翻重来** — 每一阶段都独立可用
2. **保持单体二二进制** — 不引入外部消息队列或独立 Server
3. **接口先行** — 通过抽象隔离变化，现有组件不改动

---

## 第一阶段：通知层抽象（NotifyAdapter）

在现有 `ApprovalGate` 之上增加通知接口，为未来接入微信/飞书/Telegram 等外部通道预留抽象层。**本阶段仅定义接口，不实现具体适配器。**

### 接口定义

```go
// domains/notify.go

type NotifyMessage struct {
    ID          string
    Title       string
    Body        string
    Priority    string // "high" / "normal"
    Options     []string
    Metadata    map[string]any
    RequestID   string       // 关联 ApprovalRecord.ID
    CreatedAt   time.Time
}

type NotifyDecision struct {
    RequestID  string
    Decision   string   // "approved" / "modified" / "rejected"
    ModifiedOutput string
    Feedback   string
    DecidedBy  string
    DecidedAt  time.Time
}

type NotifyAdapter interface {
    // Send 发送审批通知，返回通知ID（异步）
    Send(ctx context.Context, msg NotifyMessage) (notificationID string, err error)
    // Name 适配器名称，如 "wechat", "feishu", "telegram"
    Name() string
    // Start 启动适配器（如有后台 goroutine 或长连接）
    Start(ctx context.Context) error
    // Stop 关闭适配器
    Stop(ctx context.Context) error
}

type NotifyManager struct {
    adapters  []NotifyAdapter
    decisions chan NotifyDecision  // 外部决策输入通道
    store     ApprovalStore
}
```

### WebSocketNotifyAdapter（内置，通用监听）

适用于 MCP Client / ACP Client / Web 仪表盘等场景：

```go
type WSNotifyAdapter struct {
    pending    sync.Map  // requestID → chan NotifyDecision
}
```

- 对外提供 `GET /api/v1/hitl/pending` 和 `WS /api/v1/hitl/ws` 端点
- WebSocket 推送审批事件到已连接的客户端
- 客户端可通过 `POST /api/v1/hitl/{requestID}/decision` 提交决策

### 集成到 ApprovalGate

```go
// ApprovalGate 新增字段
type ApprovalGate struct {
    agentcore.BaseLifecycleHook
    config      ApprovalConfig
    store       ApprovalStore
    notifyMgr   *NotifyManager       // 新增

    lastTriggeredOutput string
}

// AfterModelCall 中增加：
func (g *ApprovalGate) AfterModelCall(ctx context.Context, arc LifecycleAfterModelCallContext) {
    // ... 原有触发逻辑 ...

    if needsApproval {
        // ... 原有 Steer 逻辑 ...

        // 新增：异步发送外部通知（如有已注册的适配器）
        if g.notifyMgr != nil {
            record := buildRecord(arc)
            g.notifyMgr.SendAsync(ctx, record)
        }
    }
}
```

### 决策回传路径

```
外部通道（微信/飞书/Telegram 等）
    → NotifyAdapter (具体实现)
        → NotifyManager.decisions channel
            → NotifyManager.processDecision()
                → RecordApprovalDecision()
                    → SQLiteApprovalStore.Save()
                        → agent loop 感知决策（待第二阶段 SuspendError）
```

### 未来扩展（适配器实现）

具体适配器（如 Telegram、微信、飞书）**不在本计划内实现**，仅按需添加：

```go
// notify/telegram/  —— 参考 Loopgate 设计
// notify/wechat/    —— 企业微信机器人 + 回调
// notify/feishu/    —— 飞书机器人 + 卡片消息
```

每个适配器只需实现 `NotifyAdapter` 接口，注册到 `NotifyManager` 即可生效。

---

## 第二阶段：持久化 Pending Session

当前 review_gate 中断后，任务状态仅存在内存中。需要将 pending 审批持久化，使 Server 重启后仍可恢复。

### 表结构扩展（`hitl_sessions`）

```sql
CREATE TABLE IF NOT EXISTS hitl_sessions (
    id              TEXT PRIMARY KEY,
    session_type    TEXT NOT NULL,      -- 'disclosure' / 'agent_approval'
    status          TEXT NOT NULL,      -- 'pending' / 'decided' / 'expired'
    request_data    TEXT NOT NULL,       -- JSON: 中断上下文（含 PregelState）
    decision_data   TEXT,               -- JSON: 决策结果
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    decided_at      TEXT,
    expires_at      TEXT NOT NULL        -- 超时自动过期
);
```

### PendingSessionStore 接口

```go
type PendingSessionStore interface {
    Save(ctx context.Context, session PendingSession) error
    Get(ctx context.Context, id string) (*PendingSession, error)
    ListPending(ctx context.Context) ([]PendingSession, error)
    MarkDecided(ctx context.Context, id string, decision *NotifyDecision) error
    Delete(ctx context.Context, id string) error
}
```

### Server 启动恢复

```go
func (s *Server) recoverPendingSessions(ctx context.Context) {
    sessions, err := s.pendingStore.ListPending(ctx)
    for _, sess := range sessions {
        switch sess.SessionType {
        case "disclosure":
            // 从 PregelState 重建 disclosure 任务上下文
            s.recoverDisclosureReview(ctx, sess)
        case "agent_approval":
            // 重建 ApprovalGate 上下文
            s.recoverAgentApproval(ctx, sess)
        }
    }
}
```

### 整合 NotifyManager

```go
type NotifyManager struct {
    adapters    []NotifyAdapter
    store       PendingSessionStore    // 新增
    storeReady  atomic.Bool
    decisions   chan NotifyDecision
}
```

- `SendAsync` 中先保存 `PendingSession` 到 SQLite，再投递通知
- `processDecision` 中标记会话为 `decided`
- Server 启动时遍历所有 `pending` 状态的会话，重新投递通知

---

## 第三阶段：Signal/Suspend 原语

当前 `InterruptError` 是终断性的——Agent Loop 捕获后退出，需外部重新进入。引入 `Signal` 模式：Agent Loop 可"暂停"而非"退出"，在等待期间保持上下文。

### SuspendError 定义

```go
// agentcore/suspend.go

type SuspendSignal struct {
    ID          string
    SessionType string
    Reason      string
    Context     map[string]any    // 暂停时的完整上下文
    ResumeData  map[string]any    // 恢复时注入的数据
    Status      SuspendStatus     // pending / resumed / timed_out
    CreatedAt   time.Time
    ResumeAt    *time.Time        // 恢复时间
}

type SuspendStatus string

const (
    SuspendPending  SuspendStatus = "pending"
    SuspendResumed  SuspendStatus = "resumed"
    SuspendTimedOut SuspendStatus = "timed_out"
    SuspendCanceled SuspendStatus = "canceled"
)

// SuspendError 可被 Agent Loop 捕获但不终止
type SuspendError struct {
    Signal    *SuspendSignal
    ResumeCh  chan *SuspendSignal   // 外部写入以恢复
    Timeout   time.Duration
}
```

### Agent Loop 修改

```go
// doomloop/loop.go — agent main loop
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case resumeSignal := <-suspendCh:
        // 从 SuspendError 中获取恢复信号
        // 注入 ResumeData 到 Context
        // 继续下一轮迭代
    default:
    }

    result, err := agent.Run(ctx, input)
    if errors.As(err, &suspendErr) {
        // 暂停而非退出
        agent.state = StateSuspended
        // 持久化 SuspendSignal 到 SQLite
        // 等待外部通过 NotifyManager 或 API 注入决策
        select {
        case signal := <-suspendErr.ResumeCh:
            agent.state = StateRunning
            input = signal.ResumeData
            continue
        case <-time.After(suspendErr.Timeout):
            // 超时处理
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 与 review_gate 集成

```go
func reviewGateNode() graph.PregelNode {
    return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
        // 现有逻辑...

        // 改为返回 SuspendError 而非 InterruptError
        return state, agentcore.NewSuspendErrorWithData(
            "技术交底书分析完成，请人工复核",
            data,
            suspendTimeout,           // 超时时间
            resumeCh,                 // 恢复通道
        )
    }
}
```

---

## 第四阶段：Event History 轻量实现

参考 Temporal 的 Event History 思路，为长周期 Workflow 增加持久化事件日志。

### EventLog 存储

```sql
CREATE TABLE IF NOT EXISTS workflow_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      TEXT NOT NULL,
    seq             INTEGER NOT NULL,       -- 事件序号
    event_type      TEXT NOT NULL,           -- 'agent_start' / 'model_call' / 'tool_call' / 'suspend' / 'resume' / 'finish'
    event_data      TEXT NOT NULL,           -- JSON 事件载荷
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_workflow_events_session ON workflow_events(session_id, seq);
```

### EventLogger

```go
type EventLogger struct {
    db         *sql.DB
    currentSeq sync.Map  // sessionID → int32
    bufferSize int       // 批量写入阈值
}

func (l *EventLogger) Append(ctx context.Context, sessionID string, eventType string, data any) error
func (l *EventLogger) Replay(ctx context.Context, sessionID string, fn func(EventEntry) error) error
func (l *EventLogger) GetLastState(ctx context.Context, sessionID string) (EventEntry, error)
```

### Replay 恢复

Server 重启时，对每个 `pending` 状态的 session：
1. 从 `workflow_events` 读取完整事件序列
2. 重放到最后一个事件
3. 重建 PregelState 或 Agent 上下文
4. 重新进入等待决策状态

---

## 第五阶段：审批仪表盘（可选）

一个极简的 Web 仪表盘，单一 HTML + JS，嵌入到 `server/` 中。

### 路由

| 路径 | 功能 |
|------|------|
| `GET /hitl/` | 仪表盘首页 |
| `GET /api/v1/hitl/pending` | 待审批列表 |
| `GET /api/v1/hitl/history` | 历史记录 |
| `POST /api/v1/hitl/{id}/decision` | 提交决策 |
| `WS /api/v1/hitl/ws` | 实时推送 |

仪表盘在后端用 `embed.FS` 嵌入静态文件，无需独立部署。

---

## 实施路线

| 阶段 | 内容 | 工作量估计 | 交付物 |
|------|------|------------|--------|
| **P0** | NotifyAdapter 接口 + WSNotifyAdapter | 1-2 天 | `domains/notify.go`（接口层） |
| **P1** | PendingSessionStore + Server 恢复逻辑 | 2-3 天 | `domains/pending_store.go` + SQLite 迁移 |
| **P2** | SuspendError + Agent Loop 修改 | 3-5 天 | `agentcore/suspend.go` + Loop 改造 |
| **P3** | EventLog 轻量实现 | 2-3 天 | `workflows/eventlog/` |
| **P4** | 审批仪表盘 | 2 天 | `server/hitl/` + 嵌入式前端 |

### 依赖关系

```
P0 ──→ P1 ──→ P2 ──→ P3
                │
                └──→ P4
```

P0 与 P1-P3 正交，可在任意阶段独立实施。P4 依赖 P2 的 `SuspendError` 机制（需要等待通道），但不依赖 P3。

> **外部通知适配器（微信/飞书/Telegram）** 不在 P0-P4 范围内，后续按需实现。每新增一个适配器约 1-2 天。

---

## 与现有架构的关系

```
┌─────────────────────────────────────────────────────────────────┐
│                      新增组件（绿色）                              │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────┐   │
│  │ NotifyMgr    │  │ PendingStore │  │ SuspendError          │   │
│  │ WSAdapter    │  │ 持久化等待    │  │ Agent Loop 暂停/恢复   │   │
│  │ (接口层)      │  │ 重启恢复      │  │ ResumeCh 通道         │   │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬────────────┘   │
│         │                 │                      │                │
└─────────┼─────────────────┼──────────────────────┼────────────────┘
          │                 │                      │
          ▼                 ▼                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                      现有组件（灰色）                              │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────┐   │
│  │ ApprovalGate │  │ SQLiteStore  │  │ EventBus              │   │
│  │ LifecycleHook│  │ approval_    │  │ Broker + Subscribe    │   │
│  │ 关键词触发    │  │ records 表   │  │ 异步事件分发           │   │
│  └──────┬───────┘  └──────┬───────┘  └───────────────────────┘   │
│         │                 │                                      │
│         ▼                 ▼                                      │
│  ┌──────────────┐  ┌──────────────┐                              │
│  │ ApprovalState│  │ RecordDecision│                              │
│  │ 状态机       │  │ 统一留痕入口  │                              │
│  └──────────────┘  └──────────────┘                              │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────┐   │
│  │ Pregel 图引擎 │  │ review_gate  │  │ Agent Loop (doomloop) │   │
│  │ DAG + 超步   │  │ 管线中断节点  │  │ 执行循环             │   │
│  └──────────────┘  └──────────────┘  └───────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Loopgate / Temporal 具体借鉴点

### 从 Loopgate

| 特性 | Mady 如何借鉴 |
|------|--------------|
| 通知适配器设计 | `NotifyAdapter` 接口，Telegram/微信/飞书各实现一个适配器 |
| 多选项按钮 | `NotifyMessage.Options []string`，各通道自行渲染 |
| Polling API | `/api/v1/hitl/pending` 端点 |
| MCP Tool 暴露 | 新增 MCP Tool `request_human_approval`，暴露给外部 Agent |
| API Key 认证 | 复用已有 JWT/API Key 机制 |

### 从 Temporal

| 特性 | Mady 如何借鉴（轻量化） |
|------|------------------------|
| Signal/SignalWithStart | `SuspendError` + `ResumeCh` 通道模式 |
| Event History + Replay | `workflow_events` 表 + `EventLogger.Replay()` |
| Workflow 状态持久化 | `PendingSessionStore` 持久化中断上下文 |
| 确定性重放 | Pregel 图天然确定性（纯函数节点），只需保证 replay 时 Activity 不重复执行 |
| 长时间等待 | `SuspendError.Timeout` 可配置，`hitl_sessions.expires_at` 自动清理 |

---

## 附录：关键技术决策

### 为什么不用 Loopgate 直接集成？

Loopgate 是独立进程，需要额外部署和运维。Mady 当前是**单一二进制**，引入外部依赖会破坏部署模型。更合理的做法是**借鉴其设计**而非引入依赖。

### 为什么不做完整 Temporal 移植？

Temporal 的核心价值在于**分布式持久化执行**，但 Mady 是**单进程单体应用**，工作流生命周期不跨越进程边界。当前阶段引入 Temporal Server + Worker 架构是过度工程。

### SuspendError vs InterruptError

| | InterruptError | SuspendError |
|---|---|---|
| **语义** | 终断，Agent 退出 | 暂停，Agent 保持上下文 |
| **恢复** | 需外部重新调用 | 通过 ResumeCh 通道恢复 |
| **状态** | Agent 上下文丢失 | Agent 上下文保持 |
| **适用场景** | 用户主动取消、错误终止 | 等待人工审批 |
