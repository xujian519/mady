package writing

import (
	"fmt"
	"strings"
)

// SkillCompiler compiles WritingPatterns into agent-injectable instruction blocks.
//
// In v1, the output is returned as structured text that agents can query via
// the query_writing_patterns tool. System prompt injection is deferred until
// the pattern library matures (≥30 patterns, ≥50 user feedback ratings).
type SkillCompiler struct {
	store *PatternStore
}

// NewSkillCompiler creates a compiler bound to a pattern store.
func NewSkillCompiler(store *PatternStore) *SkillCompiler {
	return &SkillCompiler{store: store}
}

// MatchAndCompile looks up patterns matching the given case type and features,
// then compiles them into a structured instruction block.
func (c *SkillCompiler) MatchAndCompile(caseType string, features []string) string {
	patterns := c.store.MatchPatterns(caseType, features)
	if len(patterns) == 0 {
		return ""
	}
	return c.CompileSkills(patterns)
}

// CompileSkills converts a list of WritingPatterns into a structured
// <writing_skills> XML block that agents can read for writing guidance.
func (c *SkillCompiler) CompileSkills(patterns []*WritingPattern) string {
	if len(patterns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<writing_skills>\n")

	for _, p := range patterns {
		writeOnePattern(&b, p)
	}

	b.WriteString("</writing_skills>")
	return b.String()
}

// CompileMarkdown converts patterns to a readable markdown format
// suitable for display in tool outputs.
func (c *SkillCompiler) CompileMarkdown(patterns []*WritingPattern) string {
	if len(patterns) == 0 {
		return "*没有找到匹配的写作模式。*"
	}
	var b strings.Builder
	b.WriteString("## 📝 推荐的写作模式\n\n")
	fmt.Fprintf(&b, "> 基于当前案件特征匹配到 %d 个写作模式\n\n", len(patterns))

	for i, p := range patterns {
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, p.Name)
		fmt.Fprintf(&b, "**场景**: %s\n\n", p.Context)
		fmt.Fprintf(&b, "**核心思路**: %s\n\n", p.Summary)

		if len(p.Steps) > 0 {
			for _, s := range p.Steps {
				fmt.Fprintf(&b, "**步骤 %d: %s**\n", s.Order, s.Name)
				fmt.Fprintf(&b, "%s\n\n", s.Instruction)
				if s.Example != "" {
					fmt.Fprintf(&b, "> 示例: %s\n\n", s.Example)
				}
			}
		}
		if len(p.Dos) > 0 {
			b.WriteString("✅ 应该遵循的原则:\n")
			for _, d := range p.Dos {
				fmt.Fprintf(&b, "- %s\n", d.Rule)
			}
			b.WriteString("\n")
		}
		if len(p.Donts) > 0 {
			b.WriteString("❌ 应该避免的错误:\n")
			for _, d := range p.Donts {
				fmt.Fprintf(&b, "- %s\n", d.Rule)
			}
			b.WriteString("\n")
		}
		if i < len(patterns)-1 {
			b.WriteString("---\n\n")
		}
	}
	return b.String()
}

// writeOnePattern writes a single pattern as a <skill> element.
func writeOnePattern(b *strings.Builder, p *WritingPattern) {
	fmt.Fprintf(b, "  <skill id=\"%s\">\n", escapeXML(p.ID))
	fmt.Fprintf(b, "    <name>%s</name>\n", escapeXML(p.Name))
	fmt.Fprintf(b, "    <category>%s</category>\n", p.Category)
	fmt.Fprintf(b, "    <summary>%s</summary>\n", escapeXML(p.Summary))
	if p.Context != "" {
		fmt.Fprintf(b, "    <context>%s</context>\n", escapeXML(p.Context))
	}
	if len(p.Steps) > 0 {
		b.WriteString("    <steps>\n")
		for _, s := range p.Steps {
			fmt.Fprintf(b, "      <step order=\"%d\">\n", s.Order)
			fmt.Fprintf(b, "        <name>%s</name>\n", escapeXML(s.Name))
			fmt.Fprintf(b, "        <instruction>%s</instruction>\n", escapeXML(s.Instruction))
			if s.Example != "" {
				fmt.Fprintf(b, "        <example>%s</example>\n", escapeXML(s.Example))
			}
			b.WriteString("      </step>\n")
		}
		b.WriteString("    </steps>\n")
	}
	if len(p.Dos) > 0 {
		b.WriteString("    <dos>\n")
		for _, d := range p.Dos {
			fmt.Fprintf(b, "      <principle>%s</principle>\n", escapeXML(d.Rule))
		}
		b.WriteString("    </dos>\n")
	}
	if len(p.Donts) > 0 {
		b.WriteString("    <donts>\n")
		for _, d := range p.Donts {
			fmt.Fprintf(b, "      <principle>%s</principle>\n", escapeXML(d.Rule))
		}
		b.WriteString("    </donts>\n")
	}
	b.WriteString("  </skill>\n")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
