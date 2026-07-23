// Package patent provides patent-related analysis workflows.
//
// Deprecated: The infringement analysis workflow (this file) has been superseded
// by domains/infringement/. Use domains/infringement.NewInfringementTool() for
// comprehensive infringement analysis with dual-perspective support (plaintiff/defendant)
// and four-layer coverage (determination, defenses, remedies, strategy).
//
// The remaining workflows (novelty, debate, OA response, etc.) are still active.
//
// The infringement workflow compares a patent's claims against an accused
// product/method under the "all-elements" (全面覆盖) rule and the doctrine
// of equivalents (等同原则). It also considers prosecution history estoppel
// (禁止反悔) and the dedication rule (捐献规则).
//
// Graph structure:
//
//	parse_claims → parse_product → full_coverage → equivalence → rule_check → conclude → __end__
//
// Each node is deterministic (no LLM calls). The graph produces a structured
// infringement analysis skeleton that a patent attorney reviews and finalizes.
// The deterministic rule engine validates the analysis for legal completeness.
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys used by the infringement workflow.
const (
	InfStatePatentClaims    = "inf_patent_claims"    // original patent claims text
	InfStateAccusedProduct  = "inf_accused_product"  // accused product description
	InfStateClaimFeatures   = "inf_claim_features"   // []string: decomposed claim features
	InfStateProductFeatures = "inf_product_features" // []string: accused product features
	InfStateLiteralMatch    = "inf_literal_match"    // literal infringement analysis
	InfStateEquivalence     = "inf_equivalence"      // equivalence analysis
	InfStateRuleCheck       = "inf_rule_check"       // rule engine check report
	InfStateRuleVerdict     = "inf_rule_verdict"     // aggregate verdict
	InfStateConclusion      = "inf_conclusion"       // final conclusion
	InfStateOutput          = "inf_output"           // final output text
)

// =============================================================================
// Pregel Nodes
// =============================================================================

// infParseClaimsNode parses the patent claims text and decomposes them into
// individual technical features (技术特征). This is the foundation of the
// all-elements analysis.
func infParseClaimsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	claims := state.GetString(InfStatePatentClaims)
	if claims == "" {
		return nil, fmt.Errorf("infringement: patent claims input is empty")
	}

	features := extractFeatures(claims)
	if len(features) == 0 {
		// Fallback: split by common separators.
		features = splitBySeparators(claims)
	}

	return graph.PregelState{
		InfStatePatentClaims:  claims,
		InfStateClaimFeatures: features,
	}, nil
}

// infParseProductNode parses the accused product description and extracts its
// technical features for comparison.
func infParseProductNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	product := state.GetString(InfStateAccusedProduct)
	if product == "" {
		return nil, fmt.Errorf("infringement: accused product input is empty")
	}

	features := extractFeatures(product)
	if len(features) == 0 {
		features = splitBySeparators(product)
	}

	out := graph.PregelState{
		InfStateAccusedProduct:  product,
		InfStateProductFeatures: features,
	}
	// Carry forward claim features.
	if cf, ok := state[InfStateClaimFeatures].([]string); ok {
		out[InfStateClaimFeatures] = cf
	}
	out[InfStatePatentClaims] = state.GetString(InfStatePatentClaims)
	return out, nil
}

