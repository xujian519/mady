package component

import (
	"fmt"
	"regexp"
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

// Render produces lines wrapped to the given width.
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

// ---------------------------------------------------------------------------
// Parser / renderer
//
// The renderer is split into two phases so the chat history can cache the
// per-block output of a streaming message and only re-render the tail block
// that is still growing:
//
//   1. parseBlocks(src) — a single-pass slicer that walks the source lines and
//      emits a []Block, where each Block records its kind and the raw source
//      lines it spans. The slicer preserves EXACTLY the same block-boundary
//      decisions the original single-pass renderer used (same regexes, same
//      lookahead, same greedy-paragraph rule).
//
//   2. renderBlock(b, width, theme) — renders ONE block to []string. This is
//      the per-block body of the old loop, factored out unchanged.
//
// renderMarkdown is now just `for _, b := range parseBlocks(src) { out += renderBlock(...) }`.
// The equivalence is pinned by TestRenderMarkdownEquivalenceGolden.
// ---------------------------------------------------------------------------

var (
	reHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reFence    = regexp.MustCompile(`^(` + "```" + `|~~~)\s*(\S*)\s*$`)
	reHR       = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	reBullet   = regexp.MustCompile(`^(\s*)([\-*+])\s+(.*)$`)
	reOrdered  = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	reQuote    = regexp.MustCompile(`^>\s?(.*)$`)
	reTableSep = regexp.MustCompile(`^\s*\|?(\s*:?-+:?\s*\|)+\s*:?-+:?\s*\|?\s*$`)

	reInlineBold   = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	reInlineItalic = regexp.MustCompile(`\*([^*\s][^*]*[^*\s]|[^*\s])\*|_([^_\s][^_]*[^_\s]|[^_\s])_`)
	reInlineCode   = regexp.MustCompile("`([^`]+)`")
	reInlineStrike = regexp.MustCompile(`~~([^~]+)~~`)
	reInlineLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// blockKind tags how a Block should be rendered.
type blockKind int

const (
	kindFence blockKind = iota
	kindHR
	kindHeading
	kindQuote
	kindTable
	kindBullet
	kindOrdered
	kindBlank
	kindParagraph
)

// Block is one markdown block: its kind plus the raw source lines it spans.
// Fence blocks carry the fence marker and language label separately because
// the closing fence line is not part of the code body.
type Block struct {
	Kind  blockKind
	Lines []string // raw source lines belonging to this block

	// Fence-only fields.
	Fence string // the ``` or ~~~ marker
	Lang  string // language label (may be "")

	// Closed indicates whether the block is definitely finished. For fence
	// blocks this is true only when a matching closing fence was seen; for
	// all other kinds it is always true. Streaming consumers (ChatHistory)
	// treat the trailing fence-or-paragraph block of a Pending message as
	// not-yet-closed so they re-render just that block on each delta.
	Closed bool
}

// parseBlocks slices src into blocks using the same boundary rules the
// historical single-pass renderer used. It does not render anything.
func parseBlocks(src string) []Block {
	lines := strings.Split(src, "\n")
	var blocks []Block
	i := 0
	for i < len(lines) {
		ln := lines[i]

		// Fenced code block.
		if fm := reFence.FindStringSubmatch(ln); fm != nil {
			fence := fm[1]
			lang := fm[2]
			start := i
			i++
			var codeLines []string
			closed := false
			for i < len(lines) {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
					closed = true
					i++ // consume closing fence
					break
				}
				codeLines = append(codeLines, lines[i])
				i++
			}
			b := Block{
				Kind: kindFence, Lines: codeLines,
				Fence: fence, Lang: lang, Closed: closed,
			}
			_ = start
			blocks = append(blocks, b)
			continue
		}

		if reHR.MatchString(ln) {
			blocks = append(blocks, Block{Kind: kindHR, Lines: []string{ln}, Closed: true})
			i++
			continue
		}

		if hm := reHeading.FindStringSubmatch(ln); hm != nil {
			blocks = append(blocks, Block{Kind: kindHeading, Lines: []string{ln}, Closed: true})
			i++
			continue
		}

		if reQuote.FindStringSubmatch(ln) != nil {
			blocks = append(blocks, Block{Kind: kindQuote, Lines: []string{ln}, Closed: true})
			i++
			continue
		}

		// Detect a table: a header row followed by a separator row.
		if strings.Contains(ln, "|") && i+1 < len(lines) && reTableSep.MatchString(lines[i+1]) {
			end := i + 2
			for end < len(lines) && strings.Contains(lines[end], "|") {
				end++
			}
			blocks = append(blocks, Block{Kind: kindTable, Lines: lines[i:end], Closed: true})
			i = end
			continue
		}

		if reBullet.FindStringSubmatch(ln) != nil {
			blocks = append(blocks, Block{Kind: kindBullet, Lines: []string{ln}, Closed: true})
			i++
			continue
		}
		if reOrdered.FindStringSubmatch(ln) != nil {
			blocks = append(blocks, Block{Kind: kindOrdered, Lines: []string{ln}, Closed: true})
			i++
			continue
		}

		if strings.TrimSpace(ln) == "" {
			blocks = append(blocks, Block{Kind: kindBlank, Lines: []string{ln}, Closed: true})
			i++
			continue
		}

		// Paragraph: join consecutive non-empty non-block lines.
		para := []string{ln}
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" &&
			!reHeading.MatchString(lines[i]) &&
			!reFence.MatchString(lines[i]) &&
			!reHR.MatchString(lines[i]) &&
			!reBullet.MatchString(lines[i]) &&
			!reOrdered.MatchString(lines[i]) &&
			!reQuote.MatchString(lines[i]) {
			para = append(para, lines[i])
			i++
		}
		blocks = append(blocks, Block{Kind: kindParagraph, Lines: para, Closed: true})
	}
	return blocks
}

// renderBlock renders a single Block to width using theme. This is the body
// of the original renderMarkdown loop, factored per-kind and unchanged.
func renderBlock(b Block, width int64, theme MarkdownTheme) []string {
	switch b.Kind {
	case kindFence:
		return renderFenceBlock(b.Lang, b.Lines, width, theme)
	case kindHR:
		return []string{theme.HRFn(core.PadToWidth(strings.Repeat("─", int(width)), width))}
	case kindHeading:
		hm := reHeading.FindStringSubmatch(b.Lines[0])
		if hm == nil {
			return nil
		}
		level := len(hm[1]) - 1
		if level > 5 {
			level = 5
		}
		text := renderInline(hm[2], theme)
		fn := theme.HeadingFn[level]
		wrapped := core.WrapAnsi(fn(text), width)
		out := make([]string, 0, len(wrapped))
		for _, w := range wrapped {
			out = append(out, core.PadToWidth(w, width))
		}
		return out
	case kindQuote:
		qm := reQuote.FindStringSubmatch(b.Lines[0])
		if qm == nil {
			return nil
		}
		text := renderInline(qm[1], theme)
		out := make([]string, 0, 2)
		for _, w := range core.WrapAnsi(text, width-2) {
			line := theme.QuoteFn("│ ") + w
			out = append(out, core.PadToWidth(line, width))
		}
		return out
	case kindTable:
		return renderTable(b.Lines, width, theme)
	case kindBullet:
		bm := reBullet.FindStringSubmatch(b.Lines[0])
		if bm == nil {
			return nil
		}
		indent := len(bm[1])
		text := renderInline(bm[3], theme)
		bullet := theme.ListBulletFn("• ")
		indentStr := strings.Repeat(" ", indent+2)
		out := make([]string, 0, 2)
		for k, w := range core.WrapAnsi(text, width-int64(indent)-3) {
			prefix := indentStr
			if k == 0 {
				prefix = strings.Repeat(" ", indent) + bullet
			}
			out = append(out, core.PadToWidth(prefix+w, width))
		}
		return out
	case kindOrdered:
		om := reOrdered.FindStringSubmatch(b.Lines[0])
		if om == nil {
			return nil
		}
		indent := len(om[1])
		num := om[2] + ". "
		text := renderInline(om[3], theme)
		indentStr := strings.Repeat(" ", indent+len(num))
		out := make([]string, 0, 2)
		for k, w := range core.WrapAnsi(text, width-int64(indent)-int64(len(num))) {
			prefix := indentStr
			if k == 0 {
				prefix = strings.Repeat(" ", indent) + theme.ListBulletFn(num)
			}
			out = append(out, core.PadToWidth(prefix+w, width))
		}
		return out
	case kindBlank:
		return []string{core.PadToWidth("", width)}
	case kindParagraph:
		joined := strings.Join(b.Lines, " ")
		text := renderInline(joined, theme)
		out := make([]string, 0, len(b.Lines))
		for _, w := range core.WrapAnsi(text, width) {
			out = append(out, core.PadToWidth(w, width))
		}
		return out
	}
	return nil
}

// renderFenceBlock renders a fenced code block. Split out of renderBlock so
// the fence-specific highlighter logic (diff coloring, syntax Highlight)
// stays readable on its own. Behavior is identical to the old inline branch.
func renderFenceBlock(lang string, codeLines []string, width int64, theme MarkdownTheme) []string {
	var out []string
	if lang != "" {
		out = append(out, theme.CodeFenceFn(core.PadToWidth("  "+lang, width)))
	}

	// If a language is specified and a highlighter exists, run it
	// over the whole block so multi-line block comments/strings
	// resolve correctly.
	var rendered []string
	if lang == "diff" {
		rendered = make([]string, len(codeLines))
		var oldLine, newLine int
		for k, cl := range codeLines {
			switch {
			case strings.HasPrefix(cl, "@@ "):
				// Parse hunk header to extract line numbers.
				rendered[k] = apitheme.CurrentPalette().Accent.Render(theme.CodeBlockFn(cl))
				if _, err := fmt.Sscanf(cl, "@@ -%d", &oldLine); err == nil {
					newLine = oldLine
					if idx := strings.Index(cl, "+"); idx > 0 {
						if _, err2 := fmt.Sscanf(cl[idx:], "+%d", &newLine); err2 != nil {
							newLine = oldLine
						}
					}
				}
			case strings.HasPrefix(cl, "+++ ") || strings.HasPrefix(cl, "--- "):
				rendered[k] = theme.CodeBlockFn(cl)
			case strings.HasPrefix(cl, "+") && !strings.HasPrefix(cl, "++"):
				rendered[k] = apitheme.CurrentPalette().Success.Render(fmt.Sprintf("%4d %s", newLine, theme.CodeBlockFn(cl)))
				newLine++
			case strings.HasPrefix(cl, "-") && !strings.HasPrefix(cl, "--"):
				rendered[k] = apitheme.CurrentPalette().Error.Render(fmt.Sprintf("%4d %s", oldLine, theme.CodeBlockFn(cl)))
				oldLine++
			default:
				rendered[k] = fmt.Sprintf("     %s", theme.CodeBlockFn(cl))
				oldLine++
				newLine++
			}
		}
	} else if spec := LookupLanguage(lang); spec != nil {
		rendered = Highlight(strings.Join(codeLines, "\n"), lang, syntaxThemeFromMarkdown(theme))
	} else {
		rendered = make([]string, len(codeLines))
		for k, cl := range codeLines {
			rendered[k] = theme.CodeBlockFn(cl)
		}
	}
	for _, cl := range rendered {
		if cl == "" {
			continue
		}
		out = append(out, core.PadToWidth("  "+cl, width))
	}
	return out
}

func renderMarkdown(src string, width int64, theme MarkdownTheme) []string {
	blocks := parseBlocks(src)
	var out []string
	for _, b := range blocks {
		out = append(out, renderBlock(b, width, theme)...)
	}
	return out
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

// ---------------------------------------------------------------------------
// Inline formatting
// ---------------------------------------------------------------------------

func renderInline(s string, t MarkdownTheme) string {
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "`")
		return t.CodeInlineFn(inner)
	})
	s = reInlineBold.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "*_")
		return t.StrongFn(inner)
	})
	s = reInlineStrike.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "~")
		return t.StrikeFn(inner)
	})
	s = reInlineItalic.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "*_")
		return t.EmphasisFn(inner)
	})
	s = reInlineLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineLink.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return t.LinkLabelFn(sub[1]) + " " + t.LinkURLFn("("+sub[2]+")")
	})
	return s
}

