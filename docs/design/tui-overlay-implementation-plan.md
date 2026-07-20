# TUI 浮层优化实施计划

> 源于 `docs/design/tui-overlay-optimization-v0.1.md` 的设计方案
> 日期：2026-07-20 | 状态：待实施

## 实施总纲

### 改动边界

- **不改**: `tui/core/`（Layer 0）、`tui/layout/`（Layer 0）、`tui/terminal/`（Layer 1）、`tui/theme/`（Layer 2）—— 不触及底层基础设施
- **微改接口**: `tui/overlay.go`（Layer 3）—— 新增 `OverlayCategory` 类型（向后兼容扩展）
- **新增组件**: `tui/component/`（Layer 4）—— 3 个新组件
- **改造应用**: `tui/chat/`（Layer 5）—— 主界面布局调整 + 覆叠门入口
- **扩展适配**: `tui/agentadapter/`（Layer 7）—— 事件数据类型扩展（如需）
- **扩展领域数据模型**: `agentcore/` —— 复核门结构化数据（如需）

### 执行顺序

```
Phase 1 ─── 复核门 (Review Gate Overlay)       ← 最高收益/最低风险，优先
Phase 2 ─── 主界面改造 (Judgment View)          ← 核心体验，依赖 Phase 1 的数据流
Phase 3 ─── 浮层分类统一 (Overlay Classification) ← 纯重构，可并行
Phase 4 ─── 系统态浮层 (System Status Overlay)    ← 增量功能，可延迟
```

---

## Phase 1：复核门覆叠层

### 目标

在关键动作前（审批/复核），将当前的"聊天流中渲染审批卡片 + 敲命令"的模式，提升为结构化浮层审阅体验。让用户打开一个专门的复核浮层，看到：
- 当前判断摘要 + 置信度
- 依据清单（可逐项查看）
- 复核 checklist（清单式勾选进度）
- 风险提示
- 三个出口：通过复核 / 返回补证据 / 标记阻塞

### 改动清单

#### 1.1 新增 `tui/component/review_gate.go`

**ReviewGate 组件** — 一个 Focusable 的 Component，渲染整个复核门浮层内容。

```go
// ReviewGate 是"复核门"浮层的内容组件。
type ReviewGate struct {
    title       string
    judgment    string
    confidence  float64
    evidences   []ReviewEvidence
    checklist   []ReviewCheckItem
    risks       []string

    selectedIdx  int
    evidenceOpen bool

    onPass      func()
    onBack      func()
    onBlock     func()
    onClose     func()

    mu          sync.RWMutex
    km          *terminal.KeybindingsManager
    theme       ReviewGateTheme
}

type ReviewEvidence struct {
    ID      string
    Title   string
    Role    string   // "核心证据" / "辅助证据"
    Summary string
    Status  EvidenceStatus
}

type ReviewCheckItem struct {
    Label   string
    Checked bool
}

type ReviewGateTheme struct {
    Title, Border, Success, Warning, Danger, Dim  func(string) string
    Checked, Unchecked, Body                       func(string) string
}
```

**必须实现的接口**: `core.Component` → `Render(width int64) []string`、`core.Focusable` → `SetFocused(bool)`

**Update 处理的键盘事件**:
- `j/k` → 上下移动焦点项
- `Space` → 切换 evidence 展开 / checklist 勾选
- `p` → 通过复核（触发 onPass）
- `b` → 返回补证据（触发 onBack）
- `f` → 标记阻塞（触发 onBlock）
- `Esc` → 关闭（触发 onClose）

**Render 输出结构**（匹配设计稿 7.2 节）:

```
┌──────────────────────────────────────────────┐
│ 复核门                                         │
├──────────────────────────────────────────────┤
│ 当前判断                                       │
│ <judgment>                                    │
│ 置信度: █████████░ 90%                        │
│                                               │
│ 主要依据                                       │
│ ▶ 核心证据 A  [confirmed]                     │
│   ○ 对照材料 B  [pending]                     │
│                                               │
│ 复核清单                                       │
│ [x] 已列明主要依据                             │
│ [x] 已标注不确定性                             │
│ [ ] 冲突证据已处理                             │
│                                               │
│ ⚠ 风险提示                                     │
│ - <risk 1>                                    │
├──────────────────────────────────────────────┤
│ [p] 通过复核  [b] 返回补证据  [f] 标记阻塞      │
│                               [Esc] 返回       │
└──────────────────────────────────────────────┘
```

#### 1.2 新增 `tui/component/review_gate_test.go`

测试项：
- `TestReviewGateRender` — 渲染输出包含关键标题和字段
- `TestReviewGateNavigate` — j/k 移动焦点
- `TestReviewGateChecklistToggle` — 空格切换勾选
- `TestReviewGateActions` — p/b/f 三个出口键
- `TestReviewGateEvidenceExpand` — 证据项展开/折叠
- `TestReviewGateEmptyRisks` — 无风险的降级显示

