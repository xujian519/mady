package theme

// SemanticTheme holds named colors for the TUI (hex "#rrggbb", decimal "0"–"255"
// for 256 palette, or "" for terminal default foreground/background).
// JSON themes use the same keys as pi-mono coding-agent dark.json / light.json.
type SemanticTheme struct {
	Name string `json:"name"`

	Accent        string `json:"accent"`
	Border        string `json:"border"`
	BorderAccent  string `json:"borderAccent"`
	BorderMuted   string `json:"borderMuted"`
	Success       string `json:"success"`
	Error         string `json:"error"`
	Warning       string `json:"warning"`
	Muted         string `json:"muted"`
	Dim           string `json:"dim"`
	Text          string `json:"text"`
	System        string `json:"system"` // system / info lines (optional)
	ThinkingText  string `json:"thinkingText"`
	UserMessage   string `json:"userMessageText"` // if empty, derive from Accent + bold
	AssistantText string `json:"assistantText"`   // extension: not in pi JSON; optional

	SelectedBg    string `json:"selectedBg"`
	UserMessageBg string `json:"userMessageBg"`
	ToolPendingBg string `json:"toolPendingBg"`
	ToolSuccessBg string `json:"toolSuccessBg"`
	ToolErrorBg   string `json:"toolErrorBg"`

	MdHeading         string `json:"mdHeading"`
	MdLink            string `json:"mdLink"`
	MdLinkURL         string `json:"mdLinkUrl"`
	MdCode            string `json:"mdCode"`
	MdCodeBlock       string `json:"mdCodeBlock"`
	MdCodeBlockBorder string `json:"mdCodeBlockBorder"`
	MdQuote           string `json:"mdQuote"`
	MdQuoteBorder     string `json:"mdQuoteBorder"`
	MdHr              string `json:"mdHr"`
	MdListBullet      string `json:"mdListBullet"`

	SyntaxComment     string `json:"syntaxComment"`
	SyntaxKeyword     string `json:"syntaxKeyword"`
	SyntaxFunction    string `json:"syntaxFunction"`
	SyntaxVariable    string `json:"syntaxVariable"`
	SyntaxString      string `json:"syntaxString"`
	SyntaxNumber      string `json:"syntaxNumber"`
	SyntaxType        string `json:"syntaxType"`
	SyntaxOperator    string `json:"syntaxOperator"`
	SyntaxPunctuation string `json:"syntaxPunctuation"`

	LoaderSpinner string `json:"loaderSpinner"` // optional; default Accent
	ProgressBar   string `json:"progressBar"`   // optional

	// 背景与表面层次（Phase 1 mady-dark 品牌主题新增 token）
	Background    string `json:"background"`
	Surface       string `json:"surface"`
	SurfaceRaised string `json:"surfaceRaised"`

	// 证据与置信度可视化
	EvidenceSupport  string `json:"evidenceSupport"`
	EvidenceCounter  string `json:"evidenceCounter"`
	ConfidenceLow    string `json:"confidenceLow"`
	ConfidenceMedium string `json:"confidenceMedium"`
	ConfidenceHigh   string `json:"confidenceHigh"`
}

// DefaultSemanticDark uses Claude Code's warm dark palette.
func DefaultSemanticDark() *SemanticTheme {
	return &SemanticTheme{
		Name: "dark",

		Accent:       "#d4a843", // warm amber
		Border:       "#d4a843",
		BorderAccent: "#e6b84c",
		BorderMuted:  "#606070",
		Success:      "#7ec87b", // soft green
		Error:        "#e06c75", // soft red
		Warning:      "#d4a843", // amber
		Muted:        "#a0a0b0",
		Dim:          "#888898",
		Text:         "#e0e0e0",
		System:       "#d4a843",
		ThinkingText: "#9098a8",

		UserMessage:   "#e6b84c",
		AssistantText: "#e0e0e0",

		SelectedBg:    "#3c3c50",
		UserMessageBg: "#2d2d3f",
		ToolPendingBg: "#2d2d3f",
		ToolSuccessBg: "#283228",
		ToolErrorBg:   "#3c2828",

		MdHeading:         "#d4a843",
		MdLink:            "#61afef",
		MdLinkURL:         "#a0a0b0",
		MdCode:            "#98c379",
		MdCodeBlock:       "#98c379",
		MdCodeBlockBorder: "#606070",
		MdQuote:           "#a0a0b0",
		MdQuoteBorder:     "#d4a843",
		MdHr:              "#606070",
		MdListBullet:      "#d4a843",

		SyntaxComment:     "#9098a8",
		SyntaxKeyword:     "#c678dd",
		SyntaxFunction:    "#e5c07b",
		SyntaxVariable:    "#e06c75",
		SyntaxString:      "#98c379",
		SyntaxNumber:      "#d19a66",
		SyntaxType:        "#56b6c2",
		SyntaxOperator:    "#abb2bf",
		SyntaxPunctuation: "#abb2bf",

		LoaderSpinner: "#d4a843",
		ProgressBar:   "#d4a843",
	}
}

