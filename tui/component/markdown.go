package component

import (
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	apitheme "github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Markdown — a block-level markdown renderer component.
//
// Supported block types:
//   - ATX headings (# … ######).
//   - Fenced code blocks (``` / ~~~) with optional language label.
//   - Blockquotes (> ...).
//   - Bulleted lists (*, -, +) with nesting.
//   - Numbered lists (1. 2. 3.).
//   - Horizontal rule (---, ***, ___).
//   - Pipe tables.
//   - Paragraphs (word-wrapped).
//
// Supported inline styles:
//   - **bold** / __bold__
//   - *italic* / _italic_
//   - `code`
//   - ~~strike~~
//   - [label](url)
//
// The goal is readable terminal rendering; not a spec-compliant parser.
// ---------------------------------------------------------------------------

// MarkdownTheme overrides the ANSI styling of rendered elements.
type MarkdownTheme struct {
	HeadingFn     [6]func(string) string // h1..h6
	EmphasisFn    func(string) string    // italic
	StrongFn      func(string) string    // bold
	StrikeFn      func(string) string
	CodeInlineFn  func(string) string
	CodeBlockFn   func(string) string
	CodeFenceFn   func(string) string // language label line
	QuoteFn       func(string) string
	QuoteBorderFn func(string) string // quote left-border bar (MdQuoteBorder)
	LinkLabelFn   func(string) string
	LinkURLFn     func(string) string
	HRFn          func(string) string
	ListBulletFn  func(string) string
	TableBorderFn func(string) string
	TableHeaderFn func(string) string
	// Syntax, when set, is used to style fenced code blocks with a
	// language tag. A nil value falls back to CodeBlockFn.
	Syntax *SyntaxTheme
}

// syntaxThemeFromMarkdown bridges a MarkdownTheme into a SyntaxTheme so
// fenced code blocks can be highlighted by the Syntax tokenizer. Falls back
// to a palette derived from CodeBlockFn when Syntax is nil.
func syntaxThemeFromMarkdown(t MarkdownTheme) SyntaxTheme {
	if t.Syntax != nil {
		return *t.Syntax
	}
	dflt := DefaultSyntaxTheme()
	if t.CodeBlockFn != nil {
		dflt.TextFn = t.CodeBlockFn
		dflt.PunctuationFn = t.CodeBlockFn
		dflt.OperatorFn = t.CodeBlockFn
	}
	return dflt
}

// Markdown is a Component that renders a markdown string.
type Markdown struct {
	mu sync.RWMutex

	source string
	theme  MarkdownTheme

	cacheWidth int64
	cacheLines []string
	dirty      bool
}

// NewMarkdown creates a Markdown component.
func NewMarkdown(source string) *Markdown {
	return &Markdown{source: source, dirty: true, theme: defaultMarkdownTheme()}
}

// SetSource replaces the markdown content.
func (m *Markdown) SetSource(s string) {
	m.mu.Lock()
	m.source = s
	m.dirty = true
	m.mu.Unlock()
}

// SetTheme installs a custom theme (missing fields fall back to defaults).
func (m *Markdown) SetTheme(t MarkdownTheme) {
	m.mu.Lock()
	m.theme = mergeMarkdownTheme(t)
	m.dirty = true
	m.mu.Unlock()
}

// Render produces lines wrapped to the given width using the custom renderer.
func (m *Markdown) Render(width int64) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.dirty && m.cacheWidth == width && m.cacheLines != nil {
		return m.cacheLines
	}
	lines := renderMarkdown(m.source, width, m.theme)
	m.cacheLines = lines
	m.cacheWidth = width
	m.dirty = false
	return lines
}

func (m *Markdown) Invalidate() {
	m.mu.Lock()
	m.dirty = true
	m.cacheLines = nil
	m.mu.Unlock()
}

func (m *Markdown) Update(msg core.Msg) core.Cmd {
	if _, ok := msg.(core.WindowSizeMsg); ok {
		m.Invalidate()
	}
	return nil
}

// BlockCache caches the rendered lines of individual markdown blocks so a
// streaming source can re-render cheaply: only blocks whose raw text or
// Closed status changed are re-rendered; the rest are reused as-is.
//
// It is used by ChatHistory for Pending (still-streaming) assistant messages,
// where the source grows by small deltas. Without it, every delta re-runs
// renderMarkdown over the entire accumulated source — O(N²) in the message
// length.
//
// The cache is keyed on (blockRaw, blockKind, width). It does NOT depend on
// theme here because ChatHistory clears the whole msgCache on theme change
// (SetTheme → clearMsgCacheLocked), so a stale theme never surfaces.
type BlockCache struct {
	// entries[i] corresponds to the i-th block at the time of the last
	// RenderBlocksIncremental call. Each entry caches that block's rendered
	// lines keyed on its width.
	entries []blockCacheEntry
}

type blockCacheEntry struct {
	kind     blockKind
	raw      string // block source (Lines joined by "\n")
	closed   bool
	width    int64
	rendered []string
}