#### 1.3 扩展 `tui/component/domain.go`

新增 DomainMessageType：
```go
const DomainMsgTypeReviewGate DomainMessageType = "review_gate"
```

在 `DomainMessage.Extra` 中定义 review_gate 专用键：
- `"checklist_items"` → JSON 复核清单项
- `"risks"` → JSON 风险项
- `"evidences"` → JSON 依据列表（含状态）

#### 1.4 扩展 `tui/chat/chat_app.go`

新增公开方法：
```go
func (a *ChatApp) OpenReviewGate(data ReviewGateData) OverlayRef
func (a *ChatApp) CloseReviewGate()
```

ReviewGateData 结构体：
```go
type ReviewGateData struct {
    Title      string
    Judgment   string
    Confidence float64
    Evidences  []component.ReviewEvidence
    Checklist  []component.ReviewCheckItem
    Risks      []string
    OnPass     func()
    OnBack     func()
    OnBlock    func()
}
```

**锁顺序注意**: 所有 Open/Close 方法必须遵循 `ToggleKeyHelp` 已建立的模式——在 `a.mu` 外调用 `host.PushOverlay()`。

#### 1.5 改造 `tui/chat/chat_app_stream.go`

修改 `onApprovalPrompt` 方法，在审批卡片渲染后增加复核门打开逻辑：

```go
func (a *ChatApp) onApprovalPrompt(e ChatEvent) {
    a.finalizeStreamLocked()
    a.Idle()
    // 原行为：构建审批卡片追加到历史
    // 新行为：如果 ev.Data 包含 review_gate 标记，调用 a.OpenReviewGate(data)
}
```

数据来源：扩展 `ApprovalPromptChatEvent.Data map[string]any`，让触发方同时传递结构化数据。改动涉及：
- `tui/chat/events.go` — `ApprovalPromptChatEvent.Data`
- `tui/agentadapter/adapter.go` — 映射 ev.Data
- `agentcore/event.go` — `ApprovalPromptEvent.Data`

#### 1.6 注册新键位

在 `tui/terminal/keybindings.go` 的 `DefaultKeybindings()` 中新增：
```go
"tui.review.pass":  {DefaultKeys: []KeyID{"p"}, Description: "通过复核"},
"tui.review.back":  {DefaultKeys: []KeyID{"b"}, Description: "返回补证据"},
"tui.review.block": {DefaultKeys: []KeyID{"f"}, Description: "标记阻塞"},
```

### 可验证检查清单

```
[ ] ReviewGate 组件能正常渲染判断、置信度、证据、清单、风险五个区域
[ ] j/k 可以在证据列表和清单之间移动焦点
[ ] Space 可以展开/折叠证据项、切换清单勾选
[ ] p/b/f 三个出口键各自触发正确的回调
[ ] Esc 关闭覆叠层，restore 编辑器焦点
[ ] onApprovalPrompt 触发时，同时追加审批卡片和打开覆叠层
[ ] 已勾选的清单项在重新打开后保持状态
[ ] ReviewGate 在有焦点时接受键盘输入，失焦时忽略
[ ] 测试覆盖所有键盘操作路径
[ ] 无数据泄露（浮层关闭后引用被清理）
```

---

## Phase 2：主界面改造 - Judgment View

### 目标

在主界面顶部增加一个"当前判断"区域，展示阶段、状态、判断摘要、置信度、待确认项、动作入口。聊天历史下移但不默认铺开。

### 改动清单

#### 2.1 新增 `tui/component/judgment_view.go`

```go
type JudgmentView struct {
    mu         sync.RWMutex
    phase      string   // "分析阶段" / "复核阶段"
    status     string   // "idle" / "running" / "awaiting_review" / "degraded"
    judgment   string   // 当前判断文本（一行）
    confidence int      // 0-100 或 -1（未知）
    pending    []string // 待确认项（最多 3 条）
    context    []string // 上下文摘要（最多 4 条）
    mode       string   // "normal" / "degraded"
    actions    []JudgmentAction
    dirty      bool
    cache      []string
    cacheW     int64
}

type JudgmentAction struct {
    Key        string // "r" / "e" / "s"
    Label      string // "进入复核" / "查看证据" / "系统态"
    OnActivate func()
}
```

**Render 输出结构**（匹配设计稿 5.2 节）:

```
┌──────────────────────────────────────────────────────┐
│ Mady · 阶段: awaiting_review · 模式: degraded         │
├──────────────────────────────────────────────────────┤
│ 当前判断                                              │
│ <judgment text>                                      │
│ 置信度: ███████░░░ 中                                │
│ 仍待确认: 引用来源 1 条、对比材料 2 份                 │
├──────────────────────────────────────────────────────┤
│ 当前上下文                                            │
│ - <context item 1>                                   │
├──────────────────────────────────────────────────────┤
│ [r] 复核  [e] 证据  [s] 系统  [c] 会话  [/] 命令     │
└──────────────────────────────────────────────────────┘
```

