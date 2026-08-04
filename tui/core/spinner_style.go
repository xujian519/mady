package core

import "time"

// SpinnerStyle defines the animation frames and speed for a spinner indicator.
// This is a pure data type with no rendering dependency, placed in core so
// the TUI component layer (Loader) can consume it without cross-dependencies.
type SpinnerStyle struct {
	Frames   []string
	Interval time.Duration
}

var (
	SpinnerDots = SpinnerStyle{
		Frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		Interval: 80 * time.Millisecond,
	}
	SpinnerLine = SpinnerStyle{
		Frames:   []string{"-", "\\", "|", "/"},
		Interval: 100 * time.Millisecond,
	}
	SpinnerBounce = SpinnerStyle{
		Frames:   []string{"⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"},
		Interval: 100 * time.Millisecond,
	}
	SpinnerGlobe = SpinnerStyle{
		Frames:   []string{"🌍", "🌎", "🌏"},
		Interval: 200 * time.Millisecond,
	}
	SpinnerMoon = SpinnerStyle{
		Frames:   []string{"🌑", "🌒", "🌓", "🌔", "🌕", "🌖", "🌗", "🌘"},
		Interval: 150 * time.Millisecond,
	}
	SpinnerCircle = SpinnerStyle{
		Frames:   []string{"◐", "◓", "◑", "◒"},
		Interval: 120 * time.Millisecond,
	}
	// SpinnerMady is the Mady brand spinner — a diamond rotation that
	// echoes the "中观" (middle view) philosophy: balanced, steady, precise.
	// Used as the default spinner in the Loader and status indicators.
	SpinnerMady = SpinnerStyle{
		Frames:   []string{"◈", "◆", "◇", "◆"},
		Interval: 100 * time.Millisecond,
	}
)
