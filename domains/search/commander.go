// Package search 实现专利检索编排器（Search Commander 的 Go 固化版）。
//
// 将 search-commander 技能（多轮渐进式检索策略）固化为确定性 Go 编排：
// 场景匹配 → 策略模板 → 每轮检索 → 反思 → 收敛或停止 → 综合报告。
// 与 SKILL.md 的差异：不依赖 LLM 逐轮决策，改用启发式规则（命中量、
// 新申请人、新术语检测），保证结果可复现、可单测。
//
// 检索器通过 domain.DomainRetriever 接口注入（通常为 ego-browser 驱动的
// CompositeRetriever：Google Patents / CNIPA / Espacenet 三源），
// 编排器本身不感知具体数据源。
package search

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/retrieval/domain"
)

// Scene 标识检索场景，决定采用哪套策略模板。
type Scene string

const (
	// SceneAuto 自动识别场景（默认）。
	SceneAuto Scene = "auto"
	// SceneOA OA 答复 — 现有技术调查：宽搜 → IPC 引证过滤 → 二次验证。
	SceneOA Scene = "oa"
	// SceneInvalidation 无效宣告 — 证据收集：宽搜 → 引证回溯 → 穷举覆盖。
	SceneInvalidation Scene = "invalidation"
	// SceneInfringement 侵权分析：特征分解 → 同族扩展 → 国际化。
	SceneInfringement Scene = "infringement"
	// SceneFTO 自由实施（FTO）：多分类号并行 → 引用网络 → 交叉验证。
	SceneFTO Scene = "fto"
	// SceneAcademic 学术+专利混合调研：学术宽搜 → 专利窄搜 → 交叉验证。
	SceneAcademic Scene = "academic"
)

// RoundPhase 描述一轮检索的阶段语义。
type RoundPhase string

const (
	// PhaseBroad 宽语义检索（第一轮，多源并行）。
	PhaseBroad RoundPhase = "宽语义检索"
	// PhaseIPC IPC/引证过滤（追加分类号与申请人约束）。
	PhaseIPC RoundPhase = "IPC/引证过滤"
	// PhaseVerify 基于已读文本的二次检索（新术语扩展）。
	PhaseVerify RoundPhase = "二次验证"
	// PhaseExhaust 穷举覆盖（多申请人/多关键词并行）。
	PhaseExhaust RoundPhase = "穷举覆盖"
	// PhaseFamily 同族/国际化扩展。
	PhaseFamily RoundPhase = "同族/国际化扩展"
	// PhaseCross 交叉验证（专利与学术互验）。
	PhaseCross RoundPhase = "交叉验证"
)

// QueryPlan 描述一轮检索的查询构造策略。
type QueryPlan struct {
	// Phase 阶段名。
	Phase RoundPhase
	// UseIPC 是否在查询中追加 IPC 约束。
	UseIPC bool
	// UseApplicant 是否按上轮提取的申请人过滤。
	UseApplicant bool
	// ExpandKeywords 是否用上轮新术语扩充关键词。
	ExpandKeywords bool
}

// Strategy 一个场景的多轮策略模板。
type Strategy struct {
	// Name 策略名称。
	Name string
	// Plans 各轮查询计划（按序执行）。
	Plans []QueryPlan
}

// strategies 五个场景 + 默认兜底的策略模板。
var strategies = map[Scene]Strategy{
	SceneOA: {
		Name: "现有技术调查（OA 答复）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true, UseApplicant: true},
			{Phase: PhaseVerify, ExpandKeywords: true},
		},
	},
	SceneInvalidation: {
		Name: "证据收集（无效宣告）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true},
			{Phase: PhaseExhaust, UseApplicant: true, ExpandKeywords: true},
		},
	},
	SceneInfringement: {
		Name: "侵权风险排查",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true},
			{Phase: PhaseFamily, ExpandKeywords: true},
		},
	},
	SceneFTO: {
		Name: "自由实施（FTO）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true, UseApplicant: true},
			{Phase: PhaseCross, ExpandKeywords: true},
		},
	},
	SceneAcademic: {
		Name: "学术+专利混合调研",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseCross, ExpandKeywords: true},
			{Phase: PhaseVerify, UseIPC: true},
		},
	},
}

