package main

// prompt_command.go implements /prompt slash command for browsing and viewing
// available prompt templates from the prompt store.
//
// Data source: fc.PromptStore (initialized in bootstrap/setup.go).

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/prompt"
)

// handlePromptCommand implements /prompt [list|<name>].
// Without arguments or with "list", shows all registered prompt templates.
// With a name, shows the template content and its variables.
func (s *tuiSession) handlePromptCommand(sub string) {
	if s.fc == nil || s.fc.PromptStore == nil {
		s.app.PrintSystem("⚠ 提示词模板库未加载。")
		return
	}

	store := s.fc.PromptStore

	if sub == "" || sub == "list" {
		// List all available templates.
		templates := store.List(prompt.ListOptions{})
		if len(templates) == 0 {
			s.app.PrintSystem("📝 提示词模板库为空。")
			return
		}

		var b strings.Builder
		fmt.Fprintf(&b, "📝 可用提示词模板（共 %d 个）\n", len(templates))
		for _, t := range templates {
			fmt.Fprintf(&b, "  · %s", t.Name)
			if t.Title != "" {
				fmt.Fprintf(&b, " — %s", t.Title)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n用法: /prompt <模板名称> — 查看模板详情")
		s.app.PrintSystem(b.String())
		return
	}

	// Show specific template details.
	tmpl, ok := store.FindByName(sub)
	if !ok {
		s.app.PrintSystem(fmt.Sprintf("⚠ 未找到模板 %q。使用 /prompt list 查看所有模板。", sub))
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📝 模板: %s\n", tmpl.Name)
	if tmpl.Title != "" {
		fmt.Fprintf(&b, "   标题: %s\n", tmpl.Title)
	}
	if tmpl.Description != "" {
		fmt.Fprintf(&b, "   说明: %s\n", tmpl.Description)
	}
	if tmpl.Category != "" {
		fmt.Fprintf(&b, "   分类: %s\n", tmpl.Category)
	}
	if tmpl.Domain != "" {
		fmt.Fprintf(&b, "   领域: %s\n", tmpl.Domain)
	}

	b.WriteString("\nSystem Prompt:\n")
	appendTruncatedPrompt(&b, tmpl.SystemPrompt, "  ", 20)

	if tmpl.UserPromptTemplate != "" {
		b.WriteString("\nUser Prompt 模板:\n")
		appendTruncatedPrompt(&b, tmpl.UserPromptTemplate, "  ", 20)
	}

	s.app.PrintSystem(b.String())
}

// appendTruncatedPrompt writes up to maxLines lines of text to b, prefixed
// with linePrefix. If the text has more than maxLines lines, "(已截断)" is
// appended after the displayed lines.
func appendTruncatedPrompt(b *strings.Builder, text, linePrefix string, maxLines int) {
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for _, line := range lines {
		b.WriteString(linePrefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(strings.Split(text, "\n")) > maxLines {
		b.WriteString("  ...（已截断）\n")
	}
}
