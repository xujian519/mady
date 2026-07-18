package chat

// This file defines the ChatHistory data model and its mutating API: types
// (ChatRole, ChatMessage, ChatHistory, ChatHistoryTheme), the constructor,
// theme/callback setters, Append/PatchMessage/AppendDelta/Finalize/Clear, and
// the per-message render cache framework (msgCache + invalidate paths).
//
// Rendering lives in chat_history_render.go.
// Input handling (mouse, scroll, click-to-toggle) lives in chat_history_input.go.

import (
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// ChatHistory — a viewport-clipped list of chat messages.
//
// Each entry is a ChatMessage with a role, optional ID (for streaming
// updates), a body rendered as Markdown, and metadata. Append/Update are
// thread-safe and invalidate the render cache.
//
// Viewport behavior:
//   - The caller sets MaxRows (typically recomputed on resize by ChatLayout).
//   - FollowTail=true (default) auto-scrolls to the bottom on every Append.
//   - The user can scroll manually with PgUp/PgDn/↑↓ when focused.
// ---------------------------------------------------------------------------

// ChatRole tags a message's origin.
type ChatRole int64

const (
	RoleUser ChatRole = iota + 1
	RoleAssistant
	RoleSystem
	RoleTool
	RoleError
	RoleDivider
)

// ChatMessage is one item in the chat transcript.
type ChatMessage struct {
	ID        string        // optional — non-empty enables in-place updates.
	Role      ChatRole      // governs default styling & prefix.
	Text      string        // raw source (markdown for assistant, plain for others).
	Pending   bool          // the message is still streaming; a cursor may be shown.
	Meta      string        // e.g. tool name, duration.
	At        time.Time     // emission time (for display).
	Duration  time.Duration // optional — displayed after Meta.
	Collapsed bool          // when true, tool output shows summary; click to expand.

	// DomainMsg 承载结构化专业产出（证据/结论/审批）。
	// 非空时 renderMessage 路由到对应的卡片组件渲染。
	DomainMsg *component.DomainMessage

	// Thinking blocks (structured content).
	ThinkingSegments []ThinkingSegment

	// Internal: last delta dedup to suppress streaming repetition loops.
	lastDelta string
}

// ThinkingSegment holds a chunk of thinking text.
type ThinkingSegment struct {
	Text      string
	Collapsed bool
}

// ChatHistoryTheme customizes prefix / styling for each role.
type ChatHistoryTheme struct {
	UserPrefix      string
	UserStyle       theme.Style
	AssistantPrefix string
	AssistantStyle  theme.Style
	SystemPrefix    string
	SystemStyle     theme.Style
	ToolPrefix      string
	ToolStyle       theme.Style
	ToolBorder      theme.Style
	SuccessStyle    theme.Style
	ErrorPrefix     string
	ErrorStyle      theme.Style
	DividerChar     string
	DimStyle        theme.Style
	ThinkingStyle   theme.Style
	SelectedBg      string // ANSI background for selection
	MarkdownTheme   component.MarkdownTheme
}

// DefaultChatHistoryTheme returns a theme built from the current palette.
func DefaultChatHistoryTheme() ChatHistoryTheme {
	pal := theme.CurrentPalette()
	return ChatHistoryTheme{
		UserPrefix:      "> ",
		UserStyle:       pal.User,
		AssistantPrefix: "",
		AssistantStyle:  pal.Assistant,
		SystemPrefix:    "",
		SystemStyle:     pal.System,
		ToolPrefix:      theme.SymbolArrow + " ",
		ToolStyle:       pal.Dim,
		ToolBorder:      pal.BorderMuted,
		SuccessStyle:    pal.Success,
		ErrorPrefix:     theme.SymbolCross + " ",
		ErrorStyle:      pal.Error,
		DividerChar:     "─",
		DimStyle:        pal.Dim,
		ThinkingStyle:   pal.Thinking,
		SelectedBg:      "\x1b[48;5;33m",
		MarkdownTheme:   component.DefaultMarkdownTheme(),
	}
}

// ChatHistory is a Component that renders ChatMessages inside a scrollable
// viewport.
type msgRange struct {
	startLine int
	endLine   int
	msgIndex  int
	toolGroup bool // true if this is a collapsed tool group
	groupFrom int  // first message index in the group
	groupTo   int  // last message index in the group
}

type selectionPos struct {
	line int64 // absolute line index in cachedAll
	col  int64 // visible column within the line
}

// cachedMessage holds the rendered lines for a single message at a specific
// width. It is invalidated when the message content or width changes.
//
// For Pending (still-streaming) assistant messages, blockCache holds the
// per-block render cache so each delta only re-renders the tail block instead
// of the whole message — turning streaming render cost from O(N²) into ~O(N).
type cachedMessage struct {
	lines      []string
	blockCache *component.BlockCache // non-nil only for Pending assistant messages
}

type ChatHistory struct {
	mu       sync.Mutex
	messages []ChatMessage
	msgIDSeq int64 // monotonic sequence for auto-generated message IDs
	theme    ChatHistoryTheme
	maxRows  int64
	offset   int64 // lines scrolled up from the bottom (0 = tail)
	follow   bool

	reasoningRenderer ReasoningRenderer

	// render cache keyed on width + invalidation counter.
	cachedWidth     int64
	cachedAll       []string
	cachedMsgRanges []msgRange
	dirty           bool
	expandedGroups  map[int]bool // group message indices that are expanded

	// msgCache maps message ID to its rendered lines at the current width.
	// It is cleared on width changes and on any global style change; single
	// messages are invalidated on PatchMessage/AppendDelta.
	msgCache map[string]cachedMessage

	// optional invalidate callback (usually TUI.RequestRender).
	onInvalidate func()

	// callback invoked when a message is copied
	onCopy func(text string)

	// text selection state — stored as absolute line indices in cachedAll
	selActive   bool
	selStart    selectionPos
	selEnd      selectionPos
	selDragging bool

	// When a left-click follows a wheel event within a short window, the entire
	// gesture (press-motion-release) is suppressed. This prevents accidental text
	// selection when the user switches from two-finger scroll to single-finger
	// slide on a trackpad.
	lastWheelAt     time.Time
	suppressGesture bool

	// renderCount tracks how many times renderMessage has been invoked. Used
	// by tests to verify incremental caching behavior.
	renderCount int

	// tailAnchorLen snapshots the rendered-content length at the moment the
	// viewport was last at the tail (follow=true). Once the user scrolls up
	// (follow=false), it freezes; new streaming content grows cachedAll beyond
	// the anchor, and Render shows "↓ N new — End to follow" where N =
	// len(cachedAll) - tailAnchorLen. Returning to the tail refreshes the
	// anchor and clears the hint.
	tailAnchorLen int64
}

// NewChatHistory returns an empty history using the default theme.
// Reasoning display defaults to hidden; set a ReasoningRenderer via
// SetReasoningRenderer to enable it.
func NewChatHistory() *ChatHistory {
	return &ChatHistory{
		theme:             DefaultChatHistoryTheme(),
		follow:            true,
		dirty:             true,
		expandedGroups:    make(map[int]bool),
		msgCache:          make(map[string]cachedMessage),
		reasoningRenderer: HiddenReasoningRenderer{},
	}
}

// SetTheme overrides the styling theme.
func (h *ChatHistory) SetTheme(t ChatHistoryTheme) {
	h.mu.Lock()
	h.theme = t
	h.dirty = true
	h.clearMsgCacheLocked()
	h.mu.Unlock()
	h.invalidate()
}

// SetReasoningRenderer installs the renderer used to display thinking
// segments. Pass nil to hide reasoning entirely (HiddenReasoningRenderer).
// Passing a *DefaultReasoningRenderer restores the legacy Show/Mode policy.
func (h *ChatHistory) SetReasoningRenderer(r ReasoningRenderer) {
	h.mu.Lock()
	if r == nil {
		r = HiddenReasoningRenderer{}
	}
	h.reasoningRenderer = r
	h.dirty = true
	h.clearMsgCacheLocked()
	h.mu.Unlock()
	h.invalidate()
}

// SetOnInvalidate wires a callback invoked on any mutation (typically
// TUI.RequestRender).
func (h *ChatHistory) SetOnInvalidate(fn func()) {
	h.mu.Lock()
	h.onInvalidate = fn
	h.mu.Unlock()
}

func (h *ChatHistory) SetOnCopy(fn func(text string)) {
	h.mu.Lock()
	h.onCopy = fn
	h.mu.Unlock()
}

// SetMaxRows clamps the visible viewport.
func (h *ChatHistory) SetMaxRows(n int64) {
	h.mu.Lock()
	if h.maxRows == n {
		h.mu.Unlock()
		return
	}
	h.maxRows = n
	h.mu.Unlock()
	h.invalidate()
}

// SetMaxRowsDirect sets the viewport height without triggering invalidation.
// Use this from within Render to avoid re-entrant RequestRender calls.
func (h *ChatHistory) SetMaxRowsDirect(n int64) {
	h.mu.Lock()
	h.maxRows = n
	h.mu.Unlock()
}

// CollapseConsecutiveTools collapses consecutive tool/system messages
// at the end when there are 2+ of them (called on turn end).
func (h *ChatHistory) CollapseConsecutiveTools() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Find the last run of consecutive tool/system messages.
	runStart := -1
	for i := len(h.messages) - 1; i >= 0; i-- {
		r := h.messages[i].Role
		if r == RoleTool || r == RoleSystem {
			runStart = i
		} else {
			break
		}
	}
	if runStart < 0 {
		return
	}
	// Check that we have at least 2
	if len(h.messages)-runStart < 2 {
		return
	}
	// Ensure expandedGroups map exists and mark this group as collapsed
	// by removing the key (default is false/collapsed).
	if h.expandedGroups == nil {
		h.expandedGroups = make(map[int]bool)
	}
	delete(h.expandedGroups, runStart)
	h.dirty = true
	h.clearMsgCacheLocked()
}

