package layout

import "github.com/xujian519/mady/tui/core"

// Direction controls how a Flex container arranges its children.
type Direction int

const (
	// DirectionVertical stacks children from top to bottom.
	DirectionVertical Direction = iota
	// DirectionHorizontal arranges children from left to right.
	DirectionHorizontal
)

// SizePolicy describes how a child is sized along the Flex main axis.
type SizePolicy int

const (
	// SizeNatural uses the component's natural height (Sizer if available,
	// otherwise a full Render-and-measure).
	SizeNatural SizePolicy = iota
	// SizeFixed assigns an explicit size from Child.Fixed.
	SizeFixed
	// SizeMin takes the larger of the natural size and Child.Min.
	SizeMin
	// SizeMax takes the smaller of the natural size and Child.Max.
	SizeMax
	// SizeFill distributes remaining space among Fill children, weighted by
	// Child.Weight.
	SizeFill
	// SizePercent assigns a percentage of the available container size.
	SizePercent
)

// BoundsProvider supplies the total available size for a Flex container.
// Terminal-backed layouts typically use the host that already implements
// TerminalSize().
type BoundsProvider interface {
	TerminalSize() (cols, rows int64)
}

// Child is one entry in a Flex container.
type Child struct {
	Component  core.Component
	Policy     SizePolicy
	Fixed      int64
	Min        int64
	Max        int64
	Weight     int
	Percent    int
	OnAllocate func(size int64)
}

// Rect describes a child's assigned screen rectangle.
type Rect struct {
	Row, Col      int64
	Width, Height int64
}

// Natural returns a Child that consumes its natural height.
func Natural(c core.Component) Child {
	return Child{Component: c, Policy: SizeNatural}
}

// Fixed returns a Child with a fixed main-axis size.
func Fixed(c core.Component, size int64) Child {
	return Child{Component: c, Policy: SizeFixed, Fixed: size}
}

// Fill returns a Child that expands to fill remaining space.
func Fill(c core.Component) Child {
	return Child{Component: c, Policy: SizeFill, Weight: 1}
}

// FillWeight returns a Fill child with a custom weight.
func FillWeight(c core.Component, weight int) Child {
	return Child{Component: c, Policy: SizeFill, Weight: weight}
}

// Percent returns a Child sized to a percentage of the container.
func Percent(c core.Component, pct int) Child {
	return Child{Component: c, Policy: SizePercent, Percent: pct}
}

// Min returns a Child whose size is at least min.
func Min(c core.Component, min int64) Child {
	return Child{Component: c, Policy: SizeMin, Min: min}
}

// Max returns a Child whose size is at most max.
func Max(c core.Component, max int64) Child {
	return Child{Component: c, Policy: SizeMax, Max: max}
}

// WithAllocate attaches an allocation callback to a Child.
func (ch Child) WithAllocate(fn func(size int64)) Child {
	ch.OnAllocate = fn
	return ch
}
