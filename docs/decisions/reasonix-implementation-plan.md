# P0 + P1 引入实施计划

> 基于 Reasonix 高价值特性分析，为 Mady 项目制订的详细落地计划。
> 每个特性严格遵循 Mady 现有架构模式（Extension / LifecycleHook / Middleware / ContextEngine）。

---

## 架构适配原则

所有新特性遵循以下映射关系，确保无缝融入 Mady：

| Reasonix 模式 | Mady 对应机制 | 说明 |
|---|---|---|
| 子 Agent 审查 | `Extension` + `LifecycleProvider` | Guardian 作为 Extension 注入 |
| 工具调用追踪 | `Middleware` + `context.Context` | Evidence Ledger 作为 Middleware |
| 文件快照 | `BeforeHook` on writer tools | Checkpoint 挂在 edit/write 的 Before |
| 上下文压缩 | `ContextEngine` 接口 | 四级压缩作为新 Engine 实现 |
| 计划模式门控 | `LifecycleHook.BeforeToolExecution` | Plan Mode 拦截工具执行 |
| 权限系统 | `Middleware`（执行前决策） | Permission 作为 Middleware 链的一环 |
| 策略学习 | `Extension` + `LifecycleProvider` | Memory Compiler 扩展现有 Memory |

依赖链：
```
Evidence Ledger (独立) ─┐
                       ├→ Guardian (依赖 Ledger 提供证据)
File Checkpoint (独立) ─┘
                       │
Permission (独立) ──────┼→ Plan Mode (依赖 Permission 的 ReadOnly 接口)
                       │
Tiered Compaction ─────┼→ (扩展现有 ContextEngine)
                       │
Memory Compiler ───────┘ (扩展现有 memory/)
```

---

## Phase 0: 基础设施（1 周）

### 0.1 Tool ReadOnly 接口

**目标：** 为所有工具添加只读/写入标记，作为后续 Guardian / Plan Mode / Permission 的基础。

**改动文件：**

| 文件 | 改动 |
|------|------|
| `agentcore/tool.go` | 新增 `ReadOnly() bool` 方法到 `Tool` 结构体 |
| `tools/edit.go` | `ReadOnly() = false` |
| `tools/write_file.go` | `ReadOnly() = false` |
| `tools/delete.go` | `ReadOnly() = false` |
| `tools/move.go` | `ReadOnly() = false` |
| `tools/bash.go` | `ReadOnly() = false` |
| `tools/read.go` | `ReadOnly() = true` |
| `tools/view.go` | `ReadOnly() = true` |
| `tools/grep.go` | `ReadOnly() = true` |
| `tools/glob.go` | `ReadOnly() = true` |
| `tools/ls.go` | `ReadOnly() = true` |
| `tools/git.go` | `ReadOnly() = false`（保守策略） |
| `tools/web_fetch.go` | `ReadOnly() = true` |
| `tools/web_search.go` | `ReadOnly() = true` |

**设计细节：**

```go
// agentcore/tool.go — 新增字段

type Tool struct {
    Name              string
    Description       string
    Parameters        map[string]any
    Func              ToolFunc
    Before            []BeforeHook
    After             []AfterHook
    DynamicParameters func() map[string]any

    // readOnly 标记此工具是否无副作用。
    // true = 只读取信息，不修改任何状态（文件、进程、网络写入）。
    // false = 可能产生副作用。
    // 未设置时默认 false（保守策略：不确定时视为写入）。
    ReadOnly bool
}
```

**验收标准：**
- [ ] `Tool` 结构体包含 `ReadOnly` 字段
- [ ] 所有 20+ 工具正确标记
- [ ] 单元测试验证标记一致性
- [ ] `go build ./...` + `go test ./agentcore/ ./tools/` 全通过

---

### 0.2 Evidence Ledger（工具调用证据账本）

**目标：** 记录当前 turn 内所有工具调用，提供验证查询接口。

**新增文件：**

```
agentcore/evidence/
├── doc.go              # 包文档
├── ledger.go           # Ledger 结构 + Record/Reset/Len
├── receipt.go          # Receipt 类型 + ReceiptFromToolCall
├── query.go            # 查询方法（HasSuccessfulWrite 等）
├── context.go          # context.Context 注入/提取
├── extension.go        # 作为 Extension 自动注册
└── ledger_test.go      # 单元测试
```

**核心类型设计：**

```go
// agentcore/evidence/receipt.go

// Receipt 记录一次工具调用的运行时证据。
// 仅存在于内存中（当前 turn），不序列化到 Prompt 或会话状态。
type Receipt struct {
    ToolName  string          `json:"tool_name"`
    Args      json.RawMessage `json:"args,omitempty"`
    Success   bool            `json:"success"`
    Command   string          `json:"command,omitempty"`  // bash 命令
    Paths     []string        `json:"paths,omitempty"`    // 涉及的文件路径
    Read      bool            `json:"read,omitempty"`
    Write     bool            `json:"write,omitempty"`
    DurationMs int64          `json:"duration_ms,omitempty"`
}
```