// RenderBlocksIncremental renders blocks to width using theme, reusing the
// per-block cache for any block whose kind/raw/closed/width matches its prior
// rendering. It returns the concatenated lines and updates the cache in place
// to reflect the current blocks.
//
// A block is re-rendered when any of: its Kind changed, its raw text changed,
// its Closed flag changed, or the target width changed. The trailing block of
// a streaming message typically flips Closed (or grows raw) on each delta, so
// only it pays the render cost; earlier blocks are O(1) lookups.
func (c *BlockCache) RenderBlocksIncremental(blocks []Block, width int64, theme MarkdownTheme) []string {
	if c == nil {
		// No cache: render everything fresh (degenerates to renderMarkdown).
		var out []string
		for _, b := range blocks {
			out = append(out, renderBlock(b, width, theme)...)
		}
		return out
	}

	// Reuse the entries slice capacity when the block count is stable.
	newEntries := make([]blockCacheEntry, len(blocks))
	var out []string
	for i, b := range blocks {
		raw := joinBlockLines(b)
		// Cache hit: same kind, same raw, same closed, same width as last time.
		if i < len(c.entries) {
			e := c.entries[i]
			if e.kind == b.Kind && e.raw == raw && e.closed == b.Closed && e.width == width && e.rendered != nil {
				newEntries[i] = e
				out = append(out, e.rendered...)
				continue
			}
		}
		rendered := renderBlock(b, width, theme)
		newEntries[i] = blockCacheEntry{
			kind: b.Kind, raw: raw, closed: b.Closed, width: width, rendered: rendered,
		}
		out = append(out, rendered...)
	}
	c.entries = newEntries
	return out
}

// joinBlockLines joins a block's raw source lines for use as a cache key.
func joinBlockLines(b Block) string {
	if len(b.Lines) == 0 {
		return ""
	}
	if len(b.Lines) == 1 {
		return b.Lines[0]
	}
	return strings.Join(b.Lines, "\n")
}

// Entries returns the number of per-block cache entries currently held. It
// exists primarily for tests that assert the cache is reused across deltas
// rather than rebuilt from scratch.
func (c *BlockCache) Entries() int {
	if c == nil {
		return 0
	}
	return len(c.entries)
}

// RenderMarkdownIncremental is a convenience wrapper that parses src and
// renders it with a BlockCache. Intended for callers that hold a long-lived
// Markdown render state (e.g. ChatHistory's per-message cache).
func RenderMarkdownIncremental(src string, width int64, theme MarkdownTheme, cache *BlockCache) []string {
	blocks := parseBlocks(src)
	return cache.RenderBlocksIncremental(blocks, width, theme)
}

// DefaultMarkdownTheme returns the default theme used when no custom theme
// is set.
func DefaultMarkdownTheme() MarkdownTheme { return defaultMarkdownTheme() }

func defaultMarkdownTheme() MarkdownTheme {
	p := apitheme.CurrentPalette()
	sem := p.Semantic
	mode := p.Mode
	h1 := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Bold().Underline().Render(s)
	}
	h23 := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Bold().Render(s)
	}
	h4 := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Render(s)
	}
	h5 := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Dim().Render(s)
	}
	h6 := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Dim().Render(s)
	}
	return MarkdownTheme{
		HeadingFn:     [6]func(string) string{h1, h23, h23, h4, h5, h6},
		EmphasisFn:    apitheme.NewStyle().Italic().Render,
		StrongFn:      apitheme.NewStyle().Bold().Render,
		StrikeFn:      apitheme.NewStyle().Strike().Render,
		CodeInlineFn:  apitheme.SemStyle(sem.MdCode, mode).Render,
		CodeBlockFn:   apitheme.SemStyle(sem.MdCodeBlock, mode).Render,
		CodeFenceFn:   apitheme.SemStyle(sem.MdCodeBlockBorder, mode).Render,
		QuoteFn:       apitheme.SemStyle(sem.MdQuote, mode).Render,
		QuoteBorderFn: apitheme.SemStyle(sem.MdQuoteBorder, mode).Render,
		LinkLabelFn:   apitheme.SemStyle(sem.MdLink, mode).Underline().Render,
		LinkURLFn:     apitheme.SemStyle(sem.MdLinkURL, mode).Render,
		HRFn:          apitheme.SemStyle(sem.MdHr, mode).Render,
		ListBulletFn:  apitheme.SemStyle(sem.MdListBullet, mode).Render,
		TableBorderFn: apitheme.SemStyle(sem.MdCodeBlockBorder, mode).Render,
		TableHeaderFn: apitheme.NewStyle().Bold().Render,
	}
}

func mergeMarkdownTheme(t MarkdownTheme) MarkdownTheme {
	d := defaultMarkdownTheme()
	if t.EmphasisFn != nil {
		d.EmphasisFn = t.EmphasisFn
	}
	if t.StrongFn != nil {
		d.StrongFn = t.StrongFn
	}
	if t.StrikeFn != nil {
		d.StrikeFn = t.StrikeFn
	}
	if t.CodeInlineFn != nil {
		d.CodeInlineFn = t.CodeInlineFn
	}
	if t.CodeBlockFn != nil {
		d.CodeBlockFn = t.CodeBlockFn
	}
	if t.CodeFenceFn != nil {
		d.CodeFenceFn = t.CodeFenceFn
	}
	if t.QuoteFn != nil {
		d.QuoteFn = t.QuoteFn
	}
	if t.QuoteBorderFn != nil {
		d.QuoteBorderFn = t.QuoteBorderFn
	}
	if t.LinkLabelFn != nil {
		d.LinkLabelFn = t.LinkLabelFn
	}
	if t.LinkURLFn != nil {
		d.LinkURLFn = t.LinkURLFn
	}
	if t.HRFn != nil {
		d.HRFn = t.HRFn
	}
	if t.ListBulletFn != nil {
		d.ListBulletFn = t.ListBulletFn
	}
	if t.TableBorderFn != nil {
		d.TableBorderFn = t.TableBorderFn
	}
	if t.TableHeaderFn != nil {
		d.TableHeaderFn = t.TableHeaderFn
	}
	for i, fn := range t.HeadingFn {
		if fn != nil {
			d.HeadingFn[i] = fn
		}
	}
	return d
}
