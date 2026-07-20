package layout

import (
	"sync"

	"github.com/xujian519/mady/tui/core"
)

// Flex arranges children along a main axis using declarative size policies.
// It is intentionally dependency-free and reuses the existing Component
// interface so it can be embedded anywhere a core.Component is accepted.
type Flex struct {
	Direction Direction
	Bounds    BoundsProvider
	Children  []Child

	mu     sync.RWMutex
	rects  []Rect
	width  int64
	height int64
}

// NewFlex returns a Flex container with the given direction and children.
func NewFlex(dir Direction, children ...Child) *Flex {
	return &Flex{
		Direction: dir,
		Children:  children,
	}
}

// AddChild appends a child.
func (f *Flex) AddChild(c Child) {
	if c.Component == nil {
		return
	}
	f.mu.Lock()
	f.Children = append(f.Children, c)
	f.mu.Unlock()
}

// ChildRect returns the most recently assigned rectangle for child i.
func (f *Flex) ChildRect(i int) Rect {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if i < 0 || i >= len(f.rects) {
		return Rect{}
	}
	return f.rects[i]
}

// Render implements core.Component.
func (f *Flex) Render(width int64) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if width <= 0 {
		width = 80
	}
	f.width = width

	switch f.Direction {
	case DirectionHorizontal:
		return f.renderHorizontal(width)
	default:
		return f.renderVertical(width)
	}
}

// Invalidate fans out to all children.
func (f *Flex) Invalidate() {
	f.mu.Lock()
	children := make([]Child, len(f.Children))
	copy(children, f.Children)
	f.mu.Unlock()
	for _, ch := range children {
		ch.Component.Invalidate()
	}
}

func (f *Flex) totalHeight() int64 {
	if f.Bounds == nil {
		return 0
	}
	_, h := f.Bounds.TerminalSize()
	return h
}

func (f *Flex) totalWidth() int64 {
	if f.Bounds == nil {
		return 0
	}
	w, _ := f.Bounds.TerminalSize()
	return w
}

func (f *Flex) measureHeight(ch Child, width int64) int64 {
	if sizer, ok := ch.Component.(core.Sizer); ok {
		return sizer.Measure(width)
	}
	lines := ch.Component.Render(width)
	return int64(len(lines))
}

// reallocateShrinkable notifies a SizeShrinkable child of its new target
// height via OnAllocate (so it can shrink its own rendering, e.g. an editor
// reducing visible rows), re-renders it, and records the assigned size.
func (f *Flex) reallocateShrinkable(i int, newSize, width int64, rendered [][]string, sizes []int64) {
	ch := f.Children[i]
	if ch.OnAllocate != nil {
		ch.OnAllocate(newSize)
	}
	rendered[i] = ch.Component.Render(width)
	sizes[i] = newSize
}