```go
// agentcore/evidence/ledger.go

// Ledger 存储当前 turn 的所有工具调用收据。
type Ledger struct {
    mu       sync.Mutex
    receipts []Receipt
}

func NewLedger() *Ledger
func (l *Ledger) Reset()                         // 每个 user turn 开始时调用
func (l *Ledger) Record(r Receipt)               // AfterToolExecution 时调用
func (l *Ledger) Len() int
```

```go
// agentcore/evidence/query.go — 查询接口

func (l *Ledger) HasSuccessfulWrite(paths []string) bool
func (l *Ledger) HasSuccessfulReadOrWrite(paths []string) bool
func (l *Ledger) HasSuccessfulCommand(command string) bool
func (l *Ledger) HasFailedCommand(command string) bool
func (l *Ledger) TouchedPaths(limit int, writtenOnly bool) []string
func (l *Ledger) HasAnySuccessfulReceipt() bool
func (l *Ledger) HasWriteOrCommandSince(index int) bool
```

```go
// agentcore/evidence/context.go — context 注入

type ledgerKey struct{}

func WithLedger(ctx context.Context, l *Ledger) context.Context
func FromContext(ctx context.Context) (*Ledger, bool)
```

```go
// agentcore/evidence/extension.go — 自动注册为 Extension

// EvidenceExtension 自动将 Ledger 注入 Agent 生命周期。
//   - BeforeTurn: Reset ledger
//   - AfterToolExecution: Record receipt
var _ agentcore.Extension = (*EvidenceExtension)(nil)
var _ agentcore.LifecycleProvider = (*EvidenceExtension)(nil)

type EvidenceExtension struct { ledger *Ledger }

func NewExtension() *EvidenceExtension
func (e *EvidenceExtension) Name() string                    // "evidence"
func (e *EvidenceExtension) Init(ctx, agent) error
func (e *EvidenceExtension) Dispose() error
func (e *EvidenceExtension) LifecycleHook() agentcore.LifecycleHook
```

**集成方式：** 通过 `LifecycleHook` 实现：
- `BeforeTurn`: 调用 `ledger.Reset()`
- `AfterToolExecution`: 从 `ToolExecutionContext` 提取工具名、参数、结果，构建 `Receipt` 并 `Record`

**路径提取逻辑：** 从 JSON 参数中提取已知键（`path`、`file_path`、`source_path`、`destination_path`、`command`），复用 `tools/path.go` 的路径规范化。

**验收标准：**
- [ ] Ledger 正确记录每次工具调用
- [ ] `HasSuccessfulWrite` 正确匹配路径
- [ ] turn 切换时正确 Reset
- [ ] 通过 `context.Context` 可在任何位置获取 Ledger
- [ ] `go test -race ./agentcore/evidence/` 通过

---

### 0.3 文件级 Checkpoint（编辑快照 + 回退）

**目标：** 在编辑工具修改文件前，保存文件原始内容，支持回退到任意 turn 的文件状态。

**新增文件：**

```
agentcore/filecheckpoint/
├── doc.go              # 包文档
├── store.go            # CheckpointStore（内存 + 可选持久化）
├── snapshot.go         # FileSnap 类型 + 快照捕获逻辑
├── restore.go          # 回退恢复逻辑
├── extension.go        # 作为 Extension + BeforeHook 自动注册
└── store_test.go       # 单元测试
```

**核心类型设计：**

```go
// agentcore/filecheckpoint/snapshot.go

// FileSnap 记录一个文件在被编辑前的状态。
// Content == nil 表示文件当时不存在（回退时删除）。
type FileSnap struct {
    Path    string  `json:"path"`
    Content *string `json:"content"` // nil = 文件不存在
}

// TurnCheckpoint 锚定一个 user turn 中所有被修改文件的编辑前状态。
type TurnCheckpoint struct {
    Turn     int64      `json:"turn"`
    Time     time.Time  `json:"time"`
    Prompt   string     `json:"prompt"`   // 用户输入
    MsgIndex int        `json:"msgIndex"` // 对话消息索引（会话回卷边界）
    Files    []FileSnap `json:"files"`
}

// Meta 是面向 UI 的摘要（不含文件内容）。
type Meta struct {
    Turn   int64
    Time   time.Time
    Prompt string
    Paths  []string
}
```

