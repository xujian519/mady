package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// overlayCategory reads the overlay category via the optional extension
// method (not part of the OverlayRef interface).
func overlayCategory(t *testing.T, ov OverlayRef) int {
	t.Helper()
	c, ok := ov.(interface{ OverlayCategory() int })
	if !ok {
		t.Fatalf("overlay %T does not expose OverlayCategory", ov)
	}
	return c.OverlayCategory()
}

// TestOpenReviewGateOverlay verifies open/close of the review gate overlay,
// including replacement of an already-open gate.
func TestOpenReviewGateOverlay(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	ov := app.OpenReviewGate(ReviewGateData{
		Title: "复核", Judgment: "结论", Confidence: 0.9,
		Evidences: []component.ReviewEvidence{{Title: "e1"}},
		Checklist: []component.ReviewCheckItem{{Label: "c1"}},
		Risks:     []string{"r1"},
	})
	if ov == nil || len(host.overlays) != 1 {
		t.Fatalf("expected review gate pushed, got %d overlays", len(host.overlays))
	}
	if overlayCategory(t, ov) != OverlayCatGate {
		t.Errorf("category = %d, want OverlayCatGate", overlayCategory(t, ov))
	}
	if _, ok := ov.OverlayContent().(*component.ReviewGate); !ok {
		t.Fatalf("content type = %T, want *ReviewGate", ov.OverlayContent())
	}

	// Opening a second gate replaces the first.
	ov2 := app.OpenReviewGate(ReviewGateData{Title: "复核2", Judgment: "j2"})
	if len(host.overlays) != 1 || ov2 == ov {
		t.Fatalf("second gate should replace first, got %d overlays", len(host.overlays))
	}

	app.CloseReviewGate()
	if len(host.overlays) != 0 {
		t.Fatalf("CloseReviewGate should remove overlay, got %d", len(host.overlays))
	}
	// Closing with nothing open is a no-op.
	app.CloseReviewGate()

	// The overlay's own close callback routes back to CloseReviewGate.
	app.OpenReviewGate(ReviewGateData{Judgment: "x"})
	host.overlays[0].OverlayContent().(core.Updatable).Update(core.KeyMsg{Data: "\x1b"})
	if len(host.overlays) != 0 {
		t.Fatal("Esc in review gate should close it")
	}
}

// TestOpenSystemStatusOverlay verifies open/close of the system status overlay.
func TestOpenSystemStatusOverlay(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	ov := app.OpenSystemStatus(SystemStatusData{
		Mode: "degraded", ModeReason: "上次操作未正常完成",
		Events:  []component.SysEvent{{Message: "m1", Level: "warn"}},
		Impacts: []string{"i1"},
	})
	if ov == nil || len(host.overlays) != 1 {
		t.Fatal("expected system status pushed")
	}
	if overlayCategory(t, ov) != OverlayCatSystem {
		t.Errorf("category = %d, want OverlayCatSystem", overlayCategory(t, ov))
	}
	if _, ok := ov.OverlayContent().(*component.SystemStatus); !ok {
		t.Fatalf("content type = %T, want *SystemStatus", ov.OverlayContent())
	}

	// Replacement path.
	app.OpenSystemStatus(SystemStatusData{Mode: "normal"})
	if len(host.overlays) != 1 {
		t.Fatalf("second status should replace first, got %d", len(host.overlays))
	}
	app.CloseSystemStatus()
	if len(host.overlays) != 0 {
		t.Fatal("CloseSystemStatus should remove overlay")
	}
	app.CloseSystemStatus() // no-op
}

