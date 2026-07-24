package core

import "github.com/xujian519/mady/tui/internal/csync"

// ---------------------------------------------------------------------------
// Component interface
//
// A Component renders itself into a slice of lines, one per terminal row.
// Each returned line MUST NOT exceed `width` visible columns; callers
// (the TUI container) will otherwise error or clip.
//
// handleInput is called only when the component currently owns focus and
// receives raw terminal input (possibly containing ANSI escapes). Use
// ParseKeys / MatchesKey / KeybindingsManager to interpret it.
//
// Invalidate is called by the container whenever cached rendering state
// must be discarded (e.g. theme changed, width changed).
// ---------------------------------------------------------------------------

// Component is the core interface every renderable element implements.
type Component interface {
	Render(width int64) []string
	Invalidate()
}

// Sizer is an optional interface that lets a component declare its natural
// height without producing a full render. Layout containers use it to avoid
// double-rendering components during measurement.
type Sizer interface {
	// Measure returns the natural height of the component at the given width.
	Measure(width int64) int64
}

// Updatable is the interface that components implement to participate in
// the message-driven update cycle. All user interaction (keys, mouse,
// paste, resize) and application events are delivered as Msg values
// through Update, following the Elm Architecture pattern.
//
// Update mutates component state and optionally returns a Cmd for
// asynchronous side effects.
//
// IMPORTANT: Update MUST NOT perform blocking I/O (network, file read,
// blocking channel operations). It runs on the TUI event-loop goroutine;
// blocking it freezes the entire UI for the duration. Defer all I/O to
// the returned Cmd, which executes asynchronously on a separate goroutine.
// See Batch and Sequence for composing multiple Cmds.
//
// Update SHOULD complete in well under 1ms. When processing a large
// ChatHistory mutation, prefer batch operations (AppendDelta over many
// per-rune Append calls) to keep the loop responsive.
type Updatable interface {
	Update(msg Msg) Cmd
}

// Focusable marks a component that can hold a visible hardware cursor
// (needed for IME candidate-window positioning with CJK input methods).
//
// A focusable component should emit CursorMarker at the cursor cell when
// SetFocused(true) has been called on it. The TUI container will strip the
// marker from output and position the real hardware cursor at its location.
type Focusable interface {
	SetFocused(focused bool)
	IsFocused() bool
}

// CursorMarker is a zero-width APC (Application Program Command) escape
// sequence terminals ignore but the TUI recognizes. Place it in rendered
// output at the desired cursor column.
const CursorMarker = "\x1b_pi:c\x07"

// WantsKeyRelease is an optional marker interface. Components that want
// Kitty key-release events must implement it and return true; otherwise
// the container filters release events out.
type WantsKeyRelease interface {
	WantsKeyRelease() bool
}

// ---------------------------------------------------------------------------
// Container — the canonical composite Component.
// ---------------------------------------------------------------------------

// Container renders a vertical stack of child components.
type Container struct {
	children csync.Slice[Component]
}

// NewContainer returns an empty container.
func NewContainer() *Container { return &Container{} }

// AddChild appends a component to the end.
func (c *Container) AddChild(child Component) {
	if child == nil {
		return
	}
	c.children.Append(child)
}

// RemoveChild removes the first occurrence of child. Returns true if removed.
func (c *Container) RemoveChild(child Component) bool {
	children := c.children.Copy()
	for i, ch := range children {
		if ch == child {
			children = append(children[:i], children[i+1:]...)
			c.children.SetSlice(children)
			return true
		}
	}
	return false
}

// Clear removes all children.
func (c *Container) Clear() {
	c.children.SetSlice(nil)
}

// Children returns a snapshot slice of the current children.
func (c *Container) Children() []Component {
	return c.children.Copy()
}

// Render concatenates child renders vertically.
func (c *Container) Render(width int64) []string {
	children := c.children.Copy()

	var lines []string
	for _, ch := range children {
		if ch == nil {
			continue
		}
		lines = append(lines, ch.Render(width)...)
	}
	return lines
}

// Invalidate fans out to all children.
func (c *Container) Invalidate() {
	for _, ch := range c.children.Copy() {
		ch.Invalidate()
	}
}

// Suggestion is one completion candidate.
type Suggestion struct {
	// Label is the visible label in the list.
	Label string
	// InsertText replaces the active token (required).
	InsertText string
	// Description is an optional dim suffix shown next to the label.
	Description string
	// GroupLabel, when non-empty, groups this suggestion under a category header
	// in the autocomplete list (e.g. "mode", "session", "case").
	GroupLabel string
	// Tag is an opaque value callers can inspect on apply.
	Tag any
}

// AutocompleteProvider supplies suggestions for a trigger.
type AutocompleteProvider interface {
	// Trigger returns the prefix characters that activate this provider
	// (e.g. "/", "@"). An empty trigger means "always consider".
	Trigger() string
	// Complete returns suggestions for the token. `token` excludes the
	// trigger prefix.
	Complete(token string) []Suggestion
}

// FullInputProvider is an optional extension to AutocompleteProvider that
// provides the full input buffer for context-aware completion (e.g. sub-command
// arguments). Providers implementing this interface are called with the full
// input instead of just the token.
type FullInputProvider interface {
	AutocompleteProvider
	CompleteWithFull(token, fullValue string, cursorPos int64) []Suggestion
}

// ---------------------------------------------------------------------------
// Small helpers for component authors.
// ---------------------------------------------------------------------------

// EnsureWidth returns lines truncated so none exceeds width cells. It does
// NOT pad to exactly width; callers may pad manually for background fills.
func EnsureWidth(lines []string, width int64) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if VisibleWidth(l) > width {
			out[i] = TruncateToWidth(l, width, "…")
		} else {
			out[i] = l
		}
	}
	return out
}

// FillLines returns `count` empty lines of the given width (all spaces).
func FillLines(count, width int64) []string {
	if count <= 0 {
		return nil
	}
	line := ""
	if width > 0 {
		line = PadToWidth("", width)
	}
	out := make([]string, count)
	for i := range out {
		out[i] = line
	}
	return out
}