// defaultStrategy 兜底策略（场景未识别时使用）。
var defaultStrategy = Strategy{
	Name: "通用渐进式检索",
	Plans: []QueryPlan{
		{Phase: PhaseBroad},
		{Phase: PhaseIPC, UseIPC: true},
		{Phase: PhaseVerify, ExpandKeywords: true},
	},
}

// Request 一次检索编排的输入。
type Request struct {
	// Query 检索主题（必填），如 "骨髓腔输液装置的现有技术"。
	Query string
	// Scene 场景；SceneAuto 时自动识别。
	Scene Scene
	// Country 国家过滤（可选）："cn" 中国 / "global" 全球。空表示不限定。
	Country string
	// IPCs 已知 IPC 约束（可选），如 "G06F 17/30"。
	IPCs []string
	// Filters 附加结构化过滤（可选），如 {"country": "cn", "applicant": "华为"}。
	// 直接透传给检索器的 DomainQuery.Filters。
	Filters map[string]string
	// MaxRounds 最大轮次，默认 4（对齐 SKILL 规则 4）。
	MaxRounds int
	// PerRound 每轮返回条数，默认 10。
	PerRound int
}

// RoundRecord 单轮检索的执行记录（对齐 search-round-N.md）。
type RoundRecord struct {
	// Number 轮次序号（从 1 开始）。
	Number int
	// Phase 阶段名。
	Phase RoundPhase
	// Query 本轮实际查询串。
	Query string
	// Reason 本轮查询构造理由。
	Reason string
	// TotalHits 本轮总命中数。
	TotalHits int
	// TopDocs 本轮高相关结果（最多 PerRound 条）。
	TopDocs []domain.DomainDocument
	// NewTerms 本轮新发现的术语。
	NewTerms []string
	// NewApplicants 本轮新发现的申请人。
	NewApplicants []string
	// Stop 是否因收敛而停止后续轮次。
	Stop bool
	// Note 反思结论（命中量评估等）。
	Note string
}

// CompareDoc 对比文件总表中的一条记录。
type CompareDoc struct {
	// Number 文献号（公开号）。
	Number string
	// Title 标题。
	Title string
	// Source 来源（数据源名）。
	Source string
	// Round 命中轮次。
	Round int
	// URL 原文链接。
	URL string
	// Assignee 申请人（可空）。
	Assignee string
}

// Report 编排器最终报告（对齐 search-commander-report.md）。
type Report struct {
	// Target 检索目标。
	Target string
	// Strategy 采用的策略名称。
	Strategy string
	// Rounds 各轮记录。
	Rounds []RoundRecord
	// Table 对比文件总表（按文献号去重、去劣）。
	Table []CompareDoc
	// Gaps 遗漏分析（未覆盖的维度）。
	Gaps []string
	// Conclusion 结论与建议。
	Conclusion string
}

// Commander 检索编排器：确定性多轮渐进式检索。
type Commander struct {
	retriever domain.DomainRetriever
	maxRounds int
	perRound  int
}

// Option 是 Commander 的可选配置。
type Option func(*Commander)

// WithMaxRounds 覆盖默认最大轮次（默认 4）。
func WithMaxRounds(n int) Option {
	return func(c *Commander) {
		if n > 0 {
			c.maxRounds = n
		}
	}
}

// WithPerRound 覆盖每轮条数（默认 10）。
func WithPerRound(n int) Option {
	return func(c *Commander) {
		if n > 0 {
			c.perRound = n
		}
	}
}

