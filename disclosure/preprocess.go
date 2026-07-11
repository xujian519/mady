package disclosure

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/graph"
)

// docIDCounter 用于生成唯一文档 ID 的原子计数器。
var docIDCounter atomic.Int64

// preprocessNode 返回文档解析的 Pregel 节点。
// 这是确定性节点（非 LLM），负责：
//  1. 按 9 段标准章节切分交底书
//  2. 识别附图标记（如 "图 1"、"图2"、"附图一"）
//  3. 输出结构化 DisclosureDoc
func preprocessNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw := state.GetString(StateKeyInput)
		if raw == "" {
			return state, nil
		}

		doc := parseDisclosure(raw)
		state[StateKeyDoc] = doc
		return state, nil
	}
}

// parseDisclosure 执行实际的文本解析逻辑。
func parseDisclosure(raw string) *DisclosureDoc {
	doc := &DisclosureDoc{
		ID:       generateDocID(),
		RawText:  raw,
		Sections: make(map[DocSection]string),
		Format:   "txt",
		ParsedAt: time.Now(),
	}

	// 统一换行符：单遍替换
	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(raw)

	sections := splitBySections(normalized)
	for _, sec := range sections {
		doc.Sections[sec.key] = sec.content
		if sec.key == SecTitle {
			title := strings.TrimSpace(sec.content)
			// 去除 "发明名称：" / "实用新型名称：" 前缀
			title = strings.TrimPrefix(title, "发明名称：")
			title = strings.TrimPrefix(title, "发明名称:")
			title = strings.TrimPrefix(title, "实用新型名称：")
			title = strings.TrimPrefix(title, "实用新型名称:")
			doc.Title = strings.TrimSpace(title)
		}
	}

	// 提取附图标记
	doc.FigureRefs = extractFigureRefs(normalized)
	doc.HasDrawings = len(doc.FigureRefs) > 0 ||
		len(doc.Sections[SecDrawings]) > 0

	return doc
}

type rawSection struct {
	key     DocSection
	content string
}

// splitBySections 按章节标题关键词切分文档。
// 去重：同一 DocSection 出现多次时，仅保留首个标记位置（避免正文中同名关键词覆盖标题段）。
func splitBySections(text string) []rawSection {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil
	}

	// 第一遍：找出所有章节标题的位置，去重保留首个
	type marker struct {
		idx    int
		secKey DocSection
	}
	var markers []marker
	seenSections := make(map[DocSection]bool)

lineLoop:
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, pat := range sectionPatterns {
			for _, kw := range pat.Keywords {
				if strings.Contains(trimmed, kw) {
					if !seenSections[pat.Key] {
						seenSections[pat.Key] = true
						markers = append(markers, marker{idx: i, secKey: pat.Key})
					}
					continue lineLoop
				}
			}
		}
	}

	if len(markers) == 0 {
		// 无章节标题，整个文档当作"发明内容"
		return []rawSection{{key: SecContent, content: text}}
	}

	// 第二遍：按标记分配段落
	// 每个标记之后的文本属于该标记对应的章节，直到下一个标记
	var result []rawSection
	for i, m := range markers {
		start := m.idx
		end := len(lines)
		if i+1 < len(markers) {
			end = markers[i+1].idx
		}
		content := strings.TrimSpace(joinLines(lines[start:end]))
		if content != "" {
			result = append(result, rawSection{key: m.secKey, content: content})
		}
	}

	return result
}

var figureRefRe = regexp.MustCompile(`(?:图|附图|Fig(?:ure)?\.?)\s*(\d+|[一二三四五六七八九十]+)`)

// extractFigureRefs 提取附图标记并去重。
func extractFigureRefs(text string) []string {
	matches := figureRefRe.FindAllString(text, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		// 归一化空白
		normalized := strings.Join(strings.Fields(m), "")
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	return result
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func generateDocID() string {
	// 使用 atomic 计数器 + 时间戳确保唯一性（避免秒级碰撞）
	counter := docIDCounter.Add(1)
	now := time.Now()
	return "doc_" + now.Format("20060102_150405") +
		fmt.Sprintf("_%06d_%04d", now.Nanosecond()/1000, counter%10000)
}
