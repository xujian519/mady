package chat

// This file defines the ChatHistory component core: its state fields, the
// constructor, theme/callback setters, and the per-message render cache
// framework (msgCache + invalidate paths).
//
// Data structures live in chat_history_types.go.
// Message mutations (Append/Delta/Finalize/Clear) live in chat_history_messages.go.
// Search lives in chat_search.go.
// Rendering lives in chat_history_render.go and chat_render_tools.go.
// Input handling (mouse, scroll, click-to-toggle) lives in chat_history_input.go.

import (
	"sync"
	"time"

	core "github.com/xujian519/mady/tui/core"
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
	cachedLinks     [][]core.LinkSpan // 与 cachedAll 行对齐的链接元数据
	dirty           bool
	expandedGroups  map[int]bool // group message indices that are expanded

	// lastLinks 是最近一次 Render 返回值（可见行）对应的链接元数据，供
	// RenderLinks（core.LinkProvider）返回。与 Render 同线程读写，无锁。
	lastLinks [][]core.LinkSpan

	// msgCache maps message ID to its rendered lines at the current width.
	// It is cleared on width changes and on any global style change; single
	// messages are invalidated on PatchMessage/AppendDelta.
	// To prevent unbounded memory growth in long sessions, the cache is
	// capped at maxCacheEntries; excess entries for the oldest non-pending
	// messages are evicted after each render cycle.
	msgCache        map[string]cachedMessage
	maxCacheEntries int

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

	// renderTimer debounces RequestRender calls during streaming. Instead of
	// rendering on every token (which can arrive at 30-50ms intervals and each
	// trigger a full frame diff), we coalesce renders into 30ms windows.
	// Guarded by mu.
	renderTimer *time.Timer

	// scrollbar configuration for the visual scrollbar on the right edge.
	sbEnabled bool
	sbWidth   int64

	// mouseConsumed is set by handleMouse to indicate the last MouseMsg was
	// handled (scroll, toggle, selection drag). The layout container checks
	// MouseConsumed() to decide whether to stop forwarding the event to other
	// children. Reset to false at the start of every handleMouse call.
	mouseConsumed bool

	// firstDirtyIdx tracks the lowest message index that changed since the
	// last renderAll. When > 0, all messages before this index are guaranteed
	// unchanged (same text, same collapsed state). The incremental render fast
	// path splices new lines starting from this index instead of rebuilding
	// the entire cachedAll. Reset to 0 on width changes, Clear, SetTheme.
	firstDirtyIdx int

	// pendingCount tracks the number of messages with Pending=true. Used to
	// decide in O(1) whether to debounce renders (streaming) or fire
	// immediately (idle), replacing the previous O(N) scan in invalidate().
	pendingCount int

	// search state for / search mode
	searchActive bool   // true while search mode is active
	searchQuery  string // current search term
	searchMatch  []int  // indices of matching messages in messages[]
	searchIdx    int    // index into searchMatch (current selection)
	searchEsc    bool   // true: next Esc exits search; false: also searches literal '/'
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
		maxCacheEntries:   200,
		reasoningRenderer: HiddenReasoningRenderer{},
		sbEnabled:         true, // 默认启用滚动条，用户可 Alt+B 关闭
		sbWidth:           1,
	}
}

// SetTheme overrides the styling theme.
func (h *ChatHistory) SetTheme(t ChatHistoryTheme) {
	h.mu.Lock()
	h.theme = t
	h.dirty = true
	h.firstDirtyIdx = 0
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
	h.firstDirtyIdx = 0
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

// ToggleFoldAtViewportCenter toggles the fold state (tool group / thinking
// segment) at the viewport's center line. Returns true if a fold was toggled.
// This enables keyboard-based fold toggling via Space/Enter.
func (h *ChatHistory) ToggleFoldAtViewportCenter() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.maxRows <= 0 || len(h.cachedAll) == 0 {
		return false
	}
	centerLine := h.offset + h.maxRows/2
	// Clamp to content boundaries
	total := int64(len(h.cachedAll))
	if centerLine >= total {
		centerLine = total - 1
	}
	return h.tryToggleThinkingAtLineLocked(centerLine)
}

