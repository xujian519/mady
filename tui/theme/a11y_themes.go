package theme

// a11y_themes.go — accessibility-enhanced themes.
//
// HighContrast uses pure black/white with bold borders for visually impaired
// users who need maximum luminance contrast.
//
// ColorBlind (deuteranopia/protanopia safe) replaces red-green color coding
// with a blue-orange palette, and ensures information is never conveyed by
// color alone (text labels / shapes supplement).

// HighContrast returns a theme with maximum luminance contrast.
// Black background, white/light text, and bold border colors for users
// with low vision or who work in bright environments.
func HighContrast() *SemanticTheme {
	return &SemanticTheme{
		Name: "high-contrast",

		Accent:       "#FFFFFF",
		Border:       "#FFFFFF",
		BorderAccent: "#FFFFFF",
		BorderMuted:  "#AAAAAA",
		Success:      "#00FF00",
		Error:        "#FF4444",
		Warning:      "#FFFF00",
		Muted:        "#CCCCCC",
		Dim:          "#999999",
		Text:         "#FFFFFF",
		System:       "#FFFFFF",
		ThinkingText: "#CCCCCC",

		UserMessage:   "#FFFFFF",
		AssistantText: "#FFFFFF",

		SelectedBg:    "#333333",
		UserMessageBg: "#1A1A1A",
		ToolPendingBg: "#1A1A1A",
		ToolSuccessBg: "#003300",
		ToolErrorBg:   "#330000",

		// Markdown
		MdHeading:         "#FFFFFF",
		MdLink:            "#88CCFF",
		MdLinkURL:         "#CCCCCC",
		MdCode:            "#00FF00",
		MdCodeBlock:       "#00FF00",
		MdCodeBlockBorder: "#AAAAAA",
		MdQuote:           "#CCCCCC",
		MdQuoteBorder:     "#FFFFFF",
		MdHr:              "#AAAAAA",
		MdListBullet:      "#FFFFFF",

		// Syntax
		SyntaxComment:     "#CCCCCC",
		SyntaxKeyword:     "#FF8888",
		SyntaxFunction:    "#88CCFF",
		SyntaxVariable:    "#FFFF88",
		SyntaxString:      "#00FF00",
		SyntaxNumber:      "#FFFF00",
		SyntaxType:        "#88CCFF",
		SyntaxOperator:    "#FFFFFF",
		SyntaxPunctuation: "#CCCCCC",

		LoaderSpinner: "#FFFFFF",
		ProgressBar:   "#FFFFFF",

		Background:    "#000000",
		Surface:       "#111111",
		SurfaceRaised: "#1A1A1A",

		EvidenceSupport:  "#88CCFF",
		EvidenceCounter:  "#FF8888",
		ConfidenceLow:    "#FFFF00",
		ConfidenceMedium: "#88CCFF",
		ConfidenceHigh:   "#00FF00",
	}
}

// ColorBlind returns a deuteranopia/protanopia-safe palette that replaces
// red-green coding with blue-orange, and maintains ≥3:1 contrast ratios.
func ColorBlind() *SemanticTheme {
	return &SemanticTheme{
		Name: "colorblind",

		Accent:       "#5BC0EB",
		Border:       "#5B7A9A",
		BorderAccent: "#7DD0F5",
		BorderMuted:  "#3A5A7A",
		Success:      "#5BC0EB", // blue instead of green
		Error:        "#E8A838", // orange instead of red
		Warning:      "#D7B65C", // amber (safe for all)
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

		// Markdown
		MdHeading:         "#5BC0EB",
		MdLink:            "#7DD0F5",
		MdLinkURL:         "#7892A5",
		MdCode:            "#52D6A0",
		MdCodeBlock:       "#52D6A0",
		MdCodeBlockBorder: "#1D3B52",
		MdQuote:           "#7892A5",
		MdQuoteBorder:     "#5BC0EB",
		MdHr:              "#1D3B52",
		MdListBullet:      "#5BC0EB",

		// Syntax
		SyntaxComment:     "#7892A5",
		SyntaxKeyword:     "#CFA7FF",
		SyntaxFunction:    "#7DD0F5",
		SyntaxVariable:    "#E8A838",
		SyntaxString:      "#52D6A0",
		SyntaxNumber:      "#D7B65C",
		SyntaxType:        "#5BC0EB",
		SyntaxOperator:    "#DCEAF3",
		SyntaxPunctuation: "#7892A5",

		LoaderSpinner: "#5BC0EB",
		ProgressBar:   "#5BC0EB",

		Background:    "#07111F",
		Surface:       "#0C1B2A",
		SurfaceRaised: "#102638",

		EvidenceSupport:  "#5BC0EB", // blue
		EvidenceCounter:  "#E8A838", // orange
		ConfidenceLow:    "#E8A838", // orange (warning)
		ConfidenceMedium: "#D7B65C", // amber
		ConfidenceHigh:   "#5BC0EB", // blue (success)
	}
}
