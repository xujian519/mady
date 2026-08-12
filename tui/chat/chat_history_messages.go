package chat

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

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
	h.trackDirtyIdx(len(h.messages) - 1)
	if m.Pending {
		h.pendingCount++
	}
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
			h.trackDirtyIdx(i)
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

	if id != "" {
		for i := range h.messages {
			if h.messages[i].ID == id {
				if !h.applyDeltaLocked(&h.messages[i], delta, kind) {
					h.mu.Unlock()
					return id
				}
				if !h.messages[i].Pending {
					h.pendingCount++
				}
				h.messages[i].Pending = true
				h.invalidateMessageLocked(id)
				h.dirty = true
				h.trackDirtyIdx(i)
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
		ID:            newID,
		Role:          RoleAssistant,
		Pending:       true,
		At:            time.Now(),
		lastDelta:     delta,
		lastDeltaKind: kind,
	}
	if kind == "thinking" {
		msg.ThinkingSegments = []ThinkingSegment{{Text: delta}}
	} else {
		msg.Text = delta
	}
	h.messages = append(h.messages, msg)
	h.dirty = true
	h.trackDirtyIdx(len(h.messages) - 1)
	h.pendingCount++
	h.selActive = false
	h.selDragging = false
	if h.follow {
		h.offset = 0
	}
	h.mu.Unlock()
	h.invalidate()
	return newID
}

// applyDeltaLocked merges `delta` into the streaming message `m` while
// suppressing the two provider-level duplication patterns that are safe to
// detect locally:
//   - an immediate re-send of the exact same delta (reconnect replay)
//   - cumulative chunks where delta starts with the current text
//
// Everything else is appended verbatim. Non-consecutive repeats are real
// output (long answers legitimately repeat "```", "**", "---", table
// separators), and look-behind overlap stripping is deliberately NOT
// attempted: a cross-chunk 叠字 ("让我想想" + "想办法") is genuine text that
// must not be silently truncated. A transient double from a provider replay
// is reconciled at stream end by FinalizeWithOutput, which corrects the
// message against the full output carried by AgentEndEvent.
//
// It returns true if the delta was applied and false if it was suppressed.
// Caller must hold h.mu.
func (h *ChatHistory) applyDeltaLocked(m *ChatMessage, delta, kind string) bool {
	// Consecutive exact duplicate: providers replay whole chunks, never
	// isolated characters, so only an immediate multi-rune repeat of the
	// previous delta (same kind, same content) counts as a replay. Rune
	// streams legitimately repeat single characters ("---|---", "...") and
	// are exempt.
	if delta == m.lastDelta && kind == m.lastDeltaKind && utf8.RuneCountInString(delta) > 1 {
		return false
	}

	var target *string
	if kind == "thinking" {
		// Start a new segment when thinking resumes after a text delta (or on
		// first contact), so distinct thinking phases stay separate blocks.
		if len(m.ThinkingSegments) == 0 || m.lastDeltaKind != "thinking" {
			m.ThinkingSegments = append(m.ThinkingSegments, ThinkingSegment{})
		}
		target = &m.ThinkingSegments[len(m.ThinkingSegments)-1].Text
	} else {
		target = &m.Text
	}

	current := *target
	if current != "" && strings.HasPrefix(delta, current) {
		// Cumulative provider chunks: the provider sent the full text so far
		// instead of an incremental delta. Replace rather than append.
		*target = delta
		m.lastDelta = delta
		m.lastDeltaKind = kind
		return true
	}

	*target += delta
	m.lastDelta = delta
	m.lastDeltaKind = kind
	return true
}

// Finalize clears the Pending flag on the given id and releases the
// per-block render cache so the blockCache can be GC'd promptly.
func (h *ChatHistory) Finalize(id string) {
	h.FinalizeWithOutput(id, "")
}

// FinalizeWithOutput behaves like Finalize and additionally reconciles the
// visible Text with the agent's final output: when output is non-empty and
// differs from the accumulated text (e.g. a delta was lost or duplicated
// upstream), the full output wins. Thinking segments are left untouched —
// output carries the pure body text only.
func (h *ChatHistory) FinalizeWithOutput(id, output string) {
	h.PatchMessage(id, func(m *ChatMessage) {
		if m.Pending {
			h.pendingCount--
		}
		m.Pending = false
		if output != "" && m.Text != output {
			m.Text = output
		}
	})
}

// Clear empties the transcript.
func (h *ChatHistory) Clear() {
	h.mu.Lock()
	h.messages = nil
	h.offset = 0
	h.dirty = true
	h.firstDirtyIdx = 0
	h.pendingCount = 0
	h.cachedMsgRanges = nil
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