// fullCoverageNode performs the all-elements / full-coverage analysis.
// Under the 全面覆盖原则, infringement exists only if the accused product
// contains EVERY technical feature of at least one claim.
func fullCoverageNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	claimFeatures, _ := state[InfStateClaimFeatures].([]string)
	productFeatures, _ := state[InfStateProductFeatures].([]string)

	var b strings.Builder
	b.WriteString("## 全面覆盖分析（字面侵权）\n\n")
	b.WriteString("**原则**：被控侵权方案须包含权利要求记载的**全部**技术特征，方可认定字面侵权。\n\n")

	b.WriteString("### 权利要求技术特征分解\n\n")
	for i, f := range claimFeatures {
		fmt.Fprintf(&b, "- 特征%c：%s\n", 'A'+i, f)
	}
	b.WriteString("\n")

	b.WriteString("### 被控方案技术特征\n\n")
	for _, f := range productFeatures {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\n")

	// Feature-by-feature matching (naive substring match for skeleton).
	b.WriteString("### 逐特征比对\n\n")
	allMatched := true
	for i, cf := range claimFeatures {
		matched := false
		matchDetail := ""
		for _, pf := range productFeatures {
			if isFeatureMatch(cf, pf) {
				matched = true
				matchDetail = pf
				break
			}
		}
		if matched {
			fmt.Fprintf(&b, "- 特征%c ✅ **匹配** — 权利要求「%s」 ↔ 被控方案「%s」\n",
				'A'+i, truncate(cf, 30), truncate(matchDetail, 30))
		} else {
			allMatched = false
			fmt.Fprintf(&b, "- 特征%c ❌ **未匹配** — 权利要求「%s」在被控方案中未找到字面对应\n",
				'A'+i, truncate(cf, 30))
		}
	}
	b.WriteString("\n")

	if allMatched {
		b.WriteString("**初步结论**：全部技术特征均匹配，**构成字面侵权**。\n\n")
	} else {
		b.WriteString("**初步结论**：部分技术特征未字面匹配，需进一步进行**等同侵权分析**。\n\n")
	}

	return graph.PregelState{
		InfStateLiteralMatch: b.String(),
	}, nil
}

// equivalenceNode performs the doctrine-of-equivalents analysis for features
// that did not match literally. Under 等同原则, a feature may still infringe
// if it is equivalent in means, function, and result (手段/功能/效果基本相同).
func equivalenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	claimFeatures, _ := state[InfStateClaimFeatures].([]string)
	productFeatures, _ := state[InfStateProductFeatures].([]string)

	var b strings.Builder
	b.WriteString("## 等同侵权分析\n\n")
	b.WriteString("**等同三要素**：手段基本相同 + 功能基本相同 + 效果基本相同，且无需创造性劳动。\n\n")

	// Identify unmatched features for equivalence analysis.
	b.WriteString("### 未字面匹配特征的等同判定\n\n")
	hasUnmatched := false
	for i, cf := range claimFeatures {
		matched := false
		for _, pf := range productFeatures {
			if isFeatureMatch(cf, pf) {
				matched = true
				break
			}
		}
		if !matched {
			hasUnmatched = true
			fmt.Fprintf(&b, "**特征%c：%s**\n", 'A'+i, truncate(cf, 40))
			b.WriteString("- 手段：需对比被控方案采用的替代手段是否基本相同\n")
			b.WriteString("- 功能：需对比该特征在被控方案中实现的功能是否基本相同\n")
			b.WriteString("- 效果：需对比该特征达到的技术效果是否基本相同\n")
			b.WriteString("- 创造性劳动：本领域技术人员无需创造性劳动即可联想到\n\n")
		}
	}

	if !hasUnmatched {
		b.WriteString("所有特征均已字面匹配，无需等同分析。\n\n")
	}

	// Limitations.
	b.WriteString("### 等同原则的限制\n\n")
	b.WriteString("- **禁止反悔原则**：专利权人在审查过程中放弃的内容不得在侵权诉讼中重新主张\n")
	b.WriteString("- **捐献规则**：说明书披露但权利要求未写入的技术方案视为捐献给社会\n\n")

	return graph.PregelState{
		InfStateEquivalence: b.String(),
	}, nil
}

// infRuleCheckNode runs the infringement rule engine on the analysis text.
func infRuleCheckNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	analysisText := state.GetString(InfStateLiteralMatch) + "\n" + state.GetString(InfStateEquivalence)

	engine := NewRuleEngine()
	engine.RegisterRules(InfringementRules())
	results := engine.Evaluate(engine.Rules(), analysisText, "patent_infringement")
	verdict := Aggregate(results)
	report := FormatRuleResults(results, verdict)

	return graph.PregelState{
		InfStateRuleCheck:   report,
		InfStateRuleVerdict: string(verdict),
	}, nil
}

