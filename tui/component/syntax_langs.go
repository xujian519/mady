package component

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
