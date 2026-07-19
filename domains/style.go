package domains

import (
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/domains/doctmpl"
	"gopkg.in/yaml.v3"
)

// DocumentStyle is a machine-readable writing style guide for Mady agents.
// Each style defines tone, voice, anti-patterns, disclaimers, citation
// format, and output conventions. It is the Mady equivalent of Open
// Design's DESIGN.md — a structured contract that agents read before
// generating any user-facing content.
//
// Style files live under $MADY_HOME/styles/<name>.yaml. Built-in styles
// for each domain (patent, legal, chat, assistant) are embedded
// (see agentcore/manifests/styles/ directory).
type DocumentStyle struct {
	Name    string `yaml:"name"`
	Domain  string `yaml:"domain"`
	Version string `yaml:"version"`

	Sections StyleSections `yaml:"sections"`
}

// StyleSections holds the individual sections of a document style.
type StyleSections struct {
	Tone              ToneSection              `yaml:"tone"`
	Voice             VoiceSection             `yaml:"voice"`
	AntiPatterns      []AntiPattern            `yaml:"anti_patterns"`
	Disclaimers       map[string]string        `yaml:"disclaimers"`
	Citation          CitationSection          `yaml:"citation"`
	OutputConventions OutputConventionsSection `yaml:"output_conventions"`
}

// ToneSection defines the writing tone parameters.
type ToneSection struct {
	Formality   string `yaml:"formality"`   // casual | professional | academic
	Perspective string `yaml:"perspective"` // first | second | third
	Language    string `yaml:"language"`    // zh-CN | en-US
}

// VoiceSection defines the voice principles.
type VoiceSection struct {
	Principles []string `yaml:"principles"`
}

// AntiPattern is a forbidden or discouraged word pattern with a replacement.
type AntiPattern struct {
	Word     string `yaml:"word"`     // the forbidden word/phrase
	Replace  string `yaml:"replace"`  // the suggested replacement
	Severity string `yaml:"severity"` // block | warn
}

// CitationSection defines citation formatting rules.
type CitationSection struct {
	Style  string `yaml:"style"`  // inline | footnote | endnote
	Format string `yaml:"format"` // e.g. "[{id}]"
}

// OutputConventionsSection controls output formatting conventions.
type OutputConventionsSection struct {
	ConfidenceLabel bool `yaml:"confidence_label"` // attach confidence to conclusions
	WeakVisual      bool `yaml:"weak_visual"`      // visually de-emphasize low-confidence content
}

// LoadStyle reads a DocumentStyle from a YAML file.
func LoadStyle(path string) (*DocumentStyle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("style: read %s: %w", path, err)
	}
	var style DocumentStyle
	if err := yaml.Unmarshal(data, &style); err != nil {
		return nil, fmt.Errorf("style: parse %s: %w", path, err)
	}
	if style.Name == "" {
		return nil, fmt.Errorf("style: %s: missing name", path)
	}
	if style.Domain == "" {
		return nil, fmt.Errorf("style: %s: missing domain", path)
	}
	return &style, nil
}

