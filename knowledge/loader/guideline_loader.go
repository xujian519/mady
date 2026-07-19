package loader

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// GuidelineSection 是审查指南中的一个解析章节。
// 携带完整的层级路径，用于知识图谱节点创建和 FTS 检索。
type GuidelineSection struct {
	DocID    string   // 唯一文档标识
	Title    string   // 章节标题
	Part     string   // 第X部分（如 "第二部分"）
	Chapter  string   // 第X章（如 "第四章"）
	Section  string   // 章节短名（如 "审查-创造性-最接近现有技术"）
	Content  string   // 章节正文
	Keywords []string // 自动提取的关键词
	LawRefs  []string // 引用的法条列表
}

// GuidelineImportStats 记录审查指南导入操作的统计信息。
type GuidelineImportStats struct {
	Parts    int            // 发现的部分数
	Chapters int            // 发现的章数
	Sections int            // 成功导入的章节数
	Skipped  int            // 跳过的文件数
	Errors   []string       // 非致命错误（最多 10 条）
	ByPart   map[string]int // 按部分统计章节数
}

// 跳过文件清单。
var guidelineSkipFiles = map[string]bool{
	"index.md":  true,
	"log.md":    true,
	"CLAUDE.md": true,
}

// 法条引用正则。
var reLawRef = regexp.MustCompile(`专利法第(\d+)条(?:第(\d+)款)?`)

// LoadGuidelineDir 遍历审查指南目录（Obsidian wiki 结构），
// 将每个 Markdown 文件作为独立章节导入 knowledge.Store。
//
// 目录结构约定：
//
//	dir/
//	  第二部分-实质审查/           ← 部分（Part）
//	    第四章-创造性/             ← 章（Chapter）
//	      审查-创造性-最接近现有技术.md  ← 节（Section）
//	      审查-创造性-区别特征与技术问题.md
//	    第三章-新颖性/
//	      审查-新颖性-单独对比.md
//	  第一部分-初步审查/
//
// 文件名前缀 "审查-" 和 "index.md" 会被自动处理。
func LoadGuidelineDir(store *knowledge.Store, dir string) (*GuidelineImportStats, error) {
	if store == nil {
		return nil, fmt.Errorf("guideline: store is nil")
	}
	stats := &GuidelineImportStats{
		ByPart: make(map[string]int),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("guideline: read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		partName := e.Name()
		// 跳过低版本指南残留或无关目录（如以 _ 开头的目录）。
		if strings.HasPrefix(partName, "_") || strings.HasPrefix(partName, ".") {
			continue
		}
		partDir := filepath.Join(dir, partName)

		// 读取部分下的章目录。
		chapters, err := os.ReadDir(partDir)
		if err != nil {
			msg := fmt.Sprintf("%s: read chapters: %v", partName, err)
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, msg)
			}
			continue
		}
		partSections := 0
		for _, ch := range chapters {
			if !ch.IsDir() {
				continue
			}
			chapterName := ch.Name()
			chapterDir := filepath.Join(partDir, chapterName)

			count, err := loadChapterDir(store, chapterDir, partName, chapterName, stats)
			if err != nil {
				msg := fmt.Sprintf("%s/%s: %v", partName, chapterName, err)
				if len(stats.Errors) < 10 {
					stats.Errors = append(stats.Errors, msg)
				}
				continue
			}
			partSections += count
			stats.Chapters++
		}
		if partSections > 0 {
			stats.Parts++
			stats.ByPart[partName] = partSections
		}
	}
	if stats.Sections == 0 {
		return stats, fmt.Errorf("guideline: no sections imported from %s", dir)
	}
	return stats, nil
}

// loadChapterDir 处理单个章目录下的所有 .md 文件。
func loadChapterDir(store *knowledge.Store, chapterDir, partName, chapterName string, stats *GuidelineImportStats) (int, error) {
	entries, err := os.ReadDir(chapterDir)
	if err != nil {
		return 0, fmt.Errorf("read chapter dir: %w", err)
	}
	imported := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if guidelineSkipFiles[e.Name()] {
			stats.Skipped++
			continue
		}
		filePath := filepath.Join(chapterDir, e.Name())
		if err := loadSectionFile(store, filePath, partName, chapterName, stats); err != nil {
			msg := fmt.Sprintf("%s/%s/%s: %v", partName, chapterName, e.Name(), err)
			if len(stats.Errors) < 10 {
				stats.Errors = append(stats.Errors, msg)
			}
			continue
		}
		imported++
		stats.Sections++
	}
	return imported, nil
}

