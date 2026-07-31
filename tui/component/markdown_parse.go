package component

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Parser phase: parseBlocks(src) is a single-pass slicer that walks the
// source lines and emits a []Block, where each Block records its kind and
// the raw source lines it spans. renderBlock (markdown_render.go) renders
// ONE block to []string. renderMarkdown is just
// `for _, b := range parseBlocks(src) { out += renderBlock(...) }`.
//
// Splitting the two phases lets the chat history cache the per-block
// output of a streaming message and only re-render the tail block that
// is still growing.
// ---------------------------------------------------------------------------

// extractHeadingText returns the visible heading text, preserving any emoji or
// symbol decorations that appear before the hash sequence (e.g. "🏆### 标题" →
// "🏆 标题"). The hash sequence itself is removed.
func extractHeadingText(line string, level int) string {
	s := strings.TrimLeft(line, " \t")
	// Find the hash sequence start, collecting decorations before it.
	hashStart := 0
	for hashStart < len(s) {
		r, size := utf8.DecodeRuneInString(s[hashStart:])
		if r == utf8.RuneError && size <= 1 {
			break
		}
		if r == '#' {
			break
		}
		if !isHeadingDecorationRune(r) {
			break
		}
		hashStart += size
	}
	prefix := strings.TrimSpace(s[:hashStart])
	text := strings.TrimSpace(s[hashStart+level:])
	if prefix != "" && text != "" {
		return prefix + " " + text
	}
	return prefix + text
}

// isHeadingDecorationRune reports whether r is an emoji or common symbol that
// LLMs frequently place before ATX headings (e.g. "🏆### 一、Top 07"). These
// characters are skipped when looking for the leading hash sequence.
func isHeadingDecorationRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1F64F: // Misc Symbols & Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport & Map
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Symbols & Pictographs Ext A
		return true
	case r >= 0x2600 && r <= 0x26FF: // Misc symbols
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return true
	case r >= 0x2B00 && r <= 0x2BFF: // Stars, arrows, zodiac, etc.
		return true
	case r >= 0x1F650 && r <= 0x1F67F: // Ornamental Dingbats (❝ ❞ ❡ …)
		return true
	case r >= 0x1F780 && r <= 0x1F7FF: // Geometric Shapes Extended (🞄 …)
		return true
	}
	return false
}

