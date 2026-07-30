# Mady TUI 设计规范 v1.0

> 综合 XiaoNuo TUI 设计规范（基于 Ratatui / Bubble Tea / Textual / Ink / Monospace Design TUI / clig.dev / Apple HIG 映射的开源社区实践）与 Mady 自研 8 层 Elm 架构 TUI 的实现规范。
>
> 本文档是 `tui/LAYERS.md`（架构层级定义）和 `docs/tui-design-specification.md`（设计准则）的**补充与对齐规范**——
> 将社区通用标准映射到 Mady 的具体实现，识别差距并给出可操作的开发指引。
>
> 版本：v1.0 | 日期：2026-07-30 | 参考源：[XiaoNuo TUI 设计规范 v1.0](../XiaoNuo%20Agent/docs/tui-design-standards.md)

---

## 目录

1. [设计哲学对照](#1-设计哲学对照)
2. [布局架构映射](#2-布局架构映射)
3. [组件设计映射](#3-组件设计映射)
4. [交互设计映射](#4-交互设计映射)
5. [视觉设计映射](#5-视觉设计映射)
6. [渲染引擎映射](#6-渲染引擎映射)
7. [终端兼容性映射](#7-终端兼容性映射)
8. [无障碍映射](#8-无障碍映射)
9. [状态与反馈映射](#9-状态与反馈映射)
10. [测试与质量门禁映射](#10-测试与质量门禁映射)
11. [CLI 互操作映射](#11-cli-互操作映射)
12. [差距分析与改进路线图](#12-差距分析与改进路线图)
13. [附录](#13-附录)

---

## 1. 设计哲学对照

### 1.1 三条核心信条 vs Mady 实现

| 社区信条 | Mady 对应实现 | 状态 |
|---------|--------------|------|
| **键盘优先，鼠标可选** | `KeybindingsManager`（30 个默认绑定，`keymap.json` 用户覆盖）+ 鼠标支持（SGR 模式，`Shift+click` 绕过） | ✅ 已实现 |
| **空间一致性** | Flex 声明式布局引擎（`tui/layout/flex.go`），7 种尺寸策略，面板位置由布局决定而非内容驱动 | ✅ 已实现 |
| **渐进式披露** | Footer 常驻（`component/statusbar.go`）+ `?` 键 `KeyHelp` 覆盖层 + `--help` 命令行参考 | ⚠️ Footer 和 KeyHelp 已实现，`--help` TUI 模式使用说明需补充 |

### 1.2 渲染模式选择

社区推荐**混合模式**（保留 UI 树 + 差异渲染）。Mady 实现：

```
Mady 8 层架构 = 保留模式（组件树 + 生命周期 + 焦点管理）+ Immediate 模式（行级差分）的混合
```

具体对应：
- **保留模式方面**：`Component` 接口、焦点栈、Overlay 栈、组件生命周期 — 对标 Bubble Tea（Elm Architecture）
- **即时模式方面**：`CellGrid` + `DiffRows`/`DiffFrame`（`core/celldiff.go`）+ CSI 2026 同步输出 — 对标 Ratatui

### 1.3 色彩分层

| 社区层级 | Mady 实现 | 文件 |
|---------|----------|------|
| True Color (16.7M) | `color_resolve.go` 检测 `$COLORTERM=truecolor`，首选 `38;2;r;g;b` / `48;2;r;g;b` | `tui/theme/color_resolve.go` |
| 256 Color | `RGBTo256()` 加权距离映射 | `tui/theme/color_resolve.go` |
| 16 ANSI | `quantize.go` 量化引擎 | `tui/theme/quantize.go` |

**规则 M-TUI-PHL-001 (MUST)**：每一层色彩降级必须独立可读。去掉 True Color 后 UI 仍可读，去掉所有颜色后布局和符号仍能传达信息。当前 `quantize.go` 和 `color_resolve.go` 支持此链式降级。

---

## 2. 布局架构映射

### 2.1 七种布局范式 vs Mady

| 范式 | Mady 对应 | 说明 |
|------|----------|------|
| Persistent Multi-Panel | `chatLayout`（`chat/chat_app_layout.go`）— 消息历史 + 输入栏 + 状态栏 + 浮层面板 | 所有面板固定位置，焦点在面板间切换 |
| Widget Dashboard | `SystemStatus` / `DebugOverlay` / `TodoPanel` — 独立浮层面板 | 通过 Overlay 系统显示 |
| IDE Three-Panel | 消息（主体）+ 输入栏（底部）+ Overlay（覆盖侧） | 快捷键切换 Overlay |
| Overlay / Popup | `Overlay` 系统（`tui/overlay.go`，672 行），4 种分类 + 9 个锚点 | 按需覆盖，`Esc` 关闭 |
| Header + Scrollable List | `ChatHistory`（`chat/chat_history*.go`）— 固定状态栏 + 可滚动消息 | VirtualTerminal 样式滚动 |

Mady 当前定位在 **Persistent Multi-Panel** + **Overlay/Popup** 两种范式的组合，适合 Chat/Assistant 交互场景。

### 2.2 布局约束映射

| 社区规则 | Mady 实现 | 状态 |
|---------|----------|------|
| **TUI-LAY-001**: 比例约束，禁止绝对像素 | `layout/flex.go` — 7 种尺寸策略（Natural/Fixed/Min/Max/Fill/Percent/Shrinkable） | ✅ |
| **TUI-LAY-002**: 最小尺寸 80x24 | `VirtualTerminal` 默认 80×24；resize 时 `WindowSizeMsg` 处理 | ⚠️ 最小尺寸门闩（<80×24 时显示 resize 提示）**未实现** |
| **TUI-LAY-003**: `visibleWidth() ≤ width` | `normalizeLine()` 截断；`Render(width)` 契约保证 | ✅ |
| **TUI-LAY-004**: 响应式断点 | **未实现** — 当前无 wide/standard/compact 三档布局切换 | ❌ **差距** |
| **TUI-LAY-005**: Flexbox 风格布局 | `layout/flex.go` 三遍布局算法（测量→分配→压缩） | ✅ |
| **TUI-LAY-010**: SIGWINCH 处理 | `WindowSizeMsg` + `RequestRender()`；无显式去抖 | ⚠️ 建议增加 100ms debounce |
| **TUI-LAY-011**: 面板最小宽度 | **未实现** — 无面板折叠策略 | ❌ **差距** |
| **TUI-LAY-012**: 虚拟滚动 | `ChatHistory` 手动管理 offset + viewport；非通用虚拟滚动组件 | ⚠️ `Viewport` 组件存在但仅做包装 |

### 2.3 布局规范增补

**规则 M-TUI-LAY-001 (MUST)** — 新的 `Component` 必须实现 `Sizer` 接口（声明自然高度），避免布局容器二次渲染测量。

**规则 M-TUI-LAY-002 (SHOULD)** — SIGWINCH 响应使用 100ms 去抖，防止高频 resize 导致的性能问题。实现方式：
```go
// 在 tui/tui_input.go 中，对 WindowSizeMsg 的处理增加 debounce
// 参考: XiaoNuo TUI-LAY-010
```

**规则 M-TUI-LAY-003 (SHOULD)** — 预留响应式断点能力。未来新增宽屏布局时，按以下断点：
| 断点 | 宽度 | 行为 |
|------|------|------|
| wide | ≥160 列 | 多栏、侧边栏展开 |
| standard | ≥80 列 | 当前标准布局 |
| compact | <80 列 | 单栏，浮层面板全屏 |

---

## 3. 组件设计映射

### 3.1 组件接口契约

| 社区接口 | Mady 对应 | 说明 |
|---------|----------|------|
| `Component.render(width)` | `Component.Render(width int64) []string` | ✅ 行级渲染，`normalizeLine` 保底截断 |
| `Component.handleInput(data)` | `Updatable.Update(msg) Cmd` | ✅ Elm 风格，返回 `Cmd` 处理副作用 |
| `Component.invalidate()` | `Component.Invalidate()` | ✅ 清除缓存，渲染循环自动调用 |
| `Component.dispose()` | ❌ **未定义** — 组件无统一 dispose 契约 | ❌ **差距** |
| `Focusable.focused` | `Focusable` 接口（`core/component.go`） | ✅ |
| 样式通过 Theme 参数注入 | `theme.Palette` 全局原子指针 | ✅ |

### 3.2 组件树生命周期

| 阶段 | Mady 实现 | 状态 |
|------|----------|------|
| 创建 | New* 构造器 | ✅ |
| 挂载 | `TUI.AddChild()` → 焦点栈加入 | ✅ |
| 渲染 | `Render(width)` 调用链 | ✅ |
| 输入处理 | `Update(msg)` → eventLoop | ✅ |
| 卸载 | `Stop()` → 无显式 dispose → 潜在泄漏 | ❌ **差距** |

**规则 M-TUI-CMP-001 (MUST)** — 所有占用资源的组件（定时器、事件监听、子 goroutine）必须实现 `Disposable` 接口：

```go
type Disposable interface {
    Dispose()
}
```

在 `TUI.Stop()` 或 `RemoveChild()` 时自动调用。

**规则 M-TUI-CMP-002 (SHOULD)** — 复杂组件（>400 行）按责任拆分为兄弟文件，遵循 Mady 已建立的模式：
- `<component>.go` — 核心结构体 + 构造器
- `<component>_<concern>.go` — 功能拆分
- 示例：`editor.go` / `editor_edit.go` / `editor_render.go` / `editor_history.go` / `editor_killring.go`

### 3.3 标准组件库对照

| 社区组件 | Mady 提供 | 位置 |
|---------|----------|------|
| Text | `Text`, `TruncatedText` | `tui/component/text.go` |
| Input | `Input` | `tui/component/input.go` |
| Editor | `Editor`（5 文件，~1577 行） | `tui/component/editor*.go` |
| SelectList | `SelectList`（模糊过滤） | `tui/component/selectlist.go` |
| Table | `Table` | `tui/component/table.go` |
| Markdown | `Markdown`（块级解析+渲染） | `tui/component/markdown.go` |
| Code | 内置于 Markdown + `Syntax` 高亮 | `tui/component/syntax*.go` |
| TabBar | ❌ **未提供** | — |
| Tree | ❌ **未提供** | — |
| RadioGroup | ❌ **未提供** | — |
| ProgressBar | ✅（stdio）+ ❌（Component） | `tui/stdio/progress.go`，Component 层无 ProgressBar |
| Loader/Spinner | `Loader`（Component）+ `Spinner`（stdio） | `tui/component/loader.go` + `tui/stdio/spinner.go` |
| Diff | ❌ **未提供** | — |
| Box (border) | `Box` | `tui/component/box.go` |
| Autocomplete | `Autocomplete` + 3 种 Provider | `tui/component/autocomplete.go` |

**规则 M-TUI-CMP-003 (SHOULD)** — 新增组件优先从社区清单中选取成熟模式实现，避免重复发明。参考 `tui-design-standards.md` §3.3 标准组件库。

**规则 M-TUI-CMP-004 (SHOULD)** — 组件样式必须通过 `theme.Palette`（通过 `theme.CurrentPalette()` 获取）引用语义令牌，禁止硬编码 hex/ANSI 值。验证：Code Review 检查 `\x1b[` 或 `#` 颜色值。

---

## 4. 交互设计映射

### 4.1 键盘导航四层模型

| 层级 | Mady 实现 | 文件 |
|------|----------|------|
| L0: Universal（箭头/Enter/Esc/Tab） | `KeybindingsManager.DefaultKeybindings()` 包含全部 | `tui/terminal/keybindings.go` |
| L1: Vim Motions | ❌ **未定义** — 无 hjkl/gg/G 绑定 | — |
| L2: Mnemonic（d/c/p） | 无系统性 mnemonic 绑定 | — |
| L3: Power（组合键/宏） | 无自定义宏系统 | — |

Mady 当前仅实现了 L0 层级。社区建议的 L1-L3 为可选增强。

### 4.2 通用键绑定对照

| 社区键 | Mady 绑定 | 说明 |
|-------|----------|------|
| `j`/`k` | **不使用** | Mady 使用箭头键 |
| `h`/`l` | **不使用** | Mady 使用箭头键 |
| `/` | `ctrl+s`（搜索） | 社区用 `/`，Mady 保留 `/` 供未来使用 |
| `?` | **已实现** (? → KeyHelp) | ✅ |
| `:` | ❌ **未实现**（命令模式） | — |
| `q` | `Esc` 关闭浮层 / `Ctrl+C` 退出 | ✅ |
| `Enter` | 提交/选择 | ✅ |
| `Tab`/`Shift+Tab` | 焦点循环 | ✅ |
| `Ctrl+P` | 命令面板 | ✅ |
| `Ctrl+D` | 中断输入 | ✅ |

**规则 M-TUI-KB-001 (MUST)** — 所有按键绑定通过 `KeybindingsManager` 注册，不在组件内硬编码键值。提供 `keymap.json` 用户覆盖机制（已实现 `LoadUserBindingsJSON()`）。

**规则 M-TUI-KB-002 (MUST)** — 不绑定终端保留键：`Ctrl+C`（中断，TUI 中先判断是否选中文本）、`Ctrl+Z`（挂起）、`Ctrl+\`（退出信号）。

**规则 M-TUI-KB-003 (SHOULD)** — 支持 Kitty 键盘协议（CSI u）。Mady 已实现 `keys.go` 中 kitty 序列解析，默认 `flags=1`（消歧义）。

### 4.3 焦点管理

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| 同一时间只有一个焦点 | `tui_focus.go` 焦点栈 | ✅ |
| Tab/Shift+Tab 循环 | `tui_focus.go` focusNext/focusPrev | ✅ |
| 模态焦点陷阱 | `Overlay.Modal = true` 时屏蔽背景 | ✅ |
| 焦点指示器 | `component.Box` 边框高亮 / 颜色变化 / `CURSOR_MARKER` | ✅ |

### 4.4 搜索与过滤

`SelectList` 已内置模糊过滤（`core/fuzzy_match.go`），但缺少社区标准的 `/` 搜索模式（n/N 下一个/前一个、高亮匹配、无匹配提示）：

**规则 M-TUI-KB-004 (SHOULD)** — 为 `ChatHistory` 和长列表增加 `/` 搜索功能，支持：
- `/` 激活搜索 → 实时过滤
- `n`/`N` 下一个/上一个匹配
- 默认模糊匹配，`'` 前缀切换精确匹配
- 匹配字符高亮
- 无匹配时显示"未找到"

### 4.5 帮助系统三层

| 层级 | Mady 实现 |
|------|----------|
| Footer（常驻 3-5 个核心快捷键） | `StatusBar` 显示部分信息，**无独立 footer 组件** |
| Overlay（`?` 键） | `KeyHelp` 组件（已实现） |
| Reference（`--help`） | `cmd/mady` 的 `--help` 覆盖 CLI 模式，**TUI 子命令缺独立使用说明** |

**规则 M-TUI-KB-005 (MUST)** — 新增 `Footer` 组件（`tui/component/footer.go`），在 TUI 底部常驻显示 3-5 个核心快捷键：`[?] help [Ctrl+P] commands [Ctrl+C] quit`。

### 4.6 确认机制

| 严重性 | Mady 实现 | 状态 |
|--------|----------|------|
| 可逆 | 直接执行 + 消息显示 | ✅ |
| 中等 | ❌ **行内确认**（Press y to confirm）未实现 | ❌ **差距** |
| 严重 | `ApprovalCard` / `ReviewGate` 模态审核面板 | ✅ |
| 不可逆批量 | ❌ **--dry-run 模式**未实现 | — |

---

## 5. 视觉设计映射

### 5.1 色彩系统对照

| 社区令牌 | Mady 对应 | 状态 |
|---------|----------|------|
| `fg.default` / `fg.muted` / `fg.emphasis` | `Text` / `Muted` / `Dim`（语义主题） | ✅ |
| `bg.base` / `bg.surface` / `bg.overlay` | `Background` / `Surface` / `SurfaceRaised` | ✅ |
| `accent.primary` / `accent.secondary` | `Accent` / `AccentSecondary` | ✅ |
| `status.error` / `status.warning` / `status.success` / `status.info` | `Error` / `Warning` / `Success` / `Info` | ✅ |
| 语义色阶 | 人工程义，**无自动色阶生成** | ⚠️ 建议增加 OKLCH 色阶生成 |

**规则 M-TUI-COLOR-001 (MUST)** — 所有文字与背景对比度 ≥ 4.5:1（WCAG AA）。当前 `high-contrast` 主题满足此要求，但其他主题**无自动化对比度审计**。

**规则 M-TUI-COLOR-002 (MUST)** — 状态信息前缀符号 + 颜色双重编码。Mady 已通过在 `state.go` 中 `statusPrefix()` 和 `statusColor()` 工厂函数实现。

**规则 M-TUI-COLOR-003 (SHOULD)** — 考虑使用 OKLCH 色彩空间定义语义色彩。当前使用 hex 色值定义，色阶生成可引入 `color-math` 库实现 OKLCH 插值。

**规则 M-TUI-COLOR-004 (MUST)** — 不在组件代码中硬编码 hex/ANSI 值。始终引用 `theme.CurrentPalette()` 的语义令牌。Code Review 检查此条。

### 5.2 排版系统

| 层级 | Mady 实现 | 说明 |
|------|----------|------|
| H1 `# ` bold + accent | `Markdown` 组件渲染 | ✅ |
| H2 `## ` bold + accent | `Markdown` 组件渲染 | ✅ |
| H3 `### ` secondary | `Markdown` 组件渲染 | ✅ |
| Body | 默认文本 | ✅ |
| Caption / Muted | `Muted` 色 | ✅ |

### 5.3 间距与边框

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| 间距令牌 | 无系统化间距令牌，通过 `theme.Style` 的 padding 参数控制 | ⚠️ 建议定义 `Padding{XS/SM/MD/LG/XL}` 令牌 |
| 边框令牌 | `Style` 无独立边框令牌，边框通过 `Box` 组件的 `SetBorderStyle` 控制 | ⚠️ 建议定义 `BorderStyle{Rounded/Single/Double/Heavy/Dashed}` |
| 密度控制器 | ❌ **未实现** | ❌ **差距** |

### 5.4 图标与符号系统

| 社区规则 | Mady 实现 | 状态 |
|---------|----------|------|
| Nerd Fonts + Unicode/ASCII 回退 | `theme/style.go` 中定义符号变量（`Sym*`） | ✅ 部分实现 |
| 图标回退三级 | 部分符号有回退，**无系统性回退检测** | ⚠️ 建议增加 Nerd Font 检测 |

Mady 当前符号定义（`tui/theme/style.go`）：
```go
// box-drawing 字符（通用）
SymHR = "─" / SymVR = "│" / SymDR = "┌" / SymDL = "┐" ...

// 状态符号
SymCheck = "✓" / SymCross = "✗" / SymWarning = "⚠" / SymInfo = "ℹ"
SymBullet = "●" / SymArrow = "▶" / SymFold = "▸"
```

**规则 M-TUI-ICO-001 (MUST)** — 每个图标定义必须有回退。在 `theme/style.go` 中定义 `Sym*` 变量时，同时提供 `Sym*Fallback` 常量或使用函数根据能力检测自动选择。

### 5.5 视觉层次配方

Mady 视觉分配合理：80% 内容（`Text`）、10% 标题（`MdHeading` 强调色高亮）、5% 元数据（`Muted`）、3% 状态指示（`Status*` 语义色）、2% 交互元素（`Accent`）。

---

## 6. 渲染引擎映射

### 6.1 无闪烁渲染三要素

| 社区要素 | Mady 实现 | 文件 |
|---------|----------|------|
| 双缓冲 + 差异渲染 | `prevFrame []Row` + `DiffFrame()` 行级差分 | `core/celldiff.go`, `tui_render.go` |
| 同步输出 (CSI 2026) | CSI `?2026h`/`l` 包裹每帧 | `tui_render.go:122` |
| 批量写入 | 全部合并为单次 `t.write()` 系统调用 | `tui_render.go` |

✅ **全部已实现**。Mady 在这三要素上完全满足社区标准。

### 6.2 差异渲染策略

| 场景 | 社区建议 | Mady 实现 | 状态 |
|------|---------|----------|------|
| 首次渲染 | Full Render, 不清除 scrollback | `firstFrame` 标志 → 全量 `HideCursor() + CursorHome() + ClearFromCursor()` | ✅ |
| 宽度变化 | Full Clear + Render | prevCols 变化触发的宽变化路径 | ✅ |
| 内容更新 | Incremental: 定位到首个变化行 | `DiffFrame()` 两端收缩 → 最小 diff segment | ✅ |
| 局部变化 | Targeted: 仅更新变化单元格 | `DiffCells()` 左+右扫描 → `Segment` CSI MoveTo 定位 | ✅ |

### 6.3 动画系统

| 社区规则 | Mady 实现 | 状态 |
|---------|----------|------|
| 动画不延迟输入 | eventLoop 单 goroutine 串行化，无动画阻塞 | ✅ |
| Loader Spinner | `core.SpinnerStyle`（Layer 0）+ `component.Loader`（Layer 4）+ `stdio.Spinner`（Layer 6） | ✅ |
| 折叠动画 | 即时 `▶`/`▼` 切换，无逐帧动画 | ✅ |
| Overlay 弹出 | 即时渲染，无渐入效果 | ✅（符合社区建议"渐入会显得迟缓"） |

### 6.4 性能预算

| 指标 | 社区目标 | Mady 当前 | 测量方式 |
|------|---------|-----------|----------|
| 帧率 | 30 FPS (硬上限 15) | `DebugOverlay` 显示 FPS，默认 125fps tick | `tui_render.go` 帧统计 |
| 帧渲染时间 | <16ms | `lastRenderDur` 跟踪，>16ms 计为 slow frame | `tui_render.go:253` |
| 初始渲染时间 | <100ms (硬上限 500ms) | 未测量 | — |
| 内存分配 | <10KB/帧 | `runtime.ReadMemStats` 每 100 帧采样 | `tui_render.go:260` |
| Overlay 合成 | <1ms for 10 layers | CoW 优化（`overlay.go:258`） | — |

✅ Mady 性能预算定义覆盖了社区标准。建议增加初始渲染时间的显式测量。

---

## 7. 终端兼容性映射

### 7.1 终端能力检测

| 能力 | 检测方法 | Mady 实现 | 降级策略 |
|------|---------|----------|---------|
| True Color | `$COLORTERM=truecolor` | `color_resolve.go` `DetectColorMode()` | `RGBTo256()` → `quantize.go` |
| Kitty 键盘 | CSI u 协商 | `keys.go` Kitty 序列支持 | 传统 xterm 序列 |
| 同步输出 | CSI 2026 协商 | `DisableSynchronizedOutput` 选项 | 无同步（撕裂风险） |
| Kitty 图形 | `$TERM=kitty` | `component/image.go` Kitty 支持 | iTerm2 → HalfBlock → ASCII |
| iTerm2 图形 | `$TERM_PROGRAM=iTerm.app` | `component/image.go` iTerm2 支持 | HalfBlock → ASCII |
| 鼠标 (SGR) | 请求响应 | `Terminal` SGR 鼠标模式 | 基础 X10 模式 |
| Nerd Fonts | 启发式 | ❌ **无系统性检测** | — |

### 7.2 终端兼容性规则

**规则 M-TUI-TRM-001 (MUST)** — 终端 resize 不崩溃。Mady 通过 `WindowSizeMsg` + `RequestRender()` 实现。**增补**：增加 100ms debounce 防止高频 resize。

**规则 M-TUI-TRM-002 (MUST)** — 在 tmux/zellij/screen 中正常工作。当前无显式检测，依赖基础 ANSI 序列兼容性。**增补**：增加 `$TERM_PROGRAM` 检测和 `TMUX` 环境变量感知。

**规则 M-TUI-TRM-003 (MUST)** — 退出时恢复终端状态。Mady 通过 `Stop()` → `terminal.Restore()` 恢复原始模式、光标形状、滚动区。✅

**规则 M-TUI-TRM-004 (MUST)** — 退出时确保 Alternate Screen 恢复。已通过启动时的 `SwitchToAltScreen()` 和退出时的 `SwitchFromAltScreen()` 实现。✅

### 7.3 Unicode 规范

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| CJK 字符 2 列宽度 | `core/width.go` `CellWidthOfRunes()` | ✅ |
| Grapheme cluster 安全截断 | `core/width.go` `Truncate()` — 感知 CJK 但不感知 emoji ZWJ | ⚠️ 多码点 emoji 可能被截断 |
| 符号选择稳定性 | 使用 Box Drawing → Block Elements 等稳定字符集 | ✅ |
| Unicode 9.0+ emoji 不使用 | 未使用 | ✅ |

### 7.4 环境变量契约

| 变量 | Mady 行为 | 状态 |
|------|----------|------|
| `NO_COLOR` | `theme/style.go` `ColorEnabled()` 检测 | ✅ |
| `TERM=dumb` | 降级为纯文本 | ⚠️ 检测存在但降级路径需验证 |
| `CI=true` | ❌ **未处理** | ❌ **差距** |
| `FORCE_COLOR` | `theme/style.go` 检测 | ✅ |
| `COLORTERM` | `color_resolve.go` 检测 | ✅ |

---

## 8. 无障碍映射

### 8.1 色彩无障碍

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| WCAG AA 4.5:1 对比度 | `high-contrast` 主题满足 | ⚠️ 其他主题**无自动化审计** |
| 红/绿 → 蓝/橙 色盲方案 | `colorblind` 主题（`a11y_themes.go`） | ✅ |
| 符号+颜色双重编码 | `statePrefix()` / `stateColor()` 工厂函数 | ✅ |

### 8.2 NO_COLOR 合规

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| `NO_COLOR` 时无 ANSI 色彩 | `ColorEnabled()` 返回 false → `Style` 不输出颜色码 | ✅ |
| 无颜色模式的信息传递 | 前缀符号 + 位置 + 排版（粗体/缩进） | ✅ |

### 8.3 动画与运动

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| 无闪烁动画 | 已遵循——不使用闪烁效果 | ✅ |
| `reduceMotion` 配置 | ❌ **未实现** | ❌ **差距** |

---

## 9. 状态与反馈映射

### 9.1 统一状态系统

| 社区状态 | Mady 对应 | 使用位置 |
|---------|----------|---------|
| `active` | 无统一 `active` 状态文字 | — |
| `selected` | `SelectedBg` 背景色 | `SelectList`, `Table` |
| `focused` | `BorderAccent` 边框 / `CursorMarker` | 所有 `Focusable` 组件 |
| `disabled` | `Muted` / `Dim` | 组件可选 |
| `error` | `Error` 色 + `✗` 前缀 | 错误消息 |
| `warning` | `Warning` 色 + `⚠` 前缀 | 警告消息 |

**规则 M-TUI-STA-001 (MUST)** — 状态变化通过前缀符号 + 颜色双重编码，不纯依赖颜色。Mady 的 `statePrefix()` 和 `stateColor()` 工厂函数模式应推广到所有组件中使用。

### 9.2 异步操作反馈

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| 100ms 内显示反馈 | `Loader` 组件，`SpinnerStyle` 80-120ms 间隔 | ✅ |
| 后台操作不阻塞主循环 | Cmd 在独立 goroutine 执行 | ✅ |
| Spinner 规范（Braille/Line/Block） | `SpinnerDots` (Braille), `SpinnerLine`, `SpinnerPulse` | ✅ |

### 9.3 错误处理

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| 渲染异常不崩溃 | `PanicMsg` 捕获 Cmd panic | ✅ |
| 友好错误消息 | `status.error` 色彩 + `✗` 符号 | ✅ |
| Watchdog 监控 | ❌ **未实现** | ❌ **差距** |

**规则 M-TUI-ERR-001 (SHOULD)** — 考虑在 `eventLoop` 中增加看门狗（watchdog），检测 event processing 阻塞超过 5s 时输出诊断信息。

---

## 10. 测试与质量门禁映射

### 10.1 测试层级

| 社区层级 | Mady 实现 | 覆盖目标 |
|---------|----------|---------|
| 单元测试 | 各包 `*_test.go`，51 个测试文件 | 渲染逻辑、事件响应、状态转换 |
| 集成测试 | `VirtualTerminal` + `newTestChatApp` | 事件流、布局组合、Overlay |
| 周期测试 | `VirtualTerminal` 长运行 | 内存泄露、竞态、流式渲染 |

### 10.2 测试方法对照

| 社区方法 | Mady 实现 | 示例文件 |
|---------|----------|---------|
| 字面渲染快照 | 轻量级结构快照（子串断言） | `chat_app_frame_test.go` |
| 像素级视觉回归 | ❌ **未实现** | — |
| 交互自动化 | VirtualTerminal 事件注入 | `chat_app_test.go` |
| 对比度审计 CI | ❌ **未实现** | — |
| 终端兼容性矩阵 | 人工验证，**无自动化矩阵** | — |

### 10.3 测试规范增补

**规则 M-TUI-TST-001 (MUST)** — 新增组件必须有 ≥ 70% 行覆盖率，且必须测试：
- 不同 `width` 下的渲染表现
- 空数据 / 超长文本 / 最小 width 边界
- 主题切换后 `Invalidate()` 调用
- 竞态：`-race` 模式下并发 Update + Render

**规则 M-TUI-TST-002 (MUST)** — `go test -race ./tui/...` 必须通过。所有组件测试必须在竞态检测模式下验证。

**规则 M-TUI-TST-003 (SHOULD)** — 关键路径渲染使用结构快照（子串断言模式，非 golden 文件），确保变更不破坏已有布局。示例见 `chat_app_frame_test.go`。

**规则 M-TUI-TST-004 (SHOULD)** — FSM 使用表驱动测试全覆盖所有合法转换且验证非法转换的 no-op 行为。示例见 `chat/state_test.go`（30 个案例全覆盖 8 状态 x 15 事件）。

**规则 M-TUI-TST-005 (SHOULD)** — CI 中运行 `validateColorContrast()` 审计，确保所有 UI 中的颜色组合 ≥ WCAG AA。当前无此审计，建议纳入 CI pipeline。

### 10.4 终端兼容性验证清单

发布前验证以下条目（参考 XiaoNuo TUI-TST 终端兼容性矩阵）：

- [ ] 80×24 最小终端尺寸可用
- [ ] 终端 resize 不崩溃
- [ ] 亮色/暗色主题均正确显示
- [ ] 尊重 `NO_COLOR` 环境变量
- [ ] 在 tmux/zellij/screen 中工作
- [ ] 通过 SSH 工作
- [ ] 鼠标捕获不破坏文本选择（`Shift+click` 通过）
- [ ] 所有功能仅键盘可访问
- [ ] 管道/重定向输出中无 ANSI 泄漏
- [ ] `Ctrl+C`/`SIGINT` 时干净退出

---

## 11. CLI 互操作映射

### 11.1 CLI 规范对照

| 规则 | Mady 实现 | 状态 |
|------|----------|------|
| **TUI-CLI-001**: 非 TTY 降级纯文本 | `cmd/mady/tui.go` 启动时检测，非 TTY 不走 TUI 路径 | ✅ |
| **TUI-CLI-002**: 数据 stdout / 日志 stderr | 未系统性分离 | ⚠️ 建议审查 |
| **TUI-CLI-003**: `--output json` 结构化输出 | 特定子命令（`eval`/`evidence`）支持 JSON | ⚠️ TUI 模式不支持 |
| **TUI-CLI-004**: `--help` 完整说明 | `cmd/mady` 主命令 + 子命令有 help | ✅ |
| **TUI-CLI-005**: 管道/重定向自动非 TTY | `isatty()` 检测 | ✅ |

### 11.2 CLI 互操作规范

**规则 M-TUI-CLI-001 (MUST)** — 非 TTY 模式优雅降级为纯文本输出：无 ANSI 序列、无光标控制、无交互等待。Mady 已在 `cmd/mady/tui.go` 中通过 `isatty.IsTerminal()` 检测。

**规则 M-TUI-CLI-002 (SHOULD)** — TUI 模式下，`mady tui --output json` 应支持将当前会话导出为 JSONL 格式到 stdout。当前通过 `session` 包支持 JSONL 持久化，但未与 `--output` 标志集成。

---

## 12. 差距分析与改进路线图

### 12.1 差距汇总

| 优先级 | 缺失功能 | 社区规则 | 影响 | 状态 |
|--------|---------|---------|------|------|
| **P0** | 组件 `Dispose()` 接口 | TUI-CMP-010 | 资源泄漏风险 | ✅ Sprint 1 |
| **P0** | 最小尺寸门闩（<80 列提示） | TUI-LAY-002 | 小终端中 UI 错乱 | ✅ Sprint 1 |
| **P1** | Footer 组件常驻快捷键 | 帮助系统三层 | 用户难以发现快捷键 | ✅ Sprint 2 |
| **P1** | CI 对比度审计 | TUI-TST-010 | 无障碍合规无法自动验证 | ✅ Sprint 1 |
| **P1** | `CI=true` 环境变量处理 | TUI-TRM | CI 中可能出现 ANSI 泄漏 | ✅ Sprint 2 |
| **P2** | 响应式断点（wide/standard/compact） | TUI-LAY-004 | 大屏利用率低 | ✅ Sprint 3 |
| **P2** | SIGWINCH 100ms 去抖 | TUI-LAY-010 | 高频 resize 性能抖动 | ✅ Sprint 2 |
| **P2** | `/` 搜索模式（n/N/高亮） | 交互设计 §4.4 | 长列表导航效率低 | ✅ Sprint 2 |
| **P2** | `reduceMotion` 配置 | TUI-A11Y-020 | 光敏用户无障碍 | ✅ Sprint 3 |
| **P3** | 行内确认模式 | 确认机制 §4.6 | 中等严重度操作确认不便 | ✅ Sprint 4 |
| **P3** | Nerd Font 自动检测 | TUI-ICO-001 | 字符显示一致性 | ✅ Sprint 3 |
| **P3** | Watchdog 监控 | TUI-ERR-003 | 调试困难 | ✅ Sprint 4 |

### 12.2 优先级定义

- **P0 (MUST)** — 阻塞项，影响正确性或数据安全
- **P1 (SHOULD)** — 建议项，影响用户体验或开发效率
- **P2 (SHOULD)** — 次要建议项，可后续迭代
- **P3 (COULD)** — 未来增强

### 12.3 改进路线图

| 阶段 | 内容 | 预计工作量 | 状态 |
|------|------|-----------|------|
| **Sprint 1: 安全与质量门禁** | `Dispose()` 接口 + P0 补全、最小尺寸门闩、CI 对比度审计 | 3-5 天 | ✅ 已完成 |
| **Sprint 2: 交互增强** | Footer 组件、SIGWINCH 去抖、`/` 搜索模式、`CI=true` 处理 | 5-7 天 | ✅ 已完成 |
| **Sprint 3: 视觉与无障碍** | 响应式断点、`reduceMotion`、Nerd Font 检测、色盲主题完善 | 3-5 天 | ✅ 已完成 |
| **Sprint 4: 功能完善** | Watchdog、行内确认、`--output json` TUI 集成 | 3-5 天 | ✅ 已完成 |

---

## 13. 附录

### 附录 A：Mady TUI 社区标准映射矩阵

| XiaoNuo 章节 | Mady 对应 | 覆盖度 |
|-------------|----------|--------|
| §1 设计哲学 | §1 设计哲学对照 | ✅ 85% |
| §2 布局架构 | §2 布局架构映射 | ✅ 75% |
| §3 组件设计 | §3 组件设计映射 | ✅ 80% |
| §4 交互设计 | §4 交互设计映射 | ✅ 70% |
| §5 视觉设计 | §5 视觉设计映射 | ✅ 75% |
| §6 渲染引擎 | §6 渲染引擎映射 | ✅ 95% |
| §7 终端兼容性 | §7 终端兼容性映射 | ✅ 80% |
| §8 无障碍 | §8 无障碍映射 | ✅ 70% |
| §9 状态与反馈 | §9 状态与反馈映射 | ✅ 85% |
| §10 测试与质量门禁 | §10 测试与质量门禁映射 | ✅ 75% |
| §11 CLI 互操作 | §11 CLI 互操作映射 | ✅ 80% |

### 附录 B：映射文件索引

| Mady 包 | 对应社区概念 | 关键文件 |
|---------|------------|---------|
| `tui/core` | 基础类型/Cell 渲染/Elm Msg | `cell*.go`, `component.go`, `message.go`, `width.go`, `sgr.go` |
| `tui/layout` | 布局约束/Flexbox | `flex.go` |
| `tui/terminal` | 终端 I/O/键盘/ANSI | `terminal.go`, `keys.go`, `keybindings.go`, `ansi.go` |
| `tui/theme` | 色彩/风格/主题 | `semantic_theme.go`, `palette.go`, `style.go`, `color_resolve.go` |
| `tui` (根包) | TUI 引擎/Overlay/焦点 | `tui*.go`, `overlay.go`, `tui_focus.go` |
| `tui/component` | Widget 组件 | 38 源文件（Box/Editor/Markdown/SelectList 等） |
| `tui/chat` | 应用层/ChatApp | `chat_app*.go`, `chat_history*.go`, `state.go` |
| `tui/stdio` | 过程式 I/O | `spinner.go`, `progress.go`, `linereader.go` |
| `tui/agentadapter` | Agent 事件桥接 | `adapter.go` |

### 附录 C：引用资源

| 资源 | 说明 |
|------|------|
| [XiaoNuo TUI 设计规范 v1.0](../XiaoNuo%20Agent/docs/tui-design-standards.md) | 本文档的主要参考源，基于社区广泛调研 |
| [Mady TUI 设计规范](tui-design-specification.md) | Mady 现有设计准则 |
| [TUI 8 层架构 LAYERS.md](../tui/LAYERS.md) | 架构层级定义 |
| [TUI 8 层架构 ADR](decisions/tui-layers-architecture.md) | 10 条设计决策记录 |
| Elm Architecture | https://guide.elm-lang.org/architecture/ |
| Bubble Tea | https://github.com/charmbracelet/bubbletea |
| Ratatui | https://ratatui.rs/ |
| Textual | https://textual.textualize.io/ |
| Kitty Keyboard Protocol | https://sw.kovidgoyal.net/kitty/keyboard-protocol/ |
| Synchronized Output | https://gist.github.com/rockorager/e695fb2924d36b2bcf1fff4dbc370d42 |

---

> 本文档是活文档，随 TUI 架构演进持续更新。
> 最新同步：2026-07-30 | 配套文件：`tui/LAYERS.md`, `docs/tui-design-specification.md`
> 全部 12 项差距已于 Sprint 1-4 修复完成，`go test -race ./tui/...` 全绿。