// DefaultSemanticLight is a readable light palette (not identical to pi light.json).
func DefaultSemanticLight() *SemanticTheme {
	return &SemanticTheme{
		Name: "light",

		Accent:       "#0066cc",
		Border:       "#335599",
		BorderAccent: "#0088aa",
		BorderMuted:  "#999999",
		Success:      "#2e7d32",
		Error:        "#c62828",
		Warning:      "#f57f17",
		Muted:        "#757575",
		Dim:          "#9e9e9e",
		Text:         "#212121",
		System:       "#bf360c",
		ThinkingText: "#757575",

		UserMessage:   "#006064",
		AssistantText: "#212121",

		SelectedBg:    "#e3f2fd",
		UserMessageBg: "#eceff1",
		ToolPendingBg: "#fff8e1",
		ToolSuccessBg: "#e8f5e9",
		ToolErrorBg:   "#ffebee",

		MdHeading:         "#b8860b",
		MdLink:            "#1565c0",
		MdLinkURL:         "#9e9e9e",
		MdCode:            "#00695c",
		MdCodeBlock:       "#2e7d32",
		MdCodeBlockBorder: "#bdbdbd",
		MdQuote:           "#616161",
		MdQuoteBorder:     "#9e9e9e",
		MdHr:              "#bdbdbd",
		MdListBullet:      "#0066cc",

		SyntaxComment:     "#008000",
		SyntaxKeyword:     "#0000ff",
		SyntaxFunction:    "#795e26",
		SyntaxVariable:    "#001080",
		SyntaxString:      "#a31515",
		SyntaxNumber:      "#098658",
		SyntaxType:        "#267f99",
		SyntaxOperator:    "#000000",
		SyntaxPunctuation: "#000000",

		LoaderSpinner: "#0066cc",
		ProgressBar:   "#1565c0",
	}
}

// DefaultMadyDark is the Mady brand dark theme using cold blue tones that match
// the Logo's deep-space blue + cyan light-arc palette. It conveys rationality and
// restraint suitable for professional patent/law workflows.
// The warm-amber DefaultSemanticDark remains available via /theme dark.
func DefaultMadyDark() *SemanticTheme {
	return &SemanticTheme{
		Name: "mady-dark",

		// 品牌冷色系：Logo 深空蓝 + 青蓝光弧
		Accent:       "#38C8F4",
		Border:       "#1D3B52",
		BorderAccent: "#5DDCFF",
		BorderMuted:  "#1D3B52",
		Success:      "#52D6A0",
		Error:        "#F17878",
		Warning:      "#D7B65C",
		Muted:        "#7892A5",
		Dim:          "#4B6378",
		Text:         "#DCEAF3",
		System:       "#5BC0EB",
		ThinkingText: "#7892A5",

		UserMessage:   "#5DDCFF",
		AssistantText: "#DCEAF3",

		SelectedBg:    "#164C63",
		UserMessageBg: "#102638",
		ToolPendingBg: "#102638",
		ToolSuccessBg: "#0F2A1E",
		ToolErrorBg:   "#2C1A1A",

		// Markdown 着色
		MdHeading:         "#38C8F4",
		MdLink:            "#5BC0EB",
		MdLinkURL:         "#7892A5",
		MdCode:            "#52D6A0",
		MdCodeBlock:       "#52D6A0",
		MdCodeBlockBorder: "#1D3B52",
		MdQuote:           "#7892A5",
		MdQuoteBorder:     "#38C8F4",
		MdHr:              "#1D3B52",
		MdListBullet:      "#38C8F4",

		// 语法高亮
		SyntaxComment:     "#7892A5",
		SyntaxKeyword:     "#CFA7FF",
		SyntaxFunction:    "#5DDCFF",
		SyntaxVariable:    "#F17878",
		SyntaxString:      "#52D6A0",
		SyntaxNumber:      "#D7B65C",
		SyntaxType:        "#5BC0EB",
		SyntaxOperator:    "#DCEAF3",
		SyntaxPunctuation: "#7892A5",

		LoaderSpinner: "#38C8F4",
		ProgressBar:   "#38C8F4",

		// Phase 1 新增 token：背景层次
		Background:    "#07111F",
		Surface:       "#0C1B2A",
		SurfaceRaised: "#102638",

		// 证据与置信度可视化
		EvidenceSupport:  "#5BC0EB",
		EvidenceCounter:  "#CFA7FF",
		ConfidenceLow:    "#D7B65C",
		ConfidenceMedium: "#38C8F4",
		ConfidenceHigh:   "#52D6A0",
	}
}
