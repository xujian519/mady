package terminal

import "testing"

func TestMatchesKeyBasic(t *testing.T) {
	cases := []struct {
		data string
		key  KeyID
		want bool
	}{
		{"\r", "enter", true},
		{"\n", "enter", true},
		{"\x1b", "escape", true},
		{"\t", "tab", true},
		{"\x7f", "backspace", true},
		{"\x03", "ctrl+c", true},
		{"\x1b[A", "up", true},
		{"\x1b[B", "down", true},
		{"\x1b[C", "right", true},
		{"\x1b[D", "left", true},
		{"\x1b[H", "home", true},
		{"\x1b[F", "end", true},
		{"\x1b[3~", "delete", true},
		{"\x1b[5~", "pageUp", true},
		{"\x1b[6~", "pageDown", true},
		{"\x1bOP", "f1", true},
		{"a", "a", true},
		{"A", "A", true},
		{"\x1bb", "alt+b", true},
	}
	for _, c := range cases {
		if got := MatchesKey(c.data, c.key); got != c.want {
			t.Errorf("MatchesKey(%q, %q) = %v, want %v", c.data, c.key, got, c.want)
		}
	}
}

func TestKittyCSIu(t *testing.T) {
	// CSI 13 u = enter (Kitty format)
	if !MatchesKey("\x1b[13u", "enter") {
		t.Error("expected kitty CSI 13 u → enter")
	}
	// CSI 99 ; 5 u = ctrl+c
	if !MatchesKey("\x1b[99;5u", "ctrl+c") {
		t.Error("expected kitty CSI 99;5 u → ctrl+c")
	}
}

func TestKittyAlternateKeyAndText(t *testing.T) {
	// Kitty CSI u with alternate key (flag 4) and associated text (flag 16):
	// ESC [ code ; mods ; event ; alt_key ; text u
	//   code=97 (a), mods=2 (shift), event=1 (press), alt=65 (A), text=41 (hex for "A")
	keys := ParseKeys("\x1b[97;2;1;65;%41u", 0)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k.Name != "a" || k.Rune != 'a' {
		t.Fatalf("name/rune: want 'a', got %q / %c", k.Name, k.Rune)
	}
	if k.Mods != ModShift {
		t.Fatalf("mods: want shift (2), got %d", k.Mods)
	}
	if k.Event != KeyPress {
		t.Fatalf("event: want press (1), got %d", k.Event)
	}
	if k.Alt != 65 {
		t.Fatalf("alt: want 65 (A), got %d", k.Alt)
	}
	if k.Text != "A" {
		t.Fatalf(`text: want "A", got %q`, k.Text)
	}
}

func TestKittyTextPercentDecode(t *testing.T) {
	// Percent-encoded text: "%48%65%6c%6c%6f" = ASCII "Hello"
	// Sequence: code=104('h'), no-mods(1), press(1), alt=0, text=%48%65%6c%6c%6f
	keys := ParseKeys("\x1b[104;1;1;0;%48%49%21u", 0)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k.Name != "h" {
		t.Fatalf(`name: want 'h', got %q`, k.Name)
	}
	if k.Text != "HI!" {
		t.Fatalf(`text: want "HI!", got %q`, k.Text)
	}
}

func TestParseKeysPrintable(t *testing.T) {
	keys := ParseKeys("hi中", 0)
	if len(keys) != 3 {
		t.Fatalf("want 3 keys, got %d", len(keys))
	}
	if keys[0].Name != "h" || keys[1].Name != "i" || keys[2].Name != "中" {
		t.Errorf("unexpected names: %v %v %v", keys[0].Name, keys[1].Name, keys[2].Name)
	}
}

// TestKittyFunctionalKeyMapping locks the Kitty keyboard protocol functional
// key codepoints to their canonical names. These are the Unicode PUA codes a
// Kitty-protocol terminal emits in CSI ... u (0xe000=Escape … 0xe01f=F12);
// a wrong mapping silently breaks navigation on Kitty/Ghostty/WezTerm/foot.
func TestKittyFunctionalKeyMapping(t *testing.T) {
	cases := []struct {
		seq  string
		want string
	}{
		{"\x1b[57348u", "insert"},
		{"\x1b[57349u", "delete"},
		{"\x1b[57350u", "left"},
		{"\x1b[57351u", "right"},
		{"\x1b[57352u", "up"},
		{"\x1b[57353u", "down"},
		{"\x1b[57354u", "pageUp"},
		{"\x1b[57355u", "pageDown"},
		{"\x1b[57356u", "home"},
		{"\x1b[57357u", "end"},
		{"\x1b[57358u", "capsLock"},
		{"\x1b[57359u", "scrollLock"},
		{"\x1b[57360u", "numLock"},
		{"\x1b[57361u", "printScreen"},
		{"\x1b[57362u", "pause"},
		{"\x1b[57363u", "menu"},
		{"\x1b[57364u", "f1"},
		{"\x1b[57368u", "f5"},
		{"\x1b[57375u", "f12"},
	}
	for _, c := range cases {
		if !MatchesKey(c.seq, c.want) {
			t.Errorf("MatchesKey(%q): want %q", c.seq, c.want)
		}
	}
}

// TestKittyFunctionalKeyWithModifier verifies modifier prefixes compose
// correctly with the functional-key mapping (ctrl+up, shift+f3).
func TestKittyFunctionalKeyWithModifier(t *testing.T) {
	if !MatchesKey("\x1b[57352;5u", "ctrl+up") {
		t.Error("expected ctrl+up from CSI 57352;5 u")
	}
	if !MatchesKey("\x1b[57366;2u", "shift+f3") {
		t.Error("expected shift+f3 from CSI 57366;2 u (57366 = 0xe016 = F3)")
	}
}

// TestKittyC0PUAKeys verifies the C0 control keys encoded as PUA codepoints
// under protocol flag 1 (0xe000=Escape … 0xe003=Enter). Without these
// mappings, Shift+Esc/Shift+Enter etc. arrive as private-use runes and get
// inserted into the editor buffer as garbage characters.
func TestKittyC0PUAKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want string
	}{
		{"\x1b[57344;2u", "shift+escape"},   // 0xe000 + Shift
		{"\x1b[57345;2u", "shift+tab"},      // 0xe001 + Shift
		{"\x1b[57346;5u", "ctrl+backspace"}, // 0xe002 + Ctrl
		{"\x1b[57347;5u", "ctrl+enter"},     // 0xe003 + Ctrl
		{"\x1b[57344;5u", "ctrl+escape"},    // 0xe000 + Ctrl
	}
	for _, c := range cases {
		if !MatchesKey(c.seq, c.want) {
			t.Errorf("MatchesKey(%q): want %q", c.seq, c.want)
		}
	}
}
