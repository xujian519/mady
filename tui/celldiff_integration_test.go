package tui

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

type staticComponent struct {
	lines []string
}

func (s *staticComponent) Render(int64) []string    { return s.lines }
func (s *staticComponent) Invalidate()              {}
func (s *staticComponent) Update(core.Msg) core.Cmd { return nil }

func TestRenderFrameCellDiff(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 5)
	app := NewTUI(vt, TUIOptions{DisableSynchronizedOutput: true})
	defer app.Stop()

	comp := &staticComponent{lines: []string{"hello world"}}
	app.AddChild(comp)

	app.renderFrame()
	vt.ResetOutput()

	comp.lines = []string{"hello WORLD"}
	app.renderFrame()

	out := vt.OutputString()
	if strings.Contains(out, "\x1b[1;1H") {
		t.Fatalf("cell diff should not rewrite from column 1, got %q", out)
	}
	if !strings.Contains(out, "\x1b[1;7H") {
		t.Fatalf("expected cursor move to column 7, got %q", out)
	}
	if !strings.Contains(out, "WORLD") {
		t.Fatalf("expected 'WORLD' in output, got %q", out)
	}
}

func TestRenderFrameCellDiffClearTail(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 5)
	app := NewTUI(vt, TUIOptions{DisableSynchronizedOutput: true})
	defer app.Stop()

	comp := &staticComponent{lines: []string{"hello world"}}
	app.AddChild(comp)

	app.renderFrame()
	vt.ResetOutput()

	comp.lines = []string{"hi"}
	app.renderFrame()

	out := vt.OutputString()
	if !strings.Contains(out, "\x1b[1;3H") {
		t.Fatalf("expected cursor move to column 3 to clear tail, got %q", out)
	}
	if !strings.Contains(out, "\x1b[0K") {
		t.Fatalf("expected EL clear tail, got %q", out)
	}
}

func TestRenderFrameCellDiffPreservesStyle(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 5)
	app := NewTUI(vt, TUIOptions{DisableSynchronizedOutput: true})
	defer app.Stop()

	comp := &staticComponent{lines: []string{"\x1b[31mhello\x1b[0m world"}}
	app.AddChild(comp)

	app.renderFrame()
	vt.ResetOutput()

	comp.lines = []string{"\x1b[31mhello\x1b[0m WORLD"}
	app.renderFrame()

	out := vt.OutputString()
	if !strings.Contains(out, "\x1b[1;7H") {
		t.Fatalf("expected cursor move to column 7, got %q", out)
	}
	if !strings.Contains(out, "WORLD") {
		t.Fatalf("expected 'WORLD' in output, got %q", out)
	}
}
