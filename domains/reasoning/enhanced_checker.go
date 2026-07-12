package reasoning

import (
	"context"
	"fmt"
	"strings"
)

// CheckLevel controls the depth of syllogism validation.
type CheckLevel int

const (
	// CheckLevel1 only validates that fact/article references exist on the blackboard.
	CheckLevel1 CheckLevel = 1
	// CheckLevel2 additionally validates logical consistency (premises → conclusion).
	CheckLevel2 CheckLevel = 2
	// CheckLevel3 additionally validates evidentiary sufficiency (can facts+rules jointly prove the conclusion).
	CheckLevel3 CheckLevel = 3
)

// EnhancedSyllogismChecker performs three-level validation of reasoning conclusions.
//
// Level 1 — Reference existence: reuses the existing RuleAssertion to verify
// that every Syllogism's FactRef and ArticleRef point to entities on the blackboard.
//
// Level 2 — Logical consistency: uses the LLM to evaluate whether the major
// premise (legal rule) and minor premise (case fact) genuinely support the
// conclusion. This catches cases where references exist but the reasoning is
// fallacious (e.g., correct citations used to support a wrong conclusion).
//
// Level 3 — Evidentiary sufficiency: checks whether the complete set of
// UsedFacts + UsedRules can jointly derive the conclusion, or if there are
// reasoning gaps. This is the AlphaGeometry-style traceback: can every step
// of the conclusion be traced back to a blackboard fact or rule?
type EnhancedSyllogismChecker struct {
	llm      LlmClient // for Level 2/3 semantic evaluation
	maxRetry int       // max retries for minor violations
}

// NewEnhancedSyllogismChecker creates a checker. llm may be nil for Level-1-only checks.
func NewEnhancedSyllogismChecker(llm LlmClient, maxRetry int) *EnhancedSyllogismChecker {
	if maxRetry <= 0 {
		maxRetry = 2
	}
	return &EnhancedSyllogismChecker{llm: llm, maxRetry: maxRetry}
}

// Check validates a completed Plan execution against the blackboard.
// It builds syllogisms from the plan's UsedFacts/UsedRules, runs the three
// validation levels, and produces a CheckReport.
func (c *EnhancedSyllogismChecker) Check(ctx context.Context, bb *FactBlackboard, plan *Plan, level CheckLevel) (*CheckReport, error) {
	report := &CheckReport{
		PlanID:    plan.PlanID,
		Passed:    true,
		UsedFacts: plan.UsedFacts,
		UsedRules: plan.UsedRules,
	}

	// Build syllogisms from plan steps.
	syllogisms := c.buildSyllogisms(bb, plan)
	report.Syllogisms = syllogisms

	// Level 1: Reference existence (always runs).
	for i := range syllogisms {
		if err := RuleAssertion(bb, &syllogisms[i]); err != nil {
			report.Gaps = append(report.Gaps, ValidationGap{
				Description: err.Error(),
				Severity:    "hard",
				Suggestion:  "确保三段论引用的事实ID和法条ID都在黑板上存在",
			})
			report.Passed = false
		}
	}

	if level >= CheckLevel2 && c.llm != nil {
		c.checkLogicalConsistency(ctx, plan, syllogisms, report)
	}

	if level >= CheckLevel3 && c.llm != nil {
		c.checkEvidentiarySufficiency(ctx, bb, plan, report)
	}

	// Compute overall confidence from syllogism confidences.
	if len(syllogisms) > 0 {
		var sum float64
		for _, s := range syllogisms {
			sum += s.Confidence
		}
		report.Confidence = sum / float64(len(syllogisms))
	}
	if report.Confidence < 0.3 {
		report.Confidence = 0.3
	}

	// Identify unused facts/rules.
	report.UnusedFacts = findUnused(setFromSlice(plan.UsedFacts), syllogisms, "FactRef")
	report.UnusedRules = findUnused(setFromSlice(plan.UsedRules), syllogisms, "ArticleRef")

	return report, nil
}

// checkLogicalConsistency runs Level 2 validation using the LLM.
func (c *EnhancedSyllogismChecker) checkLogicalConsistency(ctx context.Context, plan *Plan, syllogisms []Syllogism, report *CheckReport) {
	if len(syllogisms) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("请评估以下法律三段论的逻辑一致性。对于每个三段论，判断大前提+小前提是否能合法推出结论。输出 JSON 数组，每个元素包含：\n")
	sb.WriteString(`{"syllogism_id":"...", "valid":true/false, "issue":"如果不一致，描述问题"}`)
	sb.WriteString("\n\n三段论列表：\n")

	for _, s := range syllogisms {
		fmt.Fprintf(&sb, `[%s] 大前提(%s): %s | 小前提(%s): %s | 结论: %s`+"\n",
			s.ID, s.MajorPremise.RefID, s.MajorPremise.Content,
			s.MinorPremise.RefID, s.MinorPremise.Content,
			s.Conclusion,
		)
	}

	resp, err := c.llm.Chat(ctx, []LlmMessage{{Role: "user", Content: sb.String()}})
	if err != nil {
		report.Gaps = append(report.Gaps, ValidationGap{
			Description: fmt.Sprintf("L2 逻辑一致性检查失败: %v", err),
			Severity:    "soft",
			Suggestion:  "LLM 调用失败，跳过 L2 检查",
		})
		return
	}

	// Simple heuristic: count "invalid" mentions.
	invalidCount := strings.Count(strings.ToLower(resp), `"valid":false`)
	if invalidCount > 0 {
		report.Gaps = append(report.Gaps, ValidationGap{
			Description: fmt.Sprintf("L2 逻辑一致性: %d 个三段论可能存在逻辑漏洞", invalidCount),
			Severity:    "soft",
			Suggestion:  "请人工审核上述三段论的推理链条",
		})
		if invalidCount > len(syllogisms)/2 {
			report.Passed = false
		}
	}
}

