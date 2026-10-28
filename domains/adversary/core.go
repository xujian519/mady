package adversary

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 五步处理流程的确定性部分：
// 1. 独立争议定位（只读通知书结构化字段与权利要求书）
// 2. 独立证据评估（答复计划覆盖率与权项对齐）
// 4. 分歧检测（以法条为键比对基线覆盖）
// 5. 报告聚合
// 第 3 步（独立规则适用）当前并入 1/2；模型只补充争议点与摘要。

// LocateDisputesIndependent 从通知书结构化字段独立识别争议法条。
//
// 优先使用「驳回类型 → 法条」映射；无法映射的类型记为 unverified。
func LocateDisputesIndependent(officeAction *OfficeAction, claimsText string) []Dispute {
	if officeAction == nil {
		statutes := ScanDisputeText(claimsText)
		claims := ScanClaimNumbers(claimsText)
		disputes := make([]Dispute, 0, len(statutes))
		for _, statute := range statutes {
			disputes = append(disputes, Dispute{
				Statute: statute, Claims: claims,
				Point:      fmt.Sprintf("正文提及法条 %s，但缺少通知书结构化字段，无法确认争议要点", statute),
				Relation:   RelationMissedByBaseline,
				Confidence: 0.3,
				Tag:        TagUnverified,
			})
		}
		return disputes
	}

	var disputes []Dispute
	seen := make(map[string]bool)
	for _, ground := range officeAction.RejectionTypes {
		statute := GroundToStatute[ground]
		if statute == "" || seen[statute] {
			continue
		}
		seen[statute] = true
		disputes = append(disputes, Dispute{
			Statute: statute, Claims: claimsOrScan(officeAction.AffectedClaims, claimsText),
			Point:      fmt.Sprintf("通知书驳回理由 %s 对应法条 %s", ground, statute),
			Relation:   RelationMissedByBaseline, // 待分歧检测修正
			Confidence: 0.8,
			Tag:        TagHeuristic,
		})
	}
	for _, statute := range ScanDisputeText(claimsText) {
		if seen[statute] {
			continue
		}
		seen[statute] = true
		disputes = append(disputes, Dispute{
			Statute: statute, Claims: claimsOrScan(officeAction.AffectedClaims, claimsText),
			Point:      fmt.Sprintf("正文提及法条 %s，但通知书结构化字段未列明", statute),
			Relation:   RelationMissedByBaseline,
			Confidence: 0.5,
			Tag:        TagHeuristic,
		})
	}
	return disputes
}

func claimsOrScan(affected []int, claimsText string) []int {
	if len(affected) > 0 {
		return append([]int(nil), affected...)
	}
	return ScanClaimNumbers(claimsText)
}

// AssessEvidenceIndependent 从对立视角评估答复计划：是否覆盖全部驳回理由、
// 每条策略的权项是否与通知书涉及权项对齐。
func AssessEvidenceIndependent(officeAction *OfficeAction, plan []PlanEntry) []Dispute {
	if officeAction == nil || len(plan) == 0 {
		return nil
	}
	var disputes []Dispute
	covered := make(map[string]bool, len(plan))
	for _, entry := range plan {
		covered[entry.Ground] = true
	}
	for _, ground := range officeAction.RejectionTypes {
		if covered[ground] {
			continue
		}
		statute := GroundToStatute[ground]
		if statute == "" {
			continue
		}
		disputes = append(disputes, Dispute{
			Statute:          statute,
			Claims:           append([]int(nil), officeAction.AffectedClaims...),
			Point:            fmt.Sprintf("答复计划未覆盖驳回理由 %s，存在遗漏风险", ground),
			Relation:         RelationDisagreement,
			DivergenceReason: fmt.Sprintf("答复计划未覆盖驳回理由 %s", ground),
			Confidence:       0.9,
			Tag:              TagHeuristic,
		})
	}
	for _, entry := range plan {
		if len(entry.Claims) == 0 || len(officeAction.AffectedClaims) == 0 {
			continue
		}
		if intersects(entry.Claims, officeAction.AffectedClaims) {
			continue
		}
		statute := GroundToStatute[entry.Ground]
		if statute == "" {
			continue
		}
		disputes = append(disputes, Dispute{
			Statute: statute,
			Claims:  append([]int(nil), entry.Claims...),
			Point: fmt.Sprintf("策略 %s（理由 %s）的权项 %s 与通知书 affectedClaims %s 无交集",
				entry.Strategy, entry.Ground, joinInts(entry.Claims), joinInts(officeAction.AffectedClaims)),
			Relation:         RelationDisagreement,
			DivergenceReason: "策略权项与通知书涉及权项不对齐",
			Confidence:       0.7,
			Tag:              TagHeuristic,
		})
	}
	return disputes
}

