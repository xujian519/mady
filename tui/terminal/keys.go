package terminal

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Key identification & parsing.
//
// This layer is deliberately *lossy but structural*. We parse raw bytes into
// a Key{} value which can then be matched against a KeyID string such as
// "enter", "ctrl+c", "shift+tab", "alt+left", "ctrl+shift+p".
//
// Two input modes are supported:
//   1. Traditional xterm escape sequences (default).
//   2. Kitty keyboard protocol (CSI ... u) when the terminal reports support.
//
// ---------------------------------------------------------------------------

// Modifier bitmask compatible with the Kitty keyboard protocol.
type Modifier int64

const (
	ModNone  Modifier = 0
	ModShift Modifier = 1
	ModAlt   Modifier = 2
	ModCtrl  Modifier = 4
	ModSuper Modifier = 8
	ModHyper Modifier = 16
	ModMeta  Modifier = 32
	ModCaps  Modifier = 64
	ModNumLk Modifier = 128
)

// KeyEventType distinguishes press / repeat / release (Kitty only).
type KeyEventType int64

const (
	KeyPress KeyEventType = iota + 1
	KeyRepeat
	KeyRelease
)

// Key describes a single parsed key event.
type Key struct {
	Name  string       // canonical name: "a", "enter", "up", "f1", "tab", "pasteStart", ...
	Rune  rune         // the character for printable keys (or 0)
	Mods  Modifier     // modifier bitmask
	Event KeyEventType // press / repeat / release
	Alt   int64        // alternate key codepoint (Kitty flag 4), 0 = none
	Text  string       // associated text (Kitty flag 16), "" = none
	Raw   string       // the original bytes (for fallback / debugging)
}

// KeyID is a canonical textual identifier, e.g. "ctrl+c", "shift+tab", "enter".
type KeyID = string

// String returns the KeyID for this key.
func (k Key) String() string {
	var parts []string
	if k.Mods&ModCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if k.Mods&ModAlt != 0 {
		parts = append(parts, "alt")
	}
	if k.Mods&ModShift != 0 {
		parts = append(parts, "shift")
	}
	if k.Mods&ModSuper != 0 {
		parts = append(parts, "super")
	}
	if k.Mods&ModMeta != 0 {
		parts = append(parts, "meta")
	}
	name := k.Name
	if name == "" && k.Rune != 0 {
		name = string(k.Rune)
	}
	parts = append(parts, name)
	return strings.Join(parts, "+")
}

// IsPrintable returns true if the key represents a single printable rune
// with no modifiers beyond Shift.
func (k Key) IsPrintable() bool {
	if k.Rune == 0 {
		return false
	}
	onlyShift := k.Mods &^ ModShift
	return onlyShift == 0
}

// IsRelease reports whether this is a key release event (Kitty only).
func (k Key) IsRelease() bool { return k.Event == KeyRelease }

// IsRepeat reports whether this is a key repeat event (Kitty only).
func (k Key) IsRepeat() bool { return k.Event == KeyRepeat }

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// ParseKeys splits an arbitrary input chunk into individual Key events.
// It is safe to call with partial data — trailing incomplete escapes are
// returned as raw keys and should typically be combined with the next chunk
// by the caller (StdinBuffer handles this).
func ParseKeys(data string) []Key {
	var out []Key
	i := 0
	for i < len(data) {
		k, adv := parseOne(data, i)
		if adv <= 0 {
			break
		}
		out = append(out, k)
		i += adv
	}
	return out
}

func parseOne(s string, i int) (Key, int) {
	if i >= len(s) {
		return Key{}, 0
	}
	b := s[i]

	if b != 0x1B {
		return parsePlain(s, i)
	}

	// Starts with ESC.
	if i+1 >= len(s) {
		return Key{Name: "escape", Raw: s[i : i+1]}, 1
	}

	next := s[i+1]
	switch next {
	case '[':
		return parseCSI(s, i)
	case 'O':
		if i+2 < len(s) {
			return parseSS3(s, i)
		}
		return Key{Name: "escape", Raw: s[i : i+1]}, 1
	}

	// ESC <x>  → typically Alt+<x>
	k, adv := parsePlain(s, i+1)
	k.Mods |= ModAlt
	k.Raw = s[i : i+1+adv]
	return k, 1 + adv
}

