package core

// tween.go — 补间动画基础设施（对齐 charmbracelet/harmonica 的定位）。
// Layer 0 纯数据：Easing 函数 + Tween 插值器，无终端 I/O、无渲染依赖，
// 可被 overlay 过渡、滚动吸附等动画场景复用。

import "time"

// Easing 将归一化时间 t∈[0,1] 映射为动画进度 p。p 允许轻微越界
// （如 EaseOutBack 的回弹），需要严格 [0,1] 的调用方应自行 clamp。
type Easing func(t float64) float64

// 预置缓动函数：quad 系（平方）、cubic 系（立方）、back（带回弹）。
var (
	// EaseLinear 匀速。
	EaseLinear Easing = func(t float64) float64 { return t }

	// EaseInQuad 加速（慢→快）。
	EaseInQuad Easing = func(t float64) float64 { return t * t }

	// EaseOutQuad 减速（快→慢）。
	EaseOutQuad Easing = func(t float64) float64 { return t * (2 - t) }

	// EaseInOutQuad 两端慢、中间快。
	EaseInOutQuad Easing = func(t float64) float64 {
		if t < 0.5 {
			return 2 * t * t
		}
		u := 1 - t
		return 1 - 2*u*u
	}

	// EaseOutCubic 三次减速，overlay 弹出常用。
	EaseOutCubic Easing = func(t float64) float64 {
		u := 1 - t
		return 1 - u*u*u
	}

	// EaseOutBack 回弹（越过终点约 10% 再回落）。
	EaseOutBack Easing = func(t float64) float64 {
		const c1 = 1.70158
		const c3 = c1 + 1
		u := t - 1
		return 1 + c3*u*u*u + c1*u*u
	}
)

// Tween 描述 From→To 的一次缓动插值。Value(now) 返回当前值；
// Done(now) 报告动画是否已结束（此时 Value 恒等于 To）。
type Tween struct {
	From, To float64
	Duration time.Duration
	Start    time.Time
	Ease     Easing
}

// NewTween 创建补间。Ease 为空时退化为线性插值；Start 取当前时间。
func NewTween(from, to float64, d time.Duration, e Easing) *Tween {
	if e == nil {
		e = EaseLinear
	}
	return &Tween{From: from, To: to, Duration: d, Start: time.Now(), Ease: e}
}

// Value 返回 now 时刻的插值结果。动画结束后返回 To（不越过终点，
// 保证最终帧与静态布局完全一致，避免残留动画偏移）。
func (t *Tween) Value(now time.Time) float64 {
	if t == nil || t.Duration <= 0 {
		if t == nil {
			return 0
		}
		return t.To
	}
	p := float64(now.Sub(t.Start)) / float64(t.Duration)
	if p >= 1 {
		return t.To
	}
	if p < 0 {
		p = 0
	}
	e := t.Ease
	if e == nil {
		e = EaseLinear
	}
	return t.From + (t.To-t.From)*e(p)
}

// Done 报告动画是否已结束（now ≥ Start+Duration，或 Duration ≤ 0）。
func (t *Tween) Done(now time.Time) bool {
	if t == nil {
		return true
	}
	return t.Duration <= 0 || !now.Before(t.Start.Add(t.Duration))
}
