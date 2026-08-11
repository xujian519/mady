package chat

import (
	"context"
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
)

// ---------------------------------------------------------------------------
// ChatAppConfig
// ---------------------------------------------------------------------------

// ChatAppConfig configures a ChatApp instance. Pass to NewChatApp.
type ChatAppConfig struct {
	Title string

	KittyKeyboardMode     string
	KittyKeyboardFlags    int64
	DisableBracketedPaste bool
	AltScreen             bool
	MouseMode             string

	EditorMinRows int64
	EditorMaxRows int64
	EditorPrompt  string

	ShowTimings bool
	ShowTurns   bool

	// ContextWindow is the model's max context size in tokens, used to render
	// the StatusBar context-occupancy bar. 0 hides the bar.
	ContextWindow int64

	// ReasoningRenderer controls how thinking segments are displayed in the
	// chat history. Pass nil (default) to hide reasoning; pass a
	// *DefaultReasoningRenderer to restore the legacy Show/Mode policy; pass
	// a custom implementation for full control (sidebar, overlay, etc.).
	ReasoningRenderer ReasoningRenderer

	Theme *ChatHistoryTheme

	Providers []core.AutocompleteProvider

	// Context is the cancellation root passed to OnSubmit. When nil,
	// context.Background() is used. Callers should wire the TUI's lifecycle
	// context here so in-flight submissions are canceled on Stop.
	Context context.Context

	OnSubmit     func(ctx context.Context, input string)
	OnQuit       func()
	OnInterrupt  func()
	OnImagePaste func() // called when an image paste is detected (clipboard image, empty text)

	// OnCommandCenter is invoked by Ctrl+P to open the command palette.
	// It is a host-level concern (the palette is built from the slash
	// registry in cmd/mady), so the chat layer only forwards the key.
	// When nil, Ctrl+P is ignored by the chat layout and falls through
	// to the editor (which has no binding for it).
	OnCommandCenter func()

	Host AppHost

	// SuppressHandoffToolDisplay when true suppresses transfer_to_* tool calls
	// from appearing in the chat history. Used in integrated mode where handoffs
	// are invisible to the user. In Router mode this should be false so users
	// can see the routing process.
	SuppressHandoffToolDisplay bool
}

// ---------------------------------------------------------------------------
// chatModel — internal runtime state
// ---------------------------------------------------------------------------

// chatModel holds the mutable runtime state of a ChatApp.
type chatModel struct {
	// streamID is the message ID of the currently-streaming assistant
	// message ("" when none). It is an implementation handle for AppendDelta,
	// NOT a state signal: the FSM state below is the single source of truth
	// for "what state are we in".
	streamID string
	// activeTools tracks tool calls currently in flight. Like streamID it is
	// data (for the system-status panel's tool count), not state.
	activeTools map[string]time.Time

	// state is the FSM's current interaction state. It is the single source
	// of truth for UI state display (JudgmentView, StatusBar). Handlers
	// call Transition(state, EventKindFor(e)) to compute the next state.
	// Initialized to StateInitializing at ChatApp creation.
	state AppState

	// Token accounting for the StatusBar indicator. usagePrompt/Completion
	// accumulate across turns within one agent run; turnStarted is set on
	// AgentStart so onTurnEnd can compute tok/s = completion / elapsed.
	usagePrompt     int64
	usageCompletion int64
	turnStarted     time.Time

	// turnCompleted counts finished agent turns (user submit + agent end =
	// one turn). Surfaced on the StatusBar as "T#N" after each onAgentEnd.
	turnCompleted int64

	// judgmentSummary carries the current judgment-bar snapshot. It is
	// populated during agent execution (approval prompts, interrupts) and
	// cleared on agent start.
	judgmentSummary JudgmentSummary

	// confirm holds the active inline confirmation state, if any.
	// Non-nil only when state == StateConfirmPending.
	confirm *confirmPending

	// toolSeq is a per-run counter for tool call sequence numbers.
	// Reset on agent start, incremented on each tool call.
	toolSeq int64

	// queuedInput buffers user input entered while the assistant is still
	// streaming. When streaming ends (onAgentEnd/onAgentError), the queue
	// is flushed and inputs are submitted in FIFO order. This lets users
	// type ahead without waiting for the current turn to finish.
	queuedInput []string

	// pastedTexts stores full text content for oversized pastes. When a user
	// pastes text exceeding pasteThreshold, the full text is stored here and
	// a compact placeholder is inserted into the editor. On submit, placeholders
	// are expanded back to the original text. Entries are garbage-collected
	// after successful submission.
	pastedTexts map[int]string
	nextPasteID int
}

