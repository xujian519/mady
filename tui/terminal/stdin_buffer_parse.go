package terminal

import (
	"fmt"

	"github.com/xujian519/mady/tui/core"
)

// ---------------------------------------------------------------------------
// Event splitting
// ---------------------------------------------------------------------------

// consumeKeyEvents walks the buffer and returns (bytesConsumed, events),
// where each event is one complete logical key chunk (including ANSI escape
// sequences parsed atomically). Trailing incomplete escape is preserved.
func consumeKeyEvents(buf []byte) (int64, []string) {
	var events []string
	i := 0
	for i < len(buf) {
		if buf[i] == 0x1B {
			adv := core.SkipAnsiSeq(string(buf), i)
			if adv <= 0 {
				// no more bytes — incomplete escape, stop here
				return int64(i), events
			}
			// Guard against the "incomplete" case where skipAnsiSeq returned
			// the remaining length (no final byte seen yet).
			if !ansiSeqComplete(buf, i, adv) {
				return int64(i), events
			}
			events = append(events, string(buf[i:i+adv]))
			i += adv
			continue
		}
		// A single UTF-8 rune is one event.
		size := runeByteSize(buf[i:])
		if size == 0 || i+size > len(buf) {
			return int64(i), events
		}
		events = append(events, string(buf[i:i+size]))
		i += size
	}
	return int64(i), events
}

// splitInputIntoEvents is like consumeKeyEvents but takes a string and
// discards any incomplete trailing escape (used for pre-paste flushes).
func splitInputIntoEvents(s string) []string {
	_, ev := consumeKeyEvents([]byte(s))
	return ev
}

// ansiSeqComplete returns true if buf[i:i+adv] is a syntactically complete
// ANSI escape sequence (per the skipAnsiSeq rules).
func ansiSeqComplete(buf []byte, i int, adv int) bool {
	if adv <= 1 {
		return true
	}
	if i+adv > len(buf) {
		return false
	}
	b := buf[i+1]
	switch b {
	case '[':
		last := buf[i+adv-1]
		return last >= 0x40 && last <= 0x7E
	case ']', '_', 'P', '^':
		// Terminated by BEL or ESC '\\'
		tail := buf[i+adv-1]
		if tail == 0x07 {
			return true
		}
		if adv >= 2 && buf[i+adv-2] == 0x1B && tail == '\\' {
			return true
		}
		return false
	case 'N', 'O':
		return adv >= 3
	default:
		return adv >= 2
	}
}

// runeByteSize returns the byte length of the UTF-8 rune at the start of b,
// or 0 if b is empty.
func runeByteSize(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	c := b[0]
	switch {
	case c < 0x80:
		return 1
	case c&0xE0 == 0xC0:
		return 2
	case c&0xF0 == 0xE0:
		return 3
	case c&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
}

// indexBytes returns the index of needle in haystack, or -1.
func indexBytes(haystack, needle []byte) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// indexByteFrom returns the index of the first byte equal to a or b in
// haystack[from:], or -1.
func indexByteFrom(haystack []byte, from int, a, b byte) int {
	for i := from; i < len(haystack); i++ {
		if haystack[i] == a || haystack[i] == b {
			return i
		}
	}
	return -1
}

// parseX11Mouse decodes an X11-style mouse event from the Cb byte and
// (cx, cy) coordinates (1-based).
func parseX11Mouse(cb int, cx, cy int64) core.MouseMsg {
	m := core.MouseMsg{Col: cx - 1, Row: cy - 1}
	m.Shift = cb&0x04 != 0
	m.Alt = cb&0x08 != 0
	m.Ctrl = cb&0x10 != 0

	button := cb & 0x03
	switch {
	case cb&0x40 != 0: // wheel motion (bit 6)
		switch button {
		case 0:
			m.Action = core.MouseWheelUp
		case 1:
			m.Action = core.MouseWheelDown
		case 2:
			m.Action = core.MouseWheelLeft
		case 3:
			m.Action = core.MouseWheelRight
		}
	case cb&0x80 != 0: // extended buttons (bit 7): buttons 8-11
		m.Action = core.MousePress
		switch button {
		case 0:
			m.Action = core.MouseBackButton
			m.Button = 8
		case 1:
			m.Action = core.MouseForwardButton
			m.Button = 9
		}
	case cb&0x20 != 0: // motion (bit 5)
		m.Action = core.MouseMotion
		m.Button = int64(button)
	default:
		switch button {
		case 0, 1, 2:
			m.Action = core.MousePress
			m.Button = int64(button)
		case 3:
			m.Action = core.MouseRelease
		}
	}
	return m
}

// parseSGRMouse decodes an SGR-style mouse event from the parameter string
// (the part between "<" and "M"/m", including the terminator).
func parseSGRMouse(seq string) (core.MouseMsg, bool) {
	var cb, cx, cy int
	var release bool

	n, err := fmt.Sscanf(seq, "%d;%d;%d", &cb, &cx, &cy)
	if err != nil || n != 3 {
		return core.MouseMsg{}, false
	}

	// Check terminator: 'm' = release, 'M' = press
	if len(seq) > 0 {
		release = seq[len(seq)-1] == 'm'
	}

	m := core.MouseMsg{Col: int64(cx) - 1, Row: int64(cy) - 1}
	m.Shift = cb&0x04 != 0
	m.Alt = cb&0x08 != 0
	m.Ctrl = cb&0x10 != 0

	button := cb & 0x03
	switch {
	case cb&0x80 != 0: // extended buttons (bit 7): buttons 8-11
		m.Action = core.MousePress
		switch button {
		case 0:
			m.Action = core.MouseBackButton
			m.Button = 8
		case 1:
			m.Action = core.MouseForwardButton
			m.Button = 9
		}
	case cb&0x40 != 0: // wheel motion (bit 6)
		switch button {
		case 0:
			m.Action = core.MouseWheelUp
		case 1:
			m.Action = core.MouseWheelDown
		case 2:
			m.Action = core.MouseWheelLeft
		case 3:
			m.Action = core.MouseWheelRight
		}
	case cb&0x20 != 0: // motion (bit 5)
		m.Action = core.MouseMotion
		m.Button = int64(button)
	default:
		if release {
			m.Action = core.MouseRelease
		} else {
			m.Action = core.MousePress
			m.Button = int64(button)
		}
	}
	return m, true
}