```go
// agentcore/filecheckpoint/store.go

type Store struct {
    mu     sync.Mutex
    done   []*TurnCheckpoint  // 已完成的 turn 快照
    cur    *TurnCheckpoint    // 当前 turn 的活跃快照
    seen   map[string]bool    // 本 turn 已快照的路径（去重）
    root   string             // workspace 根目录（路径逃逸防护）
}

func New(root string) *Store
func (s *Store) BeginTurn(turn int64, prompt string, msgIndex int)  // BeforeTurn 调用
func (s *Store) SnapshotFile(path string) error                      // 编辑前调用
func (s *Store) EndTurn()                                            // AfterTurn 调用
func (s *Store) List() []Meta                                        // 列出所有快照
func (s *Store) Restore(turn int64) error                            // 回退到指定 turn
```

```go
// agentcore/filecheckpoint/extension.go

// FileCheckpointExtension 作为 Extension 自动注入。
// 使用 BeforeHook 挂载到写入工具（edit、write_file、delete、move）。
var _ agentcore.Extension = (*FileCheckpointExtension)(nil)
var _ agentcore.ToolProvider = (*FileCheckpointExtension)(nil)
var _ agentcore.LifecycleProvider = (*FileCheckpointExtension)(nil)

type FileCheckpointExtension struct {
    store    *Store
    root     string
}

// 关键：通过 BeforeHook 在文件被修改前拍摄快照
func (e *FileCheckpointExtension) beforeWriteHook(
    ctx context.Context, hc *agentcore.HookContext,
) error {
    path := extractPath(hc.Arguments)
    return e.store.SnapshotFile(path)
}
```

**集成方式：**
1. 作为 `Extension` 注册
2. `LifecycleProvider.LifecycleHook()` → `BeforeTurn` 调用 `BeginTurn`，`AfterTurn` 调用 `EndTurn`
3. `ToolProvider.Tools()` → 返回零个工具（但通过修改 Config 的全局 Before 钩子注入 `beforeWriteHook`）

> **替代方案（更简洁）：** 直接在 `Config.Extensions` 中注册，在 `Init` 时通过 `agent.config.BeforeToolCall` 或 Middleware 注入快照逻辑。需要扩展 `Extension` 接口增加 `BeforeHooks() []BeforeHook` 全局钩子方法（当前 `HookProvider` 已有此方法）。

**实际落地使用 `HookProvider`：**

```go
var _ agentcore.HookProvider = (*FileCheckpointExtension)(nil)

func (e *FileCheckpointExtension) BeforeHooks() []agentcore.BeforeHook {
    return []agentcore.BeforeHook{e.beforeWriteHook}
}
```

`beforeWriteHook` 检查 `hc.ToolName` 是否为写入工具，是则提取路径并快照。

**验收标准：**
- [ ] 编辑文件前正确捕获原始内容
- [ ] 同一文件同一 turn 只快照一次（去重）
- [ ] `Restore` 正确恢复文件状态
- [ ] 路径逃逸防护（不能快照/恢复 workspace 外的文件）
- [ ] `go test -race ./agentcore/filecheckpoint/` 通过

---

## Phase 1: 安全层（2-3 周）

### 1.1 Guardian AI 安全审查

**目标：** 独立 AI 子 Agent 审查高风险工具调用，是关键词护栏的语义升级。

**新增文件：**

```
guardrails/guardian/
├── doc.go              # 包文档
├── guardian.go         # Session 结构 + Review 方法
├── policy.go           # 策略 Prompt + 配置
├── circuitbreaker.go   # 熔断器（连续拒绝检测）
├── assessment.go       # Assessment 类型 + 解析
├── transcript.go       # 从会话消息提取证据摘要
├── extension.go        # 作为 Extension + Middleware 注册
└── guardian_test.go    # 单元测试
```

**核心类型设计：**

```go
// guardrails/guardian/guardian.go

// Session 是长期存活的审查子 Agent，复用单会话以保持 prefix cache 友好。
type Session struct {
    provider  agentcore.Provider     // 专用审查模型
    policyPpt string                 // 安全策略 Prompt
    sink      agentcore.EventSink    // 事件输出

    mu                sync.Mutex
    consecutiveDenials int            // 熔断器：连续拒绝计数
    reviewCount       int            // 审查总次数
}

// Review 评估一次待执行的工具调用。
// 返回 allow=true 或 allow=false + reason。
// 非nil error 表示审查失败（fail-closed: deny）。
func (s *Session) Review(
    ctx context.Context,
    toolName string,
    args json.RawMessage,
    messages []agentcore.Message,  // 父会话消息（构建证据）
) (allow bool, reason string, err error)
```

```go
// guardrails/guardian/assessment.go

type Assessment struct {
    RiskLevel         string `json:"risk_level"`          // low/medium/high
    Outcome           string `json:"outcome"`             // allow/deny
    Rationale         string `json:"rationale"`           // 判定理由
}
```

```go
// guardrails/guardian/circuitbreaker.go

const (
    maxConsecutiveDenials = 3      // 连续 3 次拒绝 → 中断
    maxRecentDenials      = 10     // 50 次窗口内 10 次拒绝 → 中断
    recentWindow          = 50
    reviewTimeout         = 30 * time.Second
)
```