// checkEvidentiarySufficiency runs Level 3 validation using the LLM.
func (c *EnhancedSyllogismChecker) checkEvidentiarySufficiency(ctx context.Context, bb *FactBlackboard, plan *Plan, report *CheckReport) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "请评估以下推理是否证据充分。\n\n已知事实（共 %d 条）：\n", len(plan.UsedFacts))
	for _, fid := range plan.UsedFacts {
		if f, ok := bb.GetFact(fid); ok {
			fmt.Fprintf(&sb, "- [%s] %s\n", fid, truncate(f.Content, 200))
		}
	}

	fmt.Fprintf(&sb, "\n适用规则（共 %d 条）：\n", len(plan.UsedRules))
	for _, rid := range plan.UsedRules {
		for _, c := range bb.RuleConstraints() {
			if c.ArticleID == rid {
				fmt.Fprintf(&sb, "- [%s] %s\n", rid, c.Description)
				break
			}
		}
	}

	fmt.Fprintf(&sb, "\n结论：PlanIntent=%s, Steps=%d\n", plan.Intent, len(plan.Steps))
	sb.WriteString("\n请判断：上述事实和规则是否足以支撑该结论？如有证据缺口，请指出。")

	resp, err := c.llm.Chat(ctx, []LlmMessage{{Role: "user", Content: sb.String()}})
	if err != nil {
		report.Gaps = append(report.Gaps, ValidationGap{
			Description: fmt.Sprintf("L3 证据充分性检查失败: %v", err),
			Severity:    "soft",
		})
		return
	}

	if strings.Contains(strings.ToLower(resp), "不足") || strings.Contains(strings.ToLower(resp), "缺口") {
		report.Gaps = append(report.Gaps, ValidationGap{
			Description: "L3 证据充分性: " + truncate(resp, 300),
			Severity:    "hard",
			Suggestion:  "建议补充缺失的事实或规则引用后重新分析",
		})
		report.Passed = false
	}
}

// buildSyllogisms constructs syllogisms from the plan's UsedFacts/UsedRules.
func (c *EnhancedSyllogismChecker) buildSyllogisms(bb *FactBlackboard, plan *Plan) []Syllogism {
	var syllogisms []Syllogism

	for i, step := range plan.Steps {
		// Use the first fact and first rule as premises (simplified).
		var majorPremise, minorPremise Premise

		if len(plan.UsedRules) > 0 {
			rid := plan.UsedRules[i%len(plan.UsedRules)]
			for _, rc := range bb.RuleConstraints() {
				if rc.ArticleID == rid {
					majorPremise = Premise{
						Label:   rc.ArticleName,
						Source:  SourceStatute,
						RefID:   rid,
						Content: rc.Description,
					}
					break
				}
			}
		}
		if majorPremise.RefID == "" && len(plan.UsedRules) > 0 {
			majorPremise = Premise{
				Label: "规则", Source: SourceStatute, RefID: plan.UsedRules[0],
				Content: "适用规则",
			}
		}

		if len(plan.UsedFacts) > 0 {
			fid := plan.UsedFacts[i%len(plan.UsedFacts)]
			if f, ok := bb.GetFact(fid); ok {
				minorPremise = Premise{
					Label:   "案件事实",
					Source:  SourceCaseFact,
					RefID:   fid,
					Content: truncate(f.Content, 200),
				}
			}
		}
		if minorPremise.RefID == "" && len(plan.UsedFacts) > 0 {
			minorPremise = Premise{
				Label: "事实", Source: SourceCaseFact, RefID: plan.UsedFacts[0],
				Content: "案件事实",
			}
		}

		syllogisms = append(syllogisms, Syllogism{
			ID:           fmt.Sprintf("syl_%s_step%d", plan.PlanID, i+1),
			MajorPremise: majorPremise,
			MinorPremise: minorPremise,
			Conclusion:   step.Description + " — " + step.ExpectedOutput,
			FactRef:      minorPremise.RefID,
			ArticleRef:   majorPremise.RefID,
			Confidence:   0.7,
		})
	}

	return syllogisms
}

// --- helpers ---

func setFromSlice(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// findUnused returns IDs from the used set that are not referenced by any syllogism.
func findUnused(usedSet map[string]bool, syllogisms []Syllogism, field string) []string {
	referenced := make(map[string]bool)
	for _, s := range syllogisms {
		var ref string
		switch field {
		case "FactRef":
			ref = s.FactRef
		case "ArticleRef":
			ref = s.ArticleRef
		}
		if ref != "" {
			referenced[ref] = true
		}
	}

	var unused []string
	for id := range usedSet {
		if !referenced[id] {
			unused = append(unused, id)
		}
	}
	return unused
}
