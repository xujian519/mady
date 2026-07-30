# Mady TUI 差距修复实施方案

> 对照 `docs/mady-tui-standards.md` §12 差距清单和 `docs/decisions/AI_CHANGELOG.md` 差距分析报告，
> 制定的 4 个 Sprint 详细实施方案。每个任务包含：接口/类型设计、改动文件列表、验收标准和风险提示。
>
> 版本：v1.0 | 日期：2026-07-30 | 参考源：`docs/mady-tui-standards.md` §12

---

## 目录

1. [Sprint 1: 安全与质量门禁](#sprint-1-安全与质量门禁)
2. [Sprint 2: 交互增强](#sprint-2-交互增强)
3. [Sprint 3: 视觉与无障碍](#sprint-3-视觉与无障碍)
4. [Sprint 4: 功能完善](#sprint-4-功能完善)
5. [依赖关系与执行顺序](#依赖关系与执行顺序)
6. [验收标准总表](#验收标准总表)

---

## Sprint 1: 安全与质量门禁

**目标差距**：P0-1（Dispose 接口）、P0-2（最小尺寸门闩）、P1-2（CI 对比度审计）  
**总工作量**：3-5 天  
**依赖**：无（可独立执行，互不依赖）

---

### T1.1 — 定义 Disposable 接口并集成到组件生命周期

| 属性 | 值 |
|------|-----|
| **目标差距** | P0-1：组件 `Dispose()` 接口缺失 |
| **对应规则** | M-TUI-CMP-001 |
| **工作量** | 2 天（含测试） |
| **风险** | 中 — 需要审阅现有组件，确认哪些需要实现 Dispose |

#### 接口设计

在 `tui/core/component.go` 中新增：

```go
// Disposable 接口表示组件持有需要显式释放的资源（定时器、goroutine、文件句柄等）。
// TUI 在 Stop() 和 RemoveChild() 时自动调用 Dispose()。
type Disposable interface {
    Dispose()
}
```

#### 引擎集成

在 `tui/tui.go` 中：

```go
// TUI.Stop() 末尾（关闭事件循环后）：
t.forEachChild(func(c Component) {
    if d, ok := c.(Disposable); ok {
        d.Dispose()
    }
})

// TUI.RemoveChild(name) 中：
c, ok := t.children[name]
delete(t.children, name)
if d, ok := c.(Disposable); ok {
    d.Dispose()
}
```

同时新增 `TUI.Children() []Component` 方法（供测试验证 `Dispose()` 调用）。

#### 需要实现 Dispose 的组件清单

通过代码审计，以下组件持有需要释放的资源：

| 组件 | 资源 | 文件 |
|------|------|------|
| `SessionSelector` | 定时器（自动刷新）、回调 goroutine | `tui/component/session_selector.go` |
| `Loader` | Tick goroutine（动画帧） | `tui/component/loader.go` |
| `ChatApp` | Subscriber 订阅、FSM goroutine | `tui/chat/chat_app.go` |
| `ChatHistory` | 渲染缓存 map、滚动状态 | `tui/chat/chat_history.go` |
| `FileWatcher`（theme/watch.go） | mtime 轮询 goroutine | `tui/theme/watch.go` — 注意：watcher 独立于 TUI 事件循环，由 `runWithRestart` 管理 |

**不涉及**（无资源需释放）：`Box`、`Text`、`Input`、`Editor`（不持有 goroutine/timer）、`SelectList`、`Table`、`Viewport`、`Markdown`、`Syntax`、`StatusBar`、各种卡片组件。

#### 测试策略

| 测试类型 | 内容 |
|---------|------|
| 单元测试 | `TestDisposableInterface` — 类型断言验证 | 
| 集成测试 | `TestStopDisposesChildren` — 创建持资源的 mock 组件，Start 后 Stop，验证 Dispose 被调用 |
| 回归测试 | 每个实现了 Disposable 的组件增加 `TestDispose`，验证 Dispose 后资源状态 |

#### 验收标准

- [ ] `core.Disposable` 接口定义在 `core/component.go` 中
- [ ] `TUI.Stop()` 遍历所有子组件，对 `Disposable` 调用 `Dispose()`
- [ ] `TUI.RemoveChild()` 移除时调用 `Dispose()`
- [ ] `SessionSelector` 实现 `Disposable`（关闭定时器 + 回调 goroutine）
- [ ] `Loader` 实现 `Disposable`（停止动画 tick）
- [ ] `ChatApp` 实现 `Disposable`（取消订阅、清理 FSM goroutine）
- [ ] 新增测试覆盖全部 dispose 路径
- [ ] `go test -race ./tui/...` 全绿

---

### T1.2 — 最小尺寸门闩

| 属性 | 值 |
|------|-----|
| **目标差距** | P0-2：<80×24 无提示 |
| **对应规则** | TUI-LAY-002 |
| **工作量** | 0.5 天 |
| **风险** | 低 — 零侵入，仅在渲染路径添加前置检查 |

#### 设计

在 `tui/tui_render.go` 的 `renderFrame()` 入口处添加：

```go
const minCols, minRows int64 = 80, 24

func (t *TUI) renderFrame() {
    cols, rows := t.Size()
    if cols < minCols || rows < minRows {
        t.renderResizeHint(cols, rows)
        return
    }
    // ... 现有渲染逻辑
}
```

`renderResizeHint` 在终端中央显示纯文本提示：

```
╔════════════════════════════════════════════════╗
║                                                ║
║        终端窗口太小，请放大以继续使用           ║
║                                                ║
║          Terminal too small. Please resize.     ║
║                                                ║
║        最小尺寸: 80×24  |  当前: 60×15         ║
║                                                ║
╚════════════════════════════════════════════════╝
```

**关键设计决策**：
- 纯文本 UI（不依赖组件系统），确保门闩自身在任何尺寸下都可用
- 双语言提示（中/英文）
- 实时更新当前尺寸
- 使用 `Box` 组件的基本框线字符（`─` `│` `╔` `╗` `╚` `╝`），这些是 ANSI 通用字符
- 不使用任何颜色/样式（纯文本），确保在最小尺寸下渲染不出错

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/tui_render.go` | `renderFrame` 入口 + 新增 `renderResizeHint` 方法 |
| `tui/tui_render_test.go` | 新增测试：`TestRenderResizeHint`（cols=60, rows=15 验证提示输出）|

#### 验收标准

- [ ] cols < 80 或 rows < 24 时不渲染正常 UI，显示 resize 提示
- [ ] 提示框显示当前终端尺寸
- [ ] 终端重新 resize 到 ≥80×24 后自动恢复
- [ ] 提示仅在渲染层拦截，不破坏 eventLoop 或 cmd 执行
- [ ] 新增测试 `TestRenderFrameTooSmall` 验证门闩行为
- [ ] `go test -race ./tui/...` 全绿

---

### T1.3 — CI 对比度审计

| 属性 | 值 |
|------|-----|
| **目标差距** | P1-2：CI 对比度审计缺失 |
| **对应规则** | M-TUI-TST-005 |
| **工作量** | 1 天 |
| **风险** | 低 — 新增脚本 + CI job，不影响运行时 |

#### 设计

新增 `tui/scripts/validate-colors.sh`，读取 `tui/theme/semantic_theme.go` 中的内置主题定义，验证每对 fg/bg 组合的对比度 ≥ 4.5:1。

**对比度计算算法（WCAG 2.1）**：

```
relativeLuminance(RGB):
    sRGB 线性化 → L = 0.2126*R + 0.7152*G + 0.0722*B
contrastRatio(L1, L2) = (L1 + 0.05) / (L2 + 0.05)  // L1 > L2
```

**验证对**（每个主题）：
- `Text` on `Background`（正文对比度）
- `Text` on `Surface`（面板内对比度）
- `Muted` on `Background`
- `Muted` on `Surface`
- `Emphasis` on `Background`
- `Accent` on `Background`
- `Success` on `Background`
- `Warning` on `Background`
- `Error` on `Background`
- `Info` on `Background`
- `UserMessage` on `Background`
- `AssistantText` on `Background`

**超出 4.5:1 的视为 PASS，低于的打印 FAIL 并退出码 1**。

#### 实现文件

| 文件 | 改动类型 |
|------|---------|
| `tui/scripts/validate-colors.sh` | **新增** — Bash 脚本读取 Go 源文件提取 hex 值并计算 |
| `.github/workflows/ci.yml` | **修改** — 新增 `validate-colors` job（或添加到 `verify-tui-layers` job） |

**脚本实现方案**（二选一）：

| 方案 | 优点 | 缺点 |
|------|------|------|
| **A: Bash+awk** 从 Go 源文件提取 hex 值，python3 计算对比度 | 无新依赖 | 解析 `semantic_theme.go` 的 Go 结构体较脆弱 |
| **B: Go 程序** 引入 `go run` 直接调用 Go 包解析主题 | 精确可靠 | 增加 CI 编译时间 ~5s |

**推荐方案 A** — 对比度审计只需要 hex 值对，通过 `grep "#[0-9a-fA-F]{6}"` 提取结构体字面量中的颜色值即可，不需要理解 Go 语义。

#### 验收标准

- [ ] `tui/scripts/validate-colors.sh` 对每个内置主题验证所有 fg/bg 组合
- [ ] `high-contrast` 主题全部 PASS，`mady-dark` 主题全部 PASS
- [ ] CI 中存在 `validate-colors` job
- [ ] PR 中若修改了色彩值，CI 自动检测对比度是否达标
- [ ] CI job 失败时输出具体的对比度值和不达标对

---

## Sprint 2: 交互增强

**目标差距**：P1-1（Footer 组件）、P1-3（CI=true 处理）、P2-2（SIGWINCH 去抖）、P2-3（搜索模式）  
**总工作量**：5-7 天  
**依赖**：无（可独立执行）

---

### T2.1 — Footer 组件

| 属性 | 值 |
|------|-----|
| **目标差距** | P1-1：Footer 组件缺失 |
| **对应规则** | M-TUI-KB-005，帮助系统三层 |
| **工作量** | 2 天 |
| **风险** | 低 — 新增组件，不影响现有布局 |

#### 组件设计

```go
// Footer 是 TUI 底部常驻的快捷键提示栏。
// 显示 3-5 个核心快捷键分组，跟随终端宽度自动调整密度。
type Footer struct {
    // 左对齐的快捷键组
    leftGroups []FooterGroup
    // 右对齐的快捷键组（如版本号、连接状态）
    rightItems []FooterItem
}

// FooterGroup 是一组相关快捷键
type FooterGroup struct {
    Label  string        // 组标签，如 "搜索"
    Items  []FooterItem
}

// FooterItem 是一个快捷键提示
type FooterItem struct {
    Key  string   // 按键名，如 "?"
    Desc string   // 描述，如 "help"
}
```

**布局策略**：
- 80 列以下：只显示 `[?] help` + `[Ctrl+P] cmd` + `[Ctrl+C] quit`
- 80-120 列：增加 `[/] search` + `[Tab] focus`
- 120 列以上：显示全部 5-7 组

**渲染规则**：
- 单行渲染，不换行（Footer 不占额外空间）
- 超过终端宽度时从右删除组（优先保留左对齐的核心组）
- 分隔符 `·` 连接连续项
- 按键名使用 `accent` 色，描述使用 `muted` 色
- `NO_COLOR` 模式下用括号代替颜色：`(?) help`

**注册接口**：

```go
func (f *Footer) RegisterGroup(name string, items ...FooterItem)
func (f *Footer) ClearGroup(name string)
```

#### 集成到 ChatApp

在 `chat_app_layout.go` 的 `chatLayout` 中新增 footer 行（状态栏上方）：

```
┌──────────────────────────────────────┐  ← header
│  消息历史                             │
│                                      │
│                                      │
├──────────────────────────────────────┤
│ [?] help · [Ctrl+P] cmd · [q] quit   │  ← Footer
│ Status: Ready             Tokens: 0   │  ← StatusBar
│ > input text...                      │  ← Editor
└──────────────────────────────────────┘
```

**注意**：Footer 不是 Overlay，必须常驻。放在 StatusBar 下方（Editor 上方）。

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/component/footer.go` | **新增** — Footer 组件 |
| `tui/component/footer_test.go` | **新增** — 不同宽度下的渲染快照 |
| `tui/chat/chat_app_layout.go` | **修改** — chatLayout 集成 Footer |
| `tui/chat/chat_app.go` | **修改** — 暴露 Footer 注册接口 |

#### 验收标准

- [ ] Footer 组件在 <80 列显示至少 3 个快捷键
- [ ] 快捷键按键名使用 accent 色，描述使用 muted 色
- [ ] `NO_COLOR` 模式下用括号代替颜色
- [ ] Footer 不滚动、不换行、不遮挡内容
- [ ] ChatApp 中注册 5 个默认组（help/search/commands/navigation/quit）
- [ ] 测试覆盖 <80、80-120、≥120 三种宽度
- [ ] `go test -race ./tui/...` 全绿

---

### T2.2 — CI=true 环境变量处理

| 属性 | 值 |
|------|-----|
| **目标差距** | P1-3：`CI=true` 无处理 |
| **对应规则** | TUI-TRM 环境变量契约 |
| **工作量** | 0.5 天 |
| **风险** | 低 |

#### 设计

在 `tui/tui.go` 的 `NewTUI` 构造器中（或 `tui/terminal/detect.go` 的终端能力检测中）添加：

```go
func isCIEnvironment() bool {
    // CI 环境变量参考：https://docs.github.com/en/actions/learn-github-actions/variables
    // 常见的 CI 环境变量：CI, GITHUB_ACTIONS, GITLAB_CI, JENKINS_HOME, TF_BUILD
    return os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true"
}
```

在 `NewTUI` 中：

```go
if isCIEnvironment() {
    opts.DisableSynchronizedOutput = true   // CSI 2026 在 CI 中无意义
    ForceColor(false)                       // 禁用颜色
    // 不进入 Alternate Screen（CI 中无真终端）
}
```

在 `cmd/mady/tui.go` 的 `runTui` 入口处也添加检测：

```go
if isCIEnvironment() {
    fmt.Fprintln(os.Stderr, "mady: CI environment detected, TUI mode unavailable")
    os.Exit(0)
}
```

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/terminal/detect.go` | **修改** — 新增 `IsCI()` 函数 |
| `tui/tui.go` | **修改** — `NewTUI` 中检测 CI 禁用互动功能 |
| `cmd/mady/tui.go` | **修改** — CI 下直接退出 |
| `tui/terminal/detect_test.go` | **修改** — 新增 `TestIsCI` 表驱动测试 |

#### 验收标准

- [ ] `CI=true mady tui` 不进入 TUI，直接退出并打印提示
- [ ] `CI=true` 环境下 TUI 创建时自动禁用颜色和同步输出
- [ ] `GITHUB_ACTIONS=true` 同样触发
- [ ] 环境变量不存在时不影响正常行为
- [ ] 测试覆盖 true/false/未设置三种状态

---

### T2.3 — SIGWINCH 100ms 去抖

| 属性 | 值 |
|------|-----|
| **目标差距** | P2-2：SIGWINCH 无去抖 |
| **对应规则** | M-TUI-LAY-002 |
| **工作量** | 0.5 天 |
| **风险** | 低 |

#### 设计

在 `tui/tui_input.go` 的 `processMsg` 中，对 `WindowSizeMsg` 的处理增加去抖：

```go
// Debounce window resize events
// 参考 XiaoNuo TUI-LAY-010：100ms debounce for SIGWINCH
const resizeDebounce = 100 * time.Millisecond

// TUI 字段
resizeTimer *time.Timer
```

```go
case core.WindowSizeMsg:
    t.handleResizeDebounced(m)
```

`handleResizeDebounced` 实现：

```go
func (t *TUI) handleResizeDebounced(m core.WindowSizeMsg) {
    if t.resizeTimer != nil {
        t.resizeTimer.Stop()
    }
    t.resizeTimer = time.AfterFunc(resizeDebounce, func() {
        // 在 eventLoop goroutine 中执行
        t.msgCh <- m
    })
    // 但首次 resize 立即响应（不等待去抖）
    // 使用一个标志区分"首次"和"持续高频"
}
```

**简化方案**（推荐）：在现有 `WindowSizeMsg` 处理前加 `sinceLastResize` 时间检查，小于 100ms 则丢弃：

```go
case core.WindowSizeMsg:
    now := time.Now()
    if now.Sub(t.lastResizeTime) < resizeDebounce {
        break // 丢弃高频 resize
    }
    t.lastResizeTime = now
    // 现有处理逻辑
    t.prevCols = -1 // 强制全量重绘
    t.RequestRender()
```

此方案最简单，零风险，且丢失的 resize 不会导致布局错误（最终 resize 稳定后触发一次即可）。

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/tui.go` | **修改** — 新增 `lastResizeTime time.Time` 字段 |
| `tui/tui_input.go` | **修改** — `WindowSizeMsg` handler 增加去抖判断 |

#### 验收标准

- [ ] 两次 resize 间隔 < 100ms 时第二次被丢弃
- [ ] 最终稳定尺寸的 resize 必定到达
- [ ] resize 去抖不影响其他事件处理
- [ ] 新增测试 `TestWindowSizeDebounce`（模拟连续 resize 事件）
- [ ] `go test -race ./tui/...` 全绿

---

### T2.4 — ChatHistory 搜索模式（/ 搜索）

| 属性 | 值 |
|------|-----|
| **目标差距** | P2-3：搜索模式缺失 |
| **对应规则** | M-TUI-KB-004，交互设计 §4.4 |
| **工作量** | 2-3 天 |
| **风险** | 中 — 涉及 ChatHistory 渲染状态机 |

#### 交互设计

```
键盘触发：
  /     → 激活搜索模式（在 Footer 显示搜索输入）
  n     → 下一个匹配
  N     → 上一个匹配
  Esc   → 退出搜索
  Enter → 确认搜索（保持高亮）

视觉反馈：
  - 匹配行：accent 色高亮
  - 当前选中匹配：accent+bold+反色
  - 无匹配：Footer 显示"未找到"（灰色）
  - 匹配计数：Footer 显示"3/15 条匹配"
```

#### 搜索状态机

```go
type searchState struct {
    active       bool      // 是否搜索模式
    query        string    // 搜索关键词
    matches      []int     // 匹配的消息 index 列表
    currentMatch int       // 当前选中的匹配在 matches 中的索引
    exact        bool      // 精确匹配（' 前缀）vs 模糊匹配
}

const searchExactPrefix = "'"
```

**算法**：
- 每次搜索词变化时在 `ChatHistory.messages` 中遍历，匹配消息文本
- 匹配模式：`strings.Contains`（不区分大小写）
- 精确模式：`strings.Contains`（区分大小写）
- 匹配结果缓存到 `searchState.matches`
- 消息循环 + key handler 处理 `n`/`N`/`Esc`/`Enter`

#### 渲染集成

在 `chat_history_render_message.go` 的消息渲染中：

```go
func (h *ChatHistory) isSearchMatch(idx int) bool {
    return h.search.active && slices.Contains(h.search.matches, idx)
}

func (h *ChatHistory) isCurrentSearchHit(idx int) bool {
    return h.search.active &&
        len(h.search.matches) > 0 &&
        h.search.currentMatch >= 0 &&
        h.search.currentMatch < len(h.search.matches) &&
        h.search.matches[h.search.currentMatch] == idx
}
```

匹配行渲染时使用 `Accent` 色和 `Bold` 属性，当前选中匹配额外反转背景。

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/chat/chat_history.go` | **修改** — 新增 `searchState` 字段和方法 |
| `tui/chat/chat_history_render.go` | **修改** — 搜索高亮渲染 |
| `tui/chat/chat_history_render_message.go` | **修改** — 消息行搜索匹配高亮 |
| `tui/chat/chat_history_input.go` | **修改** — `/`、`n`、`N`、`Esc` 按键路由 |
| `tui/chat/chat_history_selection.go` | **修改** — 搜索模式不冲突 |
| `tui/chat/chat_history_test.go` | **修改** — 搜索功能测试 |

#### 验收标准

- [ ] `/` 键激活搜索模式
- [ ] 实时搜索：输入关键词后即时高亮匹配
- [ ] `n` 跳转到下一个匹配（滚动视口到匹配行）
- [ ] `N` 跳转到上一个匹配
- [ ] `Esc` 退出搜索模式，清除高亮
- [ ] `'` 前缀切换精确匹配模式
- [ ] Footer/状态栏显示"3/15 matches"
- [ ] 无匹配时显示"未找到"
- [ ] 搜索不修改消息内容（只影响渲染）
- [ ] 测试覆盖：搜索激活/退出、n/N 导航、回车确认、无匹配情况
- [ ] `go test -race ./tui/...` 全绿

---

## Sprint 3: 视觉与无障碍

**目标差距**：P2-1（响应式断点）、P2-4（reduceMotion）、P3-2（Nerd Font 检测）  
**总工作量**：3-5 天  
**依赖**：Sprint 2 T2.1（Footer 组件用于显示布局模式）

---

### T3.1 — 响应式断点布局

| 属性 | 值 |
|------|-----|
| **目标差距** | P2-1：响应式断点缺失 |
| **对应规则** | M-TUI-LAY-003 |
| **工作量** | 2 天 |
| **风险** | 中 — 影响聊天布局组件 |

#### 断点定义

```go
type LayoutBreakpoint int

const (
    LayoutCompact  LayoutBreakpoint = iota // < 80 columns
    LayoutStandard                         // 80-159 columns
    LayoutWide                             // ≥ 160 columns
)

func detectLayoutBreakpoint(cols int64) LayoutBreakpoint {
    switch {
    case cols >= 160:
        return LayoutWide
    case cols >= 80:
        return LayoutStandard
    default:
        return LayoutCompact
    }
}
```

#### 布局行为

| 断点 | 布局策略 |
|------|---------|
| **Compact** (<80) | 全屏单栏：仅消息历史 + 输入栏，所有面板通过 Overlay 覆盖，Footer 最短 |
| **Standard** (80-159) | 当前标准布局：历史 + 状态栏 + 编辑器 + Footer |
| **Wide** (≥160) | 右侧显示可选的上下文面板（如证据摘要、判断视图），主区域不受影响 |

#### 实现步骤

**Phase 1**（核心）：检测 + 通知

1. 在 `tui/chat/chat_app_layout.go` 的 `chatLayout` 中增加断点检测
2. 每次 `Render(width)` 时计算当前断点
3. 根据断点调整 `chatLayout` 的子组件可见性

**Phase 2**（增强）：Wide 断点的侧边栏

1. 新增 `ContextPanel` 组件（右侧边栏）
2. Wide 模式下显示（使用 `layout/flex.go` 的水平布局）
3. 侧边栏内容：`JudgmentView` + `TodoPanel` + 系统状态

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/core/layout.go` 或 `tui/layout/breakpoint.go` | **新增/修改** — 断点检测 |
| `tui/chat/chat_app_layout.go` | **修改** — 根据断点调整布局 |
| `tui/chat/chat_app.go` | **修改** — 暴露布局模式状态 |
| `tui/component/context_panel.go` | **新增** — Wide 断点侧边栏 |
| `tui/component/context_panel_test.go` | **新增** — 断点渲染测试 |

#### 验收标准

- [ ] <80 列进入 Compact 模式（全屏单栏）
- [ ] 80-159 列进入 Standard 模式（当前布局）
- [ ] ≥160 列进入 Wide 模式（右侧侧边栏）
- [ ] resize 跨断点边界时自动切换布局
- [ ] 每个断点下所有功能可访问（只是布局不同）
- [ ] 测试覆盖三种断点的渲染输出

---

### T3.2 — reduceMotion 配置

| 属性 | 值 |
|------|-----|
| **目标差距** | P2-4：reduceMotion 配置缺失 |
| **对应规则** | TUI-A11Y-020 |
| **工作量** | 0.5 天 |
| **风险** | 低 |

#### 设计

```go
// tui/core/options.go（如果不存在则放在 tui/theme/global.go 中）

var reduceMotion atomic.Bool

func SetReduceMotion(v bool) {
    reduceMotion.Store(v)
    // 触发主题变更通知，让所有 Loader 停止动画
    OnThemeChange()
}

func IsReduceMotion() bool {
    return reduceMotion.Load()
}
```

**效果**：
- `Loader` 组件：`IsReduceMotion()` 为 true 时仅显示静态占位符（如 `...` 或固定符号），不进行帧动画
- `SpinnerStyle`：在 stdio 模式下直接输出初始帧并停止
- `ProgressBar`：无填充动画，直接跳转进度

**触发方式**：
- 自动：检测系统 `prefers-reduced-motion`（macOS 通过 `system_appearance.go` 扩展）
- 手动：`~/.mady/config.toml` 中添加 `[tui] reduce_motion = true`

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/core/spinner_style.go` 或 `tui/theme/global.go` | **修改** — 新增 `SetReduceMotion`/`IsReduceMotion` |
| `tui/component/loader.go` | **修改** — 检测 `IsReduceMotion`，true 时不启动 tick |
| `tui/stdio/spinner.go` | **修改** — 检测 `IsReduceMotion` |

#### 验收标准

- [ ] `SetReduceMotion(true)` 后 Loader 停止动画
- [ ] `SetReduceMotion(true)` 后 stdio.Spinner 停止动画
- [ ] 默认关闭（`IsReduceMotion() == false`）
- [ ] 可通过环境变量或配置文件覆盖
- [ ] 新增测试：`TestLoaderReduceMotion`、`TestSpinnerReduceMotion`

---

### T3.3 — Nerd Font 自动检测

| 属性 | 值 |
|------|-----|
| **目标差距** | P3-2：Nerd Font 检测缺失 |
| **对应规则** | TUI-ICO-001 |
| **工作量** | 1 天 |
| **风险** | 低 |

#### 检测策略

在 `tui/terminal/detect.go` 中新增检测能力：

```go
type NerdFontStatus int

const (
    NerdFontUnknown NerdFontStatus = iota  // 未检测
    NerdFontAvailable                      // Nerd Fonts 可用
    NerdFontUnavailable                    // Nerd Fonts 不可用
)

// DetectNerdFonts 通过多种启发式方法检测终端是否支持 Nerd Fonts。
// 优先使用显式环境变量覆盖，其次使用光标探测。
func DetectNerdFonts() NerdFontStatus {
    // 1. 环境变量显式覆盖
    switch os.Getenv("NERD_FONT") {
    case "1", "true":
        return NerdFontAvailable
    case "0", "false":
        return NerdFontUnavailable
    }

    // 2. 通过 $TERM 或 $TERM_PROGRAM 启发式判断
    //    iTerm2, Kitty, WezTerm, Alacritty, Ghostty 等现代终端
    //    用户大多安装了 Nerd Fonts
    termProg := os.Getenv("TERM_PROGRAM")
    switch termProg {
    case "iTerm.app", "WezTerm", "kitty", "ghostty":
        return NerdFontAvailable // 大概率支持
    }

    // 3. 光标探测：写入 Nerd Font 字符并读取光标位置变化
    //    （高级方案，初期可跳过）
    return NerdFontUnavailable
}
```

**图标回退系统**：

```go
// Icon 结构体表示一个支持多级回退的图标。
type Icon struct {
    NerdFont string // Nerd Font 字符（PUA 区）
    Unicode  string // Unicode 回退
    ASCII    string // ASCII 回退
}

// Resolve 根据终端能力返回最适合的图标字符串。
func ResolveIcon(ic Icon) string {
    switch {
    case nerdFont.Load() == NerdFontAvailable && ic.NerdFont != "":
        return ic.NerdFont
    case ic.Unicode != "":
        return ic.Unicode
    default:
        return ic.ASCII
    }
}

// 预定义图标
var (
    IconFolder  = Icon{NerdFont: "\uf07b", Unicode: "📁", ASCII: "[D]"}
    IconFile    = Icon{NerdFont: "\uf15b", Unicode: "📄", ASCII: "[F]"}
    IconGit     = Icon{NerdFont: "\uf1d3", Unicode: "⎇",  ASCII: "[G]"}
    IconSearch  = Icon{NerdFont: "\uf002", Unicode: "🔍", ASCII: "[S]"}
    IconGear    = Icon{NerdFont: "\uf013", Unicode: "⚙",  ASCII: "[C]"}
)
```

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/terminal/detect.go` | **修改** — 新增 `DetectNerdFonts()` 函数 |
| `tui/theme/style.go` 或新增 `tui/core/icon.go` | **新增/修改** — Icon 类型 + ResolveIcon 函数 + 预定义图标 |
| `tui/theme/global.go` | **修改** — 启动时检测 |
| `tui/terminal/detect_test.go` | **修改** — Nerd Font 检测测试 |

#### 验收标准

- [ ] `NERD_FONT=true` 环境变量强制启用 Nerd Fonts
- [ ] `NERD_FONT=false` 环境变量强制禁用
- [ ] 未设置时使用启发式检测
- [ ] 每个图标定义都有三级回退（Nerd Font → Unicode → ASCII）
- [ ] 图标渲染不会因字体缺失而显示空白方块
- [ ] 新增测试覆盖三种 Nerd Font 状态

---

## Sprint 4: 功能完善

**目标差距**：P3-1（行内确认模式）、P3-3（Watchdog 监控）  
**总工作量**：3-5 天  
**依赖**：Sprint 2 T2.1（Footer 用于显示确认提示）

---

### T4.1 — 行内确认模式

| 属性 | 值 |
|------|-----|
| **目标差距** | P3-1：行内确认模式缺失 |
| **对应规则** | 确认机制 §4.6 |
| **工作量** | 1-2 天 |
| **风险** | 低 |

#### 设计

新增轻量级确认原语 `InlineConfirm`，用于中等严重度操作（删除单个消息、清空输入、覆盖文件等）。不打开模态覆盖层，直接在状态栏/Footer 区域显示确认提示。

```go
// InlineConfirm 是行内确认状态。
// 当必须确认时，ChatApp 进入 ConfirmPending 状态，
// 提交按钮暂停，等待 y/n 输入。
type InlineConfirm struct {
    Prompt  string        // 确认提示，如 "Delete this message?"
    OnYes   func()        // y 执行
    OnNo    func()        // n/any 取消
    Timeout time.Duration // 超时自动取消（默认 10s）
}
```

**FSM 集成**：在 `chat/state.go` 中新增状态 `StateConfirmPending`：

```
Idle --ConfirmRequest--> ConfirmPending
ConfirmPending --y--> Idle (并执行 OnYes)
ConfirmPending --n/Esc--> Idle (并执行 OnNo)
ConfirmPending --timeout--> Idle (取消)
```

**UI 显示**：在 Footer/StatusBar 区域短暂覆盖显示确认提示：

```
[?] help ·  Delete this message? (y/N) ·
```

按键 `y`/`Y` 确认，`n`/`N`/`Esc`/超时 取消。

#### 使用场景

```go
// 在 ChatApp 中
func (a *ChatApp) ConfirmDeleteMessage(idx int) {
    a.startConfirm(InlineConfirm{
        Prompt: "Delete this message?",
        OnYes:  func() { a.deleteMessage(idx) },
        OnNo:   func() { /* nothing */ },
    })
}
```

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/chat/state.go` | **修改** — 新增 `StateConfirmPending` + 转换 |
| `tui/chat/chat_app.go` | **修改** — `startConfirm` 方法 + 事件处理 |
| `tui/chat/chat_app_layout.go` | **修改** — Footer 区域显示确认提示 |
| `tui/chat/events.go` | **修改** — 新增 `ConfirmChatEvent` |
| `tui/chat/state_test.go` | **修改** — FSM 新增转换测试 |

#### 验收标准

- [ ] `startConfirm()` 后进入 `StateConfirmPending`
- [ ] Footer/状态栏显示确认提示
- [ ] `y`/`Y` 执行 OnYes
- [ ] `n`/`N`/`Esc`/超时执行 OnNo
- [ ] 超时自动取消（默认 10s）
- [ ] 确认期间其他输入被屏蔽（焦点陷阱）
- [ ] FSM 测试覆盖：进入、确认、取消、超时四条路径
- [ ] `go test -race ./tui/...` 全绿

---

### T4.2 — Watchdog 监控

| 属性 | 值 |
|------|-----|
| **目标差距** | P3-3：Watchdog 监控缺失 |
| **对应规则** | M-TUI-ERR-001 |
| **工作量** | 1-2 天 |
| **风险** | 低 |

#### 设计

在 `tui/tui_loop.go` 的事件循环中增加 watchdog goroutine：

```go
// TUI 结构体新增字段：
type TUI struct {
    // ...
    watchdogLastEvent   time.Time        // 上次事件处理完成时间
    watchdogThreshold   time.Duration    // 默认 5s
    watchdogTriggered   atomic.Bool      // 是否已触发（避免重复日志）
}
```

**事件循环中打标记**：

```go
func (t *TUI) processMsg(msg Msg) Msg {
    defer func() {
        t.watchdogLastEvent = time.Now()
        t.watchdogTriggered.Store(false)  // 重置
    }()
    // ... 现有处理逻辑
}
```

**Watchdog goroutine**（在 `Start()` 中启动，`Stop()` 中停止）：

```go
func (t *TUI) startWatchdog() {
    go func() {
        ticker := time.NewTicker(t.watchdogThreshold)
        defer ticker.Stop()
        for {
            select {
            case <-t.ctx.Done():
                return
            case <-ticker.C:
                since := time.Since(t.watchdogLastEvent)
                if since > t.watchdogThreshold && !t.watchdogTriggered.Load() {
                    t.watchdogTriggered.Store(true)
                    // 输出诊断信息——使用 slog（写入日志文件）
                    slog.Warn("TUI watchdog: event loop appears stuck",
                        "since_last_event", since.Round(time.Millisecond).String(),
                        "threshold", t.watchdogThreshold,
                        "goroutines", runtime.NumGoroutine(),
                    )
                    // 同时尝试通过 DebugOverlay 显示
                    t.debugWatchdogStuck(since)
                }
            }
        }
    }()
}
```

**不自动恢复**：Watchdog 只诊断不干预。自动恢复（panic 重启等）需要更复杂的设计且可能掩盖 bug。

#### 实现文件

| 文件 | 改动 |
|------|------|
| `tui/tui.go` | **修改** — 新增 watchdog 字段 |
| `tui/tui_loop.go` | **修改** — eventLoop 中打标记 |
| `tui/tui_input.go` | **修改** — `processMsg` 中标记完成时间 |
| `tui/tui_lifecycle.go` | **修改** — `Start()` 启动 watchdog goroutine，`Stop()` 停止 |
| `tui/tui_lifecycle_test.go` | **修改** — watchdog 测试 |

#### 验收标准

- [ ] eventLoop 正常运行时看门狗不触发
- [ ] `processMsg` 阻塞 > 5s 时 watchdog 触发日志
- [ ] watchdog 只记录一次（`watchdogTriggered` 标志防止重复日志刷屏）
- [ ] eventLoop 恢复后自动重置
- [ ] TUI.Stop() 时 watchdog goroutine 正确退出
- [ ] 测试覆盖：正常处理、阻塞恢复、Stop 时退出三种场景
- [ ] `go test -race ./tui/...` 全绿

---

## 依赖关系与执行顺序

```
Sprint 1 (独立)      Sprint 2 (独立)       Sprint 3 (依赖 S2)     Sprint 4 (依赖 S2)
─────────────────   ─────────────────    ──────────────────     ──────────────────
T1.1 Dispose()       T2.1 Footer ◄────── T3.1 响应式断点        T4.1 行内确认 ◄────
T1.2 最小尺寸门闩    T2.2 CI=true         T3.2 reduceMotion      T4.2 Watchdog
T1.3 CI 对比度审计   T2.3 SIGWINCH 去抖   T3.3 Nerd Font 检测
                     T2.4 搜索模式
```

| 路径 | 依赖原因 |
|------|---------|
| T2.1 → T3.1 | 响应式断点的布局模式指示需要 Footer 显示 |
| T2.1 → T4.1 | 行内确认的提示文字需要 Footer 作为显示区域 |
| T2.3 优先于 T3.1 | 响应式断点依赖 resize 事件，先去抖避免干扰 |

**建议执行顺序**：Sprint 1 → Sprint 2 → Sprint 3 → Sprint 4，每个 Sprint 内部按子任务编号顺序执行。

---

## 验收标准总表

| 任务 | 验证方法 | 最低门禁 |
|------|---------|---------|
| T1.1 Dispose | `go test -race -count=1 ./tui/...` | 全绿 + 新增测试覆盖所有 dispose 路径 |
| T1.2 最小尺寸门闩 | `go test -race -count=1 ./tui/...` | 全绿 + `TestRenderFrameTooSmall` |
| T1.3 CI 对比度审计 | `bash tui/scripts/validate-colors.sh` | 全部主题通过 4.5:1 |
| T2.1 Footer | `go test -race -count=1 ./tui/...` | 全绿 + 三种宽度渲染快照 |
| T2.2 CI=true | `CI=true go test -race -count=1 ./tui/...` | 不触发交互路径 |
| T2.3 SIGWINCH 去抖 | `go test -race -count=1 ./tui/...` | 全绿 + 连续 resize 测试 |
| T2.4 搜索模式 | `go test -race -count=1 ./tui/...` | 全绿 + 搜索功能测试 |
| T3.1 响应式断点 | `go test -race -count=1 ./tui/...` | 全绿 + 三种断点渲染测试 |
| T3.2 reduceMotion | `go test -race -count=1 ./tui/...` | 全绿 + Loader/Spinner 动画停止 |
| T3.3 Nerd Font | `go test -race -count=1 ./tui/...` | 全绿 + 三级回退测试 |
| T4.1 行内确认 | `go test -race -count=1 ./tui/...` | 全绿 + FSM 四条路径测试 |
| T4.2 Watchdog | `go test -race -count=1 ./tui/...` | 全绿 + watchdog 触发/恢复测试 |

**全局门禁**：
- [ ] 每个子任务提交前：`cd tui && go test -race -count=1 ./...` 10 包全绿
- [ ] 每个子任务提交前：`golangci-lint run ./tui/...` 0 issues
- [ ] 每个子任务提交前：根模块 `go build ./...` 通过
- [ ] 完整 Sprint 交付后：`bash tui/scripts/verify_layers.sh` 文件同步检查

---

> 本文档是 `docs/mady-tui-standards.md` §12 差距分析与改进路线图的具体实施方案。
> 每个任务包含接口设计、文件改动列表、验收标准和工作量估算，可直接用于开发排期。