func (f *Flex) renderVertical(width int64) []string {
	totalHeight := f.totalHeight()
	f.height = totalHeight

	n := len(f.Children)
	sizes := make([]int64, n)
	rendered := make([][]string, n)
	rects := make([]Rect, n)

	fillCount := 0
	totalWeight := 0
	used := int64(0)

	// First pass: measure non-fill children and count Fill children.
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		switch ch.Policy {
		case SizeNatural:
			lines := ch.Component.Render(width)
			rendered[i] = lines
			sizes[i] = int64(len(lines))
			used += sizes[i]
		case SizeShrinkable:
			// Measure at natural height first; the third pass squeezes it
			// down (toward Min) only if the container is over-committed.
			lines := ch.Component.Render(width)
			rendered[i] = lines
			sizes[i] = int64(len(lines))
			used += sizes[i]
		case SizeMin:
			h := f.measureHeight(ch, width)
			if h < ch.Min {
				h = ch.Min
			}
			if _, ok := ch.Component.(core.Sizer); !ok {
				rendered[i] = ch.Component.Render(width)
			}
			sizes[i] = h
			used += h
		case SizeMax:
			h := f.measureHeight(ch, width)
			if ch.Max > 0 && h > ch.Max {
				h = ch.Max
			}
			if _, ok := ch.Component.(core.Sizer); !ok {
				rendered[i] = ch.Component.Render(width)
			}
			sizes[i] = h
			used += h
		case SizeFixed:
			sizes[i] = ch.Fixed
			used += sizes[i]
		case SizePercent:
			if totalHeight > 0 {
				sizes[i] = totalHeight * int64(ch.Percent) / 100
			} else {
				lines := ch.Component.Render(width)
				rendered[i] = lines
				sizes[i] = int64(len(lines))
			}
			used += sizes[i]
		case SizeFill:
			fillCount++
			if ch.Weight > 0 {
				totalWeight += ch.Weight
			} else {
				totalWeight++
			}
		}
	}

	// Second pass: distribute remaining space among Fill children.
	if fillCount > 0 {
		remaining := totalHeight - used
		if totalHeight <= 0 {
			remaining = 0
		}
		if remaining < 0 {
			remaining = 0
		}
		perWeight := int64(0)
		if totalWeight > 0 {
			perWeight = remaining / int64(totalWeight)
		}
		allocated := int64(0)
		fi := 0
		for i, ch := range f.Children {
			if ch.Policy != SizeFill {
				continue
			}
			fi++
			weight := ch.Weight
			if weight <= 0 {
				weight = 1
			}
			h := perWeight * int64(weight)
			if fi == fillCount {
				h = remaining - allocated
			}
			if h < 1 {
				h = 1
			}
			sizes[i] = h
			if ch.OnAllocate != nil {
				ch.OnAllocate(h)
			}
			rendered[i] = ch.Component.Render(width)
			used += h
			allocated += h
		}
	}

	// Third pass: if the container is over-committed, squeeze SizeShrinkable
	// children (down toward their Min) until the total fits totalHeight. Fill
	// children have already been clamped to their min guard of 1, so only
	// Shrinkable children can still give back space.
	if totalHeight > 0 && used > totalHeight {
		over := used - totalHeight
		type shrinkEntry struct {
			idx   int
			slack int64
		}
		var entries []shrinkEntry
		totalSlack := int64(0)
		for i, ch := range f.Children {
			if ch.Policy != SizeShrinkable {
				continue
			}
			slack := sizes[i] - ch.Min
			if slack <= 0 {
				continue
			}
			entries = append(entries, shrinkEntry{i, slack})
			totalSlack += slack
		}
		if totalSlack > 0 {
			// Proportional cut: each Shrinkable child gives back a share of
			// the overflow proportional to its slack (distance above Min).
			cutTotal := int64(0)
			for _, e := range entries {
				cut := over * e.slack / totalSlack
				newSize := sizes[e.idx] - cut
				if min := f.Children[e.idx].Min; newSize < min {
					newSize = min
				}
				cutTotal += sizes[e.idx] - newSize
				f.reallocateShrinkable(e.idx, newSize, width, rendered, sizes)
			}
			// Greedy remainder: integer division leaves a small leftover;
			// trim one row at a time from any child still above its Min.
			rest := over - cutTotal
			for rest > 0 {
				progressed := false
				for _, e := range entries {
					if rest <= 0 {
						break
					}
					min := f.Children[e.idx].Min
					if sizes[e.idx] > min {
						f.reallocateShrinkable(e.idx, sizes[e.idx]-1, width, rendered, sizes)
						rest--
						progressed = true
					}
				}
				if !progressed {
					break
				}
			}
			used = 0
			for i := range f.Children {
				used += sizes[i]
			}
		}
	}

	// Compute rectangles and compose output.
	row := int64(0)
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		rects[i] = Rect{Row: row, Col: 0, Width: width, Height: sizes[i]}
		row += sizes[i]
	}
	f.rects = rects

	var out []string
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		lines := rendered[i]
		if lines == nil {
			lines = ch.Component.Render(width)
		}
		h := sizes[i]
		for r := int64(0); r < h; r++ {
			if r < int64(len(lines)) {
				out = append(out, core.PadToWidth(lines[r], width))
			} else {
				out = append(out, core.PadToWidth("", width))
			}
		}
	}
	// Safety net: even after shrinking Shrinkable children the total may still
	// exceed totalHeight (non-shrinkable children alone overfill the screen, or
	// a Shrinkable child ignored the OnAllocate target). Drop the excess from
	// the top so the bottom — input area and status bar, the user's focus —
	// stays visible rather than scrolling off-screen.
	if totalHeight > 0 && int64(len(out)) > totalHeight {
		trim := int64(len(out)) - totalHeight
		out = out[trim:]
		// Sync rects so ChildRect reflects actual screen positions: every child
		// shifts up by the trimmed rows. Children fully scrolled off the top
		// get a negative Row; callers translate mouse coords using these, so
		// they must match the rendered output (e.g. editorTop for the editor).
		for i := range f.rects {
			f.rects[i].Row -= trim
		}
	}
	return out
}

