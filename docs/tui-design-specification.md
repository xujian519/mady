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
│  Layer 6:  Stdio           (tui/stdio)                   │
│      过程式 stdout/stdin，Spinner/ProgressBar/LineReader  │
├─────────────────────────────────────────────────────────┤
│  Layer 5:  Application     (tui/chat)                    │
│      ChatApp 应用逻辑、ChatHistory、状态机 (FSM)          │
├─────────────────────────────────────────────────────────┤
│  Layer 4:  Components      (tui/component)               │
│      35 个 UI 组件：Editor/Markdown/SelectList 等         │
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
// 注入输入
vt.Type("hello\x1b[5~") // Ctrl+E
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

### 6.3 默认主题

| 主题 | 特点 | 切换命令 |
|------|------|----------|
| `mady-dark`（默认） | 冷色深色主题，Logo 灵感：#0B1120 底 + #38BDF8 Accent | `/theme dark` |
| `mady-light` | 冰白浅色：#F8FAFC 底 + #0EA5E9 Accent | `/theme light` |
| `amber` | 暖琥珀可选主题 | `/theme amber` |

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

| 终端宽度 | 布局行为 |
|----------|----------|
| ≥ 120 列 | Sidebar + 主工作区 + 右侧辅助（未来预留） |
| 80–119 列 | 完整 Sidebar (20-24列) + 主工作区 |
| 60–79 列 | 折叠 Sidebar 为图标栏 (8列)，hover/快捷键展开 |
| < 60 列 | 隐藏 Sidebar，`Ctrl+Space` 或 `/panel` 切换浮层 |

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
2. **低帧率**：最多 125fps（8ms 间隔），默认 60fps。
3. **可跳过**：快捷键操作到目标状态应直接跳转，不等待动画完成。
4. **不闪烁**：避免背景色交替变化（对光敏用户不友好）。

### 8.2 允许的动效

| 动效 | 实现方式 | 帧率 | 说明 |
|------|----------|------|------|
| 加载 Spinner | `core.SpinnerStyle` | interval 80–120ms | `component.Loader` / `stdio.Spinner` |
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
var SpinnerPulse  = SpinnerStyle{Frames: []string("⣾","⣽","⣻","⢿","⡿","⣟","⣯","⣷"), Interval: 80}
```

- `SpinnerStyle` 在 `core` 包（Layer 0），因为 `component.Loader`（Layer 4）和 `stdio.Spinner`（Layer 6）都需要引用。
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

关键状态变化（错误、模式切换、Agent 切换）通过 `term.Write()` 输出明文到终端，使终端复用器或辅助工具可以捕获。

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

```
tui/
├── LAYERS.md              # 架构层级定义（推荐先读此文件）
├── doc.go                 # 包文档
├── tui.go                 # TUI 容器 + TUIOptions
├── tui_loop.go            # 事件循环
├── tui_lifecycle.go       # Start/Stop/Quit/Every
├── tui_input.go           # 消息处理、Cmd 执行
├── tui_render.go          # 渲染管线（差分/全帧）
├── tui_focus.go           # 焦点栈 + Overlay 栈
├── overlay.go             # Overlay 类型 + 组合器
├── chat_bridge.go         # NewChatApp 便利构造器
│
├── core/                  # Layer 0 (11源 + 5测试)
│   ├── component.go       # Component/Sizer/Updatable/Focusable
│   ├── message.go         # Msg/Cmd/Batch/Sequence
│   ├── cell.go            # Cell/Row/CellGrid
│   ├── celldiff.go        # DiffRows/DiffFrame
│   ├── cellparse.go       # ParseLine: string → Row
│   ├── cellrender.go      # SerializeRow: Row → ANSI string
│   ├── sgr.go             # ParseSGR/BuildSGR
│   ├── width.go           # East-Asian width/truncation/wrap
│   ├── runeutil.go        # 字符工具
│   ├── fuzzy_match.go     # 模糊匹配
│   └── spinner_style.go   # SpinnerStyle + 预设
│
├── layout/                # Layer 0 (2源 + 1测试)
│   ├── layout.go          # 布局辅助
│   └── flex.go            # Flex 声明式布局
│
├── terminal/              # Layer 1 (8源 + 4测试)
│   ├── terminal.go        # Terminal 接口 + ProcessTerminal + VirtualTerminal
│   ├── keys.go            # Key/KeyEventType/KeyID 解析
│   ├── keybindings.go     # KeybindingsManager
│   ├── stdin_buffer.go    # StdinBuffer
│   ├── ansi.go            # ANSI 构造器
│   ├── terminal_darwin.go # macOS termios
│   ├── terminal_linux.go  # Linux termios
│   └── terminal_other.go  # 其他系统回退
│
├── theme/                 # Layer 2 (7源 + 5测试)
│   ├── style.go           # Style/Color/Attr
│   ├── semantic_theme.go  # SemanticTheme + 默认值
│   ├── palette.go         # Palette + 构建
│   ├── color_resolve.go   # 颜色模式检测
│   ├── global.go          # SetSemanticTheme/InitThemeFromEnv
│   ├── json.go            # JSON 主题解析
│   └── watch.go           # 主题文件热重载
│
├── component/             # Layer 4 (35源 + 11测试)
│   ├── core/              # Box/Text/Input/Viewport
│   ├── content/           # Markdown/Syntax/Image/Loader
│   ├── cards/             # ToolCard/ApprovalCard/EvidenceCard
│   ├── panels/            # SessionSelector/SkillCenter/Settings
│   └── widgets/           # Autocomplete/Table/SelectList
│
├── chat/                  # Layer 5 (14源 + 5测试)
│   ├── chat_app.go        # ChatApp 构造器 + API
│   ├── chat_app_layout.go # chatLayout 根组件
│   ├── chat_app_stream.go # 流式生命周期
│   ├── chat_app_tool.go   # 工具/Handoff 处理
│   ├── chat_history*.go   # 聊天历史（6文件）
│   ├── state.go           # 显式状态机 (FSM)
│   ├── events.go          # ChatEvent 类型
│   └── clipboard.go       # 剪切板
│
├── stdio/                 # Layer 6 (5源 + 1测试)
│   ├── renderer.go        # 流式 Markdown 渲染
│   ├── spinner.go         # 过程式 Spinner
│   ├── progress.go        # ProgressBar
│   ├── linereader.go      # 行读取器
│   └── layout.go          # 布局辅助
│
└── agentadapter/          # Layer 7 (1源 + 1测试)
    └── adapter.go         # BindAgent + 事件转换
```

---

> 本文档受 TUI 架构演进驱动，持续更新。
> 最新同步：2026-07-25 | 配套文件：`tui/LAYERS.md` | 版本：v1.0
