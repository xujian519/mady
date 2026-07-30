# TUI Module Layers

Dependency direction: high-number layers may depend on low-number layers, never the reverse.

| Layer | Package | Description | Depends on |
|-------|---------|-------------|------------|
| 0 Foundation | `tui/core` | Basic types, interfaces, rune utilities, cell-level rendering model (Cell/Row/DiffRows/SGR), fuzzy match, SpinnerStyle | None (only stdlib) |
| 0 Layout | `tui/layout` | Declarative layout primitives (Flex) — pure data over `core.Component`, no theming/agentcore | Layer 0 |
| 1 Terminal I/O | `tui/terminal` | Terminal abstraction, key parsing, input buffer, ANSI builders | Layer 0 |
| 2 Theming | `tui/theme` | Palette, semantic theme, a11y theme, JSON loading, file watch, Style | Layer 0, 1 |
| 3 Engine | `tui` (root) | TUI container, event loop, overlay system, focus stack, ChatApp bridge | Layer 0–2, chat |
| 4 Components | `tui/component` | UI components (Editor, Markdown, domain cards, syntax highlighter, overlays, panels, toast, onboarding) — 41 source files | Layer 0–2, fuzzy |
| 5 Application | `tui/chat` | Chat application layer (ChatApp, ChatHistory, state machine) — 17 source files | Layer 0–2, 4 |
| 6 Stdio | `tui/stdio` | Procedural stdout/stdin tools (Spinner, Renderer, ProgressBar, LineReader, layout) | Layer 0, 1, 2 |
| 7 Adapter | `tui/agentadapter` | Agentcore → chat event conversion, BindAgent convenience | Layer 5, agentcore |

> `tui/layout` 在编号上归入 Layer 0（仅依赖 `tui/core`，不依赖 theming/agentcore），
> 但在概念上是"布局原语"——独立列出以便贡献者快速定位。

## Rules

- Higher layers may import lower layers; lower layers MUST NOT import higher layers.
- `tui/stdio` depends on Layer 0, 1, and 2 (core + terminal + theme); it MUST NOT depend on Layer 3–5.
- `tui/chat` depends on Layer 0–2 and 4 only; it does NOT depend on `tui/stdio` (stdio tools are for procedural stdout/stdin apps, not for the TUI chat app).
- `tui/chat` does NOT import `agentcore`. All agentcore integration is in `tui/agentadapter`.
- `tui/chat` uses `AppHost` interface instead of directly referencing `*TUI`, breaking the cycle.
- `tui/chat` uses `Subscriber` / `EventSubscriber` interfaces for event subscription, decoupled from agentcore.
- The root `tui` package does NOT re-export sub-package types. Consumers import sub-packages directly.
- The root `tui` package provides `NewChatApp` as a convenience constructor that creates both a `TUI` engine and a `chat.ChatApp`.

## Directory Structure

> Auto-verified: 112 source files (+ 64 test files) across 10 packages.
> Last sync: 2026-07-30.

