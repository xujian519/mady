package main

// knowledge_command.go implements /knowledge slash command for browsing and
// searching the knowledge base (FTS + vector) from the TUI.
//
// Depends on fc.KnowledgeBackend (initialized in bootstrap/setup.go as part
// of LoadWikiStore). When the backend is unavailable, shows a helpful hint.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/retrieval"
)

const maxKnowledgeResults = 15

// handleKnowledgeCommand implements /knowledge [query].
// Without arguments, shows available knowledge domains.
// With arguments, performs FTS search and returns top results.
func (s *tuiSession) handleKnowledgeCommand(query string) {
	if s.fc == nil || s.fc.KnowledgeBackend == nil {
		msg := "⚠ 知识库未加载。"
		if s.fc != nil && s.fc.WikiStore != nil {
			msg += " Wiki 存储已加载，可尝试 /evidence 查看引用证据。"
		} else {
			msg += " 请确保 MADY_HOME/knowledge.db 或 WIKI_PATH 已配置。"
		}
		s.app.PrintSystem(msg)
		return
	}

	if query == "" {
		// Show overview without a search.
		s.app.PrintSystem("📚 知识库已就绪。\n" +
			"用法: /knowledge <搜索关键词>\n" +
			"例如: /knowledge 专利法第22条\n" +
			"      /knowledge 创造性判断三步法")
		return
	}

	results, err := s.fc.KnowledgeBackend.FTSSearch(query, maxKnowledgeResults)
	if err != nil {
		s.app.PrintSystem(fmt.Sprintf("❌ 知识检索失败: %v", err))
		return
	}

	if len(results) == 0 {
		s.app.PrintSystem(fmt.Sprintf("📚 未找到与「%s」相关的知识条目。尝试更换关键词。", query))
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📚 知识检索结果 — 「%s」（%d 条）\n", query, len(results))
	for i, r := range results {
		fmt.Fprintf(&b, "\n  %d. [%.2f] ", i+1, r.Score)
		title := extractChunkTitle(r)
		if title != "" {
			fmt.Fprintf(&b, "%s\n", title)
		}
		// Excerpt: first 120 chars of content.
		excerpt := truncateRunes(r.Content, 120)
		if excerpt != "" {
			fmt.Fprintf(&b, "     %s\n", excerpt)
		}
		if src, ok := r.Metadata["source"]; ok && src != "" {
			fmt.Fprintf(&b, "     (%s)\n", src)
		}
	}
	s.app.PrintSystem(b.String())
}

// extractChunkTitle attempts to extract a meaningful title from a ScoredChunk.
func extractChunkTitle(r retrieval.ScoredChunk) string {
	if title, ok := r.Metadata["title"]; ok && title != "" {
		return title
	}
	if src, ok := r.Metadata["source"]; ok && src != "" {
		return src
	}
	if docID, ok := r.Metadata["doc_id"]; ok && docID != "" {
		return docID
	}
	// Use the first line of content as title.
	if r.Content != "" {
		if idx := strings.Index(r.Content, "\n"); idx > 0 {
			return r.Content[:idx]
		}
		if len(r.Content) > 60 {
			return r.Content[:60] + "…"
		}
		return r.Content
	}
	return ""
}
