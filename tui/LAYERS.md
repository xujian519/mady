# TUI Module Layers

Dependency direction: high-number layers may depend on low-number layers, never the reverse.

| Layer | Package | Description | Depends on |
|-------|---------|-------------|------------|
| 0 Foundation | `tui/core` | Basic types, interfaces, rune utilities, cell-level rendering model (Cell/Row/DiffRows/SGR), fuzzy match, SpinnerStyle | None (only stdlib) |
| 0 Layout | `tui/layout` | Declarative layout primitives (Flex) — pure data over `core.Component`, no theming/agentcore | Layer 0 |
| 1 Terminal I/O | `tui/terminal` | Terminal abstraction, key parsing, input buffer, ANSI builders | Layer 0 |
| 2 Theming | `tui/theme` | Palette, semantic theme, JSON loading, file watch, Style | Layer 0, 1 |
| 3 Engine | `tui` (root) | TUI container, event loop, overlay system, focus stack, ChatApp bridge | Layer 0–2, chat |
| 4 Components | `tui/component` | UI components (Editor, Markdown, domain cards, syntax highlighter, overlays, panels) — 35 source files | Layer 0–2, fuzzy |
| 5 Application | `tui/chat` | Chat application layer (ChatApp, ChatHistory, state machine) — 14 source files | Layer 0–2, 4 |
| 6 Stdio | `tui/stdio` | Procedural stdout/stdin tools (Spinner, Renderer, ProgressBar, LineReader, layout) | Layer 0, 2 |
| 7 Adapter | `tui/agentadapter` | Agentcore → chat event conversion, BindAgent convenience | Layer 5, agentcore |

> `tui/layout` 在编号上归入 Layer 0（仅依赖 `tui/core`，不依赖 theming/agentcore），
> 但在概念上是"布局原语"——独立列出以便贡献者快速定位。

## Rules

- Higher layers may import lower layers; lower layers MUST NOT import higher layers.
- `tui/stdio` depends on Layer 0 and Layer 2 only; it MUST NOT depend on Layer 3–5.
- `tui/chat` depends on Layer 0–2 and 4 only; it does NOT depend on `tui/stdio` (stdio tools are for procedural stdout/stdin apps, not for the TUI chat app).
- `tui/chat` does NOT import `agentcore`. All agentcore integration is in `tui/agentadapter`.
- `tui/chat` uses `AppHost` interface instead of directly referencing `*TUI`, breaking the cycle.
- `tui/chat` uses `Subscriber` / `EventSubscriber` interfaces for event subscription, decoupled from agentcore.
- The root `tui` package does NOT re-export sub-package types. Consumers import sub-packages directly.
- The root `tui` package provides `NewChatApp` as a convenience constructor that creates both a `TUI` engine and a `chat.ChatApp`.

## Directory Structure

> Auto-verified: 90 source files (+ 40 test files) across 8 packages.
> Last sync: 2026-07-21.

