package domains

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStyle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "patent-standard.yaml")
	content := `name: patent-standard
domain: patent
version: "1.0"
sections:
  tone:
    formality: professional
    perspective: third
    language: zh-CN
  voice:
    principles:
      - 如实呈现，留有余地
      - 不说绝对结论
  anti_patterns:
    - word: 绝对
      replace: 通常
      severity: block
  disclaimers:
    patent_analysis: "本分析由 AI 辅助生成，不构成正式法律意见。"
  citation:
    style: inline
    format: "[{id}]"
  output_conventions:
    confidence_label: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	style, err := LoadStyle(path)
	if err != nil {
		t.Fatal(err)
	}
	if style.Name != "patent-standard" {
		t.Fatalf("name = %q", style.Name)
	}
	if style.Domain != "patent" {
		t.Fatalf("domain = %q", style.Domain)
	}
	if style.Sections.Tone.Formality != "professional" {
		t.Fatalf("formality = %q", style.Sections.Tone.Formality)
	}
	if len(style.Sections.AntiPatterns) != 1 {
		t.Fatalf("anti_patterns len = %d", len(style.Sections.AntiPatterns))
	}
	if style.Sections.AntiPatterns[0].Word != "绝对" {
		t.Fatalf("anti_pattern word = %q", style.Sections.AntiPatterns[0].Word)
	}
	if !style.Sections.OutputConventions.ConfidenceLabel {
		t.Fatal("confidence_label should be true")
	}
}

func TestLoadStyle_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"missing name", "domain: patent\n"},
		{"missing domain", "name: test\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.yaml")
			os.WriteFile(path, []byte(tt.content), 0o644)
			_, err := LoadStyle(path)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSystemPrompt(t *testing.T) {
	style := &DocumentStyle{
		Name:    "patent-standard",
		Domain:  "patent",
		Version: "1.0",
		Sections: StyleSections{
			Tone: ToneSection{
				Formality:   "professional",
				Perspective: "third",
				Language:    "zh-CN",
			},
			Voice: VoiceSection{
				Principles: []string{"如实呈现", "留有余地"},
			},
			AntiPatterns: []AntiPattern{
				{Word: "绝对", Replace: "通常", Severity: "block"},
				{Word: "百分百", Replace: "大概率", Severity: "warn"},
			},
			Disclaimers: map[string]string{
				"patent_analysis": "本分析由 AI 辅助生成。",
			},
			Citation: CitationSection{
				Style:  "inline",
				Format: "[{id}]",
			},
			OutputConventions: OutputConventionsSection{
				ConfidenceLabel: true,
				WeakVisual:      true,
			},
		},
	}

	prompt := style.SystemPrompt()
	if !strings.Contains(prompt, "patent-standard") {
		t.Fatal("missing style name")
	}
	if !strings.Contains(prompt, "professional") {
		t.Fatal("missing formality")
	}
	if !strings.Contains(prompt, "绝对") {
		t.Fatal("missing anti-pattern")
	}
	if !strings.Contains(prompt, "[BLOCK]") {
		t.Fatal("missing block severity")
	}
	if !strings.Contains(prompt, "[WARN]") {
		t.Fatal("missing warn severity")
	}
	if !strings.Contains(prompt, "本分析由 AI 辅助生成") {
		t.Fatal("missing disclaimer")
	}
	if !strings.Contains(prompt, "confidence labels") {
		t.Fatal("missing confidence label convention")
	}
	if !strings.Contains(prompt, "De-emphasize") {
		t.Fatal("missing weak visual convention")
	}
}

func TestSystemPrompt_Minimal(t *testing.T) {
	style := &DocumentStyle{
		Name:    "minimal",
		Domain:  "chat",
		Version: "1.0",
	}
	prompt := style.SystemPrompt()
	if !strings.Contains(prompt, "minimal") {
		t.Fatal("missing style name")
	}
	// Should not contain section headers with empty content.
	if strings.Contains(prompt, "## Tone") {
		t.Fatal("should not have empty Tone section")
	}
}

func TestStylesForDomain(t *testing.T) {
	styles := []DocumentStyle{
		{Name: "a", Domain: "patent"},
		{Name: "b", Domain: "patent"},
		{Name: "c", Domain: "legal"},
		{Name: "d", Domain: "chat"},
	}
	patent := StylesForDomain(styles, "patent")
	if len(patent) != 2 {
		t.Fatalf("len = %d", len(patent))
	}
	legal := StylesForDomain(styles, "legal")
	if len(legal) != 1 {
		t.Fatalf("len = %d", len(legal))
	}
	none := StylesForDomain(styles, "nonexistent")
	if len(none) != 0 {
		t.Fatalf("len = %d", len(none))
	}
}

func TestFindStyleByName(t *testing.T) {
	styles := []DocumentStyle{
		{Name: "alpha"},
		{Name: "beta"},
	}
	s, ok := FindStyleByName(styles, "alpha")
	if !ok || s.Name != "alpha" {
		t.Fatal("not found")
	}
	_, ok = FindStyleByName(styles, "gamma")
	if ok {
		t.Fatal("unexpected found")
	}
}
