// Package tui provides a terminal user interface (TUI) engine built with an
// Elm-architecture, offering differential rendering, overlay/focus management,
// and a rich component library.
//
// # Module
//
// This is the standalone Go sub-module github.com/xujian519/mady/tui.
// It requires Go 1.26 and has a single external dependency:
// golang.org/x/sys (for terminal I/O on macOS/Linux/other Unix systems).
//
// # Architecture (8 Layers)
//
// The TUI module is organized into 8 layers, numbered 0–7.
// Dependencies flow upward: higher layers import lower layers, never the reverse.
//
//	Layer 0 — Foundation (tui/core)
//	  Basic types, Component/Updatable/Focusable interfaces, Container,
//	  cell-level rendering model (Cell/Row/DiffRows/SGR), spinner styles,
//	  fuzzy match utilities, rune/width helpers. No external dependencies.
//
//	Layer 0 — Layout (tui/layout)
//	  Declarative layout primitives (Flex), pure data over core.Component.
//
//	Layer 1 — Terminal I/O (tui/terminal)
//	  Terminal abstraction, key parsing (Kitty protocol), input buffer,
//	  ANSI escape builders, platform-specific termios (macOS/Linux/fallback).
//	  External dependency: golang.org/x/sys/unix.
//
//	Layer 2 — Theming (tui/theme)
//	  Palette, semantic theme (light/dark), ANSI Style (fg/bg/attrs),
//	  JSON theme loading with file-watch hot-reload.
//
//	Layer 3 — Engine (tui root package)
//	  TUI container, event loop, overlay system, focus stack,
//	  lifecycle management (Start/Stop/Quit/Tick/Every), message dispatch.
//
//	Layer 4 — Components (tui/component)
//	  35+ UI components: Editor, Markdown, autocomplete, selectlist,
//	  statusbar, table, syntax highlighter, domain cards (evidence,
//	  conclusion, approval, tool-call, review gate), overlays
//	  (command palette, skill center, session selector, system status).
//
//	Layer 5 — Application (tui/chat)
//	  Chat application layer: ChatApp struct, ChatHistory scrollable
//	  transcript, streaming lifecycle handlers, explicit FSM over
//	  interaction states.
//
//	Layer 6 — Stdio (tui/stdio)
//	  Procedural stdout/stdin tools (Spinner, Renderer, ProgressBar,
//	  LineReader, layout helpers). No TUI engine dependency.
//
//	Layer 7 — Agentcore Adapter (tui/agentadapter)
//	  Converts agentcore Agent events into TUI ChatEvent values.
//	  Optional: depends on github.com/xujian519/mady/agentcore for
//	  full integration with the Mady agent runtime framework.
//
// # Quick Start
//
// Create a minimal TUI application:
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"os"
//		"os/signal"
//
//		core "github.com/xujian519/mady/tui/core"
//		"github.com/xujian519/mady/tui/terminal"
//		"github.com/xujian519/mady/tui"
//	)
//
//	func main() {
//		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//		defer cancel()
//
//		// Create the TUI engine.
//		term := terminal.ProcessTerminal()
//		app := tui.New(term)
//
//		// Add a simple text component as the root.
//		hello := core.NewText("Hello, TUI!")
//		app.AddChild(hello)
//		app.Focus(hello)
//
//		// Alternative: use the convenience constructor for a chat-style app:
//		// tui.NewChatApp(term, config)
//
//		if err := app.Start(ctx); err != nil {
//			fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
//			os.Exit(1)
//		}
//	}
//
// # Integration with the Mady Agent Runtime
//
// When used within the Mady monorepo, the TUI module integrates with
// agentcore via tui/agentadapter.BindAgent() to display streaming
// LLM responses, tool calls, handoff transitions, and evidence cards
// in the terminal interface.
//
// The monolithic chat application is available via:
//
//	tui.NewChatApp(term, chat.ChatAppConfig{...})
//
// which creates both the TUI engine and a ChatApp wired together.
//
// # Internal Packages
//
// The tui/internal directory contains minimal copies of packages from
// the parent mady module (csync.Slice, fuzzy string utilities). These
// are present so the TUI module can build independently without a
// circular dependency on the parent module. External consumers should
// not import these internal packages.
//
// # Rendering Models
//
// The module has two parallel rendering models:
//   - TUI Engine (Layers 3–5): Elm-architecture, differential rendering
//     (cell-level frame diff), Component interface. Used by ChatApp.
//   - Stdio (Layer 6): Procedural stdout/stdin, \r-overwriting, fmt.Fprint.
//     No component model or differential rendering.
//
// Both share core.SpinnerStyle (animation frame data) and theme (styling),
// but operate on fundamentally different I/O models.
package tui