```
tui/
├── core/                  # Layer 0 — Foundation (11 source files)
│   ├── component.go       # Component/Updatable/Focusable interfaces, Container, CURSOR_MARKER
│   ├── message.go         # Msg/Cmd types, Batch/Sequence/Quit, MsgBase
│   ├── width.go           # East-Asian width, truncation, padding, wrapping
│   ├── runeutil.go        # Shared rune utilities (CellWidthOfRunes, SliceRunesByCells, etc.)
│   ├── fuzzy_match.go     # Fuzzy matching utilities
│   ├── spinner_style.go   # SpinnerStyle type + preset vars (SpinnerDots, SpinnerLine, etc.)
│   ├── cell.go            # Cell-level rendering model: Cell/Row types, CellGrid
│   ├── celldiff.go        # Cell-level frame diff (DiffRows), stricter than string diff
│   ├── cellparse.go       # string → Row parser (ANSI escape → Cell grid)
│   ├── cellrender.go      # Row → ANSI string serializer (SerializeRow)
│   └── sgr.go             # SGR state machine: ParseSGR/BuildSGR, permissive parameter parsing
│
├── terminal/              # Layer 1 — Terminal I/O (8 source files)
│   ├── keys.go            # Key parsing, MatchesKey, Kitty protocol, KeyID
│   ├── keybindings.go     # KeybindingsManager, DefaultKeybindings, KeybindingDef
│   ├── stdin_buffer.go    # StdinBuffer for reassembling fragmented input
│   ├── terminal.go        # Terminal interface, ProcessTerminal, VirtualTerminal
│   ├── ansi.go            # ANSI escape sequence builders (pure functions, no I/O)
│   ├── terminal_darwin.go # macOS termios
│   ├── terminal_linux.go  # Linux termios
│   └── terminal_other.go # Fallback for other OSes
│
├── theme/                 # Layer 2 — Theming (7 source files)
│   ├── style.go           # ANSI Style, Color, Attr, symbols, box-drawing, cursor helpers
│   ├── color_resolve.go   # Color mode detection, RGB-to-256
│   ├── semantic_theme.go  # SemanticTheme struct + defaults (light/dark)
│   ├── palette.go         # Palette struct, CurrentPalette(), BuildPalette, SyncPaletteGlobals
│   ├── global.go          # SetSemanticTheme, InitThemeFromEnv, SetOnSemanticThemeChange
│   ├── json.go            # JSON theme parsing (vars/colors + variable references)
│   └── watch.go           # File-watch hot-reload for themes (mtime polling)
│
├── component/             # Layer 4 — Components (35 source files)
│   ├── autocomplete.go    # Autocomplete dropdown, StaticProvider, FilePathProvider
│   ├── box.go             # Box (border/padding container)
│   ├── text.go            # Text, TruncatedText
│   ├── input.go           # Single-line input editor
│   ├── keyhelp.go         # Keybindings cheat sheet
│   ├── loader.go          # Animated spinner component (callback-based, uses core.SpinnerStyle)
│   ├── markdown.go        # Markdown rendering (771 lines)
│   ├── selectlist.go      # Selectable list with fuzzy filter
│   ├── statusbar.go       # StatusBar
│   ├── settings.go        # Settings panel
│   ├── image.go           # Kitty/Sixel/iTerm2 image display
│   ├── viewport.go        # Scrollable viewport wrapper for large content
│   ├── table.go           # Tabular data rendering component
│   ├── fuzzy_provider.go  # FuzzyContentProvider, NormalizeForMatch, SubstringFuzzyFilter
│   │
│   ├── domain.go          # DomainMessage / DomainAction professional card data models
│   ├── evidence_card.go   # Evidence card: source attribution, direction, collapsible snippet
│   ├── conclusion_card.go # Conclusion card: confidence bar, evidence counts, classification
│   ├── approval_card.go   # Approval gate card renderer
│   ├── tool_card.go       # Tool-call result card: left-bar + title + collapsible content
│   ├── evidence_overlay.go # EvidenceOverlay: scrollable knowledge source display
│   ├── judgment_view.go   # JudgmentView: current-judgment summary panel (386 lines)
│   ├── review_gate.go     # ReviewGate overlay: structured review checklist (577 lines)
│   ├── session_selector.go # SessionSelector: session list with fuzzy filter (545 lines)
│   ├── command_center.go  # CommandCenter: Ctrl+P command palette overlay
│   ├── skill_center.go    # SkillCenter: skill list and management overlay
│   ├── system_status.go   # SystemStatus: system-mode display overlay
│   ├── todo_panel.go      # TodoPanel: task tracking panel
│   │
│   ├── syntax.go          # Syntax highlighter core (entry point, 313 lines)
│   ├── syntax_langs.go    # Built-in language specs (Go, Bash, JSON, YAML, etc.)
│   ├── syntax_tokenizer.go # Tokenizer for syntax highlighting
│   │
│   ├── editor.go          # Editor subsystem — core struct & interface (392 lines)
│   ├── editor_edit.go     # Editor — key dispatch & editing primitives (553 lines)
│   ├── editor_render.go   # Editor — rendering & mouse hit-testing (324 lines)
│   ├── editor_history.go  # Editor — undo/redo stack & input recall (182 lines)
│   └── editor_killring.go # Editor — Emacs kill-ring (yank/yank-pop) (126 lines)
│
├── layout/                # Layer 0 — Layout primitives (depends on core only)
│   ├── flex.go            # Flex declarative layout (main-axis size policies, 506 lines)
│   └── layout.go          # Layout helpers
│
├── chat/                  # Layer 5 — Application (14 source files)
│   ├── chat_app.go        # ChatApp struct, constructor, public API (1060 lines)
│   ├── chat_app_layout.go # chatLayout root Component + input router (582 lines)
│   ├── chat_app_stream.go # ChatApp streaming lifecycle handlers (submit/delta/end/error)
│   ├── chat_app_tool.go   # ChatApp tool-call/handoff/turn/compaction handlers
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

### No Re-exports

The root `tui` package does NOT re-export sub-package types. Since the library
has not been published yet, there is no backward-compatibility constraint.
All consumers import the specific sub-package they need:

```go
import (
    core "github.com/xujian519/mady/tui/core"
    "github.com/xujian519/mady/tui/terminal"
    "github.com/xujian519/mady/tui/theme"
    "github.com/xujian519/mady/tui/component"
    "github.com/xujian519/mady/tui/chat"
    "github.com/xujian519/mady/tui/agentadapter"
    "github.com/xujian519/mady/tui"
)
```

The root `tui` package exports only the `TUI` engine, `Overlay`, and the
`NewChatApp` convenience constructor.

### stdio vs component: Two Rendering Models

The TUI module has two parallel rendering models:

1. **TUI Engine** (Layer 3–5): Elm-architecture, differential rendering, `Component` interface.
   Components render into string arrays, the engine diffs and writes only changes.
   Used by `ChatApp`.

2. **stdio** (Layer 6): Procedural stdout/stdin, `\r` overwriting, `fmt.Fprint`.
   No component model, no differential rendering. Used by standalone scripts/examples
   that want plain stdout output without the TUI engine.

Both share `core.SpinnerStyle` (animation frame data) and `theme` (styling), but
operate on fundamentally different I/O models. The name `stdio` makes this
distinct — these tools work on raw stdin/stdout, not through the TUI engine.

### SpinnerStyle in Core

`SpinnerStyle` is a pure data type (animation frames + interval) with no
rendering dependency. It lives in `core` because both `component.Loader`
(TUI component) and `stdio.Spinner` (procedural spinner) need it. Putting it
in either consumer would force the other to import upwards.

### FuzzyContentProvider in Component

`FuzzyContentProvider` implements `core.AutocompleteProvider` and is a
component-level concept (an autocomplete data source). It was previously in
`util/fuzzy_bridge.go`, but since `util` has been reclassified as `stdio`
(procedural I/O tools), the provider belongs in `component` alongside
`StaticProvider` and `FilePathProvider`.

### Circular Dependency Break: AppHost Interface

`ChatApp` (in `tui/chat`) originally held a `*TUI` pointer, creating a cycle:
`chat` → `tui` (root) → `chat` (via re-exports).

Solution: `chat.AppHost` interface abstracts the operations ChatApp needs
(`Start`, `Stop`, `AddChild`, `Focus`, `RequestRender`, `PushOverlay`,
`RemoveOverlay`, `TerminalSize`). The root `tui` package provides a
`tuiAppHost` adapter that wraps `*TUI`, living in `chat_bridge.go`.

### Circular Dependency Break: Loader Callback

`Loader` (in `tui/component`) originally held a `*TUI` pointer to call
`RequestRender()`. Replaced with a `func()` callback injected at construction
time: `NewLoader(onRequestRender func(), message string)`.

### Decoupling: agentadapter Package

`tui/chat` does NOT import `agentcore`. Instead, `chat` defines its own event
types (`ChatEvent`, `ChatEventType`) and subscription interfaces (`Subscriber`,
`EventSubscriber`). The `tui/agentadapter` package provides `BindAgent()` which
converts `agentcore.Agent` events into `chat.ChatEvent` values and registers
them via the `Subscriber` interface. This keeps `chat` reusable without
agentcore and allows other event sources to integrate via the same interface.

### Internal Types Unexported

Types that are implementation details of `chat` are unexported:
`chatModel`, `chatLayout`, `chatAppendMsg`, `chatUpdateMsg`, `chatDeltaMsg`,
`chatFinalizeMsg`, `chatClearMsg`, `chatScrollMsg`. Only the public API types
(`ChatApp`, `ChatAppConfig`, `ChatHistory`, `ChatMessage`,
event types, interfaces) are exported.

### Msg Interface: MsgMarker() Method

`core.Msg` uses an exported `MsgMarker()` method instead of unexported `msg()`,
allowing external packages (e.g. `chat`) to implement the interface and use
type switches across package boundaries. External types can also embed
`core.MsgBase` for zero-effort compliance.

### Suggestion & AutocompleteProvider in Core

These interface/struct types live in `core` because multiple packages implement
`AutocompleteProvider` and return `Suggestion` values (`component.StaticProvider`,
`component.FuzzyContentProvider`, etc.). Placing them in `core` avoids upward
dependencies that would violate the layer ordering.

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