func parsePlain(s string, i int) (Key, int) {
	b := s[i]

	if b == 0x7F || b == 0x08 {
		return Key{Name: "backspace", Raw: string(b)}, 1
	}
	if b == '\r' || b == '\n' {
		return Key{Name: "enter", Raw: string(b)}, 1
	}
	if b == '\t' {
		return Key{Name: "tab", Raw: string(b)}, 1
	}
	if b == 0x1B {
		return Key{Name: "escape", Raw: string(b)}, 1
	}
	if b < 0x20 {
		letter := rune(b) + 'a' - 1
		if b == 0 {
			letter = ' '
		}
		return Key{
			Name: string(letter),
			Rune: letter,
			Mods: ModCtrl,
			Raw:  string(b),
		}, 1
	}

	r, size := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && size <= 1 {
		return Key{Raw: string(b)}, 1
	}
	k := Key{
		Name:  string(r),
		Rune:  r,
		Event: KeyPress,
		Raw:   s[i : i+size],
	}
	if r >= 'A' && r <= 'Z' {
		k.Mods |= ModShift
	}
	return k, size
}

// parseCSI handles ESC '[' sequences: arrow keys, F1-F12, Home/End/PageUp/Down,
// Insert, Delete, bracketed paste markers, and the Kitty CSI u format.
func parseCSI(s string, i int) (Key, int) {
	// Find the final byte (0x40..0x7E) to determine sequence length.
	j := i + 2
	for j < len(s) {
		b := s[j]
		if b >= 0x40 && b <= 0x7E {
			seq := s[i : j+1]
			final := b
			params := s[i+2 : j]
			return decodeCSI(seq, params, final), j + 1 - i
		}
		j++
	}
	// Incomplete: return what we have so next chunk can finish it.
	return Key{Name: "escape", Raw: s[i:]}, len(s) - i
}

func parseSS3(s string, i int) (Key, int) {
	c := s[i+2]
	raw := s[i : i+3]
	switch c {
	case 'A':
		return Key{Name: "up", Raw: raw}, 3
	case 'B':
		return Key{Name: "down", Raw: raw}, 3
	case 'C':
		return Key{Name: "right", Raw: raw}, 3
	case 'D':
		return Key{Name: "left", Raw: raw}, 3
	case 'H':
		return Key{Name: "home", Raw: raw}, 3
	case 'F':
		return Key{Name: "end", Raw: raw}, 3
	case 'P':
		return Key{Name: "f1", Raw: raw}, 3
	case 'Q':
		return Key{Name: "f2", Raw: raw}, 3
	case 'R':
		return Key{Name: "f3", Raw: raw}, 3
	case 'S':
		return Key{Name: "f4", Raw: raw}, 3
	}
	return Key{Raw: raw}, 3
}

func decodeCSI(seq, params string, final byte) Key {
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		mods := ModNone
		if strings.Contains(params, ";") {
			_, modCode := splitTwo(params, ";")
			mods = decodeCSIMods(modCode)
		}
		name := map[byte]string{
			'A': "up", 'B': "down", 'C': "right", 'D': "left",
			'H': "home", 'F': "end",
		}[final]
		return Key{Name: name, Mods: mods, Raw: seq}

	case '~':
		head, modCode := splitTwo(params, ";")
		switch head {
		case "2":
			return Key{Name: "insert", Mods: decodeCSIMods(modCode), Raw: seq}
		case "3":
			return Key{Name: "delete", Mods: decodeCSIMods(modCode), Raw: seq}
		case "5":
			return Key{Name: "pageUp", Mods: decodeCSIMods(modCode), Raw: seq}
		case "6":
			return Key{Name: "pageDown", Mods: decodeCSIMods(modCode), Raw: seq}
		case "7":
			return Key{Name: "home", Mods: decodeCSIMods(modCode), Raw: seq}
		case "8":
			return Key{Name: "end", Mods: decodeCSIMods(modCode), Raw: seq}
		case "11", "12", "13", "14":
			fn := map[string]string{"11": "f1", "12": "f2", "13": "f3", "14": "f4"}[head]
			return Key{Name: fn, Mods: decodeCSIMods(modCode), Raw: seq}
		case "15":
			return Key{Name: "f5", Mods: decodeCSIMods(modCode), Raw: seq}
		case "17", "18", "19", "20", "21":
			fn := map[string]string{"17": "f6", "18": "f7", "19": "f8", "20": "f9", "21": "f10"}[head]
			return Key{Name: fn, Mods: decodeCSIMods(modCode), Raw: seq}
		case "23", "24":
			fn := map[string]string{"23": "f11", "24": "f12"}[head]
			return Key{Name: fn, Mods: decodeCSIMods(modCode), Raw: seq}
		case "200":
			return Key{Name: "pasteStart", Raw: seq}
		case "201":
			return Key{Name: "pasteEnd", Raw: seq}
		}
		return Key{Raw: seq}

	case 'u':
		return decodeKittyU(seq, params)
	}
	return Key{Raw: seq}
}