// parseATXHeading parses an ATX heading line. It is more lenient than
// CommonMark to accommodate real-world LLM output:
//   - Optional leading whitespace.
//   - Optional leading emoji/symbol decorations (e.g. 🏆, ⭐).
//   - Optional space between the hash sequence and the heading text.
//
// It returns the heading level (1..6), the heading text, and whether the line
// is a heading at all.
func parseATXHeading(line string) (level int, text string, ok bool) {
	s := strings.TrimLeft(line, " \t")
	for {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size <= 1 {
			break
		}
		if !isHeadingDecorationRune(r) {
			break
		}
		s = s[size:]
	}
	if len(s) == 0 || s[0] != '#' {
		return 0, "", false
	}
	for level < len(s) && s[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	return level, strings.TrimSpace(s[level:]), true
}

// isTableStart reports whether lines[i] begins a pipe table and is followed by
// a separator line. This is used by consumeParagraph to stop before a table.
func isTableStart(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	if !strings.Contains(lines[i], "|") || !strings.Contains(lines[i+1], "|") {
		return false
	}
	return reTableSep.MatchString(lines[i+1])
}

var (
	reFence    = regexp.MustCompile(`^(` + "```" + `|~~~)\s*(\S*)\s*$`)
	reHR       = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	reBullet   = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	reOrdered  = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	reQuote    = regexp.MustCompile(`^>\s?(.*)$`)
	reTableSep = regexp.MustCompile(`^\s*\|?(\s*:?-+:?\s*\|)+\s*:?-+:?\s*\|?\s*$`)
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

// isListContinuation reports whether line is a continuation line for a list
// item whose bullet starts at itemIndent.
func isListContinuation(line string, itemIndent int) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
	if leadingSpaces < itemIndent+2 {
		return false
	}
	// Not a block-level element
	if _, _, ok := parseATXHeading(line); ok || reFence.MatchString(line) || reHR.MatchString(line) || reQuote.MatchString(line) {
		return false
	}
	return true
}

// parseBlocks slices src into blocks. Each block type is consumed by a small
// tryConsume* helper returning (Block, nextIndex). If nextIndex == start the
// helper did not consume the line and parseBlocks falls through to the next
// candidate. The final default is consumeParagraph, which always consumes at
// least one line. This keeps parseBlocks a simple priority dispatcher.
func parseBlocks(src string) []Block {
	lines := strings.Split(src, "\n")
	var blocks []Block
	i := 0
	for i < len(lines) {
		if b, next := tryConsumeFenceBlock(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeHR(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeHeading(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeQuote(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeTable(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeListItem(lines, i, reBullet, kindBullet); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeListItem(lines, i, reOrdered, kindOrdered); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		if b, next := tryConsumeBlank(lines, i); next > i {
			blocks = append(blocks, b)
			i = next
			continue
		}
		b, next := consumeParagraph(lines, i)
		blocks = append(blocks, b)
		i = next
	}
	return blocks
}

// tryConsumeFenceBlock attempts to consume a fenced code block starting at i.
// It returns the block and the index after the block, or i if no fence starts
// at this line.
func tryConsumeFenceBlock(lines []string, i int) (Block, int) {
	fm := reFence.FindStringSubmatch(lines[i])
	if fm == nil {
		return Block{}, i
	}
	fence := fm[1]
	lang := fm[2]
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
	return Block{
		Kind: kindFence, Lines: codeLines,
		Fence: fence, Lang: lang, Closed: closed,
	}, i
}

// tryConsumeHR attempts to consume a horizontal rule at i.
func tryConsumeHR(lines []string, i int) (Block, int) {
	if !reHR.MatchString(lines[i]) {
		return Block{}, i
	}
	return Block{Kind: kindHR, Lines: []string{lines[i]}, Closed: true}, i + 1
}

// tryConsumeHeading attempts to consume an ATX heading at i.
func tryConsumeHeading(lines []string, i int) (Block, int) {
	if _, _, ok := parseATXHeading(lines[i]); !ok {
		return Block{}, i
	}
	return Block{Kind: kindHeading, Lines: []string{lines[i]}, Closed: true}, i + 1
}

// tryConsumeQuote attempts to consume a blockquote line at i.
func tryConsumeQuote(lines []string, i int) (Block, int) {
	if reQuote.FindStringSubmatch(lines[i]) == nil {
		return Block{}, i
	}
	return Block{Kind: kindQuote, Lines: []string{lines[i]}, Closed: true}, i + 1
}

// tryConsumeTable attempts to consume a pipe table starting at i.
// It returns the block and the index after the table, or i if no table starts
// at this line.
func tryConsumeTable(lines []string, i int) (Block, int) {
	ln := lines[i]
	if !strings.Contains(ln, "|") || i+1 >= len(lines) || !strings.Contains(lines[i+1], "|") {
		return Block{}, i
	}
	end := i + 2
	for end < len(lines) && strings.Contains(lines[end], "|") {
		end++
	}
	return Block{Kind: kindTable, Lines: lines[i:end], Closed: true}, end
}

// tryConsumeListItem parses one list item (bulleted or ordered) starting at
// lines[start] using the provided list regex and kind. It returns the block
// and the index after the item, or start if this line does not begin a valid
// list item.
func tryConsumeListItem(lines []string, start int, re *regexp.Regexp, kind blockKind) (Block, int) {
	m := re.FindStringSubmatch(lines[start])
	if m == nil {
		return Block{}, start
	}
	itemIndent := len(m[1])
	itemLines := []string{lines[start]}
	i := start + 1
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "" {
			if i+1 < len(lines) && isListContinuation(lines[i+1], itemIndent) {
				itemLines = append(itemLines, lines[i])
				i++
				continue
			}
			break
		}
		if nextBullet := reBullet.FindStringSubmatch(lines[i]); nextBullet != nil && len(nextBullet[1]) <= itemIndent {
			break
		}
		if nextOrdered := reOrdered.FindStringSubmatch(lines[i]); nextOrdered != nil && len(nextOrdered[1]) <= itemIndent {
			break
		}
		if _, _, ok := parseATXHeading(lines[i]); ok || reFence.MatchString(lines[i]) || reHR.MatchString(lines[i]) || reQuote.MatchString(lines[i]) {
			break
		}
		// A non-blank continuation must have enough indentation to belong
		// to this list item; otherwise it starts a new paragraph.
		if !isListContinuation(lines[i], itemIndent) {
			break
		}
		itemLines = append(itemLines, lines[i])
		i++
	}
	return Block{Kind: kind, Lines: itemLines, Closed: true}, i
}

// tryConsumeBlank attempts to consume a blank line at i.
func tryConsumeBlank(lines []string, i int) (Block, int) {
	if strings.TrimSpace(lines[i]) != "" {
		return Block{}, i
	}
	return Block{Kind: kindBlank, Lines: []string{lines[i]}, Closed: true}, i + 1
}

// consumeParagraph consumes consecutive non-empty non-block lines starting at i
// as a single paragraph block. It always consumes at least one line.
func consumeParagraph(lines []string, i int) (Block, int) {
	para := []string{lines[i]}
	i++
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" &&
		!reFence.MatchString(lines[i]) &&
		!reHR.MatchString(lines[i]) &&
		!reBullet.MatchString(lines[i]) &&
		!reOrdered.MatchString(lines[i]) &&
		!reQuote.MatchString(lines[i]) &&
		!isTableStart(lines, i) {
		// Stop before any ATX heading as well (including lenient forms with emoji
		// prefixes or no space after hashes).
		if _, _, ok := parseATXHeading(lines[i]); ok {
			break
		}
		para = append(para, lines[i])
		i++
	}
	return Block{Kind: kindParagraph, Lines: para, Closed: true}, i
}
