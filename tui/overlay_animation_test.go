package tui

import (
	"strings"
	"testing"
	"time"

	core "github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// --- transitionProgress / dimIntensity 纯逻辑 ---

func TestOverlayTransitionProgress(t *testing.T) {
	now := time.Now()

	// 无动画：恒为 (1, true)。
	noAnim := &Overlay{}
	if p, done := noAnim.transitionProgress(now); p != 1 || !done {
		t.Errorf("no-transition progress = (%v, %v), want (1, true)", p, done)
	}

	// 未开始（openAt 零值）：视为已完成，不进入动画分支。
	notStarted := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second}}
	if p, done := notStarted.transitionProgress(now); p != 1 || !done {
		t.Errorf("not-started progress = (%v, %v), want (1, true)", p, done)
	}

	// 中间帧：Duration=1s，openAt=now-500ms → EaseOutCubic(0.5)=0.875。
	mid := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second}}
	mid.openAt = now.Add(-500 * time.Millisecond)
	if p, done := mid.transitionProgress(now); done || p < 0.87 || p > 0.88 {
		t.Errorf("mid progress = (%v, %v), want (~0.875, false)", p, done)
	}

	// 完成：openAt=now-2s → (1, true)。
	doneOv := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second}}
	doneOv.openAt = now.Add(-2 * time.Second)
	if p, done := doneOv.transitionProgress(now); p != 1 || !done {
		t.Errorf("done progress = (%v, %v), want (1, true)", p, done)
	}

	// Duration ≤ 0：恒为完成。
	zeroDur := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionFade, Duration: 0}}
	zeroDur.openAt = now
	if _, done := zeroDur.transitionProgress(now); !done {
		t.Error("zero-duration progress done = false, want true")
	}
}

func TestOverlayDimIntensity(t *testing.T) {
	now := time.Now()

	// Fade 过渡：强度 = 进度。
	fade := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionFade, Duration: time.Second}}
	fade.openAt = now.Add(-500 * time.Millisecond)
	if got := fade.dimIntensity(now); got < 0.87 || got > 0.88 {
		t.Errorf("fade dimIntensity = %v, want ~0.875", got)
	}
	// Fade 完成后强度 = 1。
	fadeDone := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionFade, Duration: time.Second}}
	fadeDone.openAt = now.Add(-2 * time.Second)
	if got := fadeDone.dimIntensity(now); got != 1 {
		t.Errorf("fade-done dimIntensity = %v, want 1", got)
	}
	// 非 Fade（含 SlideUp、None）：恒为 1。
	slide := &Overlay{Transition: OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second}}
	slide.openAt = now.Add(-500 * time.Millisecond)
	if got := slide.dimIntensity(now); got != 1 {
		t.Errorf("slide dimIntensity = %v, want 1", got)
	}
}

// --- composeOverlays 动画合成 ---

// TestComposeOverlaySlideUpAnimation 验证 SlideUp 过渡中间帧的位置插值：
// 动画进行到一半（EaseOutCubic(0.5)=0.875）时，overlay 内容应位于目标行
// 下方 (1-0.875)*rows 处，而非目标位置。
func TestComposeOverlaySlideUpAnimation(t *testing.T) {
	base := stringRows(20, 40) // 20 行 × 40 列
	ov := &Overlay{
		UseAbsolute: true,
		Anchor:      AnchorTopLeft,
		Row:         0,
		Col:         0,
		Width:       OverlaySize{Value: 10},
		Height:      OverlaySize{Value: 2},
		Content:     &lineComp{lines: []string{"OVERLAY", "SECOND"}},
		Transition:  OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second},
	}
	// 动画进行到 500ms（EaseOutCubic 后进度 0.875）→ 行偏移 (1-0.875)*20 = 2。
	ov.openAt = time.Now().Add(-500 * time.Millisecond)

	out := composeOverlays(base, []*Overlay{ov}, 40, 20)

	if !rowContains(out[2], "OVERLAY") {
		t.Errorf("mid-animation: expected overlay at row 2, row 2 = %q", rowText(out[2]))
	}
	if rowContains(out[0], "OVERLAY") {
		t.Errorf("mid-animation: overlay must not be at target row 0 yet, row 0 = %q", rowText(out[0]))
	}
}

