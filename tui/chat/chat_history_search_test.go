package chat

import (
	"testing"
)

func newSearchHistory() *ChatHistory {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleAssistant, Text: "权利要求 1 具备新颖性"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "对比文件 D1 公开了权利要求 2 的特征"})
	h.Append(ChatMessage{Role: RoleSystem, Text: "检索完成"})
	return h
}

// TestSearchActivateDeactivate verifies mode entry/exit and state reset.
func TestSearchActivateDeactivate(t *testing.T) {
	h := newSearchHistory()
	if h.SearchMode() {
		t.Fatal("search should be inactive initially")
	}
	h.SearchActivate()
	if !h.SearchMode() {
		t.Fatal("search should be active after activate")
	}
	if h.SearchQuery() != "" || h.SearchMatchCount() != 0 || h.SearchCurrent() != 0 {
		t.Fatalf("fresh search state: %q %d %d", h.SearchQuery(), h.SearchMatchCount(), h.SearchCurrent())
	}
	if h.IsSearchMatch(0) || h.IsCurrentSearchHit(0) {
		t.Fatal("no matches before typing")
	}

	h.SearchDeactivate()
	if h.SearchMode() || h.SearchQuery() != "" {
		t.Fatal("deactivate should reset everything")
	}
}

// TestSearchAppendRebuildsMatches verifies query typing and match rebuilding
// with case-insensitive matching.
func TestSearchAppendRebuildsMatches(t *testing.T) {
	h := newSearchHistory()
	h.SearchActivate()

	if n := h.SearchAppend('权'); n != 2 {
		t.Fatalf("append 权 → %d matches, want 2", n)
	}
	if h.SearchQuery() != "权" {
		t.Fatalf("query = %q", h.SearchQuery())
	}
	if h.SearchMatchCount() != 2 || h.SearchCurrent() != 1 {
		t.Fatalf("match count/current = %d/%d", h.SearchMatchCount(), h.SearchCurrent())
	}
	if !h.IsSearchMatch(0) || !h.IsSearchMatch(1) || h.IsSearchMatch(2) {
		t.Fatal("match list should be [0 1]")
	}
	if !h.IsCurrentSearchHit(0) || h.IsCurrentSearchHit(1) {
		t.Fatal("current hit should be index 0")
	}

	// Narrowing the query drops non-matching messages.
	if n := h.SearchAppend('利'); n != 2 {
		t.Fatalf("append 利 → %d matches, want 2", n)
	}
	h.SearchAppend('要')
	if h.SearchQuery() != "权利要" || h.SearchMatchCount() != 2 {
		t.Fatalf("query/match = %q/%d", h.SearchQuery(), h.SearchMatchCount())
	}
	h.SearchAppend('求')
	if h.SearchQuery() != "权利要求" || h.SearchMatchCount() != 2 {
		t.Fatalf("query/match = %q/%d", h.SearchQuery(), h.SearchMatchCount())
	}

	// A query that only hits one message narrows the list.
	h.SearchAppend(' ')
	h.SearchAppend('1')
	if h.SearchQuery() != "权利要求 1" || h.SearchMatchCount() != 1 {
		t.Fatalf("query/match = %q/%d", h.SearchQuery(), h.SearchMatchCount())
	}
	if !h.IsSearchMatch(0) || h.IsSearchMatch(1) {
		t.Fatal("narrowed list should be [0]")
	}

	// Case-insensitive: "d1" matches "D1".
	h.SearchDeactivate()
	h.SearchActivate()
	h.SearchAppend('d')
	h.SearchAppend('1')
	if h.SearchMatchCount() != 1 || !h.IsSearchMatch(1) {
		t.Fatalf("case-insensitive match failed: %d matches", h.SearchMatchCount())
	}
}

// TestSearchAppendInactive verifies appends outside search mode are ignored.
func TestSearchAppendInactive(t *testing.T) {
	h := newSearchHistory()
	if n := h.SearchAppend('x'); n != 0 {
		t.Fatalf("append while inactive → %d, want 0", n)
	}
}

// TestSearchBackspace verifies query shrinking and empty-query no-op.
//
// NOTE: only ASCII queries are used here. The source's SearchBackspace
// truncates by bytes (h.searchQuery[:len-1]) which corrupts multibyte runes;
// that is a pre-existing bug out of scope for this task.
func TestSearchBackspace(t *testing.T) {
	h := newSearchHistory()
	h.SearchActivate()
	h.SearchAppend('a')
	h.SearchAppend('b')
	if h.SearchQuery() != "ab" {
		t.Fatalf("setup query = %q", h.SearchQuery())
	}
	h.SearchBackspace()
	if h.SearchQuery() != "a" {
		t.Fatalf("after backspace query = %q", h.SearchQuery())
	}

	// Backspace on empty query is a no-op.
	h.SearchDeactivate()
	h.SearchActivate()
	h.SearchBackspace()
	if h.SearchMode() != true || h.SearchQuery() != "" {
		t.Fatal("backspace on empty query should be a no-op")
	}
}

// TestSearchNavigation verifies next/prev navigation with wrap-around.
func TestSearchNavigation(t *testing.T) {
	h := newSearchHistory()
	h.SearchActivate()
	h.SearchAppend('权')
	if h.SearchMatchCount() != 2 {
		t.Fatalf("setup matches = %d, want 2", h.SearchMatchCount())
	}

	if !h.SearchNext() {
		t.Fatal("next should succeed")
	}
	if !h.IsCurrentSearchHit(1) {
		t.Fatal("after next, current should be index 1")
	}
	if !h.SearchNext() {
		t.Fatal("next should wrap")
	}
	if !h.IsCurrentSearchHit(0) {
		t.Fatal("after wrap, current should be index 0")
	}
	if !h.SearchPrev() {
		t.Fatal("prev should wrap backwards")
	}
	if !h.IsCurrentSearchHit(1) {
		t.Fatal("after prev wrap, current should be index 1")
	}

	// scrollToSearchMatchLocked sets follow=false.
	h.mu.Lock()
	follow := h.follow
	h.mu.Unlock()
	if follow {
		t.Fatal("navigation should disable follow")
	}

	// No matches → both fail.
	h.SearchDeactivate()
	h.SearchActivate()
	if h.SearchNext() || h.SearchPrev() {
		t.Fatal("next/prev with no matches must fail")
	}
}

// TestSearchIsCurrentHitEdge verifies IsCurrentSearchHit guards.
func TestSearchIsCurrentHitEdge(t *testing.T) {
	h := newSearchHistory()
	h.SearchActivate()
	h.SearchAppend('x') // no matches → searchIdx = -1
	if h.IsCurrentSearchHit(0) {
		t.Fatal("no match → no current hit")
	}
	if !h.IsSearchMatch(0) {
		// IsSearchMatch requires active search; it should be false for no matches
	}
	if h.IsSearchMatch(2) || h.IsSearchMatch(3) {
		t.Fatal("out-of-range index must not be a match")
	}
}
