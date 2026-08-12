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
	"strings"

	"github.com/xujian519/mady/retrieval/domain"
)

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
