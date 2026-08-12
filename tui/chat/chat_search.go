package chat

import (
	"strings"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// SearchMode returns whether the chat history is currently in search mode.
func (h *ChatHistory) SearchMode() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.searchActive
}

// SearchQuery returns the current search term (empty when not searching).
func (h *ChatHistory) SearchQuery() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.searchQuery
}

// SearchMatchCount returns the number of matching messages.
func (h *ChatHistory) SearchMatchCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.searchMatch)
}

// SearchCurrent returns the 1-based index of the current match (0 if none).
func (h *ChatHistory) SearchCurrent() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.searchMatch) == 0 {
		return 0
	}
	return h.searchIdx + 1
}

// SearchActivate enters search mode. The caller should then feed characters
// via SearchAppend or set a query directly via SearchQuery.
func (h *ChatHistory) SearchActivate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.searchActive = true
	h.searchQuery = ""
	h.searchMatch = nil
	h.searchIdx = -1
	h.searchEsc = true
}

// SearchDeactivate exits search mode and clears all search state.
func (h *ChatHistory) SearchDeactivate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.searchActive = false
	h.searchQuery = ""
	h.searchMatch = nil
	h.searchIdx = -1
	h.searchEsc = false
	h.dirty = true
}

// SearchAppend adds a character to the search query and rebuilds the match
// list. Returns the new match count.
func (h *ChatHistory) SearchAppend(ch rune) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.searchActive {
		return 0
	}
	h.searchQuery += string(ch)
	h.rebuildSearchMatchesLocked()
	return len(h.searchMatch)
}

// SearchBackspace removes the last character from the search query.
func (h *ChatHistory) SearchBackspace() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.searchActive || len(h.searchQuery) == 0 {
		return
	}
	// Trim one rune, not one byte — byte slicing would corrupt UTF-8 input.
	_, size := utf8.DecodeLastRuneInString(h.searchQuery)
	h.searchQuery = h.searchQuery[:len(h.searchQuery)-size]
	h.rebuildSearchMatchesLocked()
}

// SearchNext moves to the next match. Returns false if there is no match.
func (h *ChatHistory) SearchNext() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.searchMatch) == 0 {
		return false
	}
	h.searchIdx = (h.searchIdx + 1) % len(h.searchMatch)
	h.scrollToSearchMatchLocked()
	return true
}

// SearchPrev moves to the previous match. Returns false if there is no match.
func (h *ChatHistory) SearchPrev() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.searchMatch) == 0 {
		return false
	}
	h.searchIdx = (h.searchIdx - 1 + len(h.searchMatch)) % len(h.searchMatch)
	h.scrollToSearchMatchLocked()
	return true
}

// rebuildSearchMatchesLocked rebuilds the list of message indices whose text
// contains the search query (case-insensitive substring match). Must be
// called with h.mu held.
func (h *ChatHistory) rebuildSearchMatchesLocked() {
	h.searchMatch = h.searchMatch[:0]
	if h.searchQuery == "" {
		return
	}
	q := strings.ToLower(h.searchQuery)
	for i := range h.messages {
		if strings.Contains(strings.ToLower(h.messages[i].Text), q) {
			h.searchMatch = append(h.searchMatch, i)
		}
	}
	if len(h.searchMatch) > 0 {
		h.searchIdx = 0
	} else {
		h.searchIdx = -1
	}
}

// scrollToSearchMatchLocked scrolls the viewport so the current search match
// is visible. Must be called with h.mu held.
func (h *ChatHistory) scrollToSearchMatchLocked() {
	if h.searchIdx < 0 || h.searchIdx >= len(h.searchMatch) {
		return
	}
	// We approximate by setting follow=false and adjusting offset so the
	// matched message appears near the top. The exact rendering depends on
	// message line count, which is hard to compute without a full render,
	// so we conservatively set offset to place the match near the top.
	h.follow = false
	// Reset dirty to trigger a full re-render with the new offset.
	h.dirty = true
}

// IsSearchMatch reports whether the message at index i is a search hit.
func (h *ChatHistory) IsSearchMatch(i int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.searchActive {
		return false
	}
	for _, m := range h.searchMatch {
		if m == i {
			return true
		}
	}
	return false
}

// IsCurrentSearchHit reports whether the message at index i is the currently
// selected search match.
func (h *ChatHistory) IsCurrentSearchHit(i int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.searchActive || h.searchIdx < 0 || h.searchIdx >= len(h.searchMatch) {
		return false
	}
	return h.searchMatch[h.searchIdx] == i
}