```
tui/
├── core/                  # Layer 0 — Foundation (14 source files)
│   ├── component.go       # Component/Updatable/Focusable interfaces, Container, CURSOR_MARKER
│   ├── message.go         # Msg/Cmd types, Batch/Sequence/Quit, MsgBase
│   ├── errors.go          # Three-layer error model: TermError/NetError/LogicError
│   ├── width.go           # East-Asian width, truncation, padding, wrapping
│   ├── runeutil.go        # Shared rune utilities (CellWidthOfRunes, SliceRunesByCells, etc.)
│   ├── fuzzy_match.go     # Fuzzy matching utilities
│   ├── spinner_style.go   # SpinnerStyle type + preset vars (SpinnerDots, SpinnerLine, etc.)
│   ├── cell.go            # Cell-level rendering model: Cell/Row types, CellGrid
│   ├── celldiff.go        # Cell-level frame diff (DiffRows), stricter than string diff
│   ├── cellparse.go       # string → Row parser (ANSI escape → Cell grid)
│   ├── cellrender.go      # Row → ANSI string serializer (SerializeRow)
│   ├── sgr.go             # SGR state machine: ParseSGR/BuildSGR, permissive parameter parsing
│   ├── sanitize.go        # SanitizeRawContent: strips dangerous escape sequences from raw output
│   └── stack.go           # CaptureStack: goroutine stack trace for PanicMsg diagnostics
│
├── terminal/              # Layer 1 — Terminal I/O (9 source files)
│   ├── keys.go            # Key parsing, MatchesKey, Kitty protocol, KeyID
│   ├── keybindings.go     # KeybindingsManager, DefaultKeybindings, KeybindingDef
│   ├── stdin_buffer.go    # StdinBuffer for reassembling fragmented input
│   ├── terminal.go        # Terminal interface, ProcessTerminal, VirtualTerminal
│   ├── ansi.go            # ANSI escape sequence builders (pure functions, no I/O)
│   ├── detect.go          # Terminal capability detection (color level, kitty, etc.)
│   ├── terminal_darwin.go # macOS termios
│   ├── terminal_linux.go  # Linux termios
│   └── terminal_other.go # Fallback for other OSes
│
├── theme/                 # Layer 2 — Theming (13 source files)
│   ├── a11y_themes.go     # Accessibility theme definitions (high-contrast, color-blind safe)
│   ├── style.go           # ANSI Style, Color, Attr, symbols, box-drawing, cursor helpers
│   ├── color_resolve.go   # Color mode detection, RGB-to-256
│   ├── semantic_theme.go  # SemanticTheme struct + defaults (light/dark)
│   ├── palette.go         # Palette struct, CurrentPalette(), BuildPalette, SyncPaletteGlobals
│   ├── global.go          # SetSemanticTheme, InitThemeFromEnv, SetOnSemanticThemeChange
│   ├── json.go            # JSON theme parsing (vars/colors + variable references)
│   ├── watch.go           # File-watch hot-reload for themes (mtime polling)
│   ├── watchutil.go       # runWithRestart: shared panic-recovery for watchers
│   ├── aliases.go         # Color alias resolution (name → hex mapping)
│   ├── quantize.go        # Color quantization engine (RGB→16 ANSI, theme-level)
│   ├── system_appearance.go # macOS NSAppearance dark/light detection
│   └── theme_registry.go  # Theme registry: built-in + user theme registration
│
├── component/             # Layer 4 — Components (41 source files)
│   ├── autocomplete.go    # Autocomplete dropdown, StaticProvider, FilePathProvider
│   ├── box.go             # Box (border/padding container)
│   ├── text.go            # Text, TruncatedText
│   ├── input.go           # Single-line input editor
│   ├── keyhelp.go         # Keybindings cheat sheet
│   ├── loader.go          # Animated spinner component (callback-based, uses core.SpinnerStyle)
│   ├── markdown.go        # Markdown rendering (block-level parser + renderer)
│   ├── selectlist.go      # Selectable list with fuzzy filter
│   ├── statusbar.go       # StatusBar
│   ├── settings.go        # Settings panel
│   ├── image.go           # Kitty/iTerm2/HalfBlock/ASCII image display
│   ├── viewport.go        # Scrollable viewport wrapper for large content
│   ├── table.go           # Tabular data rendering component
│   ├── fuzzy_provider.go  # FuzzyContentProvider, NormalizeForMatch, SubstringFuzzyFilter
│   ├── footer.go          # Footer bar showing core keyboard shortcuts (responsive)
│   │
│   ├── domain.go          # DomainMessage / DomainAction professional card data models
│   ├── evidence_card.go   # Evidence card: source attribution, direction, collapsible snippet
│   ├── conclusion_card.go # Conclusion card: confidence bar, evidence counts, classification
│   ├── confidence_bar.go  # Confidence-level bar visualization component
│   ├── approval_card.go   # Approval gate card renderer
│   ├── tool_card.go       # Tool-call result card: left-bar + title + collapsible content
│   ├── evidence_overlay.go # EvidenceOverlay: scrollable knowledge source display
│   ├── judgment_view.go   # JudgmentView: current-judgment summary panel (386 lines)
│   ├── review_gate.go     # ReviewGate overlay: structured review checklist (577 lines)
│   ├── session_selector.go # SessionSelector: session list with fuzzy filter (545 lines)
│   ├── command_center.go  # CommandCenter: Ctrl+P command palette overlay
│   ├── debug_overlay.go   # DebugOverlay: ctrl+shift+d diagnostic panel (FPS, queue, events)
│   ├── skill_center.go    # SkillCenter: skill list and management overlay
│   ├── system_status.go   # SystemStatus: system-mode display overlay
│   ├── todo_panel.go      # TodoPanel: task tracking panel
│   ├── toast.go           # Toast: transient notification bar with auto-dismiss
│   ├── onboarding.go      # FirstRunWizard: welcome guide for first-time users
│   │
│   ├── syntax.go          # Syntax highlighter core (entry point, 313 lines)
│   ├── syntax_langs.go    # Built-in language specs (Go, Bash, JSON, YAML, etc.)
│   ├── syntax_tokenizer.go # Tokenizer for syntax highlighting
│   │
│   ├── editor.go          # Editor subsystem — core struct & interface (392 lines)
│   ├── editor_chip.go     # Editor — inline chips (completion hints, annotations)
│   ├── editor_edit.go     # Editor — key dispatch & editing primitives (553 lines)
│   ├── editor_render.go   # Editor — rendering & mouse hit-testing (324 lines)
│   ├── editor_history.go  # Editor — undo/redo stack & input recall (182 lines)
│   └── editor_killring.go # Editor — Emacs kill-ring (yank/yank-pop) (126 lines)
│
├── layout/                # Layer 0 — Layout primitives (depends on core only)
│   ├── breakpoint.go       # LayoutBreakpoint type + DetectLayoutBreakpoint
│   ├── flex.go            # Flex declarative layout (main-axis size policies, 506 lines)
│   └── layout.go          # Layout helpers
│
├── chat/                  # Layer 5 — Application (17 source files)
│   ├── chat_app.go        # ChatApp struct, constructor, public API (1060 lines)
│   ├── chat_app_layout.go # chatLayout root Component + input router (582 lines)
│   ├── chat_app_stream.go # ChatApp streaming lifecycle handlers (submit/delta/end/error)
│   ├── chat_app_tool.go   # ChatApp tool-call/handoff/turn/compaction handlers
│   ├── chat_app_todo.go   # ChatApp todo-list panel integration handlers
│   ├── chat_history.go    # ChatHistory scrollable transcript component (566 lines)
│   ├── chat_history_render.go        # ChatHistory rendering pipeline (viewport, separators)
│   ├── chat_history_render_message.go # Per-message rendering (role dispatch, card router)
│   ├── chat_history_render_highlight.go # Text-selection highlighting
│   ├── chat_history_input.go         # ChatHistory input & viewport scrolling, mouse handling
│   ├── chat_history_selection.go     # ChatHistory selection business logic
│   ├── events.go          # ChatEvent types (15 events), Subscriber/EventSubscriber interfaces
│   ├── state.go           # Explicit FSM over ChatApp interaction states (249 lines)
│   ├── reasoning.go       # Reasoning/thinking block rendering
│   └── clipboard.go       # Clipboard helpers (pbcopy/xclip/win32)
│   ├── layout_editor.go   # Editor frame layout helpers (ChildRect indices, baseline reset)
│   ├── layout_shortcuts.go# Copy/clipboard shortcut helpers (doCopy, hasSelection)
│
├── agentadapter/          # Layer 7 — Agentcore Adapter
│   └── adapter.go         # BindAgent, AgentRunner, event conversion (agentcore → chat)
│
├── stdio/                 # Layer 6 — Procedural stdio tools (5 source files)
│   ├── renderer.go        # Streaming markdown stdout renderer + ToolStatus/HandoffStatus helpers
│   ├── spinner.go         # Procedural spinner (stdout-based), uses core.SpinnerStyle
│   ├── progress.go        # ProgressBar, TokenUsageDisplay, Timer
│   ├── linereader.go      # Blocking stdin helper, Confirm, PromptSelect
│   └── layout.go          # Box-drawing and layout helpers (moved from theme)
│
├── internal/              # Internal helpers (not exported, used by sibling packages)
│   ├── csync/slice.go     # Concurrent slice helpers
│
├── doc.go                 # Package doc for the root `tui` package
├── tui.go                 # Layer 3 — TUI container, types, constructor (271 lines)
├── tui_loop.go            # Layer 3 — eventLoop (lifecycle/render/input junction)
├── tui_lifecycle.go       # Layer 3 — Start/Stop/Quit/Done/Context/Tick/Every
├── tui_input.go          # Layer 3 — processMsg, Cmd execution, input callbacks, mouse mode
├── tui_render.go          # Layer 3 — RequestRender, renderFrame, normalizeLine
├── tui_focus.go           # Layer 3 — focus stack + overlay stack management
├── overlay.go             # Layer 3 — Overlay data type + composition helpers (573 lines)
├── chat_bridge.go         # Layer 3 — NewChatApp convenience constructor + tuiAppHost adapter
└── LAYERS.md              # This file
```