// NewCommander 构造编排器。retriever 为 nil 时返回 nil（调用方降级处理）。
func NewCommander(retriever domain.DomainRetriever, opts ...Option) *Commander {
	if retriever == nil {
		return nil
	}
	c := &Commander{
		retriever: retriever,
		maxRounds: 4,
		perRound:  10,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// detectScene 从查询文本启发式识别场景（SceneAuto 时）。
func detectScene(q string, explicit Scene) Scene {
	if explicit != "" && explicit != SceneAuto {
		return explicit
	}
	lower := strings.ToLower(q)
	switch {
	case strings.Contains(lower, "无效") || strings.Contains(lower, "invalid"):
		return SceneInvalidation
	case strings.Contains(lower, "侵权") || strings.Contains(lower, "infring"):
		return SceneInfringement
	case strings.Contains(lower, "fto") || strings.Contains(lower, "自由实施"):
		return SceneFTO
	case strings.Contains(lower, "论文") || strings.Contains(lower, "学术") ||
		strings.Contains(lower, "paper") || strings.Contains(lower, "academic"):
		return SceneAcademic
	default:
		return SceneOA
	}
}

// Run 执行多轮渐进式检索，返回综合报告。
// 任何单轮失败不会中断整体：错误记录为 Gap 后继续后续轮次。
func (c *Commander) Run(ctx context.Context, req Request) (*Report, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("search commander: 检索主题 query 不能为空")
	}
	scene := detectScene(req.Query, req.Scene)
	strategy, ok := strategies[scene]
	if !ok {
		strategy = defaultStrategy
	}
	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = c.maxRounds
	}
	perRound := req.PerRound
	if perRound <= 0 {
		perRound = c.perRound
	}

	// 国家过滤透传（cn/global），供检索器决定源内过滤。
	// 在副本上注入，避免就地修改调用方持有的 req.Filters map。
	if req.Country != "" {
		req.Filters = ensureFilter(cloneFilters(req.Filters), "country", req.Country)
	}

	report := &Report{
		Target:   strings.TrimSpace(req.Query),
		Strategy: strategy.Name,
	}

	// 跨轮累积状态：关键词、申请人、IPC。
	var keywords []string
	var applicants []string
	ipcs := append([]string(nil), req.IPCs...)
	seenDocs := make(map[string]CompareDoc) // 文献号 → 最优记录
	var gaps []string

	for i, plan := range strategy.Plans {
		if i >= maxRounds {
			break
		}
		rec, err := c.runRound(ctx, req, plan, i+1, keywords, applicants, ipcs, perRound)
		if err != nil {
			gaps = append(gaps, fmt.Sprintf("第 %d 轮（%s）执行失败：%v", i+1, plan.Phase, err))
			continue
		}
		report.Rounds = append(report.Rounds, rec)

		// 累积新发现。
		keywords = mergeUnique(keywords, rec.NewTerms)
		applicants = mergeUnique(applicants, rec.NewApplicants)
		for _, doc := range rec.TopDocs {
			if doc.ID == "" {
				continue
			}
			best, exists := seenDocs[doc.ID]
			if !exists || rec.Number < best.Round {
				seenDocs[doc.ID] = CompareDoc{
					Number:   doc.ID,
					Title:    doc.Title,
					Source:   doc.Metadata["source"],
					Round:    rec.Number,
					URL:      doc.URL,
					Assignee: doc.Metadata["assignee"],
				}
			}
		}

		// 反思：命中量启发式决定是否提前停止。
		if rec.Stop {
			report.Conclusion = fmt.Sprintf("检索在第 %d 轮收敛（%s），命中量合理，新发现趋于稳定。", rec.Number, rec.Phase)
			break
		}
	}

	report.Table = buildTable(seenDocs)
	report.Gaps = buildGaps(gaps, applicants, ipcs, len(report.Table))
	if report.Conclusion == "" {
		report.Conclusion = fmt.Sprintf("共执行 %d 轮检索，收集对比文件 %d 篇。%s",
			len(report.Rounds), len(report.Table), report.GapsText())
	}

	// 全部轮次均失败（检索器不可用）时返回 error，调用方才能区分
	// "检索器挂了" 与 "正常无结果"。部分轮次失败已记入 Gaps，不阻塞。
	if len(report.Rounds) == 0 && len(gaps) > 0 {
		return nil, fmt.Errorf("search commander: 所有检索轮次均失败: %s", strings.Join(gaps, "; "))
	}
	return report, nil
}