func (f *Flex) renderHorizontal(width int64) []string {
	totalWidth := f.totalWidth()
	if totalWidth <= 0 {
		// Without a bounded width, sum natural widths for a best-effort layout.
		totalWidth = width
	}
	f.width = totalWidth

	n := len(f.Children)
	sizes := make([]int64, n)
	rendered := make([][]string, n)
	rects := make([]Rect, n)

	fillCount := 0
	totalWeight := 0
	used := int64(0)
	maxHeight := int64(0)

	// First pass: measure non-fill children.
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		switch ch.Policy {
		case SizeNatural:
			// In horizontal mode natural width is ambiguous without a Sizer that
			// returns a width; treat it as the full parent width for now.
			sizes[i] = width
			used += sizes[i]
			lines := ch.Component.Render(sizes[i])
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
		case SizeMin:
			lines := ch.Component.Render(width)
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
			w := naturalWidth(lines)
			if w < ch.Min {
				w = ch.Min
			}
			sizes[i] = w
			used += w
		case SizeMax:
			lines := ch.Component.Render(width)
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
			w := naturalWidth(lines)
			if ch.Max > 0 && w > ch.Max {
				w = ch.Max
			}
			sizes[i] = w
			used += w
		case SizeFixed:
			sizes[i] = ch.Fixed
			used += sizes[i]
			lines := ch.Component.Render(sizes[i])
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
		case SizePercent:
			w := totalWidth * int64(ch.Percent) / 100
			sizes[i] = w
			used += w
			lines := ch.Component.Render(w)
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
		case SizeFill:
			fillCount++
			if ch.Weight > 0 {
				totalWeight += ch.Weight
			} else {
				totalWeight++
			}
		}
	}

	// Second pass: distribute remaining width among Fill children.
	if fillCount > 0 {
		remaining := totalWidth - used
		if remaining < 0 {
			remaining = 0
		}
		perWeight := int64(0)
		if totalWeight > 0 {
			perWeight = remaining / int64(totalWeight)
		}
		allocated := int64(0)
		fi := 0
		for i, ch := range f.Children {
			if ch.Policy != SizeFill {
				continue
			}
			fi++
			weight := ch.Weight
			if weight <= 0 {
				weight = 1
			}
			w := perWeight * int64(weight)
			if fi == fillCount {
				w = remaining - allocated
			}
			if w < 1 {
				w = 1
			}
			sizes[i] = w
			if ch.OnAllocate != nil {
				ch.OnAllocate(w)
			}
			lines := ch.Component.Render(w)
			rendered[i] = lines
			if h := int64(len(lines)); h > maxHeight {
				maxHeight = h
			}
			used += w
			allocated += w
		}
	}

	if maxHeight < 1 {
		maxHeight = 1
	}
	f.height = maxHeight

	// Compute rectangles.
	col := int64(0)
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		rects[i] = Rect{Row: 0, Col: col, Width: sizes[i], Height: maxHeight}
		col += sizes[i]
	}
	f.rects = rects

	// Compose horizontally.
	out := make([]string, maxHeight)
	for i, ch := range f.Children {
		if ch.Component == nil {
			continue
		}
		lines := rendered[i]
		if lines == nil {
			lines = ch.Component.Render(sizes[i])
		}
		w := sizes[i]
		for r := int64(0); r < maxHeight; r++ {
			var line string
			if r < int64(len(lines)) {
				line = core.PadToWidth(lines[r], w)
			} else {
				line = core.PadToWidth("", w)
			}
			out[r] += line
		}
	}
	return out
}

func naturalWidth(lines []string) int64 {
	max := int64(0)
	for _, l := range lines {
		if w := core.VisibleWidth(l); w > max {
			max = w
		}
	}
	return max
}
