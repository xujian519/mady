package terminal

import (
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
//
// flags is the Kitty keyboard protocol flags bitmask (0 = default/compat).
func ParseKeys(data string, flags int64) []Key {
	var out []Key
	i := 0
	for i < len(data) {
		k, adv := parseOne(data, i, flags)
		if adv <= 0 {
			break
		}
		out = append(out, k)
		i += adv
	}
	return out
}

func parseOne(s string, i int, flags int64) (Key, int) {
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
		return parseCSI(s, i, flags)
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

// ---------------------------------------------------------------------------
// Matching helpers
// ---------------------------------------------------------------------------

// MatchesKey reports whether any key event in `data` matches the KeyID `id`
// (e.g. "ctrl+c", "enter", "shift+tab").
// Uses flags=0 (default/compat mode) for the Kitty protocol parser.
func MatchesKey(data string, id KeyID) bool {
	want := parseKeyID(id)
	for _, k := range ParseKeys(data, 0) {
		if keysEqual(k, want) {
			return true
		}
	}
	return false
}

// MatchesKeyWithFlags is like MatchesKey but passes the given Kitty keyboard
// flags to the parser. Callers that have access to the terminal's negotiated
// flags should use this to correctly decode CSI u sequences.
func MatchesKeyWithFlags(data string, id KeyID, flags int64) bool {
	want := parseKeyID(id)
	for _, k := range ParseKeys(data, flags) {
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
	// compare mods *excluding* Shift — UNLESS the binding explicitly
	// requires Shift (e.g. "ctrl+shift+z" for redo). Without the explicit
	// requirement, a shift-modified key also matches its unshifted binding,
	// so ctrl+shift+z would match both undo ("ctrl+z") and redo
	// ("ctrl+shift+z") and the earlier undo case would shadow redo.
	if len(got.Name) == 1 {
		gotM, wantM := got.Mods, want.mods
		if want.mods&ModShift == 0 {
			gotM &^= ModShift
		}
		return gotM == wantM
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
