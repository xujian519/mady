package core

import (
	"math"
	"testing"
	"time"
)

// --- Easing 纯函数 ---

func TestEasingEndpoints(t *testing.T) {
	// 所有预置缓动在 t=0 时为 0、t=1 时为 1（EaseOutBack 的 1 精确成立）。
	easings := map[string]Easing{
		"linear":     EaseLinear,
		"in_quad":    EaseInQuad,
		"out_quad":   EaseOutQuad,
		"inout_quad": EaseInOutQuad,
		"out_cubic":  EaseOutCubic,
		"out_back":   EaseOutBack,
	}
	for name, e := range easings {
		if got := e(0); math.Abs(got) > 1e-9 {
			t.Errorf("%s(0) = %v, want ~0", name, got)
		}
		if got := e(1); math.Abs(got-1) > 1e-9 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
}

func TestEasingMonotonicSamples(t *testing.T) {
	// 除 EaseOutBack（回弹）外，其余缓动在 [0,1] 上单调不减。
	monotonic := map[string]Easing{
		"linear":     EaseLinear,
		"in_quad":    EaseInQuad,
		"out_quad":   EaseOutQuad,
		"inout_quad": EaseInOutQuad,
		"out_cubic":  EaseOutCubic,
	}
	for name, e := range monotonic {
		prev := 0.0
		for i := 0; i <= 20; i++ {
			v := e(float64(i) / 20)
			if v < prev-1e-9 {
				t.Errorf("%s 在 t=%v 处递减：%v < %v", name, float64(i)/20, v, prev)
			}
			prev = v
		}
	}
}

func TestEasingMidpointValues(t *testing.T) {
	cases := []struct {
		name string
		e    Easing
		want float64
	}{
		{"in_quad", EaseInQuad, 0.25},   // 0.5²
		{"out_quad", EaseOutQuad, 0.75}, // 0.5*(2-0.5)
		{"inout_quad", EaseInOutQuad, 0.5},
		{"out_cubic", EaseOutCubic, 0.875}, // 1-(0.5)³
	}
	for _, c := range cases {
		if got := c.e(0.5); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s(0.5) = %v, want %v", c.name, got, c.want)
		}
	}
	// EaseOutBack 中点应越过 1（回弹特征）。
	if v := EaseOutBack(0.5); v <= 1 {
		t.Errorf("EaseOutBack(0.5) = %v, want > 1 (overshoot)", v)
	}
}

// --- Tween 插值 ---

func TestTweenValue(t *testing.T) {
	start := time.Unix(1000, 0)
	tw := &Tween{From: 0, To: 100, Duration: time.Second, Start: start, Ease: EaseLinear}

	if got := tw.Value(start); got != 0 {
		t.Errorf("Value(start) = %v, want 0", got)
	}
	if got := tw.Value(start.Add(500 * time.Millisecond)); math.Abs(got-50) > 1e-9 {
		t.Errorf("Value(500ms) = %v, want 50", got)
	}
	// 超过 duration 后钳制到 To，不越过终点。
	if got := tw.Value(start.Add(2 * time.Second)); got != 100 {
		t.Errorf("Value(2s) = %v, want 100", got)
	}
}

func TestTweenValueNonLinear(t *testing.T) {
	start := time.Unix(1000, 0)
	tw := &Tween{From: 0, To: 1, Duration: time.Second, Start: start, Ease: EaseInQuad}
	// 0.5s 时进度 0.25（平方加速），值 = 0.25。
	if got := tw.Value(start.Add(500 * time.Millisecond)); math.Abs(got-0.25) > 1e-9 {
		t.Errorf("Value(500ms, inQuad) = %v, want 0.25", got)
	}
}

func TestTweenDone(t *testing.T) {
	start := time.Unix(1000, 0)
	tw := &Tween{From: 0, To: 1, Duration: time.Second, Start: start, Ease: EaseLinear}

	if tw.Done(start.Add(999 * time.Millisecond)) {
		t.Error("Done(999ms) = true, want false")
	}
	if !tw.Done(start.Add(time.Second)) {
		t.Error("Done(1s) = false, want true")
	}
	if !tw.Done(start.Add(2 * time.Second)) {
		t.Error("Done(2s) = false, want true")
	}
}

func TestTweenEdgeCases(t *testing.T) {
	// 零时长：立即完成并返回 To。
	tw := &Tween{From: 0, To: 42, Duration: 0, Start: time.Now(), Ease: EaseLinear}
	if !tw.Done(time.Now()) {
		t.Error("zero-duration Done = false, want true")
	}
	if got := tw.Value(time.Now()); got != 42 {
		t.Errorf("zero-duration Value = %v, want 42", got)
	}
	// nil Tween：Value 返回 0，Done 返回 true（调用方安全）。
	var nilTw *Tween
	if got := nilTw.Value(time.Now()); got != 0 {
		t.Errorf("nil Value = %v, want 0", got)
	}
	if !nilTw.Done(time.Now()) {
		t.Error("nil Done = false, want true")
	}
	// Start 在未来：now < Start 时进度钳制为 0，返回 From。
	start := time.Now().Add(time.Hour)
	fw := &Tween{From: 10, To: 20, Duration: time.Second, Start: start, Ease: EaseLinear}
	if got := fw.Value(time.Now()); got != 10 {
		t.Errorf("future Start Value = %v, want 10", got)
	}
}
