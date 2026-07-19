package loader

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// JudgmentImportStats 记录判决/决定文书导入统计。
type JudgmentImportStats struct {
	DirsScanned  int            // 扫描的目录数
	FilesScanned int            // 扫描的文件数
	Imported     int            // 成功导入数
	Skipped      int            // 跳过数
	Errors       []string       // 非致命错误（最多 10 条）
	ByType       map[string]int // 按文档类型统计
	ByDomain     map[string]int // 按领域统计
}

// LoadJudgmentDir 遍历判决/决定文书目录，解析并导入所有符合条件的 .md 文件。
//
// 目录分类（与 wiki_metadata.go 的 classifyWikiPath 对齐）：
//
//	复审无效/    → reexam    型文档（复审无效决定分析）
//	专利判决/    → judgment  型文档（法院判决分析）
//	专利侵权/    → judgment  型文档（侵权判定分析）
//
// doc_type 元数据被设置用于后续 GraphBuilder 的节点类型映射。
func LoadJudgmentDir(store *knowledge.Store, dir string) (*JudgmentImportStats, error) {
	if store == nil {
		return nil, fmt.Errorf("judgment: store is nil")
	}
	stats := &JudgmentImportStats{
		ByType:   make(map[string]int),
		ByDomain: make(map[string]int),
	}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, fmt.Sprintf("walk %s: %v", path, err))
			}
			return nil
		}
		if info.IsDir() {
			stats.DirsScanned++
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		// 跳过索引/日志文件。
		if isMetaFile(info.Name()) {
			stats.Skipped++
			return nil
		}
		stats.FilesScanned++

		if err := importJudgmentFile(store, path, dir, stats); err != nil {
			msg := fmt.Sprintf("%s: %v", path, err)
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, msg)
			}
			return nil
		}
		stats.Imported++
		return nil
	}); err != nil {
		return nil, fmt.Errorf("judgment: walk dir %s: %w", dir, err)
	}
	if stats.Imported == 0 {
		return stats, fmt.Errorf("judgment: no documents imported from %s", dir)
	}
	return stats, nil
}

// isMetaFile 判断是否是需要跳过的元数据/索引文件。
func isMetaFile(name string) bool {
	skipped := map[string]bool{
		"index.md":  true,
		"log.md":    true,
		"CLAUDE.md": true,
	}
	return skipped[name]
}

// importJudgmentFile 解析单个判决/决定分析 Markdown 文件并导入。
func importJudgmentFile(store *knowledge.Store, filePath, rootDir string, stats *JudgmentImportStats) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	content := string(data)

	// 从相对路径推导文档类型和领域。
	relPath, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		// Rel 失败时（如跨文件系统边界）使用绝对路径作为回退。
		relPath = filePath
	}
	docID := sanitizeDocID(filepath.ToSlash(relPath))
	docType := inferDocTypeFromPath(relPath)
	domain := inferDomainFromPath(relPath)

	// 结构化解析。
	jd, err := ParseJudgmentDoc(docID, content)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	title := jd.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filePath), ".md")
	}

	// 构建元数据。
	metadata := map[string]string{
		"type":   docType,
		"domain": domain,
		"source": "wiki",
	}
	if jd.Source != "" {
		metadata["source_detail"] = jd.Source
	}
	if jd.TechField != "" {
		metadata["tech_field"] = jd.TechField
	}
	if len(jd.LawRefs) > 0 {
		metadata["law_refs"] = strings.Join(jd.LawRefs, "; ")
	}
	if len(jd.Tags) > 0 {
		metadata["tags"] = strings.Join(jd.Tags, "; ")
	}
	if jd.DecisionCount > 0 {
		metadata["decision_count"] = fmt.Sprintf("%d", jd.DecisionCount)
	}

	if err := store.AddDocument(domain, docID, title, content, "wiki"); err != nil {
		return fmt.Errorf("add document: %w", err)
	}
	if doc, ok := store.GetDocument(docID); ok {
		if doc.Metadata == nil {
			doc.Metadata = make(map[string]string, len(metadata))
		}
		maps.Copy(doc.Metadata, metadata)
		doc.Searchable = true
	}
	stats.ByType[docType]++
	stats.ByDomain[domain]++
	return nil
}

// inferDocTypeFromPath 从路径推断文档类型（与 wiki_metadata.go 对齐）。
func inferDocTypeFromPath(relPath string) string {
	p := filepath.ToSlash(relPath)
	switch {
	case strings.Contains(p, "复审无效"):
		return "reexam"
	case strings.Contains(p, "专利判决"):
		return "judgment"
	case strings.Contains(p, "专利侵权"):
		return "judgment"
	case strings.Contains(p, "侵权"):
		return "judgment"
	default:
		return "case"
	}
}

// inferDomainFromPath 从路径推断领域。
func inferDomainFromPath(relPath string) string {
	p := filepath.ToSlash(relPath)
	switch {
	case strings.Contains(p, "复审无效"):
		return "patent"
	case strings.Contains(p, "专利判决"):
		return "patent"
	case strings.Contains(p, "专利侵权"):
		return "patent"
	default:
		return "patent"
	}
}
