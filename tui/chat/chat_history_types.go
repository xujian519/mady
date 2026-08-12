package chat

import (
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

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

	// Seq is a sequence number for ordered display (e.g. tool call order).
	// 0 means no number displayed.
	Seq int

	// DomainMsg 承载结构化专业产出（证据/结论/审批）。
	// 非空时 renderMessage 路由到对应的卡片组件渲染。
	DomainMsg *component.DomainMessage

	// Thinking blocks (structured content).
	ThinkingSegments []ThinkingSegment

	// Internal: the raw delta string most recently applied to this message.
	// Used to suppress an immediate re-send of the exact same chunk (e.g. a
	// reconnect replay) while keeping legitimately repeated output elsewhere
	// in the stream.
	lastDelta string

	// Internal: Kind of the last applied delta ("text" / "thinking").
	// Used to start a new ThinkingSegment when thinking resumes after text,
	// so distinct thinking phases stay separate blocks.
	lastDeltaKind string
}

// ThinkingSegment holds a chunk of thinking text.
type ThinkingSegment struct {
	Text      string
	Collapsed bool
}

// IsCollapsible 报告该消息是否支持折叠/展开交互（点击切换摘要/全文）。
// 覆盖：证据卡（DomainMsg evidence_card）、工具结果、可折叠摘要文本
// （diff 或已处于折叠态的 assistant 消息）。
//
// 这是折叠判定的唯一入口：渲染层读 Collapsed 字段渲染折叠态，交互层
// （chat_history_input.go 的 click-to-toggle）用它判定"点击是否切换折叠"。
func (m *ChatMessage) IsCollapsible() bool {
	if m.DomainMsg != nil {
		return m.DomainMsg.Type == component.DomainMsgTypeEvidenceCard
	}
	switch m.Role {
	case RoleTool:
		return true
	case RoleAssistant:
		return m.Collapsed || m.Meta == "diff"
	}
	return false
}

// ChatHistoryTheme customizes prefix / styling for each role.
type ChatHistoryTheme struct {
	UserPrefix       string
	UserStyle        theme.Style
	UserBgStyle      theme.Style
	AssistantPrefix  string
	AssistantStyle   theme.Style
	AssistantBgStyle theme.Style
	SystemPrefix     string
	SystemStyle      theme.Style
	ToolPrefix       string
	ToolStyle        theme.Style
	ToolBorder       theme.Style
	SuccessStyle     theme.Style
	ErrorPrefix      string
	ErrorStyle       theme.Style
	DividerChar      string
	DimStyle         theme.Style
	ThinkingStyle    theme.Style
	SelectedBg       string // ANSI background for selection
	MarkdownTheme    component.MarkdownTheme
}

// DefaultChatHistoryTheme returns a theme built from the current palette.
func DefaultChatHistoryTheme() ChatHistoryTheme {
	pal := theme.CurrentPalette()
	return ChatHistoryTheme{
		UserPrefix:       "> ",
		UserStyle:        pal.User,
		UserBgStyle:      pal.UserBg,
		AssistantPrefix:  "",
		AssistantStyle:   pal.Assistant,
		AssistantBgStyle: pal.AssistantBg,
		SystemPrefix:     "",
		SystemStyle:      pal.System,
		ToolPrefix:       theme.SymbolArrow + " ",
		ToolStyle:        pal.Dim,
		ToolBorder:       pal.BorderMuted,
		SuccessStyle:     pal.Success,
		ErrorPrefix:      theme.SymbolCross + " ",
		ErrorStyle:       pal.Error,
		DividerChar:      "─",
		DimStyle:         pal.Dim,
		ThinkingStyle:    pal.Thinking,
		SelectedBg:       pal.SelectionBg.BgStrip(),
		MarkdownTheme:    component.DefaultMarkdownTheme(),
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
	links      [][]core.LinkSpan     // 与 lines 行对齐的链接元数据（受信任来源）
	width      int64                 // 渲染行时的宽度；命中时不一致需重渲染（如工具组 innerW）
	blockCache *component.BlockCache // non-nil only for Pending assistant messages
}