// TestComposeOverlaySlideUpAnimationDone 验证动画完成后 overlay 精确到达目标位置。
func TestComposeOverlaySlideUpAnimationDone(t *testing.T) {
	base := stringRows(20, 40)
	ov := &Overlay{
		UseAbsolute: true,
		Anchor:      AnchorTopLeft,
		Row:         0,
		Col:         0,
		Width:       OverlaySize{Value: 10},
		Height:      OverlaySize{Value: 2},
		Content:     &lineComp{lines: []string{"OVERLAY", "SECOND"}},
		Transition:  OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: time.Second},
	}
	ov.openAt = time.Now().Add(-2 * time.Second) // 已完成

	out := composeOverlays(base, []*Overlay{ov}, 40, 20)

	if !rowContains(out[0], "OVERLAY") {
		t.Errorf("done: expected overlay at row 0, row 0 = %q", rowText(out[0]))
	}
}

// TestComposeOverlayFadeAnimation 验证 Fade 过渡：p=0 时背景不变暗（intensity 0
// 跳过一次 applyDimToRow），完成后全强度变暗（AttrDim + 玻璃背景）。
func TestComposeOverlayFadeAnimation(t *testing.T) {
	mk := func(openOffset time.Duration) *Overlay {
		ov := &Overlay{
			UseAbsolute:   true,
			Anchor:        AnchorTopLeft,
			Row:           2,
			Col:           10,
			Width:         OverlaySize{Value: 20},
			Height:        OverlaySize{Value: 4},
			DimBackground: true,
			Content:       &lineComp{lines: []string{"AAAA", "BBBB", "CCCC", "DDDD"}},
			Transition:    OverlayTransition{Kind: OverlayTransitionFade, Duration: time.Second},
		}
		ov.openAt = time.Now().Add(openOffset)
		return ov
	}

	isDim := func(c core.Cell) bool { return c.Style.Attrs&dimTextAttr != 0 }

	// 初始帧（p 钳制到 0，用未来 openAt 保证）：无变暗。
	out0 := composeOverlays(stringRows(10, 40), []*Overlay{mk(time.Second)}, 40, 10)
	if isDim(out0[0].Cells[0]) {
		t.Error("p=0: background must not be dimmed yet")
	}

	// 完成帧（p=1）：全强度变暗（overlay 区域外）。
	outDone := composeOverlays(stringRows(10, 40), []*Overlay{mk(-2 * time.Second)}, 40, 10)
	if !isDim(outDone[0].Cells[0]) {
		t.Error("done: background must be dimmed")
	}
	if isDim(outDone[2].Cells[15]) { // overlay 内容区域（row 2, col 15）不变暗
		t.Error("done: overlay content region must not be dimmed")
	}
}

// --- PushOverlay 启动动画（集成） ---

func TestPushOverlayStartsAnimation(t *testing.T) {
	app := NewTUI(terminal.NewVirtualTerminal(80, 24), TUIOptions{})
	defer app.Stop()

	ov := &Overlay{
		UseAbsolute: true,
		Anchor:      AnchorTopLeft,
		Row:         0,
		Col:         0,
		Width:       OverlaySize{Value: 10},
		Height:      OverlaySize{Value: 2},
		Content:     &lineComp{lines: []string{"ANIM"}},
		Transition:  OverlayTransition{Kind: OverlayTransitionSlideUp, Duration: 200 * time.Millisecond},
	}
	app.PushOverlay(ov)

	if ov.openAt.IsZero() {
		t.Fatal("PushOverlay must set openAt for animated overlays")
	}
	if p, done := ov.transitionProgress(time.Now()); done || p >= 1 {
		t.Errorf("animation should be in progress right after push, got (p=%v, done=%v)", p, done)
	}
	// 动画 ticker 已启动：稍等一帧后应仍处于动画中。
	time.Sleep(50 * time.Millisecond)
	if _, done := ov.transitionProgress(time.Now()); done {
		t.Error("animation ended too early (50ms < 200ms)")
	}
	// 等待动画完成，确保链式 ticker 自终止不泄漏。
	time.Sleep(250 * time.Millisecond)
	if p, done := ov.transitionProgress(time.Now()); !done || p != 1 {
		t.Errorf("animation should be complete after duration, got (p=%v, done=%v)", p, done)
	}
}

// rowText 将 Row 序列化为可见文本（不含样式），用于断言。
func rowText(r core.Row) string {
	if r.IsRaw() {
		return r.Raw
	}
	var sb []byte
	for _, c := range r.Cells {
		if c.IsContinuation() {
			continue
		}
		sb = append(sb, string(c.Rune)...)
	}
	return string(sb)
}

// rowContains 报告 row 的可见文本是否包含子串。
func rowContains(r core.Row, substr string) bool {
	return strings.Contains(rowText(r), substr)
}