**Mady 领域定制策略 Prompt：**

```go
// guardrails/guardian/policy.go

const PatentLegalPolicy = `你是专利/法律 Agent 的安全审查员。
对每个工具调用进行评估，输出 JSON：
{"risk_level":"low|medium|high","outcome":"allow|deny","rationale":"理由"}

重点关注：
1. 是否涉及真实当事人信息（姓名、地址、证件号）的泄露
2. 是否执行了不可逆的法律操作（提交官方文件、发送正式函件）
3. 是否删除或覆盖已有案件文档
4. 是否在未经确认的情况下修改权利要求/说明书核心内容
5. 是否访问了与当前任务无关的案件文件

低风险：只读操作、格式调整、内部草稿编辑
中风险：新建文件、检索操作、草稿生成
高风险：删除文件、覆盖已有内容、外部发送、提交操作`
```

**集成方式：** 作为 `Middleware` 注入 Executor 链：

```go
// guardrails/guardian/extension.go

var (
    _ agentcore.Extension        = (*GuardianExtension)(nil)
    _ agentcore.MiddlewareProvider = (*GuardianExtension)(nil)
)

type GuardianExtension struct {
    session *Session
    enabled bool // 仅在 Strict 护栏等级下启用
}

func (e *GuardianExtension) Middleware() []agentcore.Middleware {
    if !e.enabled {
        return nil
    }
    return []agentcore.Middleware{e.guardianMiddleware}
}

func (e *GuardianExtension) guardianMiddleware(
    next agentcore.ExecuteFunc,
) agentcore.ExecuteFunc {
    return func(ctx context.Context, tc agentcore.ToolCall) (string, error) {
        // 跳过只读工具（性能优化）
        if isReadOnlyTool(tc.Name) {
            return next(ctx, tc)
        }
        // 审查
        allow, reason, err := e.session.Review(ctx, tc.Name,
            json.RawMessage(tc.Arguments), getMessages(ctx))
        if err != nil {
            // Fail-closed
            return fmt.Sprintf("blocked: 安全审查失败 — %v", err), nil
        }
        if !allow {
            return fmt.Sprintf("blocked: %s", reason), nil
        }
        return next(ctx, tc)
    }
}
```

**与现有护栏的集成：**

| 护栏等级 | Guardian 行为 |
|---------|--------------|
| Light | 不启用 |
| Standard | 仅对高风险工具（delete、bash）启用 |
| Strict | 对所有非只读工具启用 |

**验收标准：**
- [ ] 高风险调用被正确拒绝
- [ ] 只读操作不触发审查（性能保证）
- [ ] 熔断器在连续拒绝后中断
- [ ] 审查失败时 fail-closed
- [ ] `go test -race ./guardrails/guardian/` 通过

---

### 1.2 Permission System（细粒度权限门控）

**目标：** 每次工具调用独立决策 Allow/Ask/Deny，支持规则匹配。

**新增文件：**

```
agentcore/permission/
├── doc.go              # 包文档
├── permission.go       # Decision 类型 + Policy 结构
├── rule.go             # Rule 语法解析 + 匹配
├── approver.go         # Approver 接口（交互式批准）
├── extension.go        # 作为 Extension + Middleware 注册
└── permission_test.go  # 单元测试
```

**核心类型设计：**

```go
// agentcore/permission/permission.go

type Decision int
const (
    DecisionAllow Decision = iota
    DecisionAsk
    DecisionDeny
)

// Policy 评估静态规则。纯函数，无 I/O。
type Policy struct {
    Mode         Decision   // 无规则匹配时的回退（默认 Ask）
    Allow        []Rule
    Ask          []Rule
    Deny         []Rule
}

func (p Policy) Decide(
    toolName string,
    readOnly bool,
    args json.RawMessage,
) Decision
```

```go
// agentcore/permission/rule.go

// Rule 语法: "Tool" 或 "Tool(specifier)"
// 例: "Bash(go test:*)"、"Edit(docs/**)"、"Delete"
type Rule struct {
    Tool      string   // 工具名（或工具族名）
    Specifier string   // 可选限定符（命令前缀/文件路径模式）
}

func ParseRule(s string) (Rule, error)
func (r Rule) Matches(toolName string, args json.RawMessage) bool
```

```go
// agentcore/permission/approver.go

// Approver 处理 Ask 决策的交互式确认。
type Approver interface {
    Approve(
        ctx context.Context,
        toolName string,
        args json.RawMessage,
    ) Decision
}

// NonInteractiveApprover 无 TTY 时 Ask→Allow（保持自主性）
type NonInteractiveApprover struct{}
```

**集成方式：** 作为 `Middleware` 注入，排在 Guardian 之前：