// TestOpenEvidenceOverlay verifies open/close of the evidence overlay.
func TestOpenEvidenceOverlay(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	ov := app.OpenEvidenceOverlay(EvidenceOverlayData{
		Items: []component.EvidenceItem{{Title: "证据1"}},
	})
	if ov == nil || len(host.overlays) != 1 {
		t.Fatal("expected evidence overlay pushed")
	}
	if overlayCategory(t, ov) != OverlayCatReview {
		t.Errorf("category = %d, want OverlayCatReview", overlayCategory(t, ov))
	}
	if _, ok := ov.OverlayContent().(*component.EvidenceOverlay); !ok {
		t.Fatalf("content type = %T, want *EvidenceOverlay", ov.OverlayContent())
	}

	// Replacement path (empty items also fine).
	app.OpenEvidenceOverlay(EvidenceOverlayData{})
	if len(host.overlays) != 1 {
		t.Fatal("second evidence overlay should replace first")
	}
	app.CloseEvidenceOverlay()
	if len(host.overlays) != 0 {
		t.Fatal("CloseEvidenceOverlay should remove overlay")
	}
	app.CloseEvidenceOverlay() // no-op
}

// TestOpenOverlayGeneric verifies the generic overlay API and the four
// convenience wrappers, plus CloseOverlay nil-safety.
func TestOpenOverlayGeneric(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	content := component.NewText("x")

	// Defaults applied when WidthPct/HeightPct are 0.
	ov := app.OpenOverlay(content, OverlayOpts{})
	if len(host.overlays) != 1 {
		t.Fatal("expected overlay pushed")
	}
	if ov.OverlayWidthPct() != 60 || ov.OverlayHeightPct() != 60 {
		t.Errorf("default size = %d%% x %d%%, want 60 x 60", ov.OverlayWidthPct(), ov.OverlayHeightPct())
	}
	if ov.OverlayWantsFocus() != true || ov.OverlayDimBackground() {
		t.Error("generic overlay defaults: focus=true, dim=false")
	}
	app.CloseOverlay(ov)
	if len(host.overlays) != 0 {
		t.Fatal("CloseOverlay should remove overlay")
	}
	app.CloseOverlay(nil) // nil is a no-op

	// Wrappers.
	ov = app.OpenSelectionOverlay(content)
	if overlayCategory(t, ov) != OverlayCatSelection || ov.OverlayWidthPct() != 40 || ov.OverlayHeightPct() != 30 {
		t.Errorf("selection overlay wrong config: %+v", ov)
	}
	app.CloseOverlay(ov)

	ov = app.OpenReviewOverlay(content)
	if overlayCategory(t, ov) != OverlayCatReview || ov.OverlayWidthPct() != 60 || ov.OverlayHeightPct() != 60 {
		t.Errorf("review overlay wrong config: %+v", ov)
	}
	app.CloseOverlay(ov)

	ov = app.OpenGateOverlay(content)
	if overlayCategory(t, ov) != OverlayCatGate || ov.OverlayWidthPct() != 70 || ov.OverlayHeightPct() != 75 {
		t.Errorf("gate overlay wrong config: %+v", ov)
	}
	app.CloseOverlay(ov)

	ov = app.OpenSystemOverlay(content)
	if overlayCategory(t, ov) != OverlayCatSystem || ov.OverlayWidthPct() != 50 || ov.OverlayHeightPct() != 40 {
		t.Errorf("system overlay wrong config: %+v", ov)
	}
	if !ov.OverlayDimBackground() {
		t.Error("system overlay should dim background")
	}
	app.CloseOverlay(ov)
}

// TestToggleMousePassthrough verifies the F2 passthrough toggle calls the
// host's EnableMouse/DisableMouse and remembers its state across toggles.
func TestToggleMousePassthrough(t *testing.T) {
	vt := newCountingMouseHost(t, 80, 24)
	app := NewChatAppWithHost(ChatAppConfig{}, vt)
	app.SetHost(vt)
	if err := app.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { app.Stop() })

	app.ToggleMousePassthrough() // on
	if vt.disableCount != 1 || vt.enableCount != 0 {
		t.Fatalf("passthrough on: disable=%d enable=%d", vt.disableCount, vt.enableCount)
	}
	app.ToggleMousePassthrough() // off → restore "sgr"
	if vt.disableCount != 1 || vt.enableCount != 1 {
		t.Fatalf("passthrough off: disable=%d enable=%d", vt.disableCount, vt.enableCount)
	}
	app.ToggleMousePassthrough() // on again
	if vt.disableCount != 2 {
		t.Fatalf("disable=%d, want 2", vt.disableCount)
	}
}