// decodeKittyU parses CSI unicode-codepoint ; mods [; event] ; ... u
// The parameter positions depend on which Kitty keyboard flags (1/2/4/8/16)
// were negotiated. Without flag context, the parser cannot tell whether the
// third parameter is an event type, an alternate key, or associated text.
// We use kittyActiveFlags() (set via SetKittyKeyboardFlagsFromTerminal) to
// determine the layout. When flags are unknown (0), we default to the
// simplest layout (code;mods) and treat everything else as a raw sequence.
// Callers should ensure SetKittyKeyboardFlags is called before Start.
func decodeKittyU(seq, params string) Key {
	codeStr, rest := splitTwo(params, ";")
	modStr, rest2 := splitTwo(rest, ";")
	flags := kittyActiveFlags()

	// Consume positional parameters from rest2 according to negotiated flags.
	// Layout: code ; mods [; event] [; alt] [; text] u
	var evtStr, altStr, textStr string

	// When flags are unknown (0), default to the old greedy layout: parse
	// up to 4 semicolon-separated fields (code;mods;event;alt;text). This
	// preserves backward compatibility for callers that don't negotiate flags.
	if flags == 0 {
		flags = 1 | 2 | 4 | 16 // assume everything present
	}

	// Event type (flag 2).
	if flags&2 != 0 {
		if strings.Contains(rest2, ";") {
			evtStr, rest2 = splitTwo(rest2, ";")
		} else {
			evtStr = rest2
			rest2 = ""
		}
	}

	// Alternate key codepoint (flag 4).
	if flags&4 != 0 {
		if strings.Contains(rest2, ";") {
			altStr, rest2 = splitTwo(rest2, ";")
		} else if rest2 != "" {
			altStr = rest2
			rest2 = ""
		}
	}

	// Associated text (flag 16) — the remaining payload.
	if flags&16 != 0 {
		textStr = rest2
		rest2 = ""
	}

	code := parseUint(codeStr)
	mods := decodeCSIMods(modStr)
	evt := KeyPress
	switch parseUint(evtStr) {
	case 2:
		evt = KeyRepeat
	case 3:
		evt = KeyRelease
	}

	k := Key{Mods: mods, Event: evt, Alt: parseUint(altStr), Text: percentDecode(textStr), Raw: seq}
	if code == 0 {
		return k
	}
	switch code {
	case 13:
		k.Name = "enter"
	case 9:
		k.Name = "tab"
	case 27:
		k.Name = "escape"
	case 127:
		k.Name = "backspace"
	case 57358:
		k.Name = "up"
	case 57359:
		k.Name = "down"
	case 57360:
		k.Name = "right"
	case 57361:
		k.Name = "left"
	case 57362:
		k.Name = "home"
	case 57363:
		k.Name = "end"
	case 57364:
		k.Name = "pageUp"
	case 57365:
		k.Name = "pageDown"
	case 57366, 57367, 57368, 57369, 57370, 57371, 57372, 57373, 57374, 57375, 57376, 57377:
		k.Name = fmt.Sprintf("f%d", code-57365)
	default:
		r := rune(code)
		k.Rune = r
		k.Name = string(r)
	}
	return k
}

func decodeCSIMods(s string) Modifier {
	n := parseUint(s)
	if n <= 0 {
		return ModNone
	}
	return Modifier(n - 1)
}

func splitTwo(s, sep string) (string, string) {
	idx := strings.Index(s, sep)
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+len(sep):]
}