// ---------------------------------------------------------------------------
// Inline confirmation types
// ---------------------------------------------------------------------------

// InlineConfirm is a lightweight yes/no confirmation prompt displayed inline
// in the status bar. Used for medium-severity actions (delete, clear, etc.)
// that don't warrant a full modal overlay.
type InlineConfirm struct {
	Prompt  string        // confirmation text, e.g. "Delete this message?"
	OnYes   func()        // called when user confirms (y/Y)
	OnNo    func()        // called when user cancels (n/N/Esc/timeout)
	Timeout time.Duration // auto-cancel duration; 0 = default 10s
}

// confirmPending holds the active confirmation state.
type confirmPending struct {
	confirm InlineConfirm
	timer   *time.Timer
}

// ---------------------------------------------------------------------------
// Overlay data types
// ---------------------------------------------------------------------------

// ReviewGateData carries the structured data needed to render a review gate overlay.
// See component.ReviewGate for field descriptions.
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

// SystemStatusData carries the structured data needed to render a system
// status overlay. See component.SysEvent for event field descriptions.
type SystemStatusData struct {
	Mode       string
	ModeReason string
	Events     []component.SysEvent
	Impacts    []string
}

// EvidenceOverlayData carries evidence items to display in the evidence
// details overlay. Maps to component.EvidenceItem.
type EvidenceOverlayData struct {
	Items []component.EvidenceItem
}

// ---------------------------------------------------------------------------
// Overlay categories and options
// ---------------------------------------------------------------------------

// OverlayCategory constants — these match the categories in the tui package
// but are defined here so the chat package does not import tui directly.
// The bridge (chat_bridge.go in package tui) maps them via type assertion.
const (
	OverlayCatSelection = iota // 选择型
	OverlayCatReview           // 审阅型
	OverlayCatGate             // 复核型
	OverlayCatSystem           // 系统型
)

// OverlayOpts configures an overlay opened via OpenOverlay.
type OverlayOpts struct {
	WidthPct  int  // percentage of terminal width (0 = default 60)
	HeightPct int  // percentage of terminal height (0 = default 60)
	Dim       bool // dim the background while open
	Category  int  // overlay category (OverlayCat*), 0 = selection
}

// ---------------------------------------------------------------------------
// overlayHandle — concrete OverlayRef implementation
// ---------------------------------------------------------------------------

type overlayHandle struct {
	content       core.Component
	focus         bool
	dimBackground bool
	category      int // OverlayCat* constant, 0 = selection
	anchor        int
	percentX      int
	percentY      int
	widthPct      int
	heightPct     int
}

func (o *overlayHandle) OverlayContent() core.Component { return o.content }
func (o *overlayHandle) SetOverlayFocus(v bool)         { o.focus = v }
func (o *overlayHandle) SetOverlayDimBackground(v bool) { o.dimBackground = v }
func (o *overlayHandle) OverlayWantsFocus() bool        { return o.focus }
func (o *overlayHandle) OverlayDimBackground() bool     { return o.dimBackground }
func (o *overlayHandle) OverlayAnchor() int             { return o.anchor }
func (o *overlayHandle) OverlayPercentX() int           { return o.percentX }
func (o *overlayHandle) OverlayPercentY() int           { return o.percentY }
func (o *overlayHandle) OverlayWidthPct() int           { return o.widthPct }
func (o *overlayHandle) OverlayHeightPct() int          { return o.heightPct }

// OverlayCategory returns the category code for bridge propagation.
// This is NOT part of the OverlayRef interface — it is detected via type
// assertion in tuiAppHost.PushOverlay so the interface stays backward
// compatible.
func (o *overlayHandle) OverlayCategory() int { return o.category }
