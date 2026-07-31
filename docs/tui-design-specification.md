# Mady TUI 设计规范

> 综合开源社区最佳实践（Elm Architecture / Bubble Tea / Ratatui / Textual）与 Mady 自研 8 层 TUI 架构的设计规范。
>
> 适用对象：TUI 组件开发者、主题设计者、新功能贡献者。
> 本文档是 `tui/LAYERS.md` 的配套规范——前者描述**架构层级**（what），后者定义**设计准则**（how）。

---

## 目录

1. [核心原则](#1-核心原则)
2. [架构规范](#2-架构规范)
3. [组件设计规范](#3-组件设计规范)
4. [事件与消息规范](#4-事件与消息规范)
5. [终端 I/O 规范](#5-终端-io-规范)
6. [主题与视觉规范](#6-主题与视觉规范)
7. [布局规范](#7-布局规范)
8. [动画与动效规范](#8-动画与动效规范)
9. [无障碍规范](#9-无障碍规范)
10. [测试规范](#10-测试规范)
11. [性能规范](#11-性能规范)
12. [文档与贡献规范](#12-文档与贡献规范)

---

## 1. 核心原则

### 1.1 架构原则

| 原则 | 说明 | 来源 |
|------|------|------|
| **单向数据流** | Msg → Update → View（Elm 架构），无双向绑定 | Elm / Bubble Tea |
| **分层依赖** | 高编号层可依赖低编号层，反向禁止 | 项目 LAYERS.md |
| **组件自包含** | 每个组件封装自己的 Render + Update + 状态 | React / Elm |
| **分离 Side Effect** | 阻塞 I/O 放入 Cmd，Update 必须 < 1ms | Bubble Tea / TEA |
| **差分渲染** | 帧间比对仅输出变化部分，减少终端带宽 | Ratatui / 项目实现 |
| **终端独立** | 通过 `Terminal` 接口隔离平台差异 | 平台无关设计 |

### 1.2 开源社区借鉴

| 框架 | 语言 | 核心贡献 | Mady 对应 |
|------|------|----------|-----------|
| **Elm Architecture** | Elm | Msg/Cmd/Update/Sub 模式，纯函数 UI 架构 | `tui/core` Msg/Cmd 接口，`tui` eventLoop |
| **Bubble Tea** | Go | 完整的 TEA 实现（tea.Program），Cmd 组合器 | 项目自研的 eventLoop + Batch/Sequence |
| **Ratatui** | Rust | 内存缓冲 + 差分渲染，Widget 组合模式 | `core/cell.go` CellGrid + `core/celldiff.go` DiffRows |
| **Textual** | Python | 响应式 CSS 布局、消息/响应系统 | `tui/layout/flex.go` 声明式布局 |
| **Charm** | Go | 主题系统、鼠标支持、Kitty 协议 | `tui/theme` 语义主题 + `tui/terminal` Kitty 支持 |
| **tcell** | Go | 终端抽象层（termbox 后继） | `tui/terminal` Terminal 接口 + 平台实现 |

---

## 2. 架构规范

### 2.1 8 层架构概览

```
┌─────────────────────────────────────────────────────────┐
│  Layer 7:  Agent Adapter   (tui/agentadapter)            │
│      Agent 事件 → ChatEvent 转换，BindAgent              │
├─────────────────────────────────────────────────────────┤
│  Layer 5:  Application     (tui/chat)                    │
│      ChatApp 应用逻辑、ChatHistory、状态机 (FSM)          │
├─────────────────────────────────────────────────────────┤
│  Layer 4:  Components      (tui/component)               │
│      41 个 UI 组件：Editor/Markdown/SelectList 等         │
├─────────────────────────────────────────────────────────┤
│  Layer 3:  Engine          (tui 根包)                    │
│      TUI 容器、事件循环、Overlay 系统、焦点栈              │
├─────────────────────────────────────────────────────────┤
│  Layer 2:  Theming         (tui/theme)                   │
│      Palette/SemanticTheme、JSON 加载、热重载              │
├─────────────────────────────────────────────────────────┤
│  Layer 1:  Terminal I/O    (tui/terminal)                │
│      终端抽象、按键解析、Kitty 协议、ANSI 构造              │
├─────────────────────────────────────────────────────────┤
│  Layer 0:  Core + Layout   (tui/core + tui/layout)       │
│      Component/MSg/Cell/Row、Flex 声明式布局               │
│      ★ 纯数据层，无 I/O、无主题依赖                        │
└─────────────────────────────────────────────────────────┘
```

> 注：Layer 6（`tui/stdio`，过程式 stdout/stdin 渲染）曾存在，后已移除——
> 所有渲染统一走 `core.Component` 模型。层编号不连续为既有事实，勿按编号推导依赖。

### 2.2 依赖规则

- **严格单向**：Layer N 可导入 Layer < N，反向禁止。
- **循环引用禁止**：通过接口（`AppHost`、`Subscriber`、`Loader` 回调）在编译期打破循环。
- **核心零依赖**：`tui/core` 和 `tui/layout` 只能依赖标准库和内部辅助包（`internal/`）。
- **Agentcore 隔离**：`tui/chat` 不导入 `agentcore`，通过 `tui/agentadapter` 转换事件。

### 2.3 事件循环

```
┌──────────┐     ┌───────────┐     ┌───────────┐
│  Terminal │────→│  msgCh    │────→│ eventLoop │
│  (stdin)  │     │ (channel) │     │ goroutine │
└──────────┘     └───────────┘     └───────┬───┘
                                           │
              ┌────────────────────────────┤
              ▼                            ▼
       ┌───────────┐              ┌──────────────┐
       │ processMsg │              │ renderFrame  │
       │ Update()   │              │ 差分渲染 +    │
       │ → Cmd      │              │ CSI 输出      │
       └─────┬─────┘              └──────────────┘
             │ Cmd 在独立 goroutine 执行
             ▼
       ┌───────────┐
       │ result Msg │────→ msgCh (回到循环)
       └───────────┘
```

**关键约束：**
- `Update(Msg)` 必须 < 1ms，禁止阻塞 I/O。
- Cmd 在独立 goroutine 执行，panic 由 `PanicMsg` 捕获而非崩溃。
- 渲染帧通过 `RequestRender()` 合并突发请求（ticker 节流到 tickInterval）。
- `processMsg`、Cmd 执行、渲染三阶段在单 goroutine 内串行化，无需锁。

---

## 3. 组件设计规范

### 3.1 Component 接口契约

```go
type Component interface {
    Render(width int64) []string
    Invalidate()
}
```

**契约要求：**

| 方法 | 行为规范 |
|------|----------|
| `Render(width)` | 返回的每一行**不得**超过 `width` 列；超出由 `normalizeLine` 裁剪。宽字符（CJK 全角）按 2 列计算。 |
| `Invalidate()` | 丢弃缓存的渲染状态。当主题/宽度/数据变更时，TUI 引擎自动调用。 |

**可选接口：**

| 接口 | 用途 |
|------|------|
| `Sizer` | 声明自然高度，布局容器测量时避免二次渲染 |
| `Updatable` | 参与 Msg 驱动更新、返回 Cmd |
| `Focusable` | 持有硬件光标，支持 IME 候选窗口定位 |
| `WantsKeyRelease` | 可选，如需要 Kitchen 按键释放事件则实现 |

### 3.2 组件命名与文件组织

**文件拆分规则：**
- 组件核心逻辑 ≤ 400 行，超过按责任拆分到 `*_edit.go`、`*_render.go`、`*_history.go` 等兄弟文件。
- 每组命名：`<component>.go` + `*_<concern>.go`。
- 示例：Editor 拆分 `editor.go`（核心）、`editor_edit.go`（按键）、`editor_render.go`（渲染）、`editor_history.go`（历史）、`editor_killring.go`（杀环）。

**不变量：**
- 不跨包引用本组件私有类型。
- 导出类型必须有 Godoc 注释。
- 组件必须实现自己的测试文件。

### 3.3 组件状态管理

- 状态存储为组件结构体字段，不通过全局变量。
- 当状态收复杂（≥3 个互斥状态）时，使用显式 FSM（如 `tui/chat/state.go`）。
- FSM 状态转换枚举完整覆盖，禁止默认 fallthrough。

### 3.4 组件分层定位

| 类别 | 目录 | 说明 |
|------|------|------|
| 基础组件 | `component/` | 可复用的通用 UI 元素（Box/Text/Input/Table/SelectList） |
| 内容组件 | `component/` | Markdown/Editor/Loader/Image/KeyHelp |
| 专业卡片 | `component/` | 领域相关（ToolCard/ApprovalCard/EvidenceCard/ConclusionCard） |
| 面板组件 | `component/` | 浮层面板（SessionSelector/SkillCenter/Settings/TodoPanel） |
| 应用组件 | `chat/` | 聊天应用层组件（ChatApp/ChatHistory） |

---

## 4. 事件与消息规范

### 4.1 Msg 类型体系

```go
// Msg 是 Elm 风格事件循环中的消息标记接口。
// 外部包可通过嵌入 MsgBase 或定义 MsgMarker() 方法实现。
type Msg interface { MsgMarker() }

type MsgBase struct{}          // 嵌入用零值
func (MsgBase) MsgMarker() {}  // 接口实现
```

**内置 Msg 类型及用途：**

| Msg | 触发条件 | 处理策略 |
|-----|----------|----------|
| `KeyMsg` | 终端按键 | 通过 `KeybindingsManager.MatchesKey` 分发 |
| `PasteMsg` | 粘贴（Bracketed Paste） | 直接插入 Editor 缓冲区 |
| `WindowSizeMsg` | SIGWINCH | 重新布局所有组件 |
| `TickMsg` | 定时器触发 | 用于 Loader 动画帧 |
| `MouseMsg` | 鼠标事件 | 组件 hit-test + 点击响应 |
| `QuitMsg` | 退出信号 | 触发 TUI 关闭流程 |
| `BatchMsg` | 多个 Cmd 的聚合结果 | 逐一派发到 `processMsg` |
| `SequenceMessage` | 有序 Cmd 链 | 依次执行 |
| `PanicMsg` | Cmd 运行中 panic | 记录日志，不崩溃 |
| `CtxMessage` | 上下文包裹的 Msg | 解包后派发 |

### 4.2 键盘事件规范

**按键标识格式：**
`[modifier+...]keyname`，例如 `ctrl+c`、`alt+shift+enter`、`ctrl+shift+p`。

**修饰符命名：**
| 修饰符 | KeyID 标识 | 说明 |
|--------|-----------|------|
| Ctrl | `ctrl` | 如 `ctrl+c` |
| Alt | `alt` | 如 `alt+enter` |
| Shift | `shift` | 仅对非字母键有意义（字母默认大写） |
| Super | `super` | macOS Cmd / Windows Win |
| Meta | `meta` | 较少使用 |

**键盘协议：**
- 默认使用传统 xterm 转义序列。
- 终端支持时启用 **Kitty 键盘协议**（`CSI u`），获取 press/repeat/release 区分。
- KitKitty 标志位默认 `flags=1`（消歧义），CJK IME 用户需知晓 flag 8 影响候选窗口定位。

**快捷键层级：**

```
全局（引擎层） → 应用层（ChatApp） → 焦点组件（Editor/SelectList 等）
```

- 全局快捷键由 `TUI.Filter` 拦截。
- 应用层快捷键由 `ChatApp` 的 `handleKeyEvent` 处理。
- 焦点组件快捷键由组件的 `Update` 处理。

**保留快捷键约定：**

| 快捷键 | 作用 | 层级 |
|--------|------|------|
| `Ctrl+c` | 复制 / 中断 | 全局（未选中时退出） |
| `Ctrl+d` | 中断输入 | 应用层 |
| `Ctrl+p` | 命令面板 | 应用层 |
| `Ctrl+space` | 切换 Sidebar | 应用层（规划） |
| `Tab` | 聚焦切换 | 引擎层 |
| `Esc` | 取消 / 关闭浮层 | 引擎层 |

### 4.3 鼠标事件规范

- 默认关闭鼠标事件，通过 `TUIOptions.MouseMode` 开启。
- 支持模式：`off`（关闭）、`x11`（基础）、`sgr`（扩展，推荐）。
- 鼠标事件坐标系：**0-base**（Row 0 = 终端第一行 / Col 0 = 第一列）。
- 组件通过 hit-test（`Row`、`Col` 落在本组件区域内）响应鼠标事件。

---

## 5. 终端 I/O 规范

### 5.1 ANSI 转义规范

| 序列 | 用途 | 规范 |
|------|------|------|
| CSI `?25h` / `?25l` | 光标显示/隐藏 | 状态式管理，避免每帧发出 |
| CSI `?2004h` / `?2004l` | Bracketed Paste 模式 | Start/Stop 时切换 |
| CSI `?2026h` / `?2026l` | 同步输出（Synchronized Output） | 包裹每帧渲染 |
| CSI `?7h` / `?7l` | DECAWM 自动换行 | 帧内关闭避免宽字符换行 |
| CSI `[>Nu` / `[<u` | KitKey 键盘协议推/拉 | Start + AltScreen 时推送 |
| SGR `38;2;r;g;b` | 24-bit 真彩色前景 | 首选色彩表示 |
| SGR `48;2;r;g;b` | 24-bit 真彩色背景 | 首选色彩表示 |
| OSC 8 | 超链接（Hyperlink） | Markdown 渲染中可选支持 |

### 5.2 终端能力检测

通过 `TerminalContext` 在启动时检测：

```go
type TerminalContext struct {
    ColorDepth     int    // 1/4/8/24 位颜色
    KittyKeyboard bool   // 是否支持 Kitty 键盘协议
    KittyGraphics bool   // 是否支持 Kitty 图像协议
    SixelGraphics bool   // 是否支持 Sixel 图像协议
    iTerm2        bool   // 是否支持 iTerm2 图像协议
    SyncOutput    bool   // 是否支持同步输出
    Unicode       bool   // 是否支持宽字符（CJK）
}
```

**降级策略：**

| 功能 | 最佳情况 | 降级情况 |
|------|---------|----------|
| 颜色 | 24-bit 真彩色 | 256 色 → 16 色 → 8 色 |
| 图像 | Kitty 协议 | iTerm2 → Sixel → 无图像（文字回退） |
| 键盘 | KitKitty 协议 (CSI u) | xterm 传统转义序列 |
| 输出 | 同步输出 (CSI 2026) | 无同步（可能有撕裂） |
| CJK | 终端原生支持 | 字符宽度检测失败时按单列处理 |

### 5.3 平台适配

| 平台 | 实现 | 特殊处理 |
|------|------|----------|
| macOS | `terminal_darwin.go` | termios 通过 `syscall` + `golang.org/x/sys/unix` |
| Linux | `terminal_linux.go` | 同上 |
| 其他 | `terminal_other.go` | 最小化功能（有限回退） |

### 5.4 VirtualTerminal 测试双端

所有终端交互必须通过 `Terminal` 接口；测试使用 `VirtualTerminal`（`tui/terminal/terminal.go`）：

```go
vt := NewVirtualTerminal(80, 24)
// 注入输入（CSI 序列：\x1b[5~ 为 Home/PageUp 键，等价于按一次该键）
vt.Type("hello\x1b[5~")
// 获取输出
output := vt.OutputString()
```

---

## 6. 主题与视觉规范

### 6.1 语义主题 Token

`SemanticTheme` 定义了完整的主题 Token 集，分五个类别：

**颜色 Token（33 个）：**

| 类别 | Token | 用途 |
|------|-------|------|
| 基础 | `Background`, `Surface`, `SurfaceRaised` | 背景层级 |
| 核心 | `Accent`, `Text`, `Muted`, `Dim` | 文字与强调 |
| 边框 | `Border`, `BorderAccent`, `BorderMuted` | 边界线 |
| 语义 | `Success`, `Error`, `Warning`, `System` | 状态指示 |
| 角色 | `UserMessage`, `AssistantText`, `ThinkingText` | 对话角色 |
| 背景 | `UserMessageBg`, `SelectedBg` | 背景填充 |
| 工具 | `ToolPendingBg`, `ToolSuccessBg`, `ToolErrorBg` | 工具状态 |
| Markdown | `MdHeading`, `MdLink`, `MdCode`, `MdCodeBlock` 等 9 个 | Markdown 渲染 |
| 语法高亮 | `Syntax*` 9 个 | 代码语法着色 |
| 证据 | `EvidenceSupport`, `EvidenceCounter` | 正反证据 |
| 置信度 | `ConfidenceLow/Medium/High` | 置信度可视化 |
| 动画 | `LoaderSpinner`, `ProgressBar` | 加载动画 |

### 6.2 主题文件格式

JSON 格式，遵循 `theme/json.go` 的变量引用语义：

```json
{
  "name": "mady-dark",
  "accent": "#33aaff",
  "border": "$accent@0.5",
  "text": "#e2e8f0",
  "background": "#0b1120",
  "surface": "#111827"
}
```

- 支持变量引用：`$accent`、`$accent@0.5`（透明度混合）。
- 主题文件通过文件监控（`theme/watch.go`）支持热重载。

### 6.3 内置主题

注册表（`theme/theme_registry.go`）当前内置 8 个主题：

| 主题 | 特点 | 切换命令 |
|------|------|----------|
| `mady-dark`（默认） | 冷色深色主题，Logo 灵感：#0B1120 底 + #38BDF8 Accent | `/theme dark` |
| `mady-light` | 冰白浅色：#F8FAFC 底 + #0EA5E9 Accent | `/theme light` |
| `tokyo-night` | Tokyo Night 社区配色 | `/theme tokyo-night` |
| `rose-pine-moon` | Rose Pine Moon 社区配色 | `/theme rose-pine-moon` |
| `grok-night` | Grok Night 社区配色 | `/theme grok-night` |
| `high-contrast` | 无障碍：纯黑/白/蓝，WCAG AA 4.5:1 | `/theme high-contrast` |
| `colorblind` | 无障碍：蓝橙替代红绿（deuteranopia/protanopia safe） | `/theme colorblind` |
| `auto` | 跟随系统深/浅外观 | `/theme auto` |

### 6.4 组件视觉规范

| 元素 | 规范 |
|------|------|
| **边框** | 单线框（`│├─┤`），聚焦时使用 `BorderAccent` |
| **面板** | 带标题的框，标题 `Bold` + `Accent` |
| **选中项** | `SelectedBg` 背景 + text 前景色 |
| **链接** | `MdLink` + 下划线，可选 OSC 8 超链接 |
| **代码** | `MdCode` 前景 + 衬底背景 |
| **代码块** | `MdCodeBlock` + `MdCodeBlockBorder` 边框 |
| **引用** | `MdQuote` + `MdQuoteBorder` 左边框 |
| **水平线** | `MdHr` 全宽虚线 |

### 6.5 品牌色彩系统

Logo 视觉语言设计原则：

- **汇聚感**：布局两侧向中心聚焦，重要操作居中高亮。
- **理性冷色**：深蓝/冰蓝为主，琥珀仅作可选主题。
- **律令感**：清晰边框、低饱和灰背景、克制用色。
- **中道**：所有视觉决策避免极端（不刺眼、不过朴）。

---

## 7. 布局规范

### 7.1 布局系统

使用 `tui/layout/flex.go` 声明式 Flex 布局：

```go
// FlexDirection 定义主轴方向
type FlexDirection int
const (
    FlexRow    FlexDirection = iota  // 水平排列
    FlexColumn                        // 垂直排列
)

// FlexAlign 定义交叉轴对齐
type FlexAlign int
const (
    FlexAlignStart    FlexAlign = 0  // 起始对齐
    FlexAlignCenter   FlexAlign = 1  // 居中
    FlexAlignEnd      FlexAlign = 2  // 末尾对齐
    FlexAlignStretch  FlexAlign = 3  // 拉伸填充
)
```

### 7.2 总体布局结构

```
┌─── Header (1行) ──────────────────────────────────────┐
├─── Main Content Area ─────────────────────────────────┤
│   ┌──────┬───────────────────────────────────────┐   │
│   │Sidebar│          ChatHistory                  │   │
│   │20-24列│                                       │   │
│   │       │                                       │   │
│   └──────┴───────────────────────────────────────┘   │
├─── Input Area ───────────────────────────────────────┤
│   │ [autocomplete下拉]                               │
│   │ [Loader动画]                                     │
│   │ ┌Editor (输入框) ─────────────────────────┐      │
│   │ │ >> _                                     │      │
│   │ └──────────────────────────────────────────┘      │
├─── StatusBar (1行) ──────────────────────────────────┤
│ Mady · mode=agent · model=gpt4    ⚡ 1.2k tokens     │
└──────────────────────────────────────────────────────┘
```

### 7.3 响应式断点

当前实现（`tui/layout/breakpoint.go`）为 3 级断点：

| 终端宽度 | 布局行为 |
|----------|----------|
| ≥ 160 列 | Wide——预留扩展侧栏空间 |
| 80–159 列 | Standard——完整布局 |
| < 80 列 | Compact——footer 折叠为紧凑模式，浮层面板优先 |

> **规划中**：规范愿景中的 Sidebar（20-24 列会话栏、60-79 列折叠为 8 列图标栏、
> 40 列以下隐藏）尚未实现——当前 chat 应用无 Sidebar 组件。待 Sidebar 落地时
> 按下列演进断点实现：≥120 列（Sidebar + 主工作区 + 右侧辅助）/ 80–119 列
> （完整 Sidebar）/ 60–79 列（图标栏）/ <60 列（隐藏，`Ctrl+Space` 或 `/panel`
> 切换浮层）。

### 7.4 Overlay 系统

Overlay 用于浮层面板（浮于主内容之上）：

```go
type Overlay struct {
    Component core.Component  // 浮层内容
    X, Y      int64           // 左上角偏移（-1 表示居中）
    Width     int64           // 浮层宽度（0=用内容宽度）
    Height    int64           // 浮层高度（0=用内容高度）
    Modal     bool            // 模态（阻止背景交互）
}
```

**实现说明**：当前实现采用反向字段 `NonModal`（`tui/overlay.go`）——零值保持模态，
与既有 overlay 向后兼容；规范中的 `Modal: true` 行为由 `NonModal: false`（零值）表达。
非模态浮层打开时，输入在送达聚焦组件后仍会广播给背景组件（`tui/tui_input.go`），
用于不阻塞主工作区的辅助面板。

**Overlay 使用场景：**
- 快捷键帮助（`KeyHelp`）
- 命令面板（`CommandCenter`）
- 大图预览
- 确认对话框
- 可折叠的浮层面板（会话选择/技能中心/待办/设置）

**Overlay 不适用场景（应放 Sidebar）：**
- 常用导航面板
- 频繁切换的列表视图

---

## 8. 动画与动效规范

### 8.1 动效设计原则

TUI 动效依赖帧重绘，应**克制**：

1. **功能性**：动效服务于状态传达，不装饰。
2. **低帧率**：最小帧间隔 8ms（上限 125fps），默认 60fps。
3. **可跳过**：快捷键操作到目标状态应直接跳转，不等待动画完成。
4. **不闪烁**：避免背景色交替变化（对光敏用户不友好）。

### 8.2 允许的动效

| 动效 | 实现方式 | 帧率 | 说明 |
|------|----------|------|------|
| 加载 Spinner | `core.SpinnerStyle` | interval 80–120ms | `component.Loader` |
| 流式光标 | 光标状态切换 | 500ms 闪烁 | 终端原生管理，非 TUI 帧循环 |
| 思考块折叠 | `▶`/`▼` 符号 | 即时 | 不逐帧展开 |
| 工具调用折叠 | 可折叠为单行 | 即时 | 点击展开详情 |
| Overlay 弹出 | 即时渲染 | 即时 | 渐入会显得迟缓 |

### 8.3 SpinnerStyle 规范

```go
// SpinnerStyle 是纯数据类型（动画帧 + 间隔），无渲染依赖。
// 核心层提供预设变量：
var SpinnerDots   = SpinnerStyle{Frames: []string{"⠋","⠙","⠹","⠸","⠼","⠴","⠦","⠧","⠇","⠏"}, Interval: 80}
var SpinnerLine   = SpinnerStyle{Frames: []string{"─","╱","│","╲"}, Interval: 100}
var SpinnerBounce = SpinnerStyle{Frames: []string{"⠁","⠂","⠄","⡀","⢀","⠠","⠐","⠈"}, Interval: 100}
```

- `SpinnerStyle` 在 `core` 包（Layer 0），因为 `component.Loader`（Layer 4）需要引用。
- 自定义：创建新的 `SpinnerStyle` 值即可，无需注册。

---

## 9. 无障碍规范

### 9.1 焦点可见性

- **所有**可交互组件（Editor, SelectList, Button, Tab, 面板）必须显示焦点指示。
- 焦点指示方式：`BorderAccent` 边框 或 `SelectedBg` 反色背景。
- `Focusable` 接口必须在焦点变化时触发 `Invalidate()`。

### 9.2 键盘导航

| 操作 | 快捷键 | 行为 |
|------|--------|------|
| 聚焦下一个 | `Tab` | 组件间前向切换 |
| 聚焦上一个 | `Shift+Tab` | 组件间后向切换 |
| 激活/选择 | `Enter` | 当前聚焦项确认 |
| 返回/取消 | `Esc` | 关闭浮层/取消选择 |
| 命令面板 | `Ctrl+P` | 全局命令搜索 |
| 关闭应用 | `Ctrl+C`（未选中时） | 退出 TUI |

### 9.3 高对比度模式

- 提供高对比度主题（`/theme high-contrast`），确保 **4.5:1** 对比度（WCAG AA）。
- 实现方式：提供独立的 `high-contrast.json` 主题文件，使用纯黑/白/蓝三色。
- 切换后**所有组件**必须重新渲染以保证主题一致性。

### 9.4 不唯颜色传达信息

| 状态 | 视觉指示 | 非颜色回退 |
|------|---------|-----------|
| 成功 | `Success` (绿色) | `✓` 图标 + 文字 |
| 错误 | `Error` (红色) | `✗` 图标 + 文字 |
| 警告 | `Warning` (黄色) | `⚠` 图标 + 文字 |
| 系统消息 | `System` (橙色) | `ℹ` 前缀 |
| 用户角色 | `UserMessage` (强调色) | `You:` 文字前缀 |
| 助手角色 | `AssistantText` (默认色) | `Assistant:` 文字前缀 |

### 9.5 屏幕阅读器支持

> **已知限制**：当前实现不提供专门的屏幕阅读器明文输出路径——渲染帧通过
> CSI 序列输出，屏幕阅读器需依赖终端复用器/辅助工具（ORCA、brltty 等）的
> 原始输出捕获。规范愿景（关键状态变化——错误、模式切换、Agent 切换——
> 通过 `term.Write()` 输出明文）作为未来演进方向保留，暂不实现。

---

## 10. 测试规范

### 10.1 测试层级

| 层级 | 范围 | 工具 | 覆盖目标 |
|------|------|------|----------|
| 单元测试 | 单个组件/函数 | `go test` | 渲染逻辑、事件响应、状态转换 |
| 集成测试 | 组件间交互 | VirtualTerminal | 事件流、布局组合、Overlay |
| 周期测试 | 长运行 | VirtualTerminal | 内存泄露、竞态、流式渲染 |

### 10.2 组件测试要点

| 组件方面 | 测试内容 |
|----------|---------|
| 渲染 | 给定 width 应返回指定行数，不超宽 |
| 事件 | 按键 → 状态变更 → 重新渲染 |
| 边界 | 空数据、超长文本、最小 width |
| 主题 | 主题切换后 `Invalidate()` 调用 |
| 竞态 | `-race` 模式下并发 Update 和 Render |

### 10.3 VirtualTerminal 使用模式

```go
func TestChatApp(t *testing.T) {
    vt := terminal.NewVirtualTerminal(80, 24)
    tui := NewTUI(vt, ...)
    go tui.Start()  // 异步启动，不阻塞

    // 模拟用户输入
    vt.Type("hello world")

    // 验证输出
    output := vt.OutputString()
    assert.Contains(t, output, "hello world")

    tui.Stop()
}
```

### 10.4 测试验证要求

- `go test -race ./tui/...` 必须通过（竞态检测强制）。
- 新增组件必须有 ≥ 70% 行覆盖率。
- 组件 `Render` 方法必须测试不同 `width` 下的表现。

---

## 11. 性能规范

### 11.1 渲染性能

| 指标 | 目标 | 测量方式 |
|------|------|----------|
| 帧渲染时间 | < 16ms（60fps） | eventLoop tick 耗时 |
| 首帧渲染 | < 50ms | 从 Start() 到首次 ShowCursor |
| 流式文本 | ≥ 100 字符/帧 | Delta 渲染吞吐量 |
| 内存分配 | < 10KB/帧 | `go test -benchmem` |
| Overlay 合成 | < 1ms | 10 层 Overlay 叠加 |

### 11.2 渲染优化策略

```
1. 差分渲染（DiffFrame）
   → 仅输出变化的 cell 段，通过 CSI MoveTo 定位写入
   → 存储 raw string 的 prevFrame 缓存，避免重复 ParseLine
   → 帧间无变化时跳过 renderFrame（renderRequested 原子标志）

2. 渲染节流
   → RequestRender 合并突发请求（burst coalescing）
   → ticker 控制最小帧间隔（默认 8ms）

3. Cell 级缓存
   → 同一 raw string 的 ParseLine 结果跨帧复用
   → 仅比较字符串指针/长度，不重解析

4. 光标状态管理
   → 最后一帧的光标状态缓存，避免每帧发出 Hide/Show
   → 仅状态变化时才 emit CSI（保护终端光标闪烁定时器）
```

### 11.3 阻塞防护

| 规则 | 强制方式 |
|------|----------|
| `Update(msg)` < 1ms | Code Review |
| Cmd goroutine panic 不崩溃 | `PanicMsg` 捕获 |
| 不阻塞 eventLoop | 架构约束（Update 中禁 I/O） |

---

## 12. 文档与贡献规范

### 12.1 组件文档要求

每个组件文件必须包含：

1. **Go Doc**：`package` doc + 导出类型 doc。
2. **渲染契约**：组件接收的 `width` 范围、可能返回的最大行数。
3. **事件契约**：处理哪些 `Msg` 类型、产生什么副作用。
4. **示例**：`Example_*` 函数（可选但推荐）。

### 12.2 变更记录

根据 `AGENTS.md` 的"变更即记录"原则，TUI 变更须同步更新：

- `docs/decisions/AI_CHANGELOG.md`：每次功能变更追加一条记录。
- `tui/LAYERS.md`：文件数量/依赖关系变化时更新。

### 12.3 敏感路径提醒

**TUI 相关的安全敏感路径：**

| 路径 | 边界 |
|------|------|
| `tui/terminal/terminal.go` | 终端原始模式设置 |
| `tui/terminal/keys.go` | 自定义快捷键（可被用于覆盖系统快捷键） |
| `tui/component/editor_killring.go` | 剪切板数据缓存 |
| `tui/tui_lifecycle.go` | Start/Stop 生命周期管理 |

### 12.4 引用资源

| 资源 | 链接 | 说明 |
|------|------|------|
| Elm Architecture | https://guide.elm-lang.org/architecture/ | 核心架构原型 |
| Bubble Tea | https://github.com/charmbracelet/bubbletea | Go TEA 实现参考 |
| Ratatui | https://ratatui.rs/ | Rust TUI 差分渲染参考 |
| Textual | https://textual.textualize.io/ | Python TUI 响应式布局 |
| Charm | https://charm.sh/ | Go TUI 生态系统 |
| Kitty Keyboard Protocol | https://sw.kovidgoyal.net/kitty/keyboard-protocol/ | 键盘协议规范 |
| ECMA-48 / ANSI X3.64 | https://www.ecma-international.org/publications-and-standards/standards/ecma-48/ | 终端控制序列标准 |
| Synchronized Output | https://gist.github.com/rockorager/e695fb2924d36b2bcf1fff4dbc370d42 | CSI 2026 规范 |

---

## 附录 A：术语表

| 术语 | 说明 |
|------|------|
| **Elm Architecture (TEA)** | 以 Msg/Cmd/Update/Sub 为核心的单向数据流架构模式 |
| **Cell** | 终端屏幕上的一个字符单元，携带前景色、背景色、属性 |
| **Row** | 一行 Cell 的序列，对应终端中的一行文本 |
| **SGR** | Select Graphic Rendition，ANSI 转义序列中控制颜色和属性的部分 |
| **CSI** | Control Sequence Introducer (`ESC [`)，ANSI 转义序列起始 |
| **Kitty 键盘协议** | 通过 `CSI u` 序列编码的增强键盘事件协议，支持修饰符组合 |
| **Synchronized Output** | CSI 2026 包裹帧渲染，防止输出被终端撕裂 |
| **Alternate Screen** | 终端的备用屏幕缓冲区，TUI 退出时不会污染滚动缓冲区 |
| **Bracketed Paste** | CSI 2004 包裹粘贴内容，防止误解析为按键序列 |
| **Differential Rendering** | 帧间对比仅输出变化像素/单元格的渲染策略 |

## 附录 B：主题 Token 速查表

```
背景层级:   Background(0) < Surface(1) < SurfaceRaised(2)
强调色:     Accent (按钮/链接/激活态)
文字层级:   Text(0) > Muted(1) > Dim(2)
边框层级:   BorderAccent(聚焦) > Border(常规) > BorderMuted(次要)
语义色:     Success(绿) > Warning(黄) > Error(红) > System(橙)
角色色:     UserMessage(Accent) > AssistantText(Text) > ThinkingText(Dim)
工具状态:   ToolPendingBg → ToolSuccessBg → ToolErrorBg
Markdown:   MdHeading > MdLink > MdCode > MdCodeBlock > MdQuote > MdHr
语法:       SyntaxComment > SyntaxKeyword > SyntaxFunction > SyntaxString ...
证据:       EvidenceSupport(绿) > EvidenceCounter(红)
置信度:     ConfidenceLow(黄) > ConfidenceMedium(橙) > ConfidenceHigh(绿)
动画:       LoaderSpinner(Accent) + ProgressBar(Accent)
```

## 附录 C：文件清单索引

> 文件清单以 `tui/LAYERS.md` 为权威来源（`./tui/scripts/verify_layers.sh` 自动校验）。
> 以下为同步快照（2026-07-31）：113 源 + 63 测试，跨 9 个包。

```
tui/
├── LAYERS.md              # 架构层级定义（推荐先读此文件，verify_layers.sh 校验）
├── doc.go                 # 包文档
├── tui.go                 # TUI 容器 + TUIOptions（271 行）
├── tui_loop.go            # 事件循环（lifecycle/render/input 枢纽）
├── tui_lifecycle.go       # Start/Stop/Quit/Done/Context/Tick/Every
├── tui_input.go           # processMsg、Cmd 执行、鼠标模式
├── tui_render.go          # RequestRender、renderFrame、normalizeLine
├── tui_focus.go           # 焦点栈 + Overlay 栈管理
├── overlay.go             # Overlay 类型 + 组合器（573 行）
├── chat_bridge.go         # NewChatApp 便利构造器 + tuiAppHost 适配器
│
├── core/                  # Layer 0 (14源 + 7测试)
│   ├── component.go       # Component/Sizer/Updatable/Focusable + Container
│   ├── message.go         # Msg/Cmd 类型、Batch/Sequence/Quit、MsgBase
│   ├── errors.go          # 三层错误模型 TermError/NetError/LogicError
│   ├── width.go           # East-Asian width/truncation/padding/wrap
│   ├── runeutil.go        # 字符工具
│   ├── fuzzy_match.go     # 模糊匹配
│   ├── spinner_style.go   # SpinnerStyle + 预设
│   ├── cell.go            # Cell/Row 类型、CellGrid
│   ├── celldiff.go        # 帧级差分 DiffRows（严于字符串 diff）
│   ├── cellparse.go       # ParseLine: string → Row
│   ├── cellrender.go      # SerializeRow: Row → ANSI string
│   ├── sgr.go             # ParseSGR/BuildSGR（宽松参数解析）
│   ├── sanitize.go        # SanitizeRawContent：剥离危险转义序列
│   └── stack.go           # CaptureStack：PanicMsg 诊断栈捕获
│
├── layout/                # Layer 0 (3源 + 2测试)
│   ├── breakpoint.go      # LayoutBreakpoint + DetectLayoutBreakpoint
│   ├── flex.go            # Flex 声明式布局（506 行）
│   └── layout.go          # 布局辅助
│
├── terminal/              # Layer 1 (9源 + 8测试)
│   ├── keys.go            # Key 解析、MatchesKey、Kitty 协议、KeyID
│   ├── keybindings.go     # KeybindingsManager、DefaultKeybindings
│   ├── stdin_buffer.go    # StdinBuffer 碎片重组
│   ├── terminal.go        # Terminal 接口 + ProcessTerminal + VirtualTerminal
│   ├── ansi.go            # ANSI 构造器（纯函数）
│   ├── detect.go          # 终端能力检测（色彩/Kitty/品牌）
│   ├── terminal_darwin.go # macOS termios
│   ├── terminal_linux.go  # Linux termios
│   └── terminal_other.go  # 其他系统回退
│
├── theme/                 # Layer 2 (13源 + 5测试)
│   ├── a11y_themes.go     # 无障碍主题（high-contrast / colorblind）
│   ├── style.go           # ANSI Style/Color/Attr/符号/光标辅助
│   ├── color_resolve.go   # 颜色模式检测、RGB→256
│   ├── semantic_theme.go  # SemanticTheme + 默认值（light/dark）
│   ├── palette.go         # Palette + CurrentPalette + BuildPalette
│   ├── global.go          # SetSemanticTheme/InitThemeFromEnv
│   ├── json.go            # JSON 主题解析（变量引用）
│   ├── watch.go           # 文件监控热重载（mtime 轮询）
│   ├── watchutil.go       # runWithRestart：watcher panic 恢复
│   ├── aliases.go         # 颜色别名（name → hex）
│   ├── quantize.go        # 颜色量化（RGB→16 ANSI）
│   ├── system_appearance.go # macOS NSAppearance 检测
│   └── theme_registry.go  # 主题注册表（8 内置主题）
│
├── component/             # Layer 4 (41源 + 24测试)
│   ├── autocomplete.go    # Autocomplete 下拉 + StaticProvider
│   ├── box.go             # Box（边框/内边距容器）
│   ├── text.go            # Text、TruncatedText
│   ├── input.go           # 单行输入编辑器
│   ├── keyhelp.go         # 快捷键速查表
│   ├── loader.go          # 动画 Spinner 组件
│   ├── markdown.go        # Markdown 渲染（块级解析 + 渲染）
│   ├── selectlist.go      # 可过滤选择列表
│   ├── statusbar.go       # StatusBar
│   ├── settings.go        # Settings 面板
│   ├── image.go           # Kitty/iTerm2/HalfBlock/ASCII 图像
│   ├── viewport.go        # 可滚动视口
│   ├── table.go           # 表格渲染
│   ├── fuzzy_provider.go  # FuzzyContentProvider
│   ├── footer.go          # 底部快捷键栏（响应式）
│   │
│   ├── domain.go          # DomainMessage/DomainAction 数据模型
│   ├── evidence_card.go   # 证据卡片
│   ├── conclusion_card.go # 结论卡片（置信度条/证据计数）
│   ├── confidence_bar.go  # 置信度条可视化
│   ├── approval_card.go   # 批准门卡片
│   ├── tool_card.go       # 工具调用结果卡片
│   ├── evidence_overlay.go # EvidenceOverlay 可滚动知识源
│   ├── judgment_view.go   # JudgmentView 判断摘要面板（386 行）
│   ├── review_gate.go     # ReviewGate 复核清单 Overlay（577 行）
│   ├── session_selector.go # SessionSelector 会话选择（545 行）
│   ├── command_center.go  # CommandCenter Ctrl+P 命令面板
│   ├── debug_overlay.go   # DebugOverlay 诊断面板
│   ├── skill_center.go    # SkillCenter 技能中心
│   ├── system_status.go   # SystemStatus 系统模式显示
│   ├── todo_panel.go      # TodoPanel 任务跟踪
│   ├── toast.go           # Toast 瞬态通知
│   ├── onboarding.go      # FirstRunWizard 首次引导
│   │
│   ├── syntax.go          # 语法高亮核心（313 行）
│   ├── syntax_langs.go    # 内置语言规格
│   ├── syntax_tokenizer.go # 语法高亮分词器
│   │
│   ├── editor.go          # Editor 核心结构（392 行）
│   ├── editor_chip.go     # Editor 内联 chips
│   ├── editor_edit.go     # Editor 按键分发与编辑原语（553 行）
│   ├── editor_render.go   # Editor 渲染与鼠标命中（324 行）
│   ├── editor_history.go  # Editor 撤销/重做栈（182 行）
│   └── editor_killring.go # Editor Emacs kill-ring（126 行）
│
├── chat/                  # Layer 5 (22源 + 6测试)
│   ├── chat_app.go        # ChatApp 构造器 + 公开 API（1060 行）
│   ├── chat_app_layout.go # chatLayout 根组件 + 输入路由（582 行）
│   ├── chat_app_plantask.go # PlanTask 状态/批准/中断处理器（40 行）
│   ├── chat_app_stream.go # 流式生命周期（submit/delta/end/error）
│   ├── chat_app_tool.go   # 工具/Handoff/turn/压缩处理器
│   ├── chat_app_todo.go   # TodoPanel 集成
│   ├── chat_history.go    # ChatHistory 滚动转录（566 行）
│   ├── chat_history_render.go        # 渲染管线（视口/分隔线）
│   ├── chat_history_render_message.go # 单消息渲染（角色分发/卡片路由）
│   ├── chat_history_render_highlight.go # 文本选中高亮
│   ├── chat_history_input.go         # 输入与视口滚动、鼠标
│   ├── chat_history_selection.go     # 选区业务逻辑
│   ├── events.go          # ChatEvent 类型（23 种）+ Subscriber 接口
│   ├── state.go           # 显式 FSM（9 状态，249 行）
│   ├── reasoning.go       # 思考块渲染
│   ├── clipboard.go       # 剪切板（pbcopy/xclip/win32）
│   ├── layout_editor.go   # Editor 帧布局辅助
│   ├── layout_shortcuts.go # 复制/剪切板快捷键
│   ├── chat_builder.go    # ChatApp builder 模式
│   ├── chat_display.go    # 显示格式化
│   ├── chat_host.go       # AppHost 接口族（ISP 拆分）
│   └── chat_model.go      # 会话数据模型
│
├── agentadapter/          # Layer 7 (1源 + 2测试)
│   └── adapter.go         # BindAgent + 23 种事件转换（370 行）
│
└── internal/              # 内部辅助（不导出）
    └── csync/             # 并发切片
```

> 注：Layer 6 `tui/stdio/` 已移除（2026-07 架构简化），不再列入。

---

> 本文档受 TUI 架构演进驱动，持续更新。
> 最新同步：2026-07-31 | 配套文件：`tui/LAYERS.md` | 版本：v1.1