// SetScrollbarEnabled enables or disables the visual scrollbar on the right edge.
func (h *ChatHistory) SetScrollbarEnabled(enabled bool) {
	h.mu.Lock()
	h.sbEnabled = enabled
	if enabled && h.sbWidth == 0 {
		h.sbWidth = 1 // default: 1 column
	}
	h.mu.Unlock()
	h.invalidate()
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

// MaxRows returns the current viewport height.
func (h *ChatHistory) MaxRows() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maxRows
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
		h.mu.Unlock()
		return
	}
	// Check that we have at least 2
	if len(h.messages)-runStart < 2 {
		h.mu.Unlock()
		return
	}
	// Ensure expandedGroups map exists and mark this group as collapsed
	// by removing the key (default is false/collapsed).
	if h.expandedGroups == nil {
		h.expandedGroups = make(map[int]bool)
	}
	delete(h.expandedGroups, runStart)
	h.dirty = true
	h.firstDirtyIdx = 0
	h.clearMsgCacheLocked()
	// At turn-end there are no pending messages, so invalidate immediately
	// without the debounce overhead.
	cb := h.onInvalidate
	h.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// trackDirtyIdx records that the message at idx has changed, narrowing the
// incremental render range. Caller must hold mu.
//
// Semantics:
//   - firstDirtyIdx == 0 → full rebuild (default / message 0 changed)
//   - firstDirtyIdx > 0  → messages [0, firstDirtyIdx) are clean; splice from here
func (h *ChatHistory) trackDirtyIdx(idx int) {
	if idx == 0 {
		h.firstDirtyIdx = 0 // message 0 changed, need full rebuild
		return
	}
	if h.firstDirtyIdx == 0 {
		h.firstDirtyIdx = idx // first tracked change, narrow from "all"
	} else if idx < h.firstDirtyIdx {
		h.firstDirtyIdx = idx // earlier message changed, widen range
	}
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

// evictCacheEntriesLocked removes cached lines for the oldest non-pending
// messages when the cache exceeds maxCacheEntries. Pending messages (still
// streaming, holding a blockCache) are exempt from eviction. Called during
// the Render merge phase under h.mu.
func (h *ChatHistory) evictCacheEntriesLocked() {
	if h.maxCacheEntries <= 0 || len(h.msgCache) <= h.maxCacheEntries {
		return
	}
	excess := len(h.msgCache) - h.maxCacheEntries
	// Messages are stored in insertion order; iterating forward visits
	// oldest first. Delete cached entries until we're back under the cap.
	for i := range h.messages {
		if excess <= 0 {
			break
		}
		m := &h.messages[i]
		if m.Pending {
			continue
		}
		if _, ok := h.msgCache[m.ID]; ok {
			delete(h.msgCache, m.ID)
			excess--
		}
	}
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
	if cb == nil {
		h.mu.Unlock()
		return
	}
	// During streaming (any message is Pending), debounce renders to coalesce
	// rapid deltas into 30ms windows. Without debouncing, each of ~30
	// tokens/sec triggers a full renderFrame → component tree Render →
	// ParseLine on every line → DiffFrame → serialize → term.Write.
	// Coalescing cuts render frequency by 50-70% with no visible lag.
	// Use pendingCount for O(1) check instead of scanning h.messages.
	if h.pendingCount == 0 {
		h.mu.Unlock()
		cb()
		return
	}
	// Stop any pending timer and schedule a new one.
	if h.renderTimer != nil {
		h.renderTimer.Stop()
	}
	h.renderTimer = time.AfterFunc(30*time.Millisecond, func() {
		cb()
	})
	h.mu.Unlock()
}

// MouseConsumed implements core.MouseConsumer. It reports whether the most
// recent MouseMsg was handled by handleMouse (scroll, toggle, selection).
// The layout container uses this to stop forwarding the event to siblings.
func (h *ChatHistory) MouseConsumed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mouseConsumed
}
