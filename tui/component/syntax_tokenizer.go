package component

import (
	"unicode"
)

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

// indexNewline returns the rune-offset of the next '\n' in r starting at i,
// or -1 if not found.
func indexNewline(r []rune, i int64) int64 {
	n := int64(len(r))
	for j := i; j < n; j++ {
		if r[j] == '\n' {
			return j - i
		}
	}
	return -1
}

// indexSubstr returns the rune-offset of the first occurrence of s in r[i:],
// or -1 if not found.  s must be a short delimiter string (2-4 chars).
func indexSubstr(r []rune, i int64, s string) int64 {
	sr := []rune(s)
	sLen := int64(len(sr))
	n := int64(len(r))
	if sLen == 0 || sLen > n-i {
		return -1
	}
	for j := i; j+sLen <= n; j++ {
		match := true
		for k := int64(0); k < sLen; k++ {
			if r[j+k] != sr[k] {
				match = false
				break
			}
		}
		if match {
			return j - i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// tokenize helpers — each handles one lexical category.
// All return (newI, token, handled); when handled is false the caller should
// try the next category.

// tokenizeComment handles block comment continuation across lines and
// block comment start.  It also manages the inBlockComment state flag.
func tokenizeComment(r []rune, i int64, spec *LangSpec, inBlockComment *bool) (int64, Token, bool) {
	if *inBlockComment {
		end := spec.BlockComment[1]
		if end != "" {
			if idx := indexSubstr(r, i, end); idx >= 0 {
				seg := string(r[i : i+idx+int64(len([]rune(end)))])
				*inBlockComment = false
				return i + idx + int64(len([]rune(end))), Token{Kind: TokComment, Text: seg}, true
			}
			// Till end-of-line.
			nlIdx := indexNewline(r, i)
			if nlIdx < 0 {
				return int64(len(r)), Token{Kind: TokComment, Text: string(r[i:])}, true
			}
			return i + nlIdx, Token{Kind: TokComment, Text: string(r[i : i+nlIdx])}, true
		}
		// Misconfigured spec — treat as text and bail out.
		*inBlockComment = false
		return i, Token{}, false
	}
	// Block comment start.
	if spec.BlockComment[0] != "" && hasPrefixAt(r, i, spec.BlockComment[0]) {
		end := spec.BlockComment[1]
		start := i
		i += int64(len([]rune(spec.BlockComment[0])))
		if end != "" {
			if idx := indexSubstr(r, i, end); idx >= 0 {
				closeIdx := i + idx + int64(len([]rune(end)))
				return closeIdx, Token{Kind: TokComment, Text: string(r[start:closeIdx])}, true
			}
		}
		// Continues past end-of-line.
		nlIdx := indexNewline(r, i)
		if nlIdx < 0 {
			*inBlockComment = true
			return int64(len(r)), Token{Kind: TokComment, Text: string(r[start:])}, true
		}
		*inBlockComment = true
		return i + nlIdx, Token{Kind: TokComment, Text: string(r[start : i+nlIdx])}, true
	}
	return i, Token{}, false
}

// tokenizeLineComment handles //-style and #-style line comments.
func tokenizeLineComment(r []rune, i int64, spec *LangSpec) (int64, Token, bool) {
	var ok bool
	switch {
	case spec.LineComment != "" && hasPrefixAt(r, i, spec.LineComment):
		ok = true
	case spec.HashComment && r[i] == '#':
		ok = true
	}
	if !ok {
		return i, Token{}, false
	}
	nlIdx := indexNewline(r, i)
	var seg string
	if nlIdx < 0 {
		seg = string(r[i:])
		i = int64(len(r))
	} else {
		seg = string(r[i : i+nlIdx])
		i += nlIdx
	}
	return i, Token{Kind: TokComment, Text: seg}, true
}

// tokenizeString handles raw and escaped string literals.
func tokenizeString(r []rune, i int64, spec *LangSpec) (int64, Token, bool) {
	if j, ok := matchStringStart(r, i, spec.RawStringDelims); ok {
		delim := spec.RawStringDelims[j]
		end := consumeRawString(r, i+int64(len(delim)), delim)
		return end, Token{Kind: TokString, Text: string(r[i:end])}, true
	}
	if j, ok := matchStringStart(r, i, spec.StringDelims); ok {
		delim := spec.StringDelims[j]
		end := consumeEscapedString(r, i+int64(len(delim)), delim)
		return end, Token{Kind: TokString, Text: string(r[i:end])}, true
	}
	return i, Token{}, false
}

// tokenizeNumber handles numeric literals (decimal, hex, octal, binary, float).
func tokenizeNumber(r []rune, i int64, n int64) (int64, Token, bool) {
	c := r[i]
	if !unicode.IsDigit(c) {
		if c != '.' || i+1 >= n || !unicode.IsDigit(r[i+1]) {
			return i, Token{}, false
		}
	}
	end := i + 1
	sawDot := c == '.'
	for end < n {
		ch := r[end]
		if unicode.IsDigit(ch) {
			end++
			continue
		}
		if ch == '.' && !sawDot {
			sawDot = true
			end++
			continue
		}
		// hex / oct / bin / e-notation / underscores
		if (ch == 'x' || ch == 'X' || ch == 'o' || ch == 'O' || ch == 'b' || ch == 'B' ||
			ch == 'e' || ch == 'E' || ch == '_' || ch == '+' || ch == '-') && end > i {
			// Only allow +/- directly after e/E.
			if (ch == '+' || ch == '-') && (r[end-1] != 'e' && r[end-1] != 'E') {
				break
			}
			end++
			continue
		}
		if isHexDigit(ch) && end > i+1 && (r[i+1] == 'x' || r[i+1] == 'X') {
			end++
			continue
		}
		break
	}
	return end, Token{Kind: TokNumber, Text: string(r[i:end])}, true
}

// tokenizeIdent handles identifiers, keywords, type names, and function calls.
func tokenizeIdent(r []rune, i int64, n int64, spec *LangSpec) (int64, Token, bool) {
	if !isIdentStart(r[i]) {
		return i, Token{}, false
	}
	end := i + 1
	for end < n && isIdentCont(r[end]) {
		end++
	}
	text := string(r[i:end])
	kind := TokText
	switch {
	case spec.Keywords != nil && spec.Keywords[text]:
		kind = TokKeyword
	case spec.Types != nil && spec.Types[text]:
		kind = TokType
	case !spec.DisableFunctionHighlight && end < n && r[end] == '(':
		kind = TokFunction
	}
	return end, Token{Kind: kind, Text: text}, true
}

// tokenizePunct handles operators and single punctuation characters.
func tokenizePunct(r []rune, i int64, n int64) (int64, Token, bool) {
	if isOperatorRune(r[i]) {
		end := i + 1
		for end < n && isOperatorRune(r[end]) {
			end++
		}
		return end, Token{Kind: TokOperator, Text: string(r[i:end])}, true
	}
	if isPunctuation(r[i]) {
		return i + 1, Token{Kind: TokPunctuation, Text: string(r[i])}, true
	}
	return i, Token{}, false
}

// tokenize produces tokens grouped by source line, respecting block comment
// continuations across lines.
func tokenize(source string, spec *LangSpec) [][]Token {
	r := []rune(source)
	var lines [][]Token
	var cur []Token
	flush := func() { lines = append(lines, cur); cur = nil }

	n := int64(len(r))
	i := int64(0)
	inBlockComment := false

	for i < n {
		c := r[i]

		if c == '\n' {
			flush()
			i++
			continue
		}

		var tok Token
		var handled bool

		if i, tok, handled = tokenizeComment(r, i, spec, &inBlockComment); handled {
			cur = append(cur, tok)
			continue
		}
		if i, tok, handled = tokenizeLineComment(r, i, spec); handled {
			cur = append(cur, tok)
			continue
		}
		if i, tok, handled = tokenizeString(r, i, spec); handled {
			cur = append(cur, tok)
			continue
		}
		if i, tok, handled = tokenizeNumber(r, i, n); handled {
			cur = append(cur, tok)
			continue
		}
		if i, tok, handled = tokenizeIdent(r, i, n, spec); handled {
			cur = append(cur, tok)
			continue
		}
		if i, tok, handled = tokenizePunct(r, i, n); handled {
			cur = append(cur, tok)
			continue
		}

		// Whitespace / anything else.
		cur = append(cur, Token{Kind: TokText, Text: string(c)})
		i++
	}
	flush()
	return lines
}

func hasPrefixAt(r []rune, i int64, prefix string) bool {
	pr := []rune(prefix)
	if i+int64(len(pr)) > int64(len(r)) {
		return false
	}
	for k := 0; k < len(pr); k++ {
		if r[i+int64(k)] != pr[k] {
			return false
		}
	}
	return true
}

// matchStringStart returns the index into delims matching r[i:], or (-1,false).
func matchStringStart(r []rune, i int64, delims []string) (int64, bool) {
	for j, d := range delims {
		if hasPrefixAt(r, i, d) {
			return int64(j), true
		}
	}
	return -1, false
}

// consumeEscapedString returns the index just past the closing delimiter,
// honoring backslash escapes and stopping at end-of-line if not terminated.
func consumeEscapedString(r []rune, start int64, delim string) int64 {
	n := int64(len(r))
	i := start
	for i < n {
		c := r[i]
		if c == '\\' && i+1 < n {
			i += 2
			continue
		}
		if c == '\n' {
			return i
		}
		if hasPrefixAt(r, i, delim) {
			return i + int64(len(delim))
		}
		i++
	}
	return n
}

// consumeRawString scans until the delimiter, ignoring backslash escapes.
func consumeRawString(r []rune, start int64, delim string) int64 {
	n := int64(len(r))
	i := start
	for i < n {
		if hasPrefixAt(r, i, delim) {
			return i + int64(len(delim))
		}
		i++
	}
	return n
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isOperatorRune(r rune) bool {
	switch r {
	case '+', '-', '*', '/', '%', '=', '<', '>', '!', '&', '|', '^', '~', '?':
		return true
	}
	return false
}

func isPunctuation(r rune) bool {
	switch r {
	case '(', ')', '[', ']', '{', '}', ',', ';', ':', '.', '@':
		return true
	}
	return false
}
