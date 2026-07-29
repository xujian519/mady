package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the checker extension.
const ExtensionName = "checker"

// Extension is an agentcore extension that provides checker review tools.
type Extension struct {
	catalog  *Catalog
	dispatch *Dispatch
}

// Compile-time interface checks.
var (
	_ agentcore.Extension            = (*Extension)(nil)
	_ agentcore.ToolProvider         = (*Extension)(nil)
	_ agentcore.SystemPromptProvider = (*Extension)(nil)
)

// NewExtension creates a new checker extension with the given catalog.
// When catalog is nil, DefaultCatalog() is used.
func NewExtension(catalog *Catalog) *Extension {
	if catalog == nil {
		catalog = DefaultCatalog()
	}
	return &Extension{
		catalog:  catalog,
		dispatch: NewDispatch(catalog),
	}
}

// Name returns the extension name.
func (e *Extension) Name() string { return ExtensionName }

// RegisterHandler binds a handler function to a checker RoleID.
func (e *Extension) RegisterHandler(roleID string, handler CheckerHandler) {
	e.dispatch.RegisterHandler(roleID, handler)
}

// Catalog returns the checker catalog.
func (e *Extension) Catalog() *Catalog { return e.catalog }

// Dispatch returns the checker dispatch.
func (e *Extension) Dispatch() *Dispatch { return e.dispatch }

// Init initializes the extension.
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error {
	return nil
}

// Dispose cleans up the extension.
func (e *Extension) Dispose() error { return nil }

// Tools returns the checker tools registered by this extension.
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		e.newSuggestCheckersTool(),
		e.newRunCheckerReviewTool(),
	}
}

// SystemPromptSuffix appends checker usage instructions to the system prompt.
func (e *Extension) SystemPromptSuffix() string {
	var b strings.Builder
	b.WriteString("\n\n## Checker 复核工具\n\n")
	b.WriteString("在专利撰写/分析完成后，应使用以下复核流程：\n")
	b.WriteString("1. 先用 `patent_eval` 工具做自动预检\n")
	b.WriteString("2. 再用 `suggest_checkers` 查看可用的检查器\n")
	b.WriteString("3. 最后用 `run_checker_review` 执行复核\n\n")
	b.WriteString("可用检查器：\n")
	for _, entry := range e.catalog.List() {
		fmt.Fprintf(&b, "- **%s** (%s): %s\n", entry.RoleID, entry.Name, entry.Description)
	}
	return b.String()
}

// newSuggestCheckersTool creates the suggest_checkers tool.
func (e *Extension) newSuggestCheckersTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "suggest_checkers",
		Description: "根据产出文件路径，推荐适用的 Checker 复核工具。先调用此工具查看可用检查器列表，再决定是否执行复核。",
		ReadOnly:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact_path": map[string]any{
					"type":        "string",
					"description": "产出文件的路径（如 outputs/technical-analysis.md）",
				},
			},
			"required": []string{"artifact_path"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				ArtifactPath string `json:"artifact_path"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return fmt.Sprintf("参数解析错误: %v", err), nil
			}

			entries := e.catalog.Suggest(p.ArtifactPath)
			if len(entries) == 0 {
				return fmt.Sprintf("未找到匹配 %q 的检查器。可用的检查器:\n%s",
					p.ArtifactPath, formatAllCheckers(e.catalog.List())), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "匹配到 %d 个检查器：\n\n", len(entries))
			for _, entry := range entries {
				fmt.Fprintf(&b, "**%s** (%s)\n", entry.RoleID, entry.Name)
				fmt.Fprintf(&b, "  - 描述: %s\n", entry.Description)
				if len(entry.InvokesAfter) > 0 {
					fmt.Fprintf(&b, "  - 在以下步骤后执行: %s\n", strings.Join(entry.InvokesAfter, ", "))
				}
				hitl := "否"
				if entry.HITLCheckpoint {
					hitl = "是（需要人工确认）"
				}
				fmt.Fprintf(&b, "  - 需要人工审批: %s\n", hitl)
				b.WriteString("\n")
			}
			b.WriteString("使用 `run_checker_review(role_id=\"<检查器ID>\", content=\"<文件内容>\")` 执行复核。")
			return b.String(), nil
		},
	}
}

// newRunCheckerReviewTool creates the run_checker_review tool.
func (e *Extension) newRunCheckerReviewTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "run_checker_review",
		Description: "对专利产出文件执行 Checker 质检复核。返回结构化判定结果（pass/needs_revision/blocked）以及发现的问题清单和改进建议。在提交 HITL 人工审批前使用。",
		ReadOnly:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"role_id": map[string]any{
					"type":        "string",
					"description": "检查器 ID（reviewer / quality_checker / novelty_checker / oa_checker），使用 suggest_checkers 查看可用列表",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "待审查的文件内容",
				},
				"artifact_path": map[string]any{
					"type":        "string",
					"description": "文件路径（可选，用于日志和上下文）",
				},
			},
			"required": []string{"role_id", "content"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				RoleID       string `json:"role_id"`
				Content      string `json:"content"`
				ArtifactPath string `json:"artifact_path,omitempty"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return fmt.Sprintf("参数解析错误: %v", err), nil
			}

			entry := e.catalog.Get(p.RoleID)
			if entry == nil {
				return fmt.Sprintf("检查器 %q 不存在。可用检查器:\n%s",
					p.RoleID, formatAllCheckers(e.catalog.List())), nil
			}

			verdict, err := e.dispatch.RunChecker(ctx, p.RoleID, p.ArtifactPath, p.Content)
			if err != nil {
				return fmt.Sprintf("复核执行失败: %v", err), nil
			}

			return FormatVerdict(verdict), nil
		},
	}
}

func formatAllCheckers(entries []CheckerEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "- %s: %s\n", e.RoleID, e.Name)
	}
	return b.String()
}