// Append adds a new message, auto-scrolling to the bottom when follow-tail
// is enabled. If m.ID is empty, a new unique ID is generated and returned.
func (h *ChatHistory) Append(m ChatMessage) string {
	h.mu.Lock()
	if m.ID == "" {
		h.msgIDSeq++
		m.ID = fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), h.msgIDSeq)
	}
	if m.At.IsZero() {
		m.At = time.Now()
	}
	h.messages = append(h.messages, m)
	h.dirty = true
	// Clear selection since content changed invalidates absolute indices
	h.selActive = false
	h.selDragging = false
	if h.follow {
		h.offset = 0
	}
	id := m.ID
	h.mu.Unlock()
	h.invalidate()
	return id
}

// PatchMessage patches the text / pending / meta fields of a message identified
// by id. No-op if id is unknown.
func (h *ChatHistory) PatchMessage(id string, fn func(m *ChatMessage)) bool {
	if fn == nil {
		return false
	}
	h.mu.Lock()
	for i := range h.messages {
		if h.messages[i].ID == id {
			fn(&h.messages[i])
			h.invalidateMessageLocked(id)
			h.dirty = true
			h.selActive = false
			h.selDragging = false
			h.mu.Unlock()
			h.invalidate()
			return true
		}
	}
	h.mu.Unlock()
	return false
}