## Key Design Decisions

This section has been extracted into a formal Architecture Decision Record.
See [`docs/decisions/tui-layers-architecture.md`](../docs/decisions/tui-layers-architecture.md)
for the complete list of 10 design decisions:

1. [No Re-exports](../docs/decisions/tui-layers-architecture.md#决策-1不重导出no-re-exports)
2. [Two Rendering Models (stdio vs Component)](../docs/decisions/tui-layers-architecture.md#决策-2两套渲染模型stdio-vs-component)
3. [SpinnerStyle in Core](../docs/decisions/tui-layers-architecture.md#决策-3spinnerstyle-放在-core)
4. [FuzzyContentProvider in Component](../docs/decisions/tui-layers-architecture.md#决策-4fuzzycontentprovider-在-component)
5. [Circular Dependency Break: AppHost Interface](../docs/decisions/tui-layers-architecture.md#决策-5循环依赖中断--apphost-接口)
6. [Circular Dependency Break: Loader Callback](../docs/decisions/tui-layers-architecture.md#决策-6循环依赖中断--loader-回调)
7. [Decoupling: agentadapter Package](../docs/decisions/tui-layers-architecture.md#决策-7解耦--agentadapter-包)
8. [Internal Types Unexported](../docs/decisions/tui-layers-architecture.md#决策-8内部类型不导出)
9. [Msg Interface: Exported MsgMarker()](../docs/decisions/tui-layers-architecture.md#决策-9msg-接口使用导出-msgmarker)
10. [Suggestion & AutocompleteProvider in Core](../docs/decisions/tui-layers-architecture.md#决策-10suggestion-和-autocompleteprovider-在-core)

### Cell-Level Rendering Model (core/cell*.go + sgr.go)

The `core` package contains a cell-level rendering subsystem that converts
rendered strings into a 2D grid of `Cell` values, each carrying an absolute
`Style` (fg/bg/attrs). This eliminates two classes of bugs the string model has:

1. **Wide-char truncation** — splicing an overlay onto a line containing
   wide characters (e.g. `"中xx"`) at the wrong column previously corrupted
   the display.
2. **SGR encoding ambiguity** — two strings that are visually identical but
   differ in SGR encoding (e.g. `"\x1b[31m"` vs `"\x1b[38;5;1m"`) would cause
   unnecessary re-renders.

Files:
- `cell.go` — `Cell`/`Row` types, `CellGrid`
- `celldiff.go` — `DiffRows` cell-level frame diff (stricter than string diff)
- `cellparse.go` — string → `Row` parser (ANSI escape → Cell grid)
- `cellrender.go` — `Row` → ANSI string serializer (`SerializeRow`)
- `sgr.go` — SGR state machine: `ParseSGR`/`BuildSGR`, permissive parameter parsing

### Editor Subsystem (5-file architecture)

The `Editor` component is split across 5 files totaling ~1577 lines, organized
by responsibility:

| File | Lines | Responsibility |
|------|-------|---------------|
| `editor.go` | 392 | Core struct, `Component`/`Updatable`/`Focusable` implementation, constructor |
| `editor_edit.go` | 553 | Key dispatch (`processKeys`), buffer editing primitives (insert, cursor motion, delete family) |
| `editor_render.go` | 324 | Rendering (soft-wrap, prompts, `CURSOR_MARKER`), mouse hit-testing, selection |
| `editor_history.go` | 182 | Undo/redo stack + submitted-input recall history (two independent histories) |
| `editor_killring.go` | 126 | Emacs kill-ring: `pushKillRing`, `yank`, `yankPop` + low-level insert/delete helpers |

The split follows the same pattern as `chat_app_*.go` — grouping methods by
concern into sibling files within the same package, avoiding a single 1500+ line
monolith while keeping all `Editor` code co-located.

### core.Every Removal

`core.Every` has been removed because the `Cmd` signature (`func() Msg`)
cannot express repeated emission. The replacement is `TUI.Every(d, fn)` which
schedules a periodic goroutine on the TUI's lifecycle context:

```go
// Old (removed):
// core.Every(d, func() core.Msg { ... })

// New:
tui.Every(d, func(time.Time) core.Msg { ... })
```

The `TUI.Every` ticker stops automatically when the TUI stops (via context
cancellation). See `tui_lifecycle.go:190-209` for the implementation.

### ChatApp Multi-File Architecture

`ChatApp` is split across 4 `chat_app_*.go` files + 10 `chat_history_*.go` /
helper files, following the same sibling-file pattern as the Editor subsystem:

- `chat_app.go` — struct, constructor, public API (1060 lines)
- `chat_app_layout.go` — `chatLayout` root component + input router (582 lines)
- `chat_app_stream.go` — streaming lifecycle handlers (submit/delta/end/error)
- `chat_app_tool.go` — tool-call/handoff/turn/compaction handlers + diff extraction

`ChatHistory` rendering is similarly split:
- `chat_history.go` — struct + public API (566 lines)
- `chat_history_render.go` — rendering pipeline (viewport, separators, scroll)
- `chat_history_render_message.go` — per-message rendering (role dispatch, cards)
- `chat_history_render_highlight.go` — text-selection highlighting
- `chat_history_input.go` — input handling, viewport scrolling, mouse
- `chat_history_selection.go` — selection business logic

An explicit FSM (`state.go`, 249 lines) decouples interaction states from the
imperative event handlers in `chat_app_stream.go` / `chat_app_tool.go`.

---

## Known Architectural Compromises

### tui (L3) → chat (L5) Dependency

The root `tui` package imports `tui/chat` (L5) via `chat_bridge.go` to
provide the `NewChatApp` convenience constructor. This is an upward dependency
(L3 → L5) that technically violates the strict layering rule.

**Why it exists**: `NewChatApp` creates both a `TUI` engine and a `ChatApp`
wired together via the `tuiAppHost` adapter. Moving this to `chat` would
break the public API (`tui.NewChatApp` is used by `cmd/mady` and `example/`).

**Why we accept it**: The dependency is isolated to a single file
(`chat_bridge.go`) with a well-designed adapter pattern. The cycle is broken
at the interface level (`chat.AppHost`). The cost of reversing it (1-2 weeks,
breaking API change) outweighs the benefit for an internal-only module.

**When to revisit**: If TUI is ever extracted as a standalone library for
external consumption, this dependency must be reversed first.