// SystemPrompt returns the style as a system-prompt injection block
// suitable for prepending to an agent's system message.
func (s *DocumentStyle) SystemPrompt() string {
	var b strings.Builder
	b.WriteString("<!-- Document Style Guide -->\n")
	fmt.Fprintf(&b, "Style: %s (domain: %s, version: %s)\n", s.Name, s.Domain, s.Version)
	b.WriteString("\n")

	// Tone.
	t := s.Sections.Tone
	if t.Formality != "" || t.Perspective != "" || t.Language != "" {
		b.WriteString("## Tone\n")
		if t.Formality != "" {
			fmt.Fprintf(&b, "- Formality: %s\n", t.Formality)
		}
		if t.Perspective != "" {
			fmt.Fprintf(&b, "- Perspective: %s person\n", t.Perspective)
		}
		if t.Language != "" {
			fmt.Fprintf(&b, "- Language: %s\n", t.Language)
		}
		b.WriteString("\n")
	}

	// Voice.
	if len(s.Sections.Voice.Principles) > 0 {
		b.WriteString("## Voice Principles\n")
		for _, p := range s.Sections.Voice.Principles {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}

	// Anti-patterns.
	if len(s.Sections.AntiPatterns) > 0 {
		b.WriteString("## Anti-Patterns (FORBIDDEN WORDS)\n")
		for _, ap := range s.Sections.AntiPatterns {
			tag := "[BLOCK]"
			if ap.Severity == "warn" {
				tag = "[WARN]"
			}
			fmt.Fprintf(&b, "- %s Never use %q → use %q instead\n", tag, ap.Word, ap.Replace)
		}
		b.WriteString("\n")
	}

	// Disclaimers.
	if len(s.Sections.Disclaimers) > 0 {
		b.WriteString("## Required Disclaimers\n")
		for key, text := range s.Sections.Disclaimers {
			fmt.Fprintf(&b, "- %s: %s\n", key, text)
		}
		b.WriteString("\n")
	}

	// Citation.
	if s.Sections.Citation.Style != "" {
		b.WriteString("## Citation Format\n")
		fmt.Fprintf(&b, "- Style: %s\n", s.Sections.Citation.Style)
		if s.Sections.Citation.Format != "" {
			fmt.Fprintf(&b, "- Format: %s\n", s.Sections.Citation.Format)
		}
		b.WriteString("\n")
	}

	// Output conventions.
	oc := s.Sections.OutputConventions
	if oc.ConfidenceLabel || oc.WeakVisual {
		b.WriteString("## Output Conventions\n")
		if oc.ConfidenceLabel {
			b.WriteString("- Attach confidence labels to all analytical conclusions\n")
		}
		if oc.WeakVisual {
			b.WriteString("- De-emphasize low-confidence content visually\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("<!-- End Document Style Guide -->")
	return b.String()
}

// StylesForDomain filters a style slice to those matching the given domain.
func StylesForDomain(styles []DocumentStyle, domain string) []DocumentStyle {
	var matched []DocumentStyle
	for _, s := range styles {
		if s.Domain == domain {
			matched = append(matched, s)
		}
	}
	return matched
}

// FindStyleByName returns the first style matching the given name.
func FindStyleByName(styles []DocumentStyle, name string) (*DocumentStyle, bool) {
	for i, s := range styles {
		if s.Name == name {
			return &styles[i], true
		}
	}
	return nil, false
}

// ToRenderStyle converts a DocumentStyle to the lightweight doctmpl.RenderStyle
// used during template rendering. categoryHint selects the most appropriate
// disclaimer (e.g. "oa-response" → patent_analysis).
func (s *DocumentStyle) ToRenderStyle(categoryHint string) *doctmpl.RenderStyle {
	if s == nil {
		return nil
	}
	return &doctmpl.RenderStyle{
		Name:       s.Name,
		Disclaimer: s.DisclaimerFor(categoryHint),
	}
}

// DisclaimerFor selects the most appropriate disclaimer for a given template
// category. Falls back to a domain-level disclaimer when no category match.
func (s *DocumentStyle) DisclaimerFor(category string) string {
	if s == nil {
		return ""
	}
	// Category → disclaimer key mapping.
	keyMap := map[string]string{
		"specification": "patent_drafting",
		"claims":        "patent_drafting",
		"oa-response":   "patent_analysis",
		"disclosure":    "patent_analysis",
	}
	if key, ok := keyMap[category]; ok {
		if d, ok2 := s.Sections.Disclaimers[key]; ok2 {
			return d
		}
	}
	// Fallback: domain-level disclaimer.
	fallbackKey := s.Domain + "_analysis"
	if d, ok := s.Sections.Disclaimers[fallbackKey]; ok {
		return d
	}
	return ""
}

// SystemPromptForTemplate generates a combined system prompt that includes
// both the style guide and template context, so the LLM generates content
// that follows the style while filling the template.
func (s *DocumentStyle) SystemPromptForTemplate(tmpl doctmpl.DocTemplate) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.SystemPrompt())
	b.WriteString("\n<!-- Template Context -->\n")
	fmt.Fprintf(&b, "Template: %s (%s)\n", tmpl.Name, tmpl.Title)
	fmt.Fprintf(&b, "Category: %s\n", tmpl.Category)
	if d := s.DisclaimerFor(tmpl.Category); d != "" {
		fmt.Fprintf(&b, "Required Disclaimer: %s\n", d)
	}
	b.WriteString("\nGenerate content following the style guide above and filling the template variables below.\n")
	b.WriteString("<!-- End Template Context -->")
	return b.String()
}
