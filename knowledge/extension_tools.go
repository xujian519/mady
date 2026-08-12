package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (e *KnowledgeExtension) handleSearch(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Query == "" {
		return "请提供搜索查询", nil
	}
	if p.TopK <= 0 {
		p.TopK = 5
	}

	results := e.search(ctx, p.Query, p.TopK)
	if len(results) == 0 {
		return "未找到相关文档", nil
	}
	return formatToolResults(results), nil
}

// handleSearchLaws processes the search_laws tool call. It delegates to
// the configured LawSearcher function.
func (e *KnowledgeExtension) handleSearchLaws(_ context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Query == "" {
		return "请提供搜索查询", nil
	}
	if p.TopK <= 0 {
		p.TopK = 5
	}
	if e.lawSearcher == nil {
		return "法律法规搜索功能未启用", nil
	}

	results, err := e.lawSearcher(p.Query, p.TopK)
	if err != nil {
		return fmt.Sprintf("搜索法律法规失败: %v", err), nil
	}
	if len(results) == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 相关的法律法规", p.Query), nil
	}

	var b strings.Builder
	b.WriteString("法律法规搜索结果:\n")
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] %s (%s)\n", i+1, r.Name, r.Level)
		if r.Subtitle != "" {
			fmt.Fprintf(&b, "    %s\n", r.Subtitle)
		}
		fmt.Fprintf(&b, "    分类: %s\n", r.Category)
		// Truncate content to avoid overwhelming the model.
		content := r.Content
		if len(content) > 2000 {
			content = content[:2000] + "..."
		}
		fmt.Fprintf(&b, "    %s\n", content)
	}
	fmt.Fprintf(&b, "\n共 %d 条结果", len(results))
	return b.String(), nil
}

// handleAddDocument processes the add_document tool call. It delegates to
// the configured WritableBackend to chunk, embed, and persist the document.
func (e *KnowledgeExtension) handleAddDocument(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		DocID   string `json:"doc_id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.DocID == "" {
		return "请提供文档标识 (doc_id)", nil
	}
	if p.Content == "" {
		return "请提供文档内容 (content)", nil
	}
	if e.writable == nil {
		return "文档写入功能未启用", nil
	}
	if err := e.writable.AddDocument(ctx, p.DocID, p.Title, p.Content); err != nil {
		return fmt.Sprintf("文档添加失败: %v", err), nil
	}
	return fmt.Sprintf("文档 \"%s\" 已成功添加到知识库（%d 字符）", p.DocID, len(p.Content)), nil
}