// AppendDelta appends text to an existing assistant message, or creates a
// new one if `id` is empty or unknown. Returns the effective message ID.
func (h *ChatHistory) AppendDelta(id, delta string) string {
	return h.AppendDeltaWithKind(id, delta, "")
}

// AppendDeltaWithKind appends text to an existing assistant message, routing
// to thinking or text segments based on `kind` ("thinking" or "text"/"").
func (h *ChatHistory) AppendDeltaWithKind(id, delta, kind string) string {
	if delta == "" {
		return id
	}

	h.mu.Lock()

	// Suppress consecutive identical deltas (LLM streaming repetition loop).
	for i := range h.messages {
		if h.messages[i].ID == id && h.messages[i].lastDelta == delta {
			h.mu.Unlock()
			return id
		}
	}

	if id != "" {
		for i := range h.messages {
			if h.messages[i].ID == id {
				h.messages[i].lastDelta = delta
				if kind == "thinking" {
					if len(h.messages[i].ThinkingSegments) == 0 {
						h.messages[i].ThinkingSegments = append(h.messages[i].ThinkingSegments, ThinkingSegment{})
					}
					last := &h.messages[i].ThinkingSegments[len(h.messages[i].ThinkingSegments)-1]
					last.Text += delta
				} else {
					h.messages[i].Text += delta
				}
				h.messages[i].Pending = true
				h.invalidateMessageLocked(id)
				h.dirty = true
				h.selActive = false
				h.selDragging = false
				if h.follow {
					h.offset = 0
				}
				h.mu.Unlock()
				h.invalidate()
				return id
			}
		}
	}
	h.msgIDSeq++
	newID := fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), h.msgIDSeq)
	msg := ChatMessage{
		ID:      newID,
		Role:    RoleAssistant,
		Pending: true,
		At:      time.Now(),
	}
	if kind == "thinking" {
		msg.ThinkingSegments = []ThinkingSegment{{Text: delta}}
	} else {
		msg.Text = delta
	}
	h.messages = append(h.messages, msg)
	h.dirty = true
	h.selActive = false
	h.selDragging = false
	if h.follow {
		h.offset = 0
	}
	h.mu.Unlock()
	h.invalidate()
	return newID
}

// Finalize clears the Pending flag on the given id.
func (h *ChatHistory) Finalize(id string) {
	h.PatchMessage(id, func(m *ChatMessage) { m.Pending = false })
}

// Clear empties the transcript.
func (h *ChatHistory) Clear() {
	h.mu.Lock()
	h.messages = nil
	h.offset = 0
	h.dirty = true
	h.clearMsgCacheLocked()
	h.renderCount = 0
	h.selActive = false
	h.selDragging = false
	// Reset the stick-to-bottom anchor so a cleared history (e.g. /new) does
	// not carry a stale tailAnchorLen from the pre-clear era — otherwise the
	// next streaming run would show a meaningless "↓ N new" hint computed
	// against the old content length.
	h.tailAnchorLen = 0
	h.follow = true
	h.mu.Unlock()
	h.invalidate()
}

// clearMsgCacheLocked drops all per-message render caches. Caller must hold mu.
func (h *ChatHistory) clearMsgCacheLocked() {
	h.msgCache = make(map[string]cachedMessage)
}

// invalidateMessageLocked drops the render cache for a single message. Caller
// must hold mu.
func (h *ChatHistory) invalidateMessageLocked(id string) {
	delete(h.msgCache, id)
}

// Messages returns a snapshot of the transcript.
func (h *ChatHistory) Messages() []ChatMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ChatMessage, len(h.messages))
	copy(out, h.messages)
	return out
}

func (h *ChatHistory) invalidate() {
	h.mu.Lock()
	cb := h.onInvalidate
	h.mu.Unlock()
	if cb != nil {
		cb()
	}
}