// ---------------------------------------------------------------------------
// Table rendering
// ---------------------------------------------------------------------------

func renderTable(rows []string, width int64, t MarkdownTheme) []string {
	if len(rows) < 2 {
		return nil
	}
	parse := func(r string) []string {
		r = strings.TrimSpace(r)
		r = strings.TrimPrefix(r, "|")
		r = strings.TrimSuffix(r, "|")
		parts := strings.Split(r, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	header := parse(rows[0])
	body := make([][]string, 0, len(rows)-2)
	for _, r := range rows[2:] {
		body = append(body, parse(r))
	}
	cols := len(header)

	// Column widths
	colW := make([]int64, cols)
	for i, h := range header {
		colW[i] = core.VisibleWidth(h)
	}
	for _, r := range body {
		for i := 0; i < cols && i < len(r); i++ {
			if w := core.VisibleWidth(r[i]); w > colW[i] {
				colW[i] = w
			}
		}
	}
	// Squeeze to fit viewport.
	total := int64(cols)*3 + 1
	for _, w := range colW {
		total += w
	}
	if total > width {
		excess := total - width
		for excess > 0 {
			idx := 0
			for i := range colW {
				if colW[i] > colW[idx] {
					idx = i
				}
			}
			if colW[idx] <= 3 {
				break
			}
			colW[idx]--
			excess--
		}
	}

	sep := func() string {
		var b strings.Builder
		b.WriteString("+")
		for _, w := range colW {
			b.WriteString(strings.Repeat("-", int(w)+2))
			b.WriteString("+")
		}
		return t.TableBorderFn(b.String())
	}
	row := func(cells []string, headerRow bool) string {
		var b strings.Builder
		b.WriteString(t.TableBorderFn("|"))
		for i, w := range colW {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if !headerRow {
				cell = renderInline(cell, t)
			}
			padded := core.PadToWidth(core.TruncateToWidth(cell, w, "…"), w)
			if headerRow {
				padded = t.TableHeaderFn(padded)
			}
			b.WriteString(" ")
			b.WriteString(padded)
			b.WriteString(" ")
			b.WriteString(t.TableBorderFn("|"))
		}
		return b.String()
	}
	out := []string{
		core.PadToWidth(sep(), width),
		core.PadToWidth(row(header, true), width),
		core.PadToWidth(sep(), width),
	}
	for _, r := range body {
		out = append(out, core.PadToWidth(row(r, false), width))
	}
	out = append(out, core.PadToWidth(sep(), width))
	return out
}

// ---------------------------------------------------------------------------
// Default theme
// ---------------------------------------------------------------------------

// DefaultMarkdownTheme returns the built-in markdown theme used when no
// custom theme is set.
func DefaultMarkdownTheme() MarkdownTheme { return defaultMarkdownTheme() }

func defaultMarkdownTheme() MarkdownTheme {
	p := apitheme.CurrentPalette()
	sem := p.Semantic
	mode := p.Mode
	h := func(s string) string {
		return apitheme.SemStyle(sem.MdHeading, mode).Bold().Render(s)
	}
	return MarkdownTheme{
		HeadingFn:     [6]func(string) string{h, h, h, h, h, h},
		EmphasisFn:    apitheme.NewStyle().Italic().Render,
		StrongFn:      apitheme.NewStyle().Bold().Render,
		StrikeFn:      apitheme.NewStyle().Strike().Render,
		CodeInlineFn:  apitheme.SemStyle(sem.MdCode, mode).Render,
		CodeBlockFn:   apitheme.SemStyle(sem.MdCodeBlock, mode).Render,
		CodeFenceFn:   apitheme.SemStyle(sem.MdCodeBlockBorder, mode).Render,
		QuoteFn:       apitheme.SemStyle(sem.MdQuote, mode).Render,
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