```go
// agentcore/permission/extension.go

func (e *PermissionExtension) Middleware() []agentcore.Middleware {
    return []agentcore.Middleware{e.permissionMiddleware}
}

func (e *PermissionExtension) permissionMiddleware(
    next agentcore.ExecuteFunc,
) agentcore.ExecuteFunc {
    return func(ctx context.Context, tc agentcore.ToolCall) (string, error) {
        readOnly := toolReadOnly(ctx, tc.Name)
        decision := e.policy.Decide(tc.Name, readOnly, json.RawMessage(tc.Arguments))

        switch decision {
        case DecisionDeny:
            return fmt.Sprintf("blocked: 权限策略拒绝了 %s 的调用", tc.Name), nil
        case DecisionAsk:
            if e.approver != nil {
                d := e.approver.Approve(ctx, tc.Name, json.RawMessage(tc.Arguments))
                if d == DecisionDeny {
                    return fmt.Sprintf("blocked: 用户拒绝了 %s 的调用", tc.Name), nil
                }
            }
            // 无 Approver → Allow（自主模式）
        }
        return next(ctx, tc)
    }
}
```

**优先级：** `Deny > Ask > Allow > Fallback`。回退规则：只读工具 → Allow，写入工具 → Mode（默认 Ask）。

**验收标准：**
- [ ] 规则正确解析和匹配
- [ ] 优先级正确（deny 覆盖 allow）
- [ ] 非交互模式 Ask→Allow
- [ ] `go test -race ./agentcore/permission/` 通过

---

### 1.3 Plan Mode 工具门控

**目标：** 扩展现有 `/plan` 模式，在计划阶段禁止写入工具执行。

> Mady 已有 `/plan` 计划模式（commit `d8d0ff9`），当前只切换推理模型，不门控工具。

**新增文件：**

```
agentcore/planmode/
├── doc.go              # 包文档
├── policy.go           # Policy + Decision + Classify
├── readonly.go         # 只读命令分类器（bash 命令安全分析）
├── extension.go        # 作为 Extension + LifecycleHook 注册
└── policy_test.go      # 单元测试
```

**核心类型设计：**

```go
// agentcore/planmode/policy.go

// PlanSafety 是工具的计划模式安全自报告。
type PlanSafety int
const (
    PlanSafetyUnknown PlanSafety = iota  // 未实现接口，回退到白名单
    PlanSafetySafe                       // 断言可在计划阶段运行
    PlanSafetyUnsafe                     // 断言不可在计划阶段运行
)

// PlanModeClassifier 是工具可选实现的接口。
type PlanModeClassifier interface {
    PlanModeSafe() bool
}

// Call 是计划模式视角的单次工具调用视图。
type Call struct {
    Name     string
    ReadOnly bool
    Safety   PlanSafety
    Args     json.RawMessage
}

// Decision 报告计划模式是否拒绝调用。
type Decision struct {
    Blocked bool
    Message string
}

// Policy 是计划模式的工具策略。
type Policy struct {
    AllowedTools []string // 额外允许的只读工具白名单
}

func (p Policy) Decide(call Call) Decision
```

**决策逻辑：**
1. 已知阻塞工具（write_file、edit、delete、move、bash 写命令）→ Blocked
2. `PlanSafetyUnsafe` → Blocked
3. `ask`、`todo` 相关 → Allow
4. `PlanSafetySafe` 且 `ReadOnly` → Allow
5. 可信 `ReadOnly == true` → Allow
6. 其余 → Blocked（fail-closed）

**集成方式：** 作为 `LifecycleHook` 的 `BeforeToolExecution` 拦截：

```go
// agentcore/planmode/extension.go

var (
    _ agentcore.Extension         = (*PlanModeExtension)(nil)
    _ agentcore.LifecycleProvider = (*PlanModeExtension)(nil)
)

type PlanModeExtension struct {
    active  atomic.Bool  // 是否处于计划模式
    policy  Policy
}

func (e *PlanModeExtension) LifecycleHook() agentcore.LifecycleHook {
    return &planModeHook{ext: e}
}

type planModeHook struct {
    agentcore.BaseLifecycleHook
    ext *PlanModeExtension
}

func (h *planModeHook) BeforeToolExecution(
    ctx context.Context,
    arc *agentcore.AgentRunContext,
    tec *agentcore.ToolExecutionContext,
) error {
    if !h.ext.active.Load() {
        return nil // 不在计划模式，放行
    }
    for i, tc := range tec.ToolCalls {
        call := h.ext.buildCall(tc)
        decision := h.ext.policy.Decide(call)
        if decision.Blocked {
            // 替换结果为 blocked 消息
            tec.Results[i] = agentcore.ToolResult{
                ToolCallID: tc.ID,
                ToolName:   tc.Name,
                Result:     decision.Message,
                Err:        nil,
            }
        }
    }
    return nil
}

// Activate/Deactivate 由 /plan 命令调用
func (e *PlanModeExtension) Activate()
func (e *PlanModeExtension) Deactivate()
func (e *PlanModeExtension) IsActive() bool
```

