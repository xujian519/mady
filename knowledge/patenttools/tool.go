// Package patenttools provides Sati-aligned patent-specific knowledge tools
// that operate on the XiaoNuo SQLite backend. It lives outside the knowledge
// package to avoid the import cycle between knowledge, knowledge/sqlite and
// knowledge/graph.
package patenttools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
)

// ---------------------------------------------------------------------------
// patent_wiki_search
// ---------------------------------------------------------------------------

// NewPatentWikiSearchTool creates a tool that searches XiaoNuo patent wiki cards.
func NewPatentWikiSearchTool(store *ksqlite.SQLiteStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "patent_wiki_search",
		Description: "检索专利 wiki 知识卡片（说明书/权利要求/撰写/附图），用于撰写时查询充分公开、实施例、数值范围、以说明书为依据等撰写标准。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":        map[string]any{"type": "string", "description": "检索关键词（卡片标题/概念子串匹配；空串 = 按目录列出）"},
				"dir":          map[string]any{"type": "string", "enum": []string{"specification", "claims", "drafting", "figures"}, "description": "目录过滤：specification=说明书、claims=权利要求、drafting=撰写、figures=附图"},
				"limit":        map[string]any{"type": "integer", "default": 5, "description": "返回条数上限（默认 5，最大 10）"},
				"include_body": map[string]any{"type": "boolean", "default": false, "description": "是否附带卡片正文片段"},
			},
			"required": []string{"query"},
		},
		ReadOnly: true,
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Query       string `json:"query"`
				Dir         string `json:"dir"`
				Limit       int    `json:"limit"`
				IncludeBody bool   `json:"include_body"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "专利 wiki 搜索参数格式错误"), nil
			}
			limit := p.Limit
			if limit <= 0 || limit > 10 {
				limit = 5
			}
			if store == nil {
				return agentcore.NewFailureResult("wiki 搜索不可用", "当前知识库后端不支持专利 wiki 检索"), nil
			}

			results, err := store.SearchPatentWikiCards(p.Query, p.Dir, limit, p.IncludeBody)
			if err != nil {
				return agentcore.NewFailureResult("wiki 搜索失败", err.Error()), nil
			}
			if len(results) == 0 {
				return "未找到相关 wiki 卡片", nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "找到 %d 张 wiki 卡片:\n", len(results))
			for i, r := range results {
				fmt.Fprintf(&b, "\n[%d] %s (id: %s)\n", i+1, r.Title, r.ID)
				if r.Body != "" {
					fmt.Fprintf(&b, "%s\n", knowledge.TruncateRunes(r.Body, 600))
				}
			}
			return b.String(), nil
		},
	}
}

// ---------------------------------------------------------------------------
// patent_case_search
// ---------------------------------------------------------------------------

// NewPatentCaseSearchTool creates a tool that searches XiaoNuo patent case/judgment documents.
func NewPatentCaseSearchTool(store *ksqlite.SQLiteStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "patent_case_search",
		Description: "检索本地专利判例全文（无效复审决定/专利判决，FTS5 BM25）。用于无效宣告分析、OA 答复时检索相似在先决定的理由论证与证据认定。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":           map[string]any{"type": "string", "description": "检索关键词（如 创造性 三步法、技术启示）"},
				"doc_type":        map[string]any{"type": "string", "enum": []string{"case", "judgment"}, "description": "文档类型过滤：case=无效复审决定，judgment=专利判决"},
				"court":           map[string]any{"type": "string", "description": "审理法院过滤（子串匹配，如 最高人民法院）"},
				"limit":           map[string]any{"type": "integer", "default": 5, "description": "返回条数上限（默认 5，最大 10）"},
				"include_content": map[string]any{"type": "boolean", "default": true, "description": "是否附命中片段"},
			},
			"required": []string{"query"},
		},
		ReadOnly: true,
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Query          string `json:"query"`
				DocType        string `json:"doc_type"`
				Court          string `json:"court"`
				Limit          int    `json:"limit"`
				IncludeContent bool   `json:"include_content"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "专利判例搜索参数格式错误"), nil
			}
			if p.Query == "" {
				return "请提供搜索查询", nil
			}
			limit := p.Limit
			if limit <= 0 || limit > 10 {
				limit = 5
			}
			if store == nil {
				return agentcore.NewFailureResult("判例搜索不可用", "当前知识库后端不支持专利判例检索"), nil
			}

			results, err := store.SearchPatentCases(p.Query, p.DocType, p.Court, limit, p.IncludeContent)
			if err != nil {
				return agentcore.NewFailureResult("判例搜索失败", err.Error()), nil
			}
			if len(results) == 0 {
				return "未找到相关判例", nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "找到 %d 条判例/决定:\n", len(results))
			for i, r := range results {
				fmt.Fprintf(&b, "\n[%d] %s (%s)\n", i+1, r.Title, r.DocType)
				if r.DecisionNumber != "" {
					fmt.Fprintf(&b, "决定号: %s\n", r.DecisionNumber)
				}
				if r.CaseNumber != "" {
					fmt.Fprintf(&b, "案号: %s\n", r.CaseNumber)
				}
				if r.Court != "" {
					fmt.Fprintf(&b, "法院: %s\n", r.Court)
				}
				if r.Snippet != "" {
					fmt.Fprintf(&b, "%s\n", knowledge.TruncateRunes(r.Snippet, 800))
				}
			}
			return b.String(), nil
		},
	}
}

// ---------------------------------------------------------------------------
// knowledge_note_save
// ---------------------------------------------------------------------------

// NewKnowledgeNoteSaveTool creates a tool that persists project notes to the
// writable user store. In Mady the writable store is physically separate from
// the read-only knowledge.db, so notes land in user.db and participate in
// search_knowledge via the existing RRF lane.
func NewKnowledgeNoteSaveTool(writable knowledge.WritableBackend) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "knowledge_note_save",
		Description: "把项目专利产出（OA 答复要点、无效分析结论、检索心得）沉淀到可写知识库，后续可经 search_knowledge 召回。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "笔记标题（≤200 字符）"},
				"content": map[string]any{"type": "string", "description": "笔记正文（≤20,000 字符）"},
				"project": map[string]any{"type": "string", "description": "来源项目标签（可选）"},
			},
			"required": []string{"title", "content"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Title   string `json:"title"`
				Content string `json:"content"`
				Project string `json:"project"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "笔记保存参数格式错误"), nil
			}
			p.Title = strings.TrimSpace(p.Title)
			p.Content = strings.TrimSpace(p.Content)
			if p.Title == "" || p.Content == "" {
				return "标题和内容均不能为空", nil
			}
			if len([]rune(p.Title)) > 200 {
				return "标题超过 200 字符上限", nil
			}
			if len([]rune(p.Content)) > 20000 {
				return "正文超过 20,000 字符上限", nil
			}
			if writable == nil {
				return agentcore.NewFailureResult("笔记保存不可用", "可写知识库未启用"), nil
			}

			docID := noteDocumentID(p.Project, p.Title, p.Content)
			if err := writable.AddDocument(ctx, docID, p.Title, p.Content); err != nil {
				return agentcore.NewFailureResult("笔记保存失败", err.Error()), nil
			}
			return fmt.Sprintf("已沉淀笔记（id=%s，%d 字符），后续可经 search_knowledge 召回。", docID, len(p.Content)), nil
		},
	}
}

func noteDocumentID(project, title, content string) string {
	sum := sha256.Sum256([]byte(project + "|" + title + "|" + content))
	return hex.EncodeToString(sum[:])[:16]
}