func parseUint(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// ---------------------------------------------------------------------------
// Matching helpers
// ---------------------------------------------------------------------------

// MatchesKey reports whether any key event in `data` matches the KeyID `id`
// (e.g. "ctrl+c", "enter", "shift+tab").
func MatchesKey(data string, id KeyID) bool {
	want := parseKeyID(id)
	for _, k := range ParseKeys(data) {
		if keysEqual(k, want) {
			return true
		}
	}
	return false
}

// MatchesAnyKey is a convenience that tests multiple KeyIDs.
func MatchesAnyKey(data string, ids ...KeyID) bool {
	for _, id := range ids {
		if MatchesKey(data, id) {
			return true
		}
	}
	return false
}

type parsedKeyID struct {
	name string
	mods Modifier
}

func keysEqual(got Key, want parsedKeyID) bool {
	if got.Name == "" {
		return false
	}
	if !strings.EqualFold(got.Name, want.name) {
		return false
	}
	// For printable keys, Shift is encoded in the case of the rune, so we
	// compare mods *excluding* Shift.
	if len(got.Name) == 1 {
		return (got.Mods &^ ModShift) == (want.mods &^ ModShift)
	}
	return got.Mods == want.mods
}

func parseKeyID(id string) parsedKeyID {
	parts := strings.Split(strings.ToLower(id), "+")
	result := parsedKeyID{}
	for i, p := range parts {
		if i == len(parts)-1 {
			result.name = p
			continue
		}
		switch p {
		case "ctrl", "control":
			result.mods |= ModCtrl
		case "alt", "option":
			result.mods |= ModAlt
		case "shift":
			result.mods |= ModShift
		case "super", "cmd", "command":
			result.mods |= ModSuper
		case "meta":
			result.mods |= ModMeta
		case "hyper":
			result.mods |= ModHyper
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Kitty keyboard protocol flag (set when TUI successfully negotiates it)
// ---------------------------------------------------------------------------

var kittyActive int64

// SetKittyProtocolActive marks whether the terminal is currently emitting
// Kitty-format key events. It is safe to call from any goroutine.
func SetKittyProtocolActive(on bool) {
	if on {
		atomic.StoreInt64(&kittyActive, 1)
	} else {
		atomic.StoreInt64(&kittyActive, 0)
	}
}

// IsKittyProtocolActive returns the current Kitty protocol state.
func IsKittyProtocolActive() bool { return atomic.LoadInt64(&kittyActive) == 1 }

// kittyFlagsGlobal stores the currently negotiated Kitty keyboard protocol
// flags (bitmask of 1|2|4|8|16). Used by decodeKittyU to determine which
// positional parameters are present in CSI u sequences.
var kittyFlagsGlobal int64

// SetKittyKeyboardFlagsFromTerminal stores the negotiated Kitty keyboard flags
// so the CSI-u parser can interpret positional parameters correctly without
// needing to pass flags through the parse chain.
func SetKittyKeyboardFlagsFromTerminal(flags int64) {
	atomic.StoreInt64(&kittyFlagsGlobal, flags)
}

// kittyActiveFlags returns the currently negotiated flags. Safe to call
// from any goroutine.
func kittyActiveFlags() int64 { return atomic.LoadInt64(&kittyFlagsGlobal) }

// percentDecode decodes a percent-encoded string from the Kitty keyboard
// protocol's associated text field (flag 16). The text parameter contains
// %XX sequences where XX is a two-digit hex byte value (e.g. "%48" = 'H').
// Returns the decoded UTF-8 string, or the raw input if no % encoding is
// present (backward compatibility with terminals that send raw text).
func percentDecode(s string) string {
	if s == "" {
		return ""
	}
	// Fast path: no '%' means raw text, no decoding needed.
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			h := parseHexByte(s[i+1 : i+3])
			if h != 0 || (s[i+1] == '0' && s[i+2] == '0') {
				b.WriteByte(h)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func parseHexByte(s string) byte {
	var v byte
	for i := 0; i < 2 && i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			v = v*16 + (c - '0')
		case c >= 'a' && c <= 'f':
			v = v*16 + (c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v = v*16 + (c - 'A' + 10)
		default:
			return 0
		}
	}
	return v
}

// Well-known KeyID constants for readability.
const (
	KeyEnter     KeyID = "enter"
	KeyEscape    KeyID = "escape"
	KeyTab       KeyID = "tab"
	KeyBackspace KeyID = "backspace"
	KeyDelete    KeyID = "delete"
	KeyUp        KeyID = "up"
	KeyDown      KeyID = "down"
	KeyLeft      KeyID = "left"
	KeyRight     KeyID = "right"
	KeyHome      KeyID = "home"
	KeyEnd       KeyID = "end"
	KeyPageUp    KeyID = "pageUp"
	KeyPageDown  KeyID = "pageDown"
	KeySpace     KeyID = " "
)