**验收标准：**
- [ ] 计划模式下写入工具被阻塞
- [ ] 只读工具正常运行
- [ ] bash 写命令被识别并阻塞
- [ ] 非计划模式完全透明（无性能影响）
- [ ] `go test -race ./agentcore/planmode/` 通过

---

## Phase 2: 上下文与记忆（2-3 周）

### 2.1 四级渐进式压缩管线

**目标：** 将现有单级摘要压缩扩展为四级管线，最大化 cache 命中率。

**改动方式：** 新增 `TieredEngine` 作为 `ContextEngine` 实现，不修改现有 `CompressorEngine`。

**新增文件：**

```
agentcore/
├── context_engine_tiered.go       # TieredEngine 实现
├── context_engine_tiered_test.go  # 单元测试
```

**核心设计：**

```go
// agentcore/context_engine_tiered.go

// TieredEngine 实现四级渐进式压缩：
//   0.5 — Soft Notice: 仅日志报告，不修改消息
//   0.6 — Tool-Result Snip: 旧工具输出截断为头尾摘要
//   0.8 — Prune: 旧工具结果替换为占位符
//   0.9 — Force Fold: 强制 LLM 摘要压缩
type TieredEngine struct {
    contextLength     int64
    provider          Provider       // 摘要用 LLM
    compressionCnt    int64
    lastSavingsPct    float64

    // 分级阈值
    softNoticeRatio    float64  // 0.5
    snipRatio          float64  // 0.6
    pruneRatio         float64  // 0.8
    forceRatio         float64  // 0.9
    targetRatio        float64  // 0.5（压缩后目标）
    tailTokens         int64    // 16384（保留尾部预算）
}

func NewTieredEngine(cfg ContextEngineConfig) ContextEngine
```

**关键算法：**

```go
func (e *TieredEngine) ShouldCompact(
    msgs []Message, toolDefs []ToolDefinition, contextWindow int64,
) bool {
    estimated := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)
    ratio := float64(estimated) / float64(contextWindow)
    return ratio >= e.snipRatio // 0.6 起开始介入
}

func (e *TieredEngine) Compress(
    ctx context.Context, msgs []Message, focusTopic string,
) ([]Message, int64, error) {
    estimated := EstimateMessagesTokens(msgs)
    ratio := float64(estimated) / float64(e.contextLength)

    switch {
    case ratio >= e.forceRatio:
        // 第四级：强制摘要（复用现有 compaction.go 的摘要逻辑）
        return e.forceFold(ctx, msgs, focusTopic)

    case ratio >= e.pruneRatio:
        // 第三级：先 prune，再检查是否需要摘要
        msgs = e.pruneToolResults(msgs)
        if EstimateMessagesTokens(msgs) > int64(float64(e.contextLength)*e.pruneRatio) {
            return e.forceFold(ctx, msgs, focusTopic)
        }
        saved := estimated - EstimateMessagesTokens(msgs)
        return msgs, saved, nil

    case ratio >= e.snipRatio:
        // 第二级：截断旧工具输出
        msgs = e.snipToolResults(msgs)
        saved := estimated - EstimateMessagesTokens(msgs)
        return msgs, saved, nil

    default:
        return msgs, 0, nil
    }
}
```

**Tool-Result Snip 算法：**

```go
// snipToolResults 将超过阈值的旧工具结果截断为头尾摘要。
// 保留 assistant tool_calls 与 tool result 的配对（不删除消息）。
// 保留最近 tailTokens 的工具结果不修改。
func (e *TieredEngine) snipToolResults(msgs []Message) []Message {
    // 1. 计算尾部预算
    // 2. 从后向前累计，标记"近期保护区"
    // 3. 对保护区之外的工具结果：
    //    - 超过 snipThreshold 字符的
    //    - 替换为: head(500) + "\n[...已截断...]\n" + tail(200)
    // 4. 保留所有消息结构不变
}
```

**Tool-Result Prune 算法：**

```go
// pruneToolResults 将旧工具结果替换为短占位符。
// 比 snip 更激进：完全替换为 "[旧工具输出已清除以节省上下文空间]"
func (e *TieredEngine) pruneToolResults(msgs []Message) []Message {
    // 类似 snip，但替换为极短占位符
    // 已 prune 的不再二次处理
}
```

**Force Fold（摘要压缩）：**
复用现有 `compaction.go` 的 `summarizeMessages` 逻辑，增加关键保护：
- 短用户消息（< 1500 token）永不摘要
- 已有 compaction-summary 标记的消息不再二次摘要
- 摘要边界对齐到工具结果消息（避免孤立 tool_calls）

**注册方式：**