// infConcludeNode generates the final infringement analysis report.
func infConcludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	literalMatch := state.GetString(InfStateLiteralMatch)
	equivalence := state.GetString(InfStateEquivalence)
	ruleCheck := state.GetString(InfStateRuleCheck)
	ruleVerdict := state.GetString(InfStateRuleVerdict)

	var report strings.Builder

	if ruleVerdict == string(VerdictBlocked) {
		report.WriteString("> ⛔ **规则引擎检查未通过**：侵权分析存在严重缺陷，结论不宜直接采用。\n\n")
	}

	report.WriteString("# 专利侵权分析报告\n\n")
	report.WriteString("## 分析框架\n\n")
	report.WriteString("本报告按以下顺序分析：全面覆盖（字面侵权）→ 等同侵权 → 限制原则。\n\n")

	report.WriteString(literalMatch)
	report.WriteString("\n")
	report.WriteString(equivalence)
	report.WriteString("\n")
	report.WriteString(ruleCheck)

	// Conclusion.
	report.WriteString("\n## 分析结论\n\n")
	report.WriteString("基于上述分析：\n")
	report.WriteString("- 全面覆盖原则要求被控方案包含权利要求的**全部**技术特征\n")
	report.WriteString("- 等同侵权须满足**手段/功能/效果**三要素且无需创造性劳动\n")
	report.WriteString("- **禁止反悔**和**捐献规则**可能限缩等同范围\n\n")

	// Disclaimer.
	report.WriteString("---\n\n")
	report.WriteString("> ⚠️ **人工审核提醒**\n")
	report.WriteString("> \n")
	report.WriteString("> 本分析由 AI 辅助生成骨架，以下内容必须由专利代理师/律师逐项核实：\n")
	report.WriteString("> 1. 技术特征分解是否准确、完整\n")
	report.WriteString("> 2. 各特征的比对是否基于实际产品检测或鉴定报告\n")
	report.WriteString("> 3. 等同判定的三要素论证是否充分\n")
	report.WriteString("> 4. 审查历史档案是否已调取并核实禁止反悔/捐献规则适用性\n")
	report.WriteString("> \n")
	report.WriteString("> 本分析不构成正式法律意见。\n")

	final := report.String()
	return graph.PregelState{
		InfStateConclusion: final,
		InfStateOutput:     final,
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// BuildInfringementGraph constructs a Pregel graph for patent infringement analysis.
//
// Graph structure:
//
//	parse_claims → parse_product → full_coverage → equivalence → rule_check → conclude → __end__
func BuildInfringementGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_claims", infParseClaimsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("parse_product", infParseProductNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("full_coverage", fullCoverageNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("equivalence", equivalenceNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("rule_check", infRuleCheckNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", infConcludeNode); err != nil {
		return nil, err
	}

	edges := [][2]string{
		{"parse_claims", "parse_product"},
		{"parse_product", "full_coverage"},
		{"full_coverage", "equivalence"},
		{"equivalence", "rule_check"},
		{"rule_check", "conclude"},
		{"conclude", graph.PregelEnd},
	}
	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_claims", 15)
}

// =============================================================================
// Helpers
// =============================================================================

// isFeatureMatch checks whether claimFeature and productFeature match.
// This is a naive substring/overlap check for skeleton analysis.
// Real matching requires LLM or domain expertise.
//
// The overlap threshold is 5 runes (not 3) to avoid false positives from
// common Chinese phrases like "包括以下" or "其特征" that appear in nearly
// all patent claim language.
func isFeatureMatch(claimFeature, productFeature string) bool {
	cf := strings.TrimSpace(claimFeature)
	pf := strings.TrimSpace(productFeature)
	if cf == "" || pf == "" {
		return false
	}
	// Direct containment either direction.
	if strings.Contains(cf, pf) || strings.Contains(pf, cf) {
		return true
	}
	// Check for significant keyword overlap (at least 5 runes).
	cfRunes := []rune(cf)
	for i := 0; i <= len(cfRunes)-5; i++ {
		sub := string(cfRunes[i : i+5])
		if strings.Contains(pf, sub) {
			return true
		}
	}
	return false
}

// splitBySeparators splits text by common Chinese separators as a fallback
// for feature extraction.
func splitBySeparators(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// Replace common separators with newline, then split.
	replacers := []string{"，", "。", "；", "、", "以及", "和", "并"}
	result := text
	for _, sep := range replacers {
		result = strings.ReplaceAll(result, sep, "\n")
	}
	parts := strings.Split(result, "\n")
	var features []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len([]rune(p)) >= 4 { // only keep meaningful fragments
			features = append(features, p)
		}
	}
	return features
}