// runRound 执行单轮检索并生成记录。
func (c *Commander) runRound(ctx context.Context, req Request, plan QueryPlan, number int, keywords, applicants, ipcs []string, perRound int) (RoundRecord, error) {
	q := domain.DomainQuery{
		Text:       req.Query,
		MaxResults: perRound,
	}
	// 透传请求级附加过滤（country/applicant 等），每轮共享。
	q.Filters = cloneFilters(req.Filters)
	reasonParts := []string{fmt.Sprintf("第 %d 轮：%s", number, plan.Phase)}
	queryTerms := []string{req.Query}

	// IPC 约束。
	if plan.UseIPC && len(ipcs) > 0 {
		q.Filters = ensureFilter(q.Filters, "ipc", strings.Join(ipcs, " OR "))
		queryTerms = append(queryTerms, strings.Join(ipcs, " "))
		reasonParts = append(reasonParts, "追加 IPC 约束 "+strings.Join(ipcs, ","))
	}

	// 申请人约束。
	if plan.UseApplicant && len(applicants) > 0 {
		top := applicants
		if len(top) > 3 {
			top = top[:3]
		}
		q.Filters = ensureFilter(q.Filters, "applicant", strings.Join(top, " OR "))
		queryTerms = append(queryTerms, strings.Join(top, " "))
		reasonParts = append(reasonParts, "按申请人过滤 "+strings.Join(top, ","))
	}

	// 关键词扩展。
	if plan.ExpandKeywords && len(keywords) > 0 {
		top := keywords
		if len(top) > 5 {
			top = top[:5]
		}
		q.Keywords = append(q.Keywords, top...)
		reasonParts = append(reasonParts, "扩展术语 "+strings.Join(top, ","))
	}

	rec := RoundRecord{
		Number: number,
		Phase:  plan.Phase,
		Query:  strings.Join(queryTerms, " "),
		Reason: strings.Join(reasonParts, "；"),
	}

	results, err := c.retriever.Search(ctx, q)
	if err != nil {
		return rec, fmt.Errorf("检索失败: %w", err)
	}
	if results == nil {
		rec.Note = "数据源无返回"
		rec.Stop = true
		return rec, nil
	}

	rec.TotalHits = results.TotalCount
	rec.TopDocs = results.Documents
	if len(rec.TopDocs) > perRound {
		rec.TopDocs = rec.TopDocs[:perRound]
	}

	// 从本轮结果提取新发现（申请人 + 术语）。
	rec.NewApplicants = extractApplicants(rec.TopDocs, applicants)
	rec.NewTerms = extractTerms(rec.TopDocs, keywords)

	// 反思：命中量启发式。
	// 注意：当前数据源（BrowserRetriever/CompositeRetriever）的 TotalCount
	// 恒等于返回条数（被 perRound 裁剪），因此无法获知真实总命中数。
	// 用"是否满量返回"作为"源上可能还有更多结果"的代理信号。
	hits := rec.TotalHits
	full := hits >= perRound // 返回达到上限，可能被截断
	// 收敛判断从第二轮起生效：首轮的新发现相对初始状态必然非空，
	// 只有后续轮次相对已累积知识无新增时才算"趋稳"。
	stable := number > 1 && len(rec.NewTerms) == 0 && len(rec.NewApplicants) == 0 && len(rec.TopDocs) > 0
	switch {
	case hits == 0:
		rec.Note = "命中 0 条：范围过窄，建议放宽关键词或去除 IPC 约束"
		rec.Stop = true
	case hits < 5 && stable:
		rec.Note = fmt.Sprintf("命中 %d 条：偏窄且无新增发现，判定收敛", hits)
		rec.Stop = true
	case hits < 5:
		rec.Note = fmt.Sprintf("命中 %d 条：偏窄，可能遗漏；下一轮建议放宽", hits)
	case full && !stable:
		rec.Note = fmt.Sprintf("返回 %d 条达上限：源上可能还有更多，建议下一轮收紧（追加 IPC/申请人）", hits)
	case stable:
		rec.Note = fmt.Sprintf("命中 %d 条：量级合理，新发现趋稳", hits)
		rec.Stop = true
	default:
		rec.Note = fmt.Sprintf("命中 %d 条：量级合理", hits)
	}
	return rec, nil
}

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