```go
// agentcore/context_engine.go — EngineRegistry 中注册

func NewEngineRegistry() *EngineRegistry {
    r := &EngineRegistry{...}
    r.Register("compressor", NewCompressorEngine)
    r.Register("truncate", NewTruncateEngine)
    r.Register("chunked", NewChunkedEngine)
    r.Register("tiered", NewTieredEngine)  // ← 新增
    return r
}
```

**验收标准：**
- [ ] 0.6 阈值触发 snip（工具输出被截断但消息保留）
- [ ] 0.8 阈值触发 prune（工具输出被替换为占位符）
- [ ] 0.9 阈值触发摘要压缩
- [ ] tool_calls 与 tool result 配对不被破坏
- [ ] 近期尾部不被修改
- [ ] 短用户消息保留原文
- [ ] `go test -race ./agentcore/` 全通过

---

### 2.2 Memory Compiler v5（策略学习型记忆）

**目标：** 扩展现有 `memory/` 模块，增加策略学习闭环。

**改动方式：** 新增子包，不修改现有 `memory/manager.go`。通过 Extension 组合。

**新增文件：**

```
memory/
├── compiler/
│   ├── doc.go              # 包文档
│   ├── strategy.go         # Strategy 类型 + 成功率统计
│   ├── trace.go            # ExecutionTrace + 因果边
│   ├── learning.go         # 策略学习 + 更新逻辑
│   ├── ir.go               # PlannerIR + 执行契约编译
│   ├── decay.go            # 置信度衰减 + 质量分级
│   ├── extension.go        # 作为 Extension 注册
│   └── compiler_test.go    # 单元测试
```

**Phase 2a: 策略记录与成功率统计（最小可用）**

```go
// memory/compiler/strategy.go

// Strategy 记录一种执行策略的历史表现。
type Strategy struct {
    ID            string    `json:"id"`             // 如 "patent_oa_three_step"
    Description   string    `json:"description"`
    Preconditions []string  `json:"preconditions"`  // 匹配条件
    Successes     int       `json:"successes"`
    Failures      int       `json:"failures"`
    LastUsedAt    time.Time `json:"last_used_at"`
}

func (s Strategy) Samples() int      { return s.Successes + s.Failures }
func (s Strategy) SuccessRate() float64
```

```go
// memory/compiler/trace.go

// ExecutionTrace 记录一次 turn 的执行轨迹。
type ExecutionTrace struct {
    ID          string    `json:"id"`
    Goal        string    `json:"goal"`
    StrategyID  string    `json:"strategy_id"`
    Outcome     string    `json:"outcome"`  // success/failure/partial/aborted
    ToolCalls   int       `json:"tool_calls"`
    ToolErrors  int       `json:"tool_errors"`
    StartedAt   time.Time `json:"started_at"`
    CompletedAt time.Time `json:"completed_at"`
}
```

```go
// memory/compiler/learning.go

// Compiler 管理策略学习和执行轨迹。
type Compiler struct {
    mu         sync.Mutex
    dir        string  // 持久化目录
    strategies []Strategy
    traces     []ExecutionTrace
}

func NewCompiler(dir string) *Compiler

// StartTurn 在 turn 开始时，基于历史策略产出编译建议。
func (c *Compiler) StartTurn(
    ctx context.Context, input string,
) (compiled string, strategyID string)

// FinishTurn 在 turn 结束时，记录执行结果并更新策略。
func (c *Compiler) FinishTurn(
    trace ExecutionTrace,
)
```

**集成方式：** 作为 `Extension` + `LifecycleProvider`：

```go
// memory/compiler/extension.go

var (
    _ agentcore.Extension             = (*CompilerExtension)(nil)
    _ agentcore.LifecycleProvider     = (*CompilerExtension)(nil)
    _ agentcore.TransformContextProvider = (*CompilerExtension)(nil)
)

type CompilerExtension struct {
    compiler *Compiler
    current  *turnState
}

// LifecycleHook:
//   BeforeTurn → StartTurn（产出编译建议）
//   AfterTurn  → FinishTurn（记录执行结果）

// TransformContext:
//   将编译建议注入到用户消息前缀（cache-stable 方式）
```

**Phase 2b: 记忆质量分级 + 置信度衰减**

```go
// memory/compiler/decay.go

type Quality string
const (
    QualityHigh   Quality = "HIGH_SIGNAL"
    QualityMedium Quality = "MEDIUM_SIGNAL"
    QualityNoise  Quality = "NOISE"
)

// 对现有 MemoryEntry 增加 Quality 字段（通过 Metadata map）
// 衰减：每周 5% 置信度下降（复用现有 DecayFactor）
```

**Phase 2c: ε-greedy 策略选择**

```go
// memory/compiler/strategy.go — 扩展

func SelectStrategy(
    goal string,
    strategies []Strategy,
    explorationRate int,  // 3-12%
) StrategyPick
```

**专利/法律领域预置策略：**

