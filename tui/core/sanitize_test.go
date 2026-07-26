package core

import "testing"

func TestSanitizeRawContent_SGRPreserved(t *testing.T) {
	input := "\x1b[31mred text\x1b[0m normal"
	got := SanitizeRawContent(input)
	if got != input {
		t.Fatalf("SGR should be preserved:\nwant %q\ngot  %q", input, got)
	}
}

func TestSanitizeRawContent_256AndTruecolorSGR(t *testing.T) {
	cases := []string{
		"\x1b[38;5;196m256-color\x1b[0m",
		"\x1b[38;2;255;0;0mtruecolor\x1b[0m",
		"\x1b[1;4mbold+underline\x1b[0m",
	}
	for _, input := range cases {
		got := SanitizeRawContent(input)
		if got != input {
			t.Fatalf("SGR variant should be preserved:\nwant %q\ngot  %q", input, got)
		}
	}
}

func TestSanitizeRawContent_CursorMarkerPreserved(t *testing.T) {
	input := "hello" + CursorMarker + "world"
	got := SanitizeRawContent(input)
	if got != input {
		t.Fatalf("CursorMarker should be preserved:\nwant %q\ngot  %q", input, got)
	}
}

func TestSanitizeRawContent_OSC8Stripped(t *testing.T) {
	// OSC 8 hyperlink injection — must be stripped
	input := "\x1b]8;;file:///etc/passwd\x1b\\click here\x1b]8;;\x1b\\"
	got := SanitizeRawContent(input)
	want := "click here"
	if got != want {
		t.Fatalf("OSC 8 hyperlink should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_OSC0TitleStripped(t *testing.T) {
	// OSC 0 title injection
	input := "\x1b]0;Malicious Title\x07visible text"
	got := SanitizeRawContent(input)
	want := "visible text"
	if got != want {
		t.Fatalf("OSC 0 title should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_OSC52ClipboardStripped(t *testing.T) {
	// OSC 52 clipboard write — even this should be stripped from LLM output
	input := "\x1b]52;c;ZXZpbA==\x07pasted"
	got := SanitizeRawContent(input)
	want := "pasted"
	if got != want {
		t.Fatalf("OSC 52 should be stripped from raw content:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_DCSStripped(t *testing.T) {
	// DCS device query
	input := "\x1bP$qm\x1b\\after dcs"
	got := SanitizeRawContent(input)
	want := "after dcs"
	if got != want {
		t.Fatalf("DCS should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_APCKittyGraphicsStripped(t *testing.T) {
	// Kitty graphics APC — stripped from LLM output
	input := "\x1b_Ga=q,s=100,v=100\tpayload\x1b\\after graphics"
	got := SanitizeRawContent(input)
	want := "after graphics"
	if got != want {
		t.Fatalf("Kitty graphics APC should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_DecPrivateModeStripped(t *testing.T) {
	// DEC private mode: ?1049h = switch to alternate screen
	input := "before\x1b[?1049hafter"
	got := SanitizeRawContent(input)
	want := "beforeafter"
	if got != want {
		t.Fatalf("DEC private mode should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_CursorMovementStripped(t *testing.T) {
	// Non-SGR CSI: cursor up/down — stripped
	cases := []string{
		"\x1b[5Aup",    // cursor up
		"\x1b[3Bright", // cursor right
		"\x1b[2Jclear", // erase display
	}
	wants := []string{"up", "right", "clear"}
	for i, input := range cases {
		got := SanitizeRawContent(input)
		if got != wants[i] {
			t.Fatalf("non-SGR CSI should be stripped:\ninput %q\nwant  %q\ngot   %q", input, wants[i], got)
		}
	}
}

func TestSanitizeRawContent_PlainText(t *testing.T) {
	input := "Hello, 世界! 🌍 no escapes here"
	got := SanitizeRawContent(input)
	if got != input {
		t.Fatalf("plain text should pass through:\nwant %q\ngot  %q", input, got)
	}
}

func TestSanitizeRawContent_LoneESCTrailing(t *testing.T) {
	// Incomplete escape at end of string — ESC stripped, text kept
	input := "text\x1b"
	got := SanitizeRawContent(input)
	want := "text"
	if got != want {
		t.Fatalf("trailing lone ESC should be stripped:\nwant %q\ngot  %q", want, got)
	}
}

func TestSanitizeRawContent_MixedContent(t *testing.T) {
	// Realistic mix: LLM output with color + injection attempts
	input := "\x1b[32mSuccess:\x1b[0m \x1b]8;;https://evil.com\x1b\\link\x1b]8;;\x1b\\ done"
	got := SanitizeRawContent(input)
	want := "\x1b[32mSuccess:\x1b[0m link done"
	if got != want {
		t.Fatalf("mixed content:\nwant %q\ngot  %q", want, got)
	}
}