**设计要点**:
- confidence = -1 时隐藏置信度行
- pending 为空时隐藏"仍待确认"区域
- context 为空时隐藏"当前上下文"分隔线
- 所有渲染结果缓存，仅 dirty 时重建

#### 2.2 修改 `tui/chat/chat_app_layout.go`

在 `chatLayout` 中新增 `judgmentView` 字段，修改 `buildFlex()`：
```
Before: header → history → autocomplete → loader → editor → footer → statusBar
After:  header → judgmentView → history → autocomplete → loader → editor → footer → statusBar
```

新增 `updateJudgmentView(model chatModel)` 方法，根据模型状态自动更新判断区。

#### 2.3 修改 `tui/chat/chat_app.go`

新增公开方法供外部注入判断信息：
```go
func (a *ChatApp) SetJudgment(judgment string, confidence int, pending, context []string)
func (a *ChatApp) SetPhase(phase string)
func (a *ChatApp) SetMode(mode string)
```

#### 2.4 更新事件处理器

在关键事件处理器中触发 `updateJudgmentView()`：

| AgentStart → "running" | MessageDelta → "streaming" | ApprovalPrompt → "awaiting_review" | AgentEnd → "done" | AgentError → "failed" |

#### 2.5 主界面条件渲染

- **正常模式**: judgmentView 显示简短摘要
- **awaiting_review**: judgmentView 展开显示判断区
- **degraded**: judgmentView 额外显示模式标记

history 高度通过 Flex 的 FillWeight + OnAllocate 动态调整。

#### 2.6 测试

**`tui/component/judgment_view_test.go`**:
- `TestJudgmentViewRender` — 渲染包含阶段/判断
- `TestJudgmentViewEmptyStates` — 空 pending/context 隐藏
- `TestJudgmentViewActions` — 动作行渲染

**`chat_app_frame_test.go`**:
- 更新 golden snapshot

### 可验证检查清单

```
[ ] JudgmentView 在主界面顶部正确渲染
[ ] 阶段/状态/置信度信息显示正确
[ ] "仍待确认"最多 3 条，超出截断
[ ] pending/context 为空时隐藏对应区域
[ ] awaiting_review 模式下 judgmentView 展开
[ ] idle/streaming 模式下 judgmentView 缩略
[ ] degraded 模式显示标记
[ ] 历史区域随 judgmentView 高度动态调整
[ ] 动作入口键位正确路由到覆叠层
[ ] 测试覆盖所有渲染状态
```

---

## Phase 3：浮层分类统一

### 目标

将现有的单一 `overlayHandle` 扩展为四种分类浮层的统一抽象，使每类浮层的尺寸、锚点、关闭规则有明确约定。

### 改动清单

#### 3.1 扩展 `tui/overlay.go`

```go
type OverlayCategory int

const (
    OverlaySelection OverlayCategory = iota  // 选择型—快速切换
    OverlayReview                             // 审阅型—查看细节
    OverlayGate                               // 复核型—结构化审阅
    OverlaySystem                             // 系统型—运行条件解释
)

// Overlay 新增字段（向后兼容，默认 OverlaySelection）
type Overlay struct {
    // 现有字段...
    Category OverlayCategory
}

// 分类默认尺寸映射
func DefaultOverlaySize(cat OverlayCategory) (wPct, hPct int64) {
    switch cat {
    case OverlaySelection: return 40, 30
    case OverlayReview:    return 60, 60
    case OverlayGate:      return 70, 75
    case OverlaySystem:    return 50, 40
    default:               return 60, 60
    }
}
```

**不在接口新增方法**：只在 `Overlay` struct 层新增 `Category` 字段，保持 `OverlayRef` 接口不变，向后兼容。

#### 3.2 改造 `tui/chat/chat_app.go`

在 `overlayHandle` 中增加 `category` 字段，为每类浮层创建专用打开方法：
```go
func (a *ChatApp) OpenSelectionOverlay(content core.Component) OverlayRef
func (a *ChatApp) OpenReviewOverlay(content core.Component) OverlayRef
func (a *ChatApp) OpenGateOverlay(content core.Component) OverlayRef
func (a *ChatApp) OpenSystemOverlay(content core.Component) OverlayRef
```

#### 3.3 清理使用点

| 现有覆盖层 | 新分类 |
|-----------|--------|
| ToggleKeyHelp | OverlayReview（审阅型） |
| OpenReviewGate | OverlayGate（复核型） |
| SessionSelector | OverlaySelection（选择型） |
| Settings Panel | OverlayReview（审阅型） |

