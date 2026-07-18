package domains

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DocumentStyle is a machine-readable writing style guide for Mady agents.
// Each style defines tone, voice, anti-patterns, disclaimers, citation
// format, and output conventions. It is the Mady equivalent of Open
// Design's DESIGN.md — a structured contract that agents read before
// generating any user-facing content.
//
// Style files live under $MADY_HOME/styles/<name>.yaml. Built-in styles
// for each domain (patent, legal, chat, assistant) are embedded via
// go:embed in agentcore/manifests/ (see styles/ directory).
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
	b.WriteString(fmt.Sprintf("Style: %s (domain: %s, version: %s)\n", s.Name, s.Domain, s.Version))
	b.WriteString("\n")

	// Tone.
	t := s.Sections.Tone
	if t.Formality != "" || t.Perspective != "" || t.Language != "" {
		b.WriteString("## Tone\n")
		if t.Formality != "" {
			b.WriteString(fmt.Sprintf("- Formality: %s\n", t.Formality))
		}
		if t.Perspective != "" {
			b.WriteString(fmt.Sprintf("- Perspective: %s person\n", t.Perspective))
		}
		if t.Language != "" {
			b.WriteString(fmt.Sprintf("- Language: %s\n", t.Language))
		}
		b.WriteString("\n")
	}

	// Voice.
	if len(s.Sections.Voice.Principles) > 0 {
		b.WriteString("## Voice Principles\n")
		for _, p := range s.Sections.Voice.Principles {
			b.WriteString(fmt.Sprintf("- %s\n", p))
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
			b.WriteString(fmt.Sprintf("- %s Never use %q → use %q instead\n", tag, ap.Word, ap.Replace))
		}
		b.WriteString("\n")
	}

	// Disclaimers.
	if len(s.Sections.Disclaimers) > 0 {
		b.WriteString("## Required Disclaimers\n")
		for key, text := range s.Sections.Disclaimers {
			b.WriteString(fmt.Sprintf("- %s: %s\n", key, text))
		}
		b.WriteString("\n")
	}

	// Citation.
	if s.Sections.Citation.Style != "" {
		b.WriteString("## Citation Format\n")
		b.WriteString(fmt.Sprintf("- Style: %s\n", s.Sections.Citation.Style))
		if s.Sections.Citation.Format != "" {
			b.WriteString(fmt.Sprintf("- Format: %s\n", s.Sections.Citation.Format))
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
