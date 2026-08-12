package search

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/retrieval/domain"
)

// extractApplicants 从结果中提取未见过的申请人（去重）。
func extractApplicants(docs []domain.DomainDocument, known []string) []string {
	seen := make(map[string]bool, len(known))
	for _, k := range known {
		seen[k] = true
	}
	var out []string
	for _, d := range docs {
		a := strings.TrimSpace(d.Metadata["assignee"])
		if a == "" || a == "-" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

// extractTerms 从结果标题/摘要中提取未见过的候选术语（≥2 字词）。
func extractTerms(docs []domain.DomainDocument, known []string) []string {
	seen := make(map[string]bool, len(known))
	for _, k := range known {
		seen[k] = true
	}
	// 用空格/常见分隔符分词（中文专利标题通常无空格，此启发式为尽力而为）。
	var candidates []string
	freq := make(map[string]int)
	for _, d := range docs {
		text := d.Title + " " + d.Snippet
		fields := strings.FieldsFunc(text, func(r rune) bool {
			return r == ' ' || r == ',' || r == '；' || r == '。' || r == '：' || r == '、'
		})
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if len([]rune(f)) < 2 || seen[f] {
				continue
			}
			freq[f]++
		}
	}
	for w, n := range freq {
		if n >= 2 {
			candidates = append(candidates, w)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return freq[candidates[i]] > freq[candidates[j]] })
	return candidates
}

// ensureFilter 向 Filters 中写入键值（保留已有键）。
func ensureFilter(f map[string]string, key, val string) map[string]string {
	if f == nil {
		f = make(map[string]string)
	}
	f[key] = val
	return f
}

// cloneFilters 深拷贝过滤 map，防止共享 map 被后续轮次修改污染请求级配置。
func cloneFilters(f map[string]string) map[string]string {
	if len(f) == 0 {
		return nil
	}
	out := make(map[string]string, len(f))
	for k, v := range f {
		out[k] = v
	}
	return out
}

// mergeUnique 合并两个字符串切片并去重。
func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// buildTable 将去重后的文献映射转为按轮次排序的总表。
func buildTable(seen map[string]CompareDoc) []CompareDoc {
	out := make([]CompareDoc, 0, len(seen))
	for _, doc := range seen {
		out = append(out, doc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Round != out[j].Round {
			return out[i].Round < out[j].Round
		}
		return out[i].Number < out[j].Number
	})
	return out
}

// buildGaps 汇总遗漏维度：轮次失败、无 IPC、无申请人、无结果等。
func buildGaps(gaps []string, applicants, ipcs []string, tableLen int) []string {
	var out []string
	out = append(out, gaps...)
	if len(ipcs) == 0 {
		out = append(out, "未获取 IPC 分类约束（如需按分类号过滤，请在请求中提供 IPCs）")
	}
	if len(applicants) == 0 {
		out = append(out, "未识别到主要申请人（结果页元数据未含 assignee 时属正常）")
	}
	if tableLen == 0 {
		out = append(out, "未检索到任何对比文件（请放宽关键词或检查数据源可用性）")
	}
	return out
}

// mdCell 清理 Markdown 表格单元格内容：转义管道符、折叠换行、截断过长值。
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// GapsText 返回 Gap 的紧凑文本（供结论拼接）。
func (r *Report) GapsText() string {
	if len(r.Gaps) == 0 {
		return "未发现明显遗漏维度。"
	}
	return "遗漏维度：" + strings.Join(r.Gaps, "；") + "。"
}

// Markdown 渲染最终报告（对齐 search-commander-report.md 结构）。
func (r *Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# 检索指挥官报告\n\n")
	fmt.Fprintf(&b, "## 检索目标\n%s\n\n", r.Target)
	fmt.Fprintf(&b, "## 总体策略\n%s\n\n", r.Strategy)

	if len(r.Rounds) > 0 {
		b.WriteString("## 各轮摘要\n\n")
		for _, rec := range r.Rounds {
			fmt.Fprintf(&b, "### 第 %d 轮：%s\n", rec.Number, rec.Phase)
			fmt.Fprintf(&b, "- 查询：`%s`\n", rec.Query)
			fmt.Fprintf(&b, "- 理由：%s\n", rec.Reason)
			fmt.Fprintf(&b, "- 命中：%d 条\n", rec.TotalHits)
			if len(rec.NewApplicants) > 0 {
				fmt.Fprintf(&b, "- 新申请人：%s\n", strings.Join(rec.NewApplicants, "、"))
			}
			if len(rec.NewTerms) > 0 {
				fmt.Fprintf(&b, "- 新术语：%s\n", strings.Join(rec.NewTerms, "、"))
			}
			fmt.Fprintf(&b, "- 反思：%s\n\n", rec.Note)
		}
	}

	if len(r.Table) > 0 {
		b.WriteString("## 对比文件总表\n\n")
		b.WriteString("| # | 文献号 | 标题 | 来源 | 命中轮次 | 申请人 |\n")
		b.WriteString("|---|--------|------|------|---------|--------|\n")
		for i, doc := range r.Table {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %d | %s |\n",
				i+1, doc.Number, mdCell(doc.Title), mdCell(doc.Source), doc.Round, mdCell(doc.Assignee))
		}
		b.WriteString("\n")
	}

	if len(r.Gaps) > 0 {
		b.WriteString("## 遗漏分析（Gap Report）\n\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(&b, "- %s\n", g)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 结论与建议\n")
	b.WriteString(r.Conclusion + "\n")
	return b.String()
}
