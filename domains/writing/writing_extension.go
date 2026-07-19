package writing

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

const ExtensionName = "writing"

// Extension adapts the Skill Distillation system as an agentcore Extension.
//
// In v1, patterns are tool-queryable only — agents call query_writing_patterns
// when they need writing guidance. System prompt injection is deferred until
// the pattern library matures (≥30 patterns, ≥50 user feedback ratings).
//
// Maturity thresholds for system prompt injection:
//   - Pattern count ≥ 30
//   - User feedback ≥ 50 ratings (excellent/good ≥ 40)
//   - Auto-score vs user-score agreement ≥ 80%
type Extension struct {
	store    *PatternStore
	compiler *SkillCompiler
}

// NewExtension creates a writing extension with the given pattern store.
func NewExtension(store *PatternStore) *Extension {
	return &Extension{
		store:    store,
		compiler: NewSkillCompiler(store),
	}
}

// Interface assertions.
var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

func (e *Extension) Name() string                                     { return ExtensionName }
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *Extension) Dispose() error                                   { return nil }

// Tools registers writing-related tools.
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "query_writing_patterns",
			Description: "查询写作模式库，获取匹配当前案件的写作指导。输入案件类型和/或技术特征标签，返回推荐的写作模式和技巧。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"case_type": map[string]any{
						"type":        "string",
						"description": "案件类型，如 invalidation/patentability/oa_response/claim_drafting/spec_drafting/disclosure",
					},
					"features": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "技术特征或关键词，如 [\"创造性三步法\", \"功能性限定\"]",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "自由文本搜索关键词（可选，设置了则按此搜索而非按案件类型匹配）",
					},
				},
			},
			Func: e.handleQuery,
		},
		{
			Name:        "list_writing_patterns",
			Description: "列出写作模式库中的所有模式及其分类。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Func: e.handleList,
		},
	}
}

// handleQuery implements the query_writing_patterns tool.
func (e *Extension) handleQuery(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		CaseType string   `json:"case_type"`
		Features []string `json:"features"`
		Query    string   `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return fmt.Errorf("query_writing_patterns: invalid parameters: %w", err), nil
	}

	var patterns []*WritingPattern
	if params.Query != "" {
		patterns = e.store.SearchPatterns(params.Query, "", 5)
	} else if params.CaseType != "" {
		patterns = e.store.MatchPatterns(params.CaseType, params.Features)
	} else {
		// List all if no filter provided.
		all := e.store.All()
		if len(all) > 5 {
			all = all[:5]
		}
		patterns = all
	}
	if len(patterns) == 0 {
		return "未找到匹配的写作模式。你可以尝试不同的关键词，或使用 list_writing_patterns 查看所有可用模式。", nil
	}
	return e.compiler.CompileMarkdown(patterns), nil
}

// handleList implements the list_writing_patterns tool.
func (e *Extension) handleList(ctx context.Context, args json.RawMessage) (any, error) {
	all := e.store.All()
	if len(all) == 0 {
		return "写作模式库为空。", nil
	}
	// Group by category.
	byCategory := make(map[string][]*WritingPattern)
	for _, p := range all {
		cat := p.Category
		if cat == "" {
			cat = "未分类"
		}
		byCategory[cat] = append(byCategory[cat], p)
	}
	// Sort categories.
	cats := make([]string, 0, len(byCategory))
	for c := range byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	var b strings.Builder
	fmt.Fprintf(&b, "## 📚 写作模式库（共 %d 个模式）\n\n", len(all))
	for _, cat := range cats {
		patterns := byCategory[cat]
		fmt.Fprintf(&b, "### %s (%d 个)\n\n", cat, len(patterns))
		for _, p := range patterns {
			fmt.Fprintf(&b, "- **%s** — %s\n", p.Name, p.Summary)
		}
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	b.WriteString("使用 `query_writing_patterns` 工具查询具体模式。\n")
	return b.String(), nil
}
