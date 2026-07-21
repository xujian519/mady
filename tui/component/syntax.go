package component

import (
	"strings"
	"sync"
	"unicode"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Syntax — a lightweight source-code highlighter and Component.
//
// Design goals:
//   - Pluggable LangSpec values drive tokenisation — register new languages at
//     runtime via RegisterLanguage.
//   - Works as both a standalone Component and a helper embedded inside the
//     Markdown renderer (for fenced code blocks).
//
// Non-goals: spec-compliant parsing. The tokenizer is deliberately greedy and
// forgiving; it recognizes the most common lexical categories (keywords,
// types, strings, numbers, comments, function names, operators) and paints
// them with ANSI styles.
// ---------------------------------------------------------------------------

// TokenKind classifies a single token.
type TokenKind int64

const (
	TokText TokenKind = iota
	TokKeyword
	TokType
	TokString
	TokNumber
	TokComment
	TokFunction
	TokOperator
	TokPunctuation
)

// Token is one lexical element in a line of source code.
type Token struct {
	Kind TokenKind
	Text string
}

// LangSpec describes how to tokenise a language.
type LangSpec struct {
	// Name is a canonical identifier (lowercase), e.g. "go", "python".
	Name string
	// Aliases are additional identifiers (lowercased) matching this spec.
	Aliases []string

	// Keywords is a set of reserved words (exact match, case-sensitive).
	Keywords map[string]bool
	// Types is a set of built-in type names highlighted separately.
	Types map[string]bool

	// LineComment starts a comment that runs to end-of-line (e.g. "//").
	LineComment string
	// HashComment additionally enables "#..." style line comments.
	HashComment bool
	// BlockComment defines a paired block-comment delimiter, e.g. ["/*", "*/"].
	BlockComment [2]string

	// StringDelims lists the characters that open/close string literals.
	// Each entry may be single (") or multi-character (""").
	StringDelims []string

	// RawStringDelims lists delimiters that treat backslashes literally
	// (Go backtick, Python r"...", etc).
	RawStringDelims []string

	// DisableFunctionHighlight skips the ident-followed-by-'(' heuristic.
	DisableFunctionHighlight bool
}

// SyntaxTheme provides ANSI styling for each TokenKind.
type SyntaxTheme struct {
	TextFn        func(string) string
	KeywordFn     func(string) string
	TypeFn        func(string) string
	StringFn      func(string) string
	NumberFn      func(string) string
	CommentFn     func(string) string
	FunctionFn    func(string) string
	OperatorFn    func(string) string
	PunctuationFn func(string) string
}

// DefaultSyntaxTheme returns the built-in palette used when no theme is set.
func DefaultSyntaxTheme() SyntaxTheme {
	p := theme.CurrentPalette()
	sem := p.Semantic
	mode := p.Mode
	textFn := identStyleFn
	if strings.TrimSpace(sem.Text) != "" {
		textFn = theme.SemStyle(sem.Text, mode).Render
	}
	return SyntaxTheme{
		TextFn:        textFn,
		KeywordFn:     theme.SemStyle(sem.SyntaxKeyword, mode).Bold().Render,
		TypeFn:        theme.SemStyle(sem.SyntaxType, mode).Render,
		StringFn:      theme.SemStyle(sem.SyntaxString, mode).Render,
		NumberFn:      theme.SemStyle(sem.SyntaxNumber, mode).Render,
		CommentFn:     theme.SemStyle(sem.SyntaxComment, mode).Italic().Render,
		FunctionFn:    theme.SemStyle(sem.SyntaxFunction, mode).Render,
		OperatorFn:    theme.SemStyle(sem.SyntaxOperator, mode).Render,
		PunctuationFn: theme.SemStyle(sem.SyntaxPunctuation, mode).Render,
	}
}

// identStyleFn is the identity — no styling applied.
func identStyleFn(s string) string { return s }

func mergeSyntaxTheme(t SyntaxTheme) SyntaxTheme {
	d := DefaultSyntaxTheme()
	if t.TextFn == nil {
		t.TextFn = d.TextFn
	}
	if t.KeywordFn == nil {
		t.KeywordFn = d.KeywordFn
	}
	if t.TypeFn == nil {
		t.TypeFn = d.TypeFn
	}
	if t.StringFn == nil {
		t.StringFn = d.StringFn
	}
	if t.NumberFn == nil {
		t.NumberFn = d.NumberFn
	}
	if t.CommentFn == nil {
		t.CommentFn = d.CommentFn
	}
	if t.FunctionFn == nil {
		t.FunctionFn = d.FunctionFn
	}
	if t.OperatorFn == nil {
		t.OperatorFn = d.OperatorFn
	}
	if t.PunctuationFn == nil {
		t.PunctuationFn = d.PunctuationFn
	}
	return t
}

// renderToken applies the theme to one token.
func renderToken(tok Token, theme SyntaxTheme) string {
	switch tok.Kind {
	case TokKeyword:
		return theme.KeywordFn(tok.Text)
	case TokType:
		return theme.TypeFn(tok.Text)
	case TokString:
		return theme.StringFn(tok.Text)
	case TokNumber:
		return theme.NumberFn(tok.Text)
	case TokComment:
		return theme.CommentFn(tok.Text)
	case TokFunction:
		return theme.FunctionFn(tok.Text)
	case TokOperator:
		return theme.OperatorFn(tok.Text)
	case TokPunctuation:
		return theme.PunctuationFn(tok.Text)
	default:
		return theme.TextFn(tok.Text)
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var (
	langMu   sync.RWMutex
	langReg  = map[string]*LangSpec{}
	langInit sync.Once
)

// RegisterLanguage installs a LangSpec under its Name and all Aliases.
func RegisterLanguage(spec *LangSpec) {
	if spec == nil || spec.Name == "" {
		return
	}
	langMu.Lock()
	langReg[strings.ToLower(spec.Name)] = spec
	for _, a := range spec.Aliases {
		langReg[strings.ToLower(a)] = spec
	}
	langMu.Unlock()
}

// LookupLanguage returns the spec for the given language identifier (nil if
// unknown). The generic fallback (empty string, "text", "plain") maps to a
// minimal LangSpec that produces only TokText tokens.
func LookupLanguage(name string) *LangSpec {
	ensureDefaultLanguages()
	key := strings.ToLower(strings.TrimSpace(name))
	langMu.RLock()
	defer langMu.RUnlock()
	if spec, ok := langReg[key]; ok {
		return spec
	}
	return nil
}

// ---------------------------------------------------------------------------
// Syntax component
// ---------------------------------------------------------------------------

// Syntax is a Component that renders source code with ANSI highlighting.
type Syntax struct {
	mu       sync.RWMutex
	source   string
	language string
	theme    SyntaxTheme

	cacheWidth int64
	cacheLines []string
	dirty      bool
}

// NewSyntax creates a Syntax component for the given source / language.
func NewSyntax(source, language string) *Syntax {
	return &Syntax{
		source:   source,
		language: language,
		theme:    DefaultSyntaxTheme(),
		dirty:    true,
	}
}

// SetSource replaces the source text.
func (s *Syntax) SetSource(src string) {
	s.mu.Lock()
	s.source = src
	s.dirty = true
	s.mu.Unlock()
}

// SetLanguage replaces the language identifier.
func (s *Syntax) SetLanguage(lang string) {
	s.mu.Lock()
	s.language = lang
	s.dirty = true
	s.mu.Unlock()
}

// SetTheme installs a custom theme (missing fields inherit defaults).
func (s *Syntax) SetTheme(t SyntaxTheme) {
	s.mu.Lock()
	s.theme = mergeSyntaxTheme(t)
	s.dirty = true
	s.mu.Unlock()
}

// Render produces highlighted lines wrapped to width.
func (s *Syntax) Render(width int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty && s.cacheWidth == width && s.cacheLines != nil {
		return s.cacheLines
	}
	lines := Highlight(s.source, s.language, s.theme)
	if width > 0 {
		var wrapped []string
		for _, ln := range lines {
			wrapped = append(wrapped, core.WrapAnsi(ln, width)...)
		}
		lines = wrapped
	}
	s.cacheLines = lines
	s.cacheWidth = width
	s.dirty = false
	return lines
}

func (s *Syntax) Invalidate() {
	s.mu.Lock()
	s.dirty = true
	s.cacheLines = nil
	s.mu.Unlock()
}

func (s *Syntax) Update(msg core.Msg) core.Cmd { return nil }

// Highlight tokenises `source` under `language` and returns one styled
// string per source line. A nil / unknown language falls through to plain
// text.
func Highlight(source, language string, theme SyntaxTheme) []string {
	theme = mergeSyntaxTheme(theme)
	spec := LookupLanguage(language)
	rawLines := strings.Split(source, "\n")
	if spec == nil {
		out := make([]string, len(rawLines))
		for i, ln := range rawLines {
			out[i] = theme.TextFn(ln)
		}
		return out
	}
	tokensPerLine := tokenize(source, spec)
	out := make([]string, 0, len(tokensPerLine))
	for _, toks := range tokensPerLine {
		var b strings.Builder
		for _, t := range toks {
			b.WriteString(renderToken(t, theme))
		}
		out = append(out, b.String())
	}
	return out
}

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

// ---------------------------------------------------------------------------
// Built-in language specs
// ---------------------------------------------------------------------------

func ensureDefaultLanguages() {
	langInit.Do(func() {
		for _, spec := range builtinLanguages() {
			RegisterLanguage(spec)
		}
	})
}

func builtinLanguages() []*LangSpec {
	return []*LangSpec{
		goSpec(),
		pythonSpec(),
		jsSpec(),
		tsSpec(),
		rustSpec(),
		jsonSpec(),
		yamlSpec(),
		bashSpec(),
		plainSpec(),
	}
}

func asSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

func goSpec() *LangSpec {
	return &LangSpec{
		Name:    "go",
		Aliases: []string{"golang"},
		Keywords: asSet(
			"break", "case", "chan", "const", "continue", "default", "defer",
			"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
			"interface", "map", "package", "range", "return", "select", "struct",
			"switch", "type", "var", "true", "false", "nil", "iota",
		),
		Types: asSet(
			"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
			"int", "int8", "int16", "int32", "int64", "rune", "string", "uint",
			"uint8", "uint16", "uint32", "uint64", "uintptr", "any", "comparable",
		),
		LineComment:     "//",
		BlockComment:    [2]string{"/*", "*/"},
		StringDelims:    []string{`"`, `'`},
		RawStringDelims: []string{"`"},
	}
}

func pythonSpec() *LangSpec {
	return &LangSpec{
		Name:    "python",
		Aliases: []string{"py"},
		Keywords: asSet(
			"False", "None", "True", "and", "as", "assert", "async", "await",
			"break", "class", "continue", "def", "del", "elif", "else", "except",
			"finally", "for", "from", "global", "if", "import", "in", "is",
			"lambda", "nonlocal", "not", "or", "pass", "raise", "return", "try",
			"while", "with", "yield", "match", "case",
		),
		Types: asSet(
			"int", "float", "str", "bool", "list", "dict", "set", "tuple",
			"bytes", "bytearray", "object", "type", "frozenset", "complex",
			"range",
		),
		HashComment:  true,
		StringDelims: []string{`"""`, `'''`, `"`, `'`},
	}
}

func jsSpec() *LangSpec {
	return &LangSpec{
		Name:    "javascript",
		Aliases: []string{"js", "mjs", "cjs", "jsx"},
		Keywords: asSet(
			"await", "break", "case", "catch", "class", "const", "continue",
			"debugger", "default", "delete", "do", "else", "enum", "export",
			"extends", "false", "finally", "for", "function", "if", "import",
			"in", "instanceof", "let", "new", "null", "of", "return", "super",
			"switch", "this", "throw", "true", "try", "typeof", "undefined",
			"var", "void", "while", "with", "yield", "async", "static",
		),
		Types: asSet(
			"Array", "Boolean", "Date", "Error", "Function", "Map", "Number",
			"Object", "Promise", "RegExp", "Set", "String", "Symbol", "WeakMap",
			"WeakSet",
		),
		LineComment:  "//",
		BlockComment: [2]string{"/*", "*/"},
		StringDelims: []string{"`", `"`, `'`},
	}
}

func tsSpec() *LangSpec {
	spec := jsSpec()
	spec.Name = "typescript"
	spec.Aliases = []string{"ts", "tsx"}
	// TypeScript-specific keywords & types.
	for _, k := range []string{
		"abstract", "as", "declare", "interface", "is", "keyof", "namespace",
		"readonly", "satisfies", "type", "unique", "unknown",
	} {
		spec.Keywords[k] = true
	}
	for _, k := range []string{"any", "bigint", "boolean", "never", "number", "string", "symbol", "void"} {
		spec.Types[k] = true
	}
	return spec
}

func rustSpec() *LangSpec {
	return &LangSpec{
		Name:    "rust",
		Aliases: []string{"rs"},
		Keywords: asSet(
			"as", "break", "const", "continue", "crate", "dyn", "else", "enum",
			"extern", "false", "fn", "for", "if", "impl", "in", "let", "loop",
			"match", "mod", "move", "mut", "pub", "ref", "return", "self",
			"Self", "static", "struct", "super", "trait", "true", "type",
			"unsafe", "use", "where", "while", "async", "await", "box",
		),
		Types: asSet(
			"bool", "char", "f32", "f64", "i8", "i16", "i32", "i64", "i128",
			"isize", "str", "u8", "u16", "u32", "u64", "u128", "usize",
			"String", "Vec", "Option", "Result", "Box", "Rc", "Arc",
		),
		LineComment:     "//",
		BlockComment:    [2]string{"/*", "*/"},
		StringDelims:    []string{`"`},
		RawStringDelims: []string{`r"`, `r#"`},
	}
}

func jsonSpec() *LangSpec {
	return &LangSpec{
		Name:         "json",
		Keywords:     asSet("true", "false", "null"),
		StringDelims: []string{`"`},
		// JSON strictly has no comments, but we allow jsonc-style tolerance.
		LineComment:              "//",
		BlockComment:             [2]string{"/*", "*/"},
		DisableFunctionHighlight: true,
	}
}

func yamlSpec() *LangSpec {
	return &LangSpec{
		Name:                     "yaml",
		Aliases:                  []string{"yml"},
		Keywords:                 asSet("true", "false", "null", "yes", "no", "on", "off"),
		HashComment:              true,
		StringDelims:             []string{`"`, `'`},
		DisableFunctionHighlight: true,
	}
}

func bashSpec() *LangSpec {
	return &LangSpec{
		Name:    "bash",
		Aliases: []string{"sh", "shell", "zsh"},
		Keywords: asSet(
			"case", "do", "done", "elif", "else", "esac", "fi", "for", "function",
			"if", "in", "return", "select", "then", "until", "while", "time",
			"coproc", "break", "continue", "exit", "export", "local", "readonly",
			"source", "eval", "exec", "trap", "unset",
		),
		Types: asSet(
			"echo", "printf", "read", "set", "cd", "pwd", "true", "false",
		),
		HashComment:  true,
		StringDelims: []string{`"`, `'`},
	}
}

func plainSpec() *LangSpec {
	return &LangSpec{
		Name:                     "plain",
		Aliases:                  []string{"text", "txt", ""},
		DisableFunctionHighlight: true,
	}
}
