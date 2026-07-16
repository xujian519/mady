package main

// This file holds TUI-specific formatting and parsing helpers used by
// tui_session.go: thinking-config display/parse, the status-bar mode label,
// project-context formatting, matter-type→CaseType mapping, and the Markdown
// export formatter. They have no dependencies on the TUI engine itself, only
// on agentcore / domains / chat data types.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/terminal"
)

// --- thinking config helpers (ported from example/cli-chat) ---

func cloneThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	return &cp
}

func compactThinkingConfig(cfg *agentcore.ThinkingConfig) *agentcore.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	if !cfg.IncludeThoughts &&
		cfg.Display == agentcore.ThinkingDisplayDefault &&
		cfg.Effort == agentcore.ThinkingEffortDefault &&
		cfg.Budget == 0 {
		return nil
	}
	return cfg
}

func formatThinkingConfig(cfg *agentcore.ThinkingConfig) string {
	if cfg == nil {
		return "default"
	}
	parts := []string{
		"display=" + string(cfg.NormalizedDisplay()),
	}
	if cfg.Effort != "" {
		parts = append(parts, "effort="+string(cfg.Effort))
	}
	if cfg.Budget != 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", cfg.Budget))
	}
	parts = append(parts, fmt.Sprintf("include_thoughts=%t", cfg.IncludeThoughts))
	return strings.Join(parts, " ")
}

func parseThinkingCommand(input string, current *agentcore.ThinkingConfig) (*agentcore.ThinkingConfig, bool, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) <= 1 {
		return agentcore.CloneThinkingConfig(current), false, nil
	}

	next := agentcore.CloneThinkingConfig(current)
	if next == nil {
		next = &agentcore.ThinkingConfig{}
	}

	switch strings.ToLower(fields[1]) {
	case "reset":
		return nil, true, nil
	case "on", "summarized":
		next.IncludeThoughts = true
		next.Display = agentcore.ThinkingDisplaySummarized
		return compactThinkingConfig(next), true, nil
	case "off", "omitted":
		next.IncludeThoughts = false
		next.Display = agentcore.ThinkingDisplayOmitted
		return compactThinkingConfig(next), true, nil
	case "effort":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking effort <low|medium|high|max|default>")
		}
		switch strings.ToLower(fields[2]) {
		case "default", "reset":
			next.Effort = agentcore.ThinkingEffortDefault
		case "low", "medium", "high", "max":
			next.Effort = agentcore.ThinkingEffort(strings.ToLower(fields[2]))
		default:
			return nil, false, fmt.Errorf("invalid thinking effort %q", fields[2])
		}
		return compactThinkingConfig(next), true, nil
	case "budget":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking budget <n|default>")
		}
		if strings.EqualFold(fields[2], "default") || strings.EqualFold(fields[2], "reset") {
			next.Budget = 0
			return compactThinkingConfig(next), true, nil
		}
		v, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking budget %q", fields[2])
		}
		next.Budget = v
		return compactThinkingConfig(next), true, nil
	case "include":
		if len(fields) < 3 {
			return nil, false, fmt.Errorf("usage: /thinking include <true|false>")
		}
		v, err := strconv.ParseBool(fields[2])
		if err != nil {
			return nil, false, fmt.Errorf("invalid thinking include value %q", fields[2])
		}
		next.IncludeThoughts = v
		if next.Display == agentcore.ThinkingDisplayDefault {
			if v {
				next.Display = agentcore.ThinkingDisplaySummarized
			} else {
				next.Display = agentcore.ThinkingDisplayOmitted
			}
		}
		return compactThinkingConfig(next), true, nil
	default:
		return nil, false, fmt.Errorf("usage: /thinking [on|off|summarized|omitted|effort <...>|budget <...>|include <true|false>|reset]")
	}
}

// statusBarModeLabel 生成状态栏的模式标签（中文友好）。
func statusBarModeLabel(planMode, useMultiDomain bool, thinking *agentcore.ThinkingConfig) string {
	if planMode {
		return "🧠 计划"
	}
	label := "集成"
	if useMultiDomain {
		label = "多域路由"
	}
	if thinking != nil && thinking.IncludeThoughts {
		if thinking.Effort != "" && thinking.Effort != agentcore.ThinkingEffortDefault {
			label += " · 推理" + string(thinking.Effort)
		} else {
			label += " · 推理"
		}
	}
	return label
}

