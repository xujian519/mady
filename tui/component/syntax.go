package component

import (
	"strings"
	"sync"

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
