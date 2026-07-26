# TUI 8 层架构设计决策

> **状态**：已采纳 · 2026-07-26
> **关联**：`tui/LAYERS.md`（原始定义）

---

## 背景

TUI 模块采用 8 层 Elm 架构（参见 `tui/LAYERS.md`），在实现过程中形成了若干重要的设计决策。
本文档将这些决策从 LAYERS.md 中提取为正式 ADR 记录，以便在架构评审时集中审阅。

---

## 决策 1：不重导出（No Re-exports）

`tui` 根包不重导出子包的类型。由于该库尚未发布为独立包，无须向后兼容。
所有消费者直接导入所需的子包：

```go
import (
    core   "github.com/xujian519/mady/tui/core"
    "github.com/xujian519/mady/tui/terminal"
    "github.com/xujian519/mady/tui/theme"
    "github.com/xujian519/mady/tui/component"
    "github.com/xujian519/mady/tui/chat"
    "github.com/xujian519/mady/tui/agentadapter"
    "github.com/xujian519/mady/tui"
)
```

根包仅导出 `TUI` 引擎、`Overlay` 和 `NewChatApp` 便利构造器。

---

## 决策 2：两套渲染模型（stdio vs Component）

TUI 模块有两套并行的渲染模型：

1. **TUI 引擎**（Layer 3–5）：Elm 架构，差分渲染，`Component` 接口。组件渲染为字符串数组，引擎做 diff 后只写变更。ChatApp 使用此模型。

2. **stdio**（Layer 6）：过程式 stdout/stdin，`\r` 覆写，`fmt.Fprint`。无组件模型，无差分渲染。供独立脚本/示例使用。

两者共享 `core.SpinnerStyle`（动画帧数据）和 `theme`（样式），但 I/O 模型完全不同。
`stdio` 的名称体现了这一区别——这些工具操作裸 stdin/stdout，不经过 TUI 引擎。

---

## 决策 3：SpinnerStyle 放在 Core

`SpinnerStyle` 是纯数据类型（动画帧 + 间隔），无渲染依赖。
放在 `core` 是因为 `component.Loader`（TUI 组件）和 `stdio.Spinner`（过程式）都需要它。
放在任一消费者包都会迫使另一方向上导入。

---

## 决策 4：FuzzyContentProvider 在 Component

`FuzzyContentProvider` 实现 `core.AutocompleteProvider`，属于组件层概念（自动补全数据源）。
之前位于 `util/fuzzy_bridge.go`，但 `util` 被重分类为 `stdio`（过程式 I/O 工具）后，
该 provider 与 `StaticProvider` 和 `FilePathProvider` 一同归入 `component`。

---

## 决策 5：循环依赖中断 — AppHost 接口

`ChatApp`（`tui/chat`）原本持有 `*TUI` 指针，形成循环依赖：`chat` → `tui`（根包）→ `chat`（通过重导出）。

**方案**：`chat.AppHost` 接口抽象了 ChatApp 所需的操作（`Start`、`Stop`、`AddChild`、
`Focus`、`RequestRender`、`PushOverlay`、`RemoveOverlay`、`TerminalSize`）。
根包 `tui` 中的 `tuiAppHost` 适配器包装 `*TUI`，位于 `chat_bridge.go`。

**后续改进（T3.9）**：AppHost 进一步拆分为 4 个窄接口（`LifecycleHost`、`ComponentHost`、
`OverlayHost`、`TerminalHost`），遵循接口隔离原则（ISP）。

---

## 决策 6：循环依赖中断 — Loader 回调

`Loader`（`tui/component`）原本持有 `*TUI` 指针以调用 `RequestRender()`。
替换为在构造时注入的 `func()` 回调：`NewLoader(onRequestRender func(), message string)`。

---

## 决策 7：解耦 — agentadapter 包

`tui/chat` 不导入 `agentcore`。`chat` 定义自己的事件类型（`ChatEvent`、`ChatEventType`）
和订阅接口（`Subscriber`、`EventSubscriber`）。`tui/agentadapter` 包提供 `BindAgent()`，
将 `agentcore.Agent` 事件转换为 `chat.ChatEvent` 并通过 `Subscriber` 接口注册。

这让 `chat` 可在无 agentcore 的环境下复用，也允许其他事件源通过同一接口集成。

---

## 决策 8：内部类型不导出

`chat` 的实现细节（`chatModel`、`chatLayout`、`chatAppendMsg` 等）均不导出。
仅公共 API 类型（`ChatApp`、`ChatAppConfig`、`ChatHistory`、`ChatMessage`、
事件类型、接口）导出。

---

## 决策 9：Msg 接口使用导出 MsgMarker()

`core.Msg` 使用导出的 `MsgMarker()` 方法（而非未导出的 `msg()`），
允许外部包（如 `chat`）跨包边界实现该接口并使用类型 switch。
外部类型也可以嵌入 `core.MsgBase` 以零成本符合接口。

---

## 决策 10：Suggestion 和 AutocompleteProvider 在 Core

这些接口/结构体类型位于 `core`，因为多个包实现了 `AutocompleteProvider`
并返回 `Suggestion` 值（`component.StaticProvider`、`component.FuzzyContentProvider` 等）。
放在 `core` 避免了违反分层顺序的向上依赖。

---

## 附录：相关文件

- `tui/LAYERS.md` — 8 层架构的完整定义、依赖矩阵和文件清单
- `tui/LAYERS.md#Known-Architectural-Compromises` — 已知架构妥协（tui→chat 向上依赖）
- `docs/decisions/AI_CHANGELOG.md` — T3.16/T3.17/T3.9 等架构变更的执行记录
