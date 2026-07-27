package stdio

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	core "github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// Spinner displays an animated progress indicator in the terminal.
type Spinner struct {
	style   core.SpinnerStyle
	message string
	color   theme.Style
	writer  io.Writer

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewSpinner creates a Spinner with the given animation style. Output goes to
// os.Stdout with the loader-spinner color from the current palette.
func NewSpinner(style core.SpinnerStyle) *Spinner {
	return &Spinner{
		style:  style,
		writer: os.Stdout,
		color:  theme.CurrentPalette().LoaderSpinner,
	}
}

// SetMessage updates the spinner's label text. SetColor overrides the spinner
// Style. SetWriter redirects output (default os.Stdout). All are thread-safe.
func (s *Spinner) SetMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
}

// SetColor overrides the spinner's color Style.
func (s *Spinner) SetColor(c theme.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.color = c
}

// SetWriter redirects spinner output.
func (s *Spinner) SetWriter(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writer = w
}

// Start begins the animation goroutine, displaying the given message.
// Calling Start while already running is a no-op.
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.message = message
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	go s.animate()
}

// Stop halts the animation and clears the spinner line. Safe to call when
// not running (no-op).
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
	<-s.doneCh

	_, _ = fmt.Fprint(s.writer, "\r"+terminal.ClearLine())
}

// StopWith halts the spinner and prints finalMessage on the cleared line.
func (s *Spinner) StopWith(finalMessage string) {
	s.Stop()
	_, _ = fmt.Fprintln(s.writer, finalMessage)
}

// StopSuccess halts the spinner and prints a check-marked success message.
func (s *Spinner) StopSuccess(msg string) {
	s.Stop()
	pal := theme.CurrentPalette()
	_, _ = fmt.Fprintln(s.writer, pal.Success.Render(theme.SymbolCheck)+" "+msg)
}

// StopFail halts the spinner and prints a cross-marked failure message.
func (s *Spinner) StopFail(msg string) {
	s.Stop()
	pal := theme.CurrentPalette()
	_, _ = fmt.Fprintln(s.writer, pal.Error.Render(theme.SymbolCross)+" "+msg)
}

func (s *Spinner) animate() {
	defer close(s.doneCh)

	idx := 0
	ticker := time.NewTicker(s.style.Interval)
	defer ticker.Stop()

	_, _ = fmt.Fprint(s.writer, terminal.HideCursor())
	defer func() { _, _ = fmt.Fprint(s.writer, terminal.ShowCursor()) }()

	for {
		s.mu.Lock()
		frame := s.color.Render(s.style.Frames[idx%len(s.style.Frames)])
		msg := s.message
		s.mu.Unlock()

		_, _ = fmt.Fprintf(s.writer, "\r%s %s %s", terminal.ClearLine(), frame, msg)
		idx++

		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
	}
}

// WithSpinner runs fn while showing a spinner. Returns fn's result.
func WithSpinner[T any](message string, fn func() (T, error)) (T, error) {
	sp := NewSpinner(core.SpinnerDots)
	sp.Start(message)
	result, err := fn()
	if err != nil {
		sp.StopFail(err.Error())
	} else {
		sp.StopSuccess(message)
	}
	return result, err
}