### 可验证检查清单

```
[ ] OverlayCategory 定义完整，4 类浮层有明确枚举值
[ ] 选择型浮层默认尺寸 40/30
[ ] 审阅型浮层默认尺寸 60/60
[ ] 复核型浮层默认尺寸 70/75
[ ] 系统型浮层默认尺寸 50/40
[ ] 现有使用点均已迁移到对应分类
[ ] 向后兼容：未指定分类的浮层默认行为不变
```

---

## Phase 4：系统态浮层

### 目标

在降级/阻塞/异常时，提供一个浮层展示系统运行条件，透明但不扰民。

### 改动清单

#### 4.1 新增 `tui/component/system_status.go`

```go
type SystemStatus struct {
    mu         sync.RWMutex
    mode       string     // "normal" / "degraded"
    modeReason string     // 降级原因
    events     []SysEvent // 最近事件
    impacts    []string   // 对当前任务的影响
    onLogDetail func()
    onClose     func()
    km         *terminal.KeybindingsManager
    theme      SystemStatusTheme
}

type SysEvent struct {
    Time    string
    Message string
    Level   string // "info" / "warn" / "error"
}
```

**Render 输出结构**（匹配设计稿 7.3 节）:

```
┌──────────────────────────────────────────────┐
│ 系统态                                         │
├──────────────────────────────────────────────┤
│ 当前运行                                       │
│ - 模式: degraded                              │
│ - 原因: Provider 不支持 json_schema            │
│                                               │
│ 最近事件                                       │
│ - 18:41 MCP 发现超时，已跳过                   │
│                                               │
│ 当前影响                                       │
│ - 输出仍可继续                                 │
│ - 结构化格式能力受限                           │
├──────────────────────────────────────────────┤
│ [l] 详细日志  [Esc] 返回                       │
└──────────────────────────────────────────────┘
```

#### 4.2 扩展 `tui/chat/chat_app.go`

```go
func (a *ChatApp) OpenSystemStatus(data SystemStatusData) OverlayRef
func (a *ChatApp) CloseSystemStatus()
```

#### 4.3 注册系统态入口

在 judgmentView 动作行中增加 `[s] 系统态` 入口。键位注册：
```go
"tui.system.open": {DefaultKeys: []KeyID{"s"}, Description: "打开系统态"}
```

### 可验证检查清单

```
[ ] SystemStatus 渲染模式、原因、事件时间线、影响摘要
[ ] 最近事件仅显示最后 3 条
[ ] 无影响时隐藏"当前影响"区域
[ ] s 键在主界面打开系统态浮层
[ ] Esc 关闭浮层
[ ] degraded 模式下 judgmentView 的模式标记与 SystemStatus 联动
```

---

## 工期估算参考

| Phase | 新增文件 | 修改文件 | 测试文件 | 估算人日 |
|-------|---------|---------|---------|---------|
| P1 复核门 | 2 | 4 | 1 | 3-4 日 |
| P2 主界面 | 2 | 3 | 1 | 4-5 日 |
| P3 浮层分类 | 0 | 3 | 0 | 1-2 日 |
| P4 系统态 | 2 | 2 | 1 | 2-3 日 |
| **总计** | **6** | **12** | **3** | **10-14 日** |

P2 和 P3 可并行执行，实际上线时间约 7-10 个工作日。

---

## 风险登记

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| `ApprovalPromptChatEvent` 缺少结构化数据 | P1 复核门无数据源 | 扩展 agentcore 事件结构（方案 A），或 TUI 侧规则解析（方案 B 后备） |
| judgmentView 与 history flex 冲突 | P2 布局闪烁 | 在 buildFlex 中用 wrapper 组件包裹，条件渲染切换模式 |
| 锁顺序反转（ChatApp.mu → host.mu） | 死锁 | 所有 Open/Close 方法在 a.mu 外调用 host 方法 |
| 现有 golden snapshot 失效 | P2 测试失败 | 修改 buildFlex 前先更新 snapshot 基准 |
| `OverlayRef` 实现者需更新 | 编译错误 | 只在 struct 层新增字段，不修改接口 |

---

## 依赖关系图

```
P1 (复核门) → 无外部依赖，可独立启动
P2 (主界面) → 依赖 P1 提供的 ReviewGateData 数据结构
P3 (浮层分类) → 无外部依赖，可独立启动
P4 (系统态) → 依赖 P2 提供的 judgmentView 动作入口集成

推荐执行顺序: P1 → P2 ← P3 (并行) → P4
```

---

## 变更记录要求

每个 Phase 完成后，必须在 `docs/decisions/AI_CHANGELOG.md` 追加记录，格式：
```
## 2026-07-20: [Phase N 标题]
### 背景
### 变更（文件列表）
### 验证
```