func intersects(left, right []int) bool {
	set := make(map[int]bool, len(right))
	for _, value := range right {
		set[value] = true
	}
	for _, value := range left {
		if set[value] {
			return true
		}
	}
	return false
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%d", value)
	}
	return strings.Join(parts, ", ")
}

// MergeHeuristicDisputes 合并独立定位与证据评估结果：按法条去重，
// 置信度高者优先；置信度相同时取权项并集。
func MergeHeuristicDisputes(located, assessed []Dispute) []Dispute {
	byStatute := make(map[string]int)
	var merged []Dispute
	add := func(dispute Dispute) {
		index, seen := byStatute[dispute.Statute]
		if !seen {
			byStatute[dispute.Statute] = len(merged)
			merged = append(merged, dispute)
			return
		}
		existing := merged[index]
		if dispute.Confidence > existing.Confidence {
			merged[index] = dispute
			return
		}
		if dispute.Confidence == existing.Confidence {
			existing.Claims = unionInts(existing.Claims, dispute.Claims)
			merged[index] = existing
		}
	}
	for _, dispute := range located {
		add(dispute)
	}
	for _, dispute := range assessed {
		add(dispute)
	}
	return merged
}

// MergeWithModel 把模型争议点合并到确定性结果上：同法条时模型须置信度更高才覆盖。
// 用严格大于而非「不低于」：模型未写 confidence 时按 0.5 计，若允许相等覆盖，
// 一个空壳模型条目就能挤掉确定性结果，削弱「模型不可靠时回退确定性」的语义。
func MergeWithModel(heuristic, model []Dispute) []Dispute {
	byStatute := make(map[string]int, len(heuristic)+len(model))
	merged := make([]Dispute, 0, len(heuristic)+len(model))
	for _, dispute := range heuristic {
		byStatute[dispute.Statute] = len(merged)
		merged = append(merged, dispute)
	}
	for _, dispute := range model {
		index, seen := byStatute[dispute.Statute]
		if !seen {
			byStatute[dispute.Statute] = len(merged)
			merged = append(merged, dispute)
			continue
		}
		if dispute.Confidence > merged[index].Confidence {
			merged[index] = dispute
		}
	}
	return merged
}

func unionInts(left, right []int) []int {
	set := make(map[int]bool, len(left)+len(right))
	for _, value := range left {
		set[value] = true
	}
	for _, value := range right {
		set[value] = true
	}
	return normalizeClaims(set)
}

// DetectDivergence 以法条为键比对基线答复计划覆盖情况并标记关系。
func DetectDivergence(disputes []Dispute, plan []PlanEntry) []Dispute {
	covered := make(map[string]bool, len(plan))
	for _, entry := range plan {
		if statute := GroundToStatute[entry.Ground]; statute != "" {
			covered[statute] = true
		}
	}
	marked := make([]Dispute, len(disputes))
	for i, dispute := range disputes {
		switch {
		case dispute.Relation == RelationDisagreement:
			marked[i] = dispute
		case covered[dispute.Statute]:
			dispute.Relation = RelationAgreement
			marked[i] = dispute
		default:
			dispute.Relation = RelationMissedByBaseline
			marked[i] = dispute
		}
	}
	return marked
}

// ComputeOverallConfidence 按争议点自身置信度加权：Σ(c²)/Σ(c)；无争议点为 0。
func ComputeOverallConfidence(disputes []Dispute) float64 {
	var total, weighted float64
	for _, dispute := range disputes {
		total += dispute.Confidence
		weighted += dispute.Confidence * dispute.Confidence
	}
	if total == 0 {
		return 0
	}
	return weighted / total
}

// BuildReport 聚合争议点为结构化报告。
func BuildReport(disputes []Dispute, summary string) Report {
	report := Report{Disputes: disputes, Summary: summary}
	for _, dispute := range disputes {
		switch dispute.Relation {
		case RelationAgreement:
			report.Agreements = append(report.Agreements, dispute)
		case RelationDisagreement:
			report.Disagreements = append(report.Disagreements, dispute)
		default:
			report.MissedByBaseline = append(report.MissedByBaseline, dispute)
		}
		if dispute.Tag == TagUnverified {
			report.Unverified = append(report.Unverified, dispute)
		}
	}
	report.OverallConfidence = ComputeOverallConfidence(disputes)
	return report
}