// countingMouseHost is a testAppHost that counts Enable/DisableMouse calls.
type countingMouseHost struct {
	testAppHost
	enableCount  int
	disableCount int
}

func newCountingMouseHost(t *testing.T, cols, rows int64) *countingMouseHost {
	t.Helper()
	return &countingMouseHost{testAppHost: testAppHost{vt: terminal.NewVirtualTerminal(cols, rows)}}
}

func (h *countingMouseHost) EnableMouse(_ string) { h.enableCount++ }
func (h *countingMouseHost) DisableMouse()        { h.disableCount++ }

// TestToggleMousePassthroughWithMode verifies the configured MouseMode is
// restored (not the hardcoded default).
func TestToggleMousePassthroughWithMode(t *testing.T) {
	vt := newCountingMouseHost(t, 80, 24)
	app := NewChatAppWithHost(ChatAppConfig{MouseMode: "x10"}, vt)
	app.SetHost(vt)
	if err := app.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { app.Stop() })

	app.ToggleMousePassthrough()
	app.ToggleMousePassthrough()
	if vt.enableCount != 1 {
		t.Fatalf("enable=%d, want 1", vt.enableCount)
	}
}

// TestPrintWelcome verifies the welcome message renders provider/model/mode.
func TestPrintWelcome(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.PrintWelcome("deepseek", "v4", "patent", "案件A")

	msgs := app.History().Messages()
	if len(msgs) != 1 || msgs[0].Role != RoleSystem {
		t.Fatalf("expected 1 system welcome, got %+v", msgs)
	}
	if !strings.Contains(msgs[0].Text, "deepseek") || !strings.Contains(msgs[0].Text, "v4") {
		t.Errorf("welcome should mention provider/model: %q", msgs[0].Text)
	}
	if !strings.Contains(msgs[0].Text, "案件A") {
		t.Errorf("welcome should mention case: %q", msgs[0].Text)
	}
}

// TestPrintStatus verifies PrintStatus updates the loader message (visible
// once the loader is running; PrintStatus itself must not start it).
func TestPrintStatus(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.Busy("busy")
	app.PrintStatus("processing...") // replaces the loader message while running
	lines := app.Loader().Render(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "processing...") {
		t.Fatalf("loader should show message, got %q", joined)
	}
	app.Idle()
}

// TestClearJudgmentSummary verifies the judgment summary reset path.
func TestClearJudgmentSummary(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.SetJudgmentSummary(JudgmentSummary{Phase: "分析阶段", Judgment: "j", Confidence: 0.8, Pending: []string{"p"}})
	if app.judgmentView.Status() == "" {
		t.Fatal("setup: judgment view should have a status")
	}
	app.ClearJudgmentSummary()
	app.mu.Lock()
	js := app.model.judgmentSummary
	app.mu.Unlock()
	if js.Phase != "" || js.Judgment != "" || js.Confidence != 0 {
		t.Fatalf("judgment summary not cleared: %+v", js)
	}
}

// TestPrintErrorNil verifies PrintError ignores nil errors.
func TestPrintErrorNil(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.PrintError(nil)
	if n := len(app.History().Messages()); n != 0 {
		t.Fatalf("nil error must not append, got %d messages", n)
	}
}

// TestOpenOverlayContentTypes sanity-checks that overlay content components
// render without panic.
func TestOpenOverlayContentTypes(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	_ = app.OpenReviewGate(ReviewGateData{Judgment: "j"})
	_ = app.OpenSystemStatus(SystemStatusData{Mode: "m"})
	_ = app.OpenEvidenceOverlay(EvidenceOverlayData{})
	_ = app.OpenOverlay(component.NewText("t"), OverlayOpts{})
	_ = app.OpenSelectionOverlay(component.NewText("s"))
	_ = app.OpenReviewOverlay(component.NewText("r"))
	_ = app.OpenGateOverlay(component.NewText("g"))
	_ = app.OpenSystemOverlay(component.NewText("o"))
	host := app.host.(*testAppHost)
	if len(host.overlays) == 0 {
		t.Fatal("expected overlays pushed")
	}
}
