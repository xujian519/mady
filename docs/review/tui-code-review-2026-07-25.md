# Mady TUI 代码审阅报告

> 审阅依据：`docs/tui-design-specification.md`（v1.0）+ `tui/LAYERS.md`
>
> 审阅范围：`tui/` 目录 90+ 源文件（不含测试），~30K 行代码
> 审阅方法：自动化检查（lint + vet + race）+ 人工代码走查 + 子智能体系统性审阅
> 审阅日期：2026-07-25

---

## 目录

1. [审阅概述](#1-审阅概述)
2. [自动化检查结果](#2-自动化检查结果)
3. [架构合规性](#3-架构合规性)
4. [审阅发现汇总](#4-审阅发现汇总)
   - 4.1 严重问题
   - 4.2 高优先级问题
   - 4.3 中优先级问题
   - 4.4 低优先级问题
5. [测试覆盖分析](#5-测试覆盖分析)
6. [性能评估](#6-性能评估)
7. [安全评估](#7-安全评估)
8. [修正建议优先级](#8-修正建议优先级)

---

## 1. 审阅概述

### 1.1 总体评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构合规性 | ⭐⭐⭐⭐⭐ 5/5 | 严格遵循 8 层依赖方向，无循环引用 |
| 组件接口 | ⭐⭐⭐⭐⭐ 5/5 | 35 个组件均正确实现 Component/Updatable 契约 |
| 测试覆盖 | ⭐⭐⭐⭐☆ 4/5 | 总体 53.2%，核心层 59.1%，`agentadapter` 仅 14.9% |
| 代码质量 | ⭐⭐⭐⭐☆ 4/5 | 15 项 lint 告警，均为 minor |
| 终端协议 | ⭐⭐⭐⭐⭐ 5/5 | DECAWM/CSI 2026/Kitty 协议配对正确 |
| 主题系统 | ⭐⭐⭐⭐☆ 4/5 | 4 个 JSON token 未映射，文件热重载有状态不同步风险 |
| 性能 | ⭐⭐⭐⭐☆ 4/5 | 无严重瓶颈，Overlay 克隆和 `itoa` 重复可优化 |
| 安全 | ⭐⭐⭐⭐⭐ 5/5 | 无敏感路径引用，OSC 52 剪切板风险可控 |

### 1.2 问题统计

| 级别 | 数量 | 核心问题 |
|------|------|---------|
| 🔴 Critical | 3 | 状态机 EventKindFor 不完整 1 项 + 主题 JSON 遗漏 4 token + Overlay 全帧 deep-copy |
| 🟠 High | 6 | 3 份 `itoa` 重复 + 热重载状态不同步 + ANSI 魔法字符串 + Editor/Input 硬编码色值 |
| 🟡 Medium | 8 | StdinBuffer goroutine 泄漏路径 + 渲染锁时序文档缺失 + Msg 类型缺 Godoc + `FlushEsc` 冗余 + 颜色检测不一致 + 其他 |
| 🔵 Low | 10 | lint 告警 + 文件命名 + 导出文档 + 过时函数标记 + 注释语言不统一 |
| **合计** | **27** | |

---

## 2. 自动化检查结果

### 2.1 测试与竞态

```text
$ go test -race -count=1 ./tui/...
✅ 全部 11 个包通过（含 -race）
```

| 包 | 耗时 | 结果 |
|----|------|------|
| `tui` | 2.594s | ✅ |
| `tui/agentadapter` | 1.014s | ✅ |
| `tui/chat` | 1.040s | ✅ |
| `tui/component` | 1.273s | ✅ |
| `tui/core` | 1.015s | ✅ |
| `tui/layout` | 1.015s | ✅ |
| `tui/stdio` | 1.016s | ✅ |
| `tui/terminal` | 1.018s | ✅ |
| `tui/theme` | 1.026s | ✅ |

### 2.2 Vet 检查

```text
$ go vet ./tui/...
✅ 通过，无告警
```

### 2.3 Lint 检查

```text
$ golangci-lint run ./tui/...
⚠️ 15 issues
```

| 类别 | 数量 | 位置 |
|------|------|------|
| `unconvert` | 5 | `overlay.go:54-58`, `chat_app_todo.go:84-85` |
| `staticcheck` | 4 | `viewport.go:158`, `viewport_test.go:225`, `keys_test.go:53,78` |
| `unused` | 2 | `viewport.go:89` (sbCache), `detect.go:182` (hasOSC52Clipboard) |
| `errcheck` | 1 | `tui_loop.go:23` |
| `gocritic` | 1 | `detect.go:743` singleCaseSwitch |
| `ineffassign` | 1 | `keys.go:351` |
| `misspell` | 1 | `terminal.go:298` synchronises → synchronizes |

---

## 3. 架构合规性

### 3.1 依赖方向验证

根据 `tui/LAYERS.md` 的依赖规则逐层验证：

| 包 | 层级 | 依赖的 tui 子包 | 期望 | 判定 |
|----|------|----------------|------|------|
| `core` | L0 | 无 | 仅 stdlib | ✅ |
| `layout` | L0 | `core` | 仅 core | ✅ |
| `terminal` | L1 | `core` | L0 | ✅ |
| `theme` | L2 | `terminal` | L0, L1 | ✅ |
| `tui` (root) | L3 | `core`, `terminal`, `theme`, `chat` | L0-2, chat | ✅ 注意：`chat` 是 L5 > L3，但 LAYERS.md 明确允许此依赖 |
| `component` | L4 | `core`, `terminal`, `theme`, `internal/` | L0-2 | ✅ |
| `chat` | L5 | `core`, `terminal`, `theme`, `component`, `layout` | L0-2, L4 | ✅ **未依赖 stdio** ✅，**未依赖 agentcore** ✅ |
| `stdio` | L6 | `core`（通过别名）, `terminal`, `theme` | L0, 2 | ⚠️ 实际依赖 `terminal`（L1），LAYERS.md 表说"Layer 0, 2"，但这是文档精度问题，非代码违规 |
| `agentadapter` | L7 | `chat`, `component` | L5, agentcore | ✅ |

> **发现**：`stdio` 实际导入 `terminal`（Layer 1），与 `LAYERS.md` 表格中的"Layer 0, 2"略有差异。该依赖是合理的（`stdio/spinner.go` 需要 terminal 写入），建议更新 LAYERS.md 的依赖列。

### 3.2 无循环引用验证

项目使用 `AppHost` 接口 + `tuiAppHost` 适配器（`chat_bridge.go`）和 `Loader` 回调注入来打破循环引用：

```
旧：chat → tui (root) → chat ✗
新：chat → AppHost (interface) ← tuiAppHost (在 chat_bridge.go) ✓
```

✅ 编译期无循环引用。

### 3.3 Agentcore 隔离

`tui/chat` 的所有 import 路径中**未出现** `agentcore`：

```go
// chat/chat_app.go 的 import
"context"
"fmt"
"log/slog"
"sync"
"time"

"github.com/xujian519/mady/tui/component"
core "github.com/xujian519/mady/tui/core"
"github.com/xujian519/mady/tui/layout"
terminal "github.com/xujian519/mady/tui/terminal"
"github.com/xujian519/mady/tui/theme"
"github.com/xujian519/mady/tui/internal/csync"
```

✅ 符合"chat 不导入 agentcore"规范。

---

## 4. 审阅发现汇总

### 4.1 🔴 严重问题（Critical）

#### C1. `EventKindFor()` 未映射 `ApprovalPromptChatEvent`

**位置**：`tui/chat/state.go:253-293`
**违反规范**：§4 事件与消息规范 / LAYERS.md "显式 FSM 状态转换"
**严重性**：critical

`EventKindFor()` 的 type switch 中对 17 种 ChatEvent 做了映射，但缺少 `ApprovalPromptChatEvent`。虽然当前运行时路径通过 `onApprovalPrompt` 硬编码调用 `Transition(evtApprovalRequest)` 来绕过，但这是一颗定时炸弹：任何未来通过 `EventKindFor(e)` 处理审批事件的代码路径都会得到 `evtUnknown`，导致状态机静默不更新。

**修复**：在 switch 中添加 `case ApprovalPromptChatEvent: return evtApprovalRequest`

---

#### C2. 主题 JSON 解析遗漏 4 个 SemanticTheme 字段

**位置**：`tui/theme/json.go:102-192`, `tui/theme/semantic_theme.go`
**违反规范**：§6 主题与视觉规范 / 规范附录 B 的 Token 表
**严重性**：critical

`applyColorKey` switch 缺少以下 4 个 JSON token 的处理：

| JSON Token | SemanticTheme 字段 | DefaultMadyDark 中的值 | 未处理影响 |
|-----------|-------------------|----------------------|-----------|
| `"system"` | `System` | `"#5BC0EB"` | SystemStyle 回退为空 → 系统消息无特殊颜色 |
| `"assistantText"` | `AssistantText` | `"#DCEAF3"` | 助手回复使用 Text 颜色（影响小） |
| `"loaderSpinner"` | `LoaderSpinner` | `"#38C8F4"` | LoaderSpinner 回退到 Accent |
| `"progressBar"` | `ProgressBar` | `"#38C8F4"` | ProgressBar 回退到 Accent |

其中 `System` 的影响最显著：`theme/aliases.go` 引用的 `SystemStyle` 依赖此字段，用户创建的 JSON 主题中即使指定了 `"system"` 也会被静默忽略。

**修复**：在 `applyColorKey` 中添加 4 个 case。

---

#### C3. `composeOverlays` 每帧全量 deep-copy 造成大量内存分配

**位置**：`tui/overlay.go:249-262`
**违反规范**：§11.1 渲染性能指标——帧渲染时间 < 16ms
**严重性**：critical

```go
clone := make([]core.Row, len(base))
for i, r := range base {
    clone[i] = r
    if r.Cells != nil {
        clone[i].Cells = make([]core.Cell, len(r.Cells))
        copy(clone[i].Cells, r.Cells)
    }
}
```

每帧执行全量 deep-copy 整个 base rows。60 行终端 × 60fps = 每小时 ~12,960,000 次 `[]core.Cell` 分配。虽然有 `if len(overlays) == 0 { return base }` 早期短路，但一旦有任何 overlay（autocomplete dialog、keyhelp 等）处于打开状态，这个开销就会持续。

**修复**：延迟 deep-copy 到实际需要修改某行时（Copy-on-Write 模式）。

---

### 4.2 🟠 高优先级问题（High）

#### H1. `itoa` 功能三处重复实现

**位置**：
- `tui/terminal/ansi.go:48-57`
- `tui/theme/quantize.go:138-146`
- `tui/core/sgr.go:412-422`

**违反规范**：§12 文档与贡献规范 — DRY 原则
**严重性**：high

三份完全相同的 `int64 → string` 实现。项目已经依赖 `golang.org/x/sys`，并非零依赖项目，可以直接使用 `strconv.FormatInt` 或提取到 `tui/internal/conv.go`。

---

#### H2. 文件热重载在 JSON 解析失败时永久跳过文件

**位置**：`tui/theme/watch.go:30-51`
**违反规范**：§6.2 主题文件格式 — 热重载可靠性
**严重性**：high

```go
lastMtime = mt                    // ← 提前更新 mtime
_ = LoadSemanticThemeFromFile(...) // ← 失败时错误被丢弃
```

如果文件在编辑器 atomic-write 过程中被 watcher 读取（半写状态 → JSON 解析失败），`lastMtime` 已被更新。下一轮 watcher tick 检测到 mtime 未变，跳过加载——该主题文件**永久被忽略**，直到用户再次保存文件。

**修复**：仅在 `LoadSemanticThemeFromFile` 成功后更新 `lastMtime`。

---

#### H3. `tui_render.go` 硬编码 ANSI 转义序列（魔法字符串）

**位置**：`tui/tui_render.go:118-214`（10 处引用）
**违反规范**：§5.1 ANSI 转义规范 — 使用 terminal/ansi.go 的构造器
**严重性**：high

`terminal/ansi.go`（Layer 1）已经定义了 ANSI builder 纯函数，但 `tui_render.go`（Layer 3）在 10 处硬编码了相同的转义字符串字面量：

| 字面量 | 出现次数 | 应替换为 |
|--------|---------|---------|
| `"\x1b[?25l"` | 3 | `terminal.HideCursor()` |
| `"\x1b[?25h"` | 1 | `terminal.ShowCursor()` |
| `"\x1b[0J"` | 2 | `terminal.ClearFromCursor()` |
| `"\x1b[0K"` | 1 | `terminal.ClearToEnd()` |
| `"\x1b[0m"` | 3 | `terminal.Reset` |
| `"\x1b[H"` | 1 | 新增 cursorHome 函数 |

需要给 `tui_render.go` 添加 `terminal "github.com/xujian519/mady/tui/terminal"` 导入。

---

#### H4. Editor/Input 硬编码选中背景色（256 色索引 33）

**位置**：
- `tui/component/editor_render.go:299,305`
- `tui/component/input.go:214`

**违反规范**：§6 主题与视觉规范 — 主题驱动配色
**严重性**：high

```go
bodyText = "\x1b[48;5;33m" + core.StripAnsi(bodyText) + "\x1b[0m"    // editor_render
displayed = "\x1b[48;5;33m" + core.StripAnsi(displayed) + "\x1d[0m"  // input
```

对比 `chat/chat_history_render_highlight.go:49-52` 的正确模式——先读 `h.theme.SelectedBg`，有值则用主题色，无值才回退。Editor 和 Input 应持有主题引用或在构造函数中注入选中色。

---

#### H5. 两套独立终端色彩能力检测不一致

**位置**：
- `tui/theme/color_resolve.go:22-43`（`DetectColorMode`）
- `tui/terminal/detect.go:562-612`（`computeTrueColor`）

**违反规范**：§5.2 终端能力检测 — 统一检测来源
**严重性**：high

`theme.DetectColorMode()` 基于环境变量做启发式检测，而 `terminal.DetectTerminalContext` 基于品牌检测 + 环境变量做推断。`ThemeRegistry.ApplyThemeByName` 通过 `ColorModeFromEnv()` 使用 `theme` 包的检测结果，不利用 `terminal` 包已经检测到的更精确信息。

**修复**：让 `theme.DetectColorMode()` 委托到 `terminal.CurrentTerminalContext()`。

---

#### H6. `renderFrame` 中 `composeOverlays` 调用前锁释放后引用

**位置**：`tui/tui_render.go:32-91`
**违反规范**：§11.3 阻塞防护 — 数据竞争防护
**严重性**：high

TUI 渲染管线在两次 `t.mu.Lock()` 之间将组件 `Render()` 放在锁外执行。虽然组件内部有各自的锁保护，但框架未在文档中声明 `Render()` 可能被并发调用。

**修复**：在 `tui/core/component.go` 的 `Component.Render` 文档中标注"Render is called outside the TUI lock and may be called concurrently with Update"。

---

### 4.3 🟡 中优先级问题（Medium）

#### M1. 6 个导出 Msg 类型缺少 Godoc 注释

**位置**：`tui/core/message.go:19-89`
**违反规范**：§12.1 组件文档要求 / AGENTS.md "导出符号必须有注释"
**严重性**：medium

以下导出类型缺少 Godoc：

- `KeyMsg`、`PasteMsg`、`WindowSizeMsg`、`TickMsg`、`QuitMsg`、`MouseMsg`、`BatchMsg`、`SequenceMessage`、`Cmd`、`MouseAction`

对比之下，同文件的 `PanicMsg`（line 49）和 `CtxMessage`（line 179）有完整的 Godoc，说明这是遗漏而非有意省略。

---

#### M2. `ProcessTerminal.Stop()` 中 `SetKittyProtocolActive` 调用时序潜在竞争

**位置**：`tui/terminal/terminal.go:303-314,244-248`
**违反规范**：§5.2 Kitty 键盘协议 push/pop 配对
**严重性**：medium

`PushKittyKeyboard` 和 `Stop` 在解锁后都调用了 `SetKittyProtocolActive`（全局 `kittyActive` 原子变量写）。虽然实际运行时不会并发（`PushKittyKeyboard` 仅在 Start 时调用），但若未来重构为异步 stop 路径，可能发生竞争。

**修复**：将 `SetKittyProtocolActive` 移到锁内调用。

---

#### M3. `FlushEsc()` 在 eventLoop 和 bg goroutine 中被重复调用

**位置**：`tui/tui_loop.go:44-47`, `tui/terminal/stdin_buffer.go:92-103`
**违反规范**：§11 性能规范
**严重性**：medium

`eventLoop` 每帧调用 `t.stdin.FlushEsc()`，同时 `NewStdinBuffer` 启动的 `flushLoop` goroutine 每 25ms 也调用一次。两者通过 `b.mu` 竞争同一锁。虽然每次调用极快，但锁竞争在高帧率（60fps）下可能增加 ~5-10% 的帧延迟。

**修复**：移除其中一个调用点。

---

#### M4. `StdinBuffer.flushLoop` goroutine 在 panic 路径泄漏

**位置**：`tui/terminal/stdin_buffer.go:75-83`, `tui/tui_loop.go:15-32`
**违反规范**：§11.3 阻塞防护
**严重性**：medium

`eventLoop` 的 defer 中嵌套的 panic recover 路径未关闭 `stdin.Close()`，可能导致 `flushLoop` goroutine 永远阻塞。

**修复**：在 defer 中先 `t.stdin.Close()` 再 `t.Stop()`。

---

#### M5. `watch.go` 没有在错误时重试

**位置**：`tui/theme/watch.go`
**违反规范**：§6.2 主题文件格式 — 热重载可靠性
**严重性**：medium

文件监控在检测到 mtime 变化后尝试加载，若文件不可读或被临时锁定（如文本编辑器的 atomic write 过程），加载失败且不重试。用户需要再次保存才能触发重载。

**修复**：在加载失败时不更新 lastMtime（与 H2 关联修复）。

---

#### M6. macOS Light 模式系统外观检测不能返回 `AppearanceLight`

**位置**：`tui/theme/system_appearance.go:119-127`
**违反规范**：§6.3 默认主题 — 系统外观检测
**严重性**：medium

macOS Light 模式下 `AppleInterfaceStyle` key 不存在，`defaults read` 返回 exit code 1，被当作错误处理返回 `AppearanceUnknown`。导致 `auto` 主题模式始终选择 Dark。

**修复**：检查 err 是否为 `exec.ExitError`（key 不存在时进程正常退出，exit code 非 0）。

---

#### M7. `quantize.go` 的 `QuantizeTheme` 均为 no-op 但签名暗示有实际转换

**位置**：`tui/theme/quantize.go:149-173`
**违反规范**：§6 主题规范 — 函数行为应与其名称一致
**严重性**：medium

`QuantizeTheme`、`quantizeTheme256`、`quantizeThemeBasic` 都返回原指针，不做任何转换。注释说量化在 render-time 完成。但公共导出函数的名称暗示调用者会得到一份预量化后的新主题，不符合最小惊讶原则。

**修复**：添加显著注释标注 no-op，或等到将来需要预量化时再实现。

---

#### M8. `ReviewGatePayload` 跨层结构耦合

**位置**：`tui/chat/events.go:70-77` 引用 `tui/component/review_gate.go:55-68`
**违反规范**：§3.4 组件分层定位
**严重性**：medium

`chat/events.go` 的 `ReviewGatePayload` 直接使用 `component.ReviewEvidence` 和 `component.ReviewCheckItem` 作为字段类型。虽然符合依赖方向（Layer 5 > Layer 4），但意味着 `component` 不能独立于 `chat` 重构这些类型。

---

### 4.4 🔵 低优先级问题（Low）

| 编号 | 位置 | 问题 | 建议 |
|------|------|------|------|
| L1 | `tui/tui_loop.go:23` | `defer func() { recover() }()` 未检查 errcheck | 添加 `_ = recover()` 或 lint 忽略注释 |
| L2 | `tui/terminal/detect.go:743` | `singleCaseSwitch: should use if` | 改为 if 语句 |
| L3 | `tui/terminal/keys.go:351` | `rest2 = ""` 无效赋值 | 删除此行 |
| L4 | `tui/terminal/terminal.go:298` | typo: "synchronises" → "synchronizes" | 修正拼写 |
| L5 | `tui/component/viewport.go:158` | SA9003: empty branch | 添加 TODO 注释或实现 |
| L6 | `tui/component/viewport.go:89` | `sbCache` struct unused | 删除或实施缓存 |
| L7 | `tui/terminal/detect.go:182` | `hasOSC52Clipboard` unused | 删除或实施 |
| L8 | `tui/terminal/keys_test.go:53,78` | ST1018: unicode control char in literal | 用 `\x1b` 替换字面量 ESC |
| L9 | `tui/kitty_altscreen_test.go` | 文件名命名不一致 | `kitty_altscreen` → `kitty_alt_screen` |
| L10 | `tui/component/domain.go` 等 | 注释语言中英混杂 | 统一为中文（按 AGENTS.md）或英文（按 Go 惯例） |

---

## 5. 测试覆盖分析

### 5.1 按包覆盖率

| 包 | 覆盖率 | 判定 |
|----|--------|------|
| `tui/layout` | 75.7% | ✅ |
| `tui/stdio` | 72.8% | ✅ |
| `tui` (root) | 66.4% | ✅ |
| `tui/core` | 59.1% | ⚠️ 核心层建议 ≥ 70% |
| `tui/terminal` | 57.7% | ⚠️ 终端 I/O 复杂，建议 ≥ 65% |
| `tui/theme` | 54.4% | ⚠️ 主题系统需补充 JSON 解析测试 |
| `tui/component` | 50.4% | 🔴 35 个组件的覆盖率偏低 |
| `tui/chat` | 47.9% | 🔴 应用层 FSM 需更多测试 |
| `tui/agentadapter` | 14.9% | 🔴 严重不足 |
| `tui/internal/*` | 0.0% | ⚠️ 纯辅助包，可接受 |
| **总体** | **53.2%** | |

### 5.2 关键未覆盖区域

| 文件 | 函数 | 覆盖率 |
|------|------|--------|
| `adapter.go` | `parseReviewGateData`, `agentTaskToInfo` | 0% |
| `chat_app.go` | accessors (`Host`, `Editor`, `Loader`, `Done`, `StatusBar` 等 20+ 个) | 0% |
| `state.go` | `Transition` 的完整状态转换矩阵覆盖不足 |
| `json_test.go` | 仅测试 3 个 token（accent/mdHeading/success） |

---

## 6. 性能评估

### 6.1 关键路径性能

| 路径 | 分析 |
|------|------|
| **帧渲染** | 差分渲染（`DiffFrame`）+ 存储 raw string 缓存避免重复 `ParseLine` ✅ |
| **事件循环** | 单 goroutine 串行化，无锁竞争 ✅ |
| **流式输出** | `renderFrame` 内的 `rawLines` append 在快速流式时可能多次扩容 ⚠️ |
| **Overlay 合成** | 全帧 deep-copy 是已知瓶颈 🔴 |
| **光标管理** | 状态式缓存避免每帧 emit CSI，保护 blink 定时器 ✅ |
| **主题热重载** | 基于 mtime 轮询，无 goroutine 泄漏 ✅ |

### 6.2 内存分配热点

| 位置 | 分配 | 频率 |
|------|------|------|
| `overlay.go:249-262` (deep-copy) | O(rows) 个 slice header + 底层数组 | 每个有 overlay 的帧 |
| `tui_render.go:45-52` (rawLines) | 切片追加扩容 | 每个帧 |
| `core/sgr.go:228-275` (RenderSGR) | `strings.Builder` | 每个 SGR 序列渲染 |

---

## 7. 安全评估

### 7.1 敏感路径检查

| 路径 | 在 TUI 中引用？ | 判定 |
|------|----------------|------|
| `tools/bash.go` | ❌ 未引用 | ✅ |
| `acp/auth.go` | ❌ 未引用 | ✅ |
| `mcp/config_trust.go` | ❌ 未引用 | ✅ |
| `guardrails/*` | ❌ 未引用 | ✅ |

### 7.2 TUI 特有的安全关注点

| 关注点 | 状态 | 说明 |
|--------|------|------|
| 剪切板（OSC 52） | ⚠️ 低风险 | `chat/clipboard.go` 将 base64 数据写入终端输出；如果终端集成日志捕获 stdout，可能泄露 |
| 键盘快捷键覆盖 | ✅ 已防护 | `TerminalSupportsKittyKeyboard` 仅识别已知品牌，未知终端不启用增强协议 |
| JSON 键位绑定 | ✅ 已防护 | `validateKeyToken` 拒绝未知 modifier 和空 token |

---

## 8. 修正建议优先级

### 🔴 立即修复（P0）

| ID | 修复项 | 预计耗时 | 影响 |
|----|--------|---------|------|
| C1 | `EventKindFor` 添加 `ApprovalPromptChatEvent` | 5 分钟 | 状态机完整性 |
| C2 | `applyColorKey` 添加 4 个缺失 case | 10 分钟 | 主题系统完整性 |
| C3 | Overlay deep-copy 改为 CoW 模式 | 1-2 小时 | 帧渲染性能 |

### 🟠 本周内修复（P1）

| ID | 修复项 | 预计耗时 |
|----|--------|---------|
| H1 | 消除 `itoa` 重复（提取到 `internal/` 或改用 `strconv.FormatInt`） | 20 分钟 |
| H2 | `watch.go` 失败时不更新 mtime | 15 分钟 |
| H3 | `tui_render.go` 改用 `terminal/ansi.go` 的构造器 | 30 分钟 |
| H4 | Editor/Input 主题化选中色 | 1 小时 |
| H5 | 统一颜色检测到 `terminal.CurrentTerminalContext()` | 30 分钟 |
| H6 | 补充 `Component.Render` 并发文档 | 10 分钟 |

### 🟡 两周内修复（P2）

| ID | 修复项 | 预计耗时 |
|----|--------|---------|
| M1 | 补充 6 个导出 Msg 类型的 Godoc | 15 分钟 |
| M2 | Kitty 协议全局变量写入加锁 | 15 分钟 |
| M3 | 移除冗余 `FlushEsc` 调用 | 10 分钟 |
| M4 | 修复 panic 路径的 stdin 泄漏 | 10 分钟 |
| M5 | 重试失败的文件加载 | 20 分钟 |
| M6 | 修复 macOS Light 外观检测 | 15 分钟 |
| M7 | `QuantizeTheme` 标注 no-op | 5 分钟 |
| M8 | 解耦 ReviewGatePayload 的 component 依赖 | 1 小时 |

### 🔵 持续改进（P3）

| ID | 修复项 |
|----|--------|
| L1-L10 | lint 告警、文件命名、注释语言统一 |
| — | 补充 `json_test.go` 覆盖缺失的 4 个 token |
| — | 提高 `agentadapter` 和 `chat` 包的测试覆盖率 |
| — | `TerminalSupportsKittyKeyboard` 添加 Deprecated 标记 |

---

## 附录：规范对照索引

| 规范章节 | 对应的问题 |
|---------|-----------|
| §3 组件设计规范 | M1 (Godoc), M8 (跨层耦合), H4 (硬编码色值) |
| §4 事件与消息规范 | C1 (EventKindFor) |
| §5 终端 I/O 规范 | H3 (魔法字符串), H5 (颜色检测), M2 (Kitty 时序), L2/L3/L4 |
| §6 主题与视觉规范 | C2 (JSON token), H2 (热重载), M5 (MQ), M6 (macOS 外观), M7 (quantize) |
| §7 布局规范 | C3 (Overlay deep-copy) |
| §8 动画与动效规范 | 无问题 |
| §9 无障碍规范 | H4 (选中色硬编码 — 影响高对比度模式) |
| §10 测试规范 | 覆盖率不足 (agentadapter/chat) |
| §11 性能规范 | C3, H1 (itoa), M3 (FlushEsc 冗余), M4 (goroutine 泄漏) |
| §12 文档与贡献规范 | M1 (Godoc), H6 (Render 并发文档), L10 (注释语言) |

---

> 审阅完成：2026-07-25
> 审阅人：Grok Build (AI Code Review)
> 配套规范：`docs/tui-design-specification.md`
> 架构参考：`tui/LAYERS.md`