// DegradedSummary 生成模型不可用时的降级摘要。
func DegradedSummary(disputes []Dispute) string {
	if len(disputes) == 0 {
		return "模型不可用，未识别到争议点。"
	}
	unverified := 0
	for _, dispute := range disputes {
		if dispute.Tag == TagUnverified {
			unverified++
		}
	}
	if unverified == len(disputes) {
		return fmt.Sprintf("模型不可用，%d 条争议点均为未验证占位。", len(disputes))
	}
	return fmt.Sprintf("模型不可用，基于确定性规则识别 %d 条争议点（%d 条未验证）。", len(disputes), unverified)
}

// RenderReport 渲染报告为可读文本。
func RenderReport(report Report) string {
	lines := []string{
		fmt.Sprintf("对抗式检验报告：共 %d 条争议点（一致 %d、分歧 %d、基线未覆盖 %d、未验证 %d）",
			len(report.Disputes), len(report.Agreements), len(report.Disagreements),
			len(report.MissedByBaseline), len(report.Unverified)),
		fmt.Sprintf("整体置信度：%.1f%%", report.OverallConfidence*100),
	}
	if strings.TrimSpace(report.Summary) != "" {
		lines = append(lines, "", "摘要："+report.Summary)
	}
	appendDisputeSection := func(title string, disputes []Dispute) {
		if len(disputes) == 0 {
			return
		}
		lines = append(lines, "", title+":")
		for _, dispute := range disputes {
			claims := "未指定"
			if len(dispute.Claims) > 0 {
				claims = joinInts(dispute.Claims)
			}
			lines = append(lines, fmt.Sprintf("- %s（权项 %s）：%s", dispute.Statute, claims, dispute.Point))
			if dispute.DivergenceReason != "" {
				lines = append(lines, "  原因："+dispute.DivergenceReason)
			}
		}
	}
	appendDisputeSection("分歧项（需重点关注）", report.Disagreements)
	appendDisputeSection("基线未覆盖项（答复计划可能遗漏）", report.MissedByBaseline)
	appendDisputeSection("一致项（双方识别相同）", report.Agreements)
	appendDisputeSection("未验证项（模型不可用时的占位）", report.Unverified)
	lines = append(lines, "", "限制说明：本报告从独立视角评估答复计划，不消费己方中间推理。"+
		"确定性部分覆盖法条识别、计划覆盖率与权项对齐；更深的论证强度评估需人工复核。")
	return strings.Join(lines, "\n")
}

// ModelOutput 是模型输出的结构化结果。
type ModelOutput struct {
	Disputes          []Dispute
	OverallAssessment string
	DivergenceReason  string
}

type modelDisputeJSON struct {
	Statute    string   `json:"statute"`
	Claims     []int    `json:"claims"`
	Point      string   `json:"point"`
	Weakness   string   `json:"weakness"`
	Confidence *float64 `json:"confidence"`
}

type modelOutputJSON struct {
	Disputes          []modelDisputeJSON `json:"disputes"`
	OverallAssessment string             `json:"overall_assessment"`
	DivergenceReason  string             `json:"divergence_reason"`
}

// ParseModelOutput 解析模型返回的 JSON（容忍 markdown 代码围栏）。
// 结构不合法时返回 false，调用方走降级路径。
func ParseModelOutput(raw string) (ModelOutput, bool) {
	text := strings.TrimSpace(raw)
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}
	var parsed modelOutputJSON
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return ModelOutput{}, false
	}
	disputes := make([]Dispute, 0, len(parsed.Disputes))
	for _, item := range parsed.Disputes {
		statute := strings.TrimSpace(item.Statute)
		if statute == "" {
			continue
		}
		confidence := 0.5
		if item.Confidence != nil && *item.Confidence >= 0 && *item.Confidence <= 1 {
			confidence = *item.Confidence
		}
		point := item.Point
		dispute := Dispute{
			Statute: statute, Claims: normalizeClaims(toIntSet(item.Claims)),
			Point: point, Relation: RelationDisagreement,
			Confidence: confidence, Tag: TagHeuristic,
		}
		if item.Weakness != "" {
			if point != "" {
				dispute.Point = point + "——" + item.Weakness
			} else {
				dispute.Point = item.Weakness
			}
			dispute.DivergenceReason = item.Weakness
		}
		disputes = append(disputes, dispute)
	}
	return ModelOutput{
		Disputes:          disputes,
		OverallAssessment: parsed.OverallAssessment,
		DivergenceReason:  parsed.DivergenceReason,
	}, true
}