func formatProjectContext(rec *domains.ProjectRecord, meta *domains.ProjectMeta) string {
	s := "\n\n---\n## 当前案件上下文\n"
	s += fmt.Sprintf("- 案件: %s（%s）\n", rec.Alias, rec.ProjectID)
	s += fmt.Sprintf("- 领域: %s\n", rec.Domain)
	if meta != nil {
		if meta.MatterType != "" {
			s += fmt.Sprintf("- 事项类型: %s\n", meta.MatterType)
		}
		if meta.ClientName != "" {
			s += fmt.Sprintf("- 客户: %s\n", meta.ClientName)
		}
		if len(meta.Deadlines) > 0 {
			s += "- 期限:\n"
			for _, d := range meta.Deadlines {
				s += fmt.Sprintf("  - %s: %s\n", d.Type, d.DueDate)
			}
		}
	}
	s += fmt.Sprintf("- 工作目录: %s\n", rec.RootPath)
	return s
}

func formatProjectInfo(rec *domains.ProjectRecord, meta *domains.ProjectMeta) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "案件: %s\n", rec.Alias)
	fmt.Fprintf(&sb, "ID: %s\n", rec.ProjectID)
	fmt.Fprintf(&sb, "领域: %s\n", rec.Domain)
	fmt.Fprintf(&sb, "状态: %s\n", rec.Status)
	fmt.Fprintf(&sb, "工作目录: %s\n", rec.RootPath)
	fmt.Fprintf(&sb, "注册时间: %s\n", rec.RegisteredAt.Format("2006-01-02"))
	if meta != nil {
		if meta.MatterType != "" {
			fmt.Fprintf(&sb, "事项类型: %s\n", meta.MatterType)
		}
		if meta.ClientName != "" {
			fmt.Fprintf(&sb, "客户: %s\n", meta.ClientName)
		}
		if len(meta.Deadlines) > 0 {
			sb.WriteString("期限:\n")
			for _, d := range meta.Deadlines {
				mark := ""
				if d.Reminded {
					mark = "✓ "
				}
				fmt.Fprintf(&sb, "  %s%s: %s\n", mark, d.Type, d.DueDate)
			}
		}
	}
	return sb.String()
}

// mapMatterTypeToCaseType 将案件事项类型映射到 reasoning 工作流的 CaseType。
func mapMatterTypeToCaseType(meta *domains.ProjectMeta) reasoning.CaseType {
	if meta == nil || meta.MatterType == "" {
		return reasoning.CaseGeneralLegal
	}
	m := strings.ToLower(meta.MatterType)
	switch {
	case strings.Contains(m, "无效"):
		return reasoning.CaseInvalidation
	case strings.Contains(m, "自由实施") || strings.Contains(m, "fto"):
		return reasoning.CaseFTO
	case strings.Contains(m, "新颖性"):
		return reasoning.CaseNoveltySearch
	case strings.Contains(m, "专利性") || strings.Contains(m, "创造性"):
		return reasoning.CasePatentability
	case strings.Contains(m, "侵权"):
		return reasoning.CaseInfringement
	case strings.Contains(m, "审查意见") || strings.Contains(m, "oa") || strings.Contains(m, "答复"):
		return reasoning.CaseRejection
	case strings.Contains(m, "复审"):
		return reasoning.CaseReexamination
	case strings.Contains(m, "撰写") || strings.Contains(m, "申请"):
		return reasoning.CaseDrafting
	default:
		return reasoning.CaseGeneralLegal
	}
}

func formatExportMarkdown(msgs []chat.ChatMessage, threadID string, project *domains.ProjectRecord) string {
	var b strings.Builder
	b.WriteString("# Mady 对话记录\n\n")
	fmt.Fprintf(&b, "**导出时间**: %s  \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "**会话ID**: %s  \n", threadID)
	if project != nil {
		fmt.Fprintf(&b, "**案件**: %s (%s)  \n", project.Alias, project.ProjectID)
	}
	b.WriteString("\n---\n\n")
	for _, msg := range msgs {
		switch msg.Role {
		case chat.RoleUser:
			b.WriteString("## 👤 用户\n\n")
		case chat.RoleAssistant:
			b.WriteString("## 🤖 助手\n\n")
		case chat.RoleSystem:
			b.WriteString("## 💬 系统\n\n")
		case chat.RoleTool:
			label := "## 🔧 工具"
			if msg.Meta != "" {
				label += " (" + msg.Meta + ")"
			}
			b.WriteString(label + "\n\n")
		case chat.RoleError:
			b.WriteString("## ❌ 错误\n\n")
		default:
			continue
		}
		if msg.Text != "" {
			b.WriteString(msg.Text)
			b.WriteString("\n\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String()
}

// loadKeymapOverrides reads ~/.mady/keymap.json (if it exists) and applies it
// to km as the user-override layer. Returns warnings about unrecognized key
// tokens so the caller can surface them; a missing file is not an error.
func loadKeymapOverrides(madyHome string, km *terminal.KeybindingsManager) []string {
	path := filepath.Join(madyHome, "keymap.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Missing file is the common case — no keymap customization.
		return nil
	}
	warnings, err := km.LoadUserBindingsJSON(data)
	if err != nil {
		return []string{fmt.Sprintf("failed to parse %s: %v", path, err)}
	}
	return warnings
}
