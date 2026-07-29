package checker

import (
	"context"
	"fmt"
	"strings"
)

// CheckerHandler is a function that performs a checker review on an artifact.
// It receives the full artifact content and should return a structured verdict.
type CheckerHandler func(ctx context.Context, artifactPath string, content string) (CheckerVerdict, error)

// Dispatch selects and runs appropriate checkers for given artifacts.
// When no handler is registered for a checker, it returns a placeholder verdict
// indicating the checker was not executed.
type Dispatch struct {
	catalog  *Catalog
	handlers map[string]CheckerHandler
}

// NewDispatch creates a Dispatch backed by the given Catalog.
func NewDispatch(catalog *Catalog) *Dispatch {
	return &Dispatch{
		catalog:  catalog,
		handlers: make(map[string]CheckerHandler),
	}
}

// RegisterHandler binds a handler function to a checker RoleID.
func (d *Dispatch) RegisterHandler(roleID string, handler CheckerHandler) {
	d.handlers[roleID] = handler
}

// GetHandler returns the handler for a given RoleID, or nil.
func (d *Dispatch) GetHandler(roleID string) CheckerHandler {
	return d.handlers[roleID]
}

// SuggestCheckers returns the checkers that match the given artifact path.
func (d *Dispatch) SuggestCheckers(artifactPath string) []CheckerEntry {
	return d.catalog.Suggest(artifactPath)
}

// RunChecker executes a single checker by RoleID on the given artifact content.
// If no handler is registered, returns a placeholder verdict.
func (d *Dispatch) RunChecker(ctx context.Context, roleID string, artifactPath string, content string) (CheckerVerdict, error) {
	entry := d.catalog.Get(roleID)
	if entry == nil {
		return CheckerVerdict{}, fmt.Errorf("checker %q not found in catalog", roleID)
	}

	handler, ok := d.handlers[roleID]
	if !ok {
		// Placeholder: return a "not executed" verdict
		return CheckerVerdict{
			RoleID:  roleID,
			Status:  StatusNeedsRevision,
			Score:   0.5,
			Summary: fmt.Sprintf("检查器 %q (%s) 尚未实现，请配置 handler 后重新运行。", roleID, entry.Name),
		}, nil
	}

	return handler(ctx, artifactPath, content)
}

// RunAllMatching runs all checkers whose RequiredInputs match the artifact path.
// Returns all individual verdicts plus an aggregated composite.
func (d *Dispatch) RunAllMatching(ctx context.Context, artifactPath string, content string) ([]CheckerVerdict, CheckerVerdict, error) {
	entries := d.catalog.Suggest(artifactPath)
	if len(entries) == 0 {
		return nil, CheckerVerdict{}, fmt.Errorf("没有找到匹配 %q 的检查器", artifactPath)
	}

	var verdicts []CheckerVerdict
	for _, e := range entries {
		v, err := d.RunChecker(ctx, e.RoleID, artifactPath, content)
		if err != nil {
			return nil, CheckerVerdict{}, fmt.Errorf("运行检查器 %q 失败: %w", e.RoleID, err)
		}
		v.RoleID = e.RoleID
		verdicts = append(verdicts, v)
	}

	agg := NewVerdictAggregator(verdicts).Aggregate()
	return verdicts, agg, nil
}

// FormatReviewPrompt builds a prompt for a sub-agent to perform a checker review.
// This prompt can be fed to the agent's LLM to get a structured verdict response.
func FormatReviewPrompt(entry CheckerEntry, artifactPath string, content string) string {
	var b strings.Builder
	b.WriteString("## Checker 审查任务\n\n")
	fmt.Fprintf(&b, "你正在扮演 **%s**（%s）。\n", entry.Name, entry.Description)
	b.WriteString("\n请审查以下文件，输出 JSON 格式的判定结果。\n\n")

	if len(entry.AllowedTools) > 0 {
		fmt.Fprintf(&b, "可用工具: %s\n\n", strings.Join(entry.AllowedTools, ", "))
	}

	b.WriteString("### 审查文件\n\n")
	fmt.Fprintf(&b, "路径: %s\n\n", artifactPath)
	b.WriteString("### 文件内容\n\n")
	b.WriteString("```\n")
	// Truncate content to a reasonable size for the prompt
	contentRunes := []rune(content)
	if len(contentRunes) > 8000 {
		content = string(contentRunes[:8000]) + "\n...（内容过长，已截断至前 8000 字符）"
	}
	b.WriteString(content)
	b.WriteString("\n```\n\n")

	b.WriteString("### 输出格式\n\n")
	b.WriteString("请输出以下 JSON 结构（请使用 JSON 代码块包裹）：\n\n")
	b.WriteString("```json\n{\n")
	b.WriteString(`  "status": "pass" | "needs_revision" | "blocked",` + "\n")
	b.WriteString(`  "score": 0.0-1.0,` + "\n")
	b.WriteString(`  "summary": "审查摘要",` + "\n")
	b.WriteString(`  "issues": [` + "\n")
	b.WriteString(`    {` + "\n")
	b.WriteString(`      "severity": "error" | "warning" | "info",` + "\n")
	b.WriteString(`      "location": "问题所在章节或行号",` + "\n")
	b.WriteString(`      "description": "问题描述",` + "\n")
	b.WriteString(`      "suggestion": "修改建议"` + "\n")
	b.WriteString(`    }` + "\n")
	b.WriteString(`  ],` + "\n")
	b.WriteString(`  "suggestions": ["改进建议1", "改进建议2"]` + "\n")
	b.WriteString("}\n```\n")

	return b.String()
}
