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
	keys := ParseKeys("[97;2;1;65;%41u")
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
	keys := ParseKeys("[104;1;1;0;%48%49%21u")
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
	keys := ParseKeys("hi中")
	if len(keys) != 3 {
		t.Fatalf("want 3 keys, got %d", len(keys))
	}
	if keys[0].Name != "h" || keys[1].Name != "i" || keys[2].Name != "中" {
		t.Errorf("unexpected names: %v %v %v", keys[0].Name, keys[1].Name, keys[2].Name)
	}
}