func toIntSet(values []int) map[int]bool {
	set := make(map[int]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// BuildPrompt 构建对立审查员提示词（外部数据放在【】区块内，与指令区分离）。
func BuildPrompt(officeAction *OfficeAction, claimsText string, plan []PlanEntry, planSummary string) string {
	lines := []string{
		"你是一位严格的专利审查员，请从对立视角评估以下意见陈述书答复计划的薄弱环节。",
		"",
		"任务要求：",
		"1. 独立识别审查意见中的全部驳回理由与涉及权利要求",
		"2. 逐条评估答复计划中每个策略的证据支撑强度与论证逻辑",
		"3. 识别答复计划的遗漏、权项不对齐、证据不足等薄弱环节",
		"4. 对每个争议点给出置信度评估（0-1）",
		"",
		"请严格输出以下 JSON：",
		`{"disputes":[{"statute":"A22.3","claims":[1,2],"point":"争议要点","weakness":"薄弱环节","confidence":0.8}],` +
			`"overall_assessment":"整体评估","divergence_reason":"主要分歧原因"}`,
		"",
		"【审查意见】",
		formatOfficeAction(officeAction),
		"",
		"【权利要求】",
		fallbackText(claimsText, "(无权利要求文本)"),
		"",
		"【答复计划】",
		formatPlan(plan),
	}
	if strings.TrimSpace(planSummary) != "" {
		lines = append(lines, "", "【答复计划摘要】", planSummary)
	}
	return strings.Join(lines, "\n")
}

func formatOfficeAction(officeAction *OfficeAction) string {
	if officeAction == nil {
		return "(无审查意见结构化字段)"
	}
	var parts []string
	if len(officeAction.RejectionTypes) > 0 {
		parts = append(parts, "驳回理由："+strings.Join(officeAction.RejectionTypes, "、"))
	}
	if len(officeAction.AffectedClaims) > 0 {
		parts = append(parts, "涉及权利要求："+joinInts(officeAction.AffectedClaims))
	}
	if len(officeAction.ExaminerArguments) > 0 {
		parts = append(parts, "审查员论点原文："+strings.Join(officeAction.ExaminerArguments, "\n"))
	}
	if len(parts) == 0 {
		return "(审查意见结构化字段为空)"
	}
	return strings.Join(parts, "\n")
}

func formatPlan(plan []PlanEntry) string {
	if len(plan) == 0 {
		return "(无答复计划)"
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "(答复计划序列化失败)"
	}
	return string(data)
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ForbiddenKeys 是禁止出现在对立审查输入中的键（己方中间推理产物）。
var ForbiddenKeys = []string{
	"dispute_framing_report",
	"dispute_framing_candidates",
	"dispute_framing_claims",
	"dispute_framing_refined",
	"dispute_framing_refined_candidates",
	"claim_chart",
	"draft",
}

// ForbiddenOutputTerms 是禁止出现在对立审查输出中的术语（防止反向泄漏）。
var ForbiddenOutputTerms = []string{"dispute_framing", "claim_chart", "己方推理", "内部分析"}

// IsolationResult 是隔离检查结果。
type IsolationResult struct {
	Clean      bool     `json:"clean"`
	LeakedKeys []string `json:"leaked_keys"`
}

// CheckInputIsolation 检查输入键列表是否包含禁止键。
func CheckInputIsolation(stateKeys []string) IsolationResult {
	present := make(map[string]bool, len(stateKeys))
	for _, key := range stateKeys {
		present[key] = true
	}
	var leaked []string
	for _, key := range ForbiddenKeys {
		if present[key] {
			leaked = append(leaked, key)
		}
	}
	return IsolationResult{Clean: len(leaked) == 0, LeakedKeys: leaked}
}

// ScanOutputForLeaks 扫描输出文本是否包含禁止术语。
func ScanOutputForLeaks(report string) IsolationResult {
	var leaked []string
	for _, term := range ForbiddenOutputTerms {
		if strings.Contains(report, term) {
			leaked = append(leaked, term)
		}
	}
	return IsolationResult{Clean: len(leaked) == 0, LeakedKeys: leaked}
}