| 策略 ID | 描述 | 预置条件 |
|---------|------|---------|
| `oa_three_step` | 审查意见三步法答复 | "审查意见", "答复", "OA" |
| `claim_split` | 权利要求拆分分析 | "权利要求", "拆分", "技术特征" |
| `invalidity_search` | 无效检索策略 | "无效", "检索", "对比文件" |
| `patent_draft` | 专利撰写流程 | "撰写", "说明书", "交底书" |
| `legal_analysis` | 法律分析框架 | "法律", "分析", "合规" |

**验收标准：**
- [ ] 策略正确记录成功/失败
- [ ] 成功率统计准确
- [ ] 执行轨迹可追溯
- [ ] 编译建议正确注入（cache-stable 方式）
- [ ] `go test -race ./memory/compiler/` 通过

---

## 实施时间线

```
Week 1:  [Phase 0] 基础设施
         ├── Tool ReadOnly 接口 (0.1)
         ├── Evidence Ledger (0.2)
         └── 文件 Checkpoint (0.3)

Week 2:  [Phase 0 集成测试] + [Phase 1 启动]
         ├── Phase 0 集成测试 + 文档
         └── Permission System (1.2)

Week 3:  [Phase 1]
         ├── Guardian AI 审查 (1.1)
         └── Plan Mode 门控 (1.3)

Week 4:  [Phase 1 集成测试] + [Phase 2 启动]
         ├── Phase 1 集成测试 + 文档
         └── 四级压缩管线 (2.1)

Week 5:  [Phase 2]
         └── Memory Compiler Phase 2a (2.2)

Week 6:  [Phase 2]
         ├── Memory Compiler Phase 2b + 2c
         └── 全量集成测试 + 文档
```

---

## Extension 注册顺序

Middleware 执行顺序（从外到内）至关重要：

```
请求 → Permission Middleware    (最先：快速 deny)
      → Guardian Middleware     (其次：AI 审查)
      → Plan Mode LifecycleHook (工具执行前：计划阶段门控)
      → Evidence Ledger         (记录：AfterToolExecution)
      → File Checkpoint Hook    (Before：快照写入工具)
      → Tool Func               (实际执行)
```

在 `Config.Extensions` 中注册顺序：
```go
cfg.Extensions = []agentcore.Extension{
    evidence.NewExtension(),            // 1. Evidence（无 Middleware，仅 LifecycleHook）
    filecheckpoint.NewExtension(root),  // 2. Checkpoint（BeforeHook）
    permission.NewExtension(policy),    // 3. Permission（Middleware）
    guardian.NewExtension(session),     // 4. Guardian（Middleware）
    planmode.NewExtension(),            // 5. PlanMode（LifecycleHook）
    memory.NewExtension(...),           // 6. Memory（现有）
    compiler.NewExtension(...),         // 7. Compiler（新增）
}
```

> Middleware 按注册顺序构建链。Permission 在 Guardian 之前确保快速 deny 不浪费 AI 审查 token。

---

## 敏感路径影响评估

根据 `AGENTS.md` 定义的安全敏感路径：

| 新增/修改路径 | 安全边界影响 | 需要人工审阅 |
|---|---|---|
| `agentcore/tool.go` (新增 ReadOnly) | 影响工具能力门控 | **是** |
| `agentcore/permission/` | 新增权限门控 | **是** |
| `guardrails/guardian/` | 新增 AI 安全审查 | **是** |
| `agentcore/planmode/` | 计划模式工具门控 | **是** |
| `agentcore/filecheckpoint/` | 文件系统操作 | **是** |
| `agentcore/evidence/` | 仅内存追踪，无副作用 | 否 |
| `agentcore/context_engine_tiered.go` | 上下文压缩逻辑 | 否 |
| `memory/compiler/` | 记忆系统扩展 | 否 |

---

## 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Guardian 延迟过高 | 用户等待感差 | 只对非只读工具触发；设置 30s 超时；熔断器 |
| 压缩管线破坏消息配对 | 模型报错 | snip/prune 不删除消息；单元测试验证配对完整性 |
| Permission 误拦 | 正常操作被阻断 | 非交互模式 Ask→Allow；默认策略保守 |
| Memory Compiler 注入膨胀 | Token 开销增加 | 监控 IR overhead；超预算时自动 suppress |
| Plan Mode 交互混乱 | 用户困惑 | 明确的进入/退出信号；UI 指示器 |

---

## AI_CHANGELOG 记录

每个 Phase 完成后，在 `docs/decisions/AI_CHANGELOG.md` 追加记录：

```markdown
## [日期] — Phase X: [特性名]

**变更类型:** feat
**影响范围:** [涉及的包]
**安全敏感:** 是/否

**描述:**
[简述实现内容和设计决策]

**测试:**
- [x] 单元测试通过
- [x] 集成测试通过
- [x] go test -race 通过
```