// loadSectionFile 解析单个章节 Markdown 文件，提取标题和正文，导入 Store。
func loadSectionFile(store *knowledge.Store, filePath, partName, chapterName string, stats *GuidelineImportStats) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	content := string(data)

	sectionName := sectionNameFromFile(filepath.Base(filePath))
	title := extractTitle(content, sectionName)
	docID := buildGuidelineDocID(partName, chapterName, sectionName)

	lawRefs := extractLawRefs(content)
	keywords := extractKeywords(title, content)

	sec := GuidelineSection{
		DocID:    docID,
		Title:    title,
		Part:     partName,
		Chapter:  chapterName,
		Section:  sectionName,
		Content:  content,
		Keywords: keywords,
		LawRefs:  lawRefs,
	}
	return importSection(store, sec)
}

// sectionNameFromFile 从文件名提取章节名。
// "审查-创造性-最接近现有技术.md" → "最接近现有技术"
// 去除 "审查-<domain>-" 前缀（前两段）和 "-拆分-N" 后缀。
func sectionNameFromFile(filename string) string {
	name := strings.TrimSuffix(filename, ".md")
	// 去掉 "-拆分-N" 及其之后的后缀（如 "-拆分-01-审查标准" → 清除）。
	name = regexp.MustCompile(`-拆分-\d+.*$`).ReplaceAllString(name, "")
	// 去掉 "审查-<domain>-" 前缀。
	if parts := strings.SplitN(name, "-", 3); len(parts) >= 3 && parts[0] == "审查" {
		name = parts[2]
	} else if len(parts) >= 2 && parts[0] == "审查" {
		name = parts[1]
	}
	return name
}

// extractTitle 从文件内容提取标题。
// 优先取首个 H1（# 标题），fallback 到文件名推导的章节名。
func extractTitle(content, fallbackTitle string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if title, ok := strings.CutPrefix(trimmed, "# "); ok {
			return strings.TrimSpace(title)
		}
	}
	// 如果没有 H1，尝试 H2（## 标题）作为标题。
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			return strings.TrimSpace(title)
		}
	}
	return fallbackTitle
}

// buildGuidelineDocID 从层级路径生成稳定文档 ID。
// 格式：guideline_2023/第二部分-实质审查/第四章-创造性/最接近现有技术
func buildGuidelineDocID(part, chapter, section string) string {
	return fmt.Sprintf("guideline_2023/%s/%s/%s", part, chapter, section)
}

// importSection 将单个 GuidelineSection 导入 knowledge.Store。
func importSection(store *knowledge.Store, sec GuidelineSection) error {
	metadata := map[string]string{
		"type":     "guideline_rule",
		"level":    "审查指南",
		"domain":   "patent",
		"part":     sec.Part,
		"chapter":  sec.Chapter,
		"section":  sec.Section,
		"doc_type": "guideline_rule",
		"source":   "patent-exam-guidelines-2023",
	}
	if len(sec.LawRefs) > 0 {
		metadata["law_refs"] = strings.Join(sec.LawRefs, "; ")
	}
	if len(sec.Keywords) > 0 {
		metadata["keywords"] = strings.Join(sec.Keywords, "; ")
	}

	if err := store.AddDocument("patent", sec.DocID, sec.Title, sec.Content, "guideline"); err != nil {
		return fmt.Errorf("add document: %w", err)
	}
	if doc, ok := store.GetDocument(sec.DocID); ok {
		if doc.Metadata == nil {
			doc.Metadata = make(map[string]string, len(metadata))
		}
		maps.Copy(doc.Metadata, metadata)
		doc.Searchable = true
	}
	return nil
}

// extractKeywords 从章节标题和正文中提取检索关键词。
func extractKeywords(title, content string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if len([]rune(s)) < 2 || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(title)
	// 从正文前 200 字提取中文词段。
	preview := content
	if len([]rune(preview)) > 200 {
		preview = string([]rune(preview)[:200])
	}
	reWord := regexp.MustCompile(`[\p{Han}]{2,8}`)
	for _, match := range reWord.FindAllString(preview, -1) {
		add(match)
	}
	return out
}

// extractLawRefs 从正文中提取引用的法条。
func extractLawRefs(content string) []string {
	matches := reLawRef.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var refs []string
	for _, m := range matches {
		ref := "专利法第" + m[1] + "条"
		if m[2] != "" {
			ref += "第" + m[2] + "款"
		}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}
