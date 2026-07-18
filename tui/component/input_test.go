package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestInputInsertAndSubmit(t *testing.T) {
	var submitted string
	in := NewInput(nil)
	in.SetFocused(true)
	in.OnSubmit(func(v string) { submitted = v })

	in.Update(core.KeyMsg{Data: "hello"})
	if got := in.GetValue(); got != "hello" {
		t.Fatalf("want hello, got %q", got)
	}
	lines := in.Render(40)
	if !strings.Contains(lines[0], core.CursorMarker) {
		t.Fatalf("expected cursor marker in render")
	}
	in.Update(core.KeyMsg{Data: "\r"})
	if submitted != "hello" {
		t.Fatalf("want hello, got %q", submitted)
	}
}

func TestInputBackspaceAndWordDelete(t *testing.T) {
	in := NewInput(nil)
	in.SetFocused(true)
	in.Update(core.KeyMsg{Data: "foo bar"})
	if in.GetValue() != "foo bar" {
		t.Fatalf("unexpected value: %q", in.GetValue())
	}
	in.Update(core.KeyMsg{Data: "\x17"})
	if in.GetValue() != "foo " {
		t.Fatalf("after ctrl+w want %q, got %q", "foo ", in.GetValue())
	}
	in.Update(core.KeyMsg{Data: "\x7f"})
	if in.GetValue() != "foo" {
		t.Fatalf("after backspace want %q, got %q", "foo", in.GetValue())
	}
}

func TestInputCursorMoves(t *testing.T) {
	in := NewInput(nil)
	in.SetFocused(true)
	in.Update(core.KeyMsg{Data: "abcd"})
	in.Update(core.KeyMsg{Data: "\x01"})
	in.Update(core.KeyMsg{Data: "X"})
	if in.GetValue() != "Xabcd" {
		t.Fatalf("expected Xabcd, got %q", in.GetValue())
	}
	in.Update(core.KeyMsg{Data: "\x05"})
	in.Update(core.KeyMsg{Data: "Y"})
	if in.GetValue() != "XabcdY" {
		t.Fatalf("expected XabcdY, got %q", in.GetValue())
	}
}

func TestInputSelectAllReplacesOnlyInputText(t *testing.T) {
	in := NewInput(nil)
	in.SetFocused(true)
	in.Update(core.KeyMsg{Data: "hello"})
	in.Update(core.KeyMsg{Data: "\x1b[97;9u"}) // Kitty CSI-u: super+a

	if got := in.GetSelectedText(); got != "hello" {
		t.Fatalf("selected text: want %q, got %q", "hello", got)
	}

	in.Update(core.KeyMsg{Data: "x"})
	if got := in.GetValue(); got != "x" {
		t.Fatalf("typing should replace selected input text, got %q", got)
	}
}

func TestInputPlaceholder(t *testing.T) {
	in := NewInput(nil)
	in.SetPlaceholder("type here")
	in.SetFocused(false)
	lines := in.Render(20)
	if !strings.Contains(lines[0], "type here") {
		t.Errorf("expected placeholder in render, got %q", lines[0])
	}
}

func TestInputOnChange(t *testing.T) {
	in := NewInput(nil)
	in.SetFocused(true)
	var changed string
	in.OnChange(func(v string) { changed = v })
	in.Update(core.KeyMsg{Data: "abc"})
	if changed != "abc" {
		t.Errorf("OnChange = %q, want abc", changed)
	}
}

func TestInputSetValue(t *testing.T) {
	in := NewInput(nil)
	in.SetValue("preset")
	if in.GetValue() != "preset" {
		t.Errorf("GetValue = %q, want preset", in.GetValue())
	}
}

func TestInputSetPrompt(t *testing.T) {
	in := NewInput(nil)
	in.SetPrompt("> ")
	lines := in.Render(20)
	if !strings.Contains(lines[0], "> ") {
		t.Errorf("expected prompt in render, got %q", lines[0])
	}
}
