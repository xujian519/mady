package disclosure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/graph"
)

// StateKeyDraftClaims is the state key for the drafted claims output.
const StateKeyDraftClaims = "draft_claims"

// =============================================================================
// Claim Drafting Pregel Node
// =============================================================================

// draftClaimsNode generates patent claims (权利要求书) from the ExtractionResult.
//
// This node sits after review_gate in the disclosure pipeline, bridging the
// analysis output to a deliverable patent document.
//
// Drafting logic:
//   - The first PFE triple's problem and its associated features form the
//     pre-characterizing portion (前序部分) of independent claim 1.
//   - The remaining features form the characterizing portion (特征部分).
//   - Additional PFE triples produce dependent claims (从属权利要求).
//   - Orphan features (no associated triple) are added as additional dependent claims.
func draftClaimsNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		ext, ok := state[StateKeyExtraction].(*ExtractionResult)
		if !ok {
			return state, fmt.Errorf("disclosure: no extraction result in state, cannot draft claims")
		}

		if ext == nil || len(ext.Features) == 0 {
			return state, fmt.Errorf("disclosure: extraction result has no features, cannot draft claims")
		}

		if len(ext.PFETriples) == 0 {
			return state, fmt.Errorf("disclosure: no PFE triples; cannot generate independent claim 1, refusing to draft claims that would reference a non-existent claim")
		}

		claims := buildClaimSet(ext)
		state[StateKeyDraftClaims] = claims
		// Always attach claims to output — prepend separator if prior content exists.
		if existing, _ := state[StateKeyOutput].(string); existing != "" {
			state[StateKeyOutput] = existing + "\n\n---\n\n" + claims
		} else {
			state[StateKeyOutput] = claims
		}

		return state, nil
	}
}

// =============================================================================
// Claim Builder
// =============================================================================

// claimSet represents a complete set of patent claims.
type claimSet struct {
	IndependentClaims []claim
	DependentClaims   []claim
}

type claim struct {
	Number        int
	Type          string // "independent" or "dependent"
	Preamble      string // 前序部分
	Characterized string // 特征部分（"其特征在于"之后）
	DependsOn     int    // 0 for independent claims
	Feature       string // additional feature for dependent claims
}

// String renders the claim set in standard Chinese patent format.
func (cs *claimSet) String() string {
	var b strings.Builder
	b.WriteString("# 权利要求书\n\n")

	for _, c := range cs.IndependentClaims {
		b.WriteString(c.String())
		b.WriteString("\n")
	}

	for _, c := range cs.DependentClaims {
		b.WriteString(c.String())
		b.WriteString("\n")
	}

	b.WriteString("\n> ⚠️ 本权利要求书由 AI 辅助生成，应由专利代理人审核确认后定稿。\n")
	b.WriteString("> 特别需要核实：\n")
	b.WriteString("> 1. 独立权利要求的必要技术特征是否完整\n")
	b.WriteString("> 2. 前序部分与特征部分的划界是否正确\n")
	b.WriteString("> 3. 从属权利要求的引用关系是否正确\n")
	b.WriteString("> 4. 所有特征是否得到说明书的支持\n")

	return b.String()
}

func (c claim) String() string {
	var b strings.Builder
	dependPrefix := ""
	if c.DependsOn > 0 {
		dependPrefix = fmt.Sprintf("根据权利要求%d所述的", c.DependsOn)
	}

	fmt.Fprintf(&b, "%d. ", c.Number)

	if c.Type == "independent" {
		b.WriteString(c.Preamble)
		b.WriteString("，其特征在于，")
		b.WriteString(c.Characterized)
	} else {
		b.WriteString(dependPrefix)
		b.WriteString(c.Feature)
	}
	b.WriteString("。\n")
	return b.String()
}

// buildClaimSet constructs a full claim set from the extraction result.
func buildClaimSet(ext *ExtractionResult) string {
	cs := &claimSet{}

	// Build feature map for fast lookup.
	featureMap := make(map[string]TechFeature)
	for _, f := range ext.Features {
		featureMap[f.ID] = f
	}

	// Sort features by importance.
	featuresByImportance := sortFeaturesByImportance(ext.Features)

	// Build independent claim 1 from the primary PFE triple.
	if len(ext.PFETriples) > 0 {
		primary := ext.PFETriples[0]
		// Pre-characterizing portion: problem context + first feature
		preamble := buildPreamble(ext.Problems, primary)
		// Characterizing portion: the distinguishing features
		characterized := buildCharacterizedFeatures(primary, featureMap, featuresByImportance)

		cs.IndependentClaims = append(cs.IndependentClaims, claim{
			Number:        1,
			Type:          "independent",
			Preamble:      preamble,
			Characterized: characterized,
		})
	}

	// Build dependent claims from remaining PFE triples and orphan features.
	claimNum := 2

	// Remaining PFE triples become dependent claims.
	for i := 1; i < len(ext.PFETriples); i++ {
		triple := ext.PFETriples[i]
		featureDesc := buildFeatureDescription(triple, featureMap)
		if featureDesc != "" {
			cs.DependentClaims = append(cs.DependentClaims, claim{
				Number:    claimNum,
				Type:      "dependent",
				DependsOn: 1,
				Feature:   featureDesc,
			})
			claimNum++
		}
	}

	// Orphan features (not part of any triple) become additional dependent claims.
	orphanFeatures := findOrphanFeatures(ext, featureMap)
	for _, f := range orphanFeatures {
		cs.DependentClaims = append(cs.DependentClaims, claim{
			Number:    claimNum,
			Type:      "dependent",
			DependsOn: 1,
			Feature:   formatFeatureClaim(f),
		})
		claimNum++
	}

	return cs.String()
}

// buildPreamble constructs the pre-characterizing portion from the problem description.
func buildPreamble(problems []string, primary PFETriple) string {
	if len(problems) > 0 {
		// Use the first problem as context.
		problem := problems[0]
		// Remove common redundant prefixes.
		problem = strings.TrimPrefix(problem, "技术问题：")
		problem = strings.TrimPrefix(problem, "现有技术中")
		problem = strings.TrimSuffix(problem, "的问题")
		problem = strings.TrimSuffix(problem, "的缺陷")
		problem = strings.TrimSuffix(problem, "的不足")

		if len(problem) > 3 {
			return fmt.Sprintf("一种%s的装置", problem)
		}
	}
	return "一种技术方案"
}

// buildCharacterizedFeatures builds the characterizing portion from PFE features.
func buildCharacterizedFeatures(primary PFETriple, featureMap map[string]TechFeature,
	featuresByImportance []TechFeature) string {

	var parts []string
	seen := make(map[string]bool)

	// First, add features directly linked to the primary PFE triple.
	for _, fid := range primary.FeatureIDs {
		if f, ok := featureMap[fid]; ok {
			desc := formatFeatureClaim(f)
			if !seen[desc] {
				seen[desc] = true
				parts = append(parts, desc)
			}
		}
	}

	// Add remaining high-importance features not yet covered.
	for _, f := range featuresByImportance {
		if f.Importance != "high" {
			break // sorted: no more high-importance features
		}
		desc := formatFeatureClaim(f)
		if !seen[desc] {
			seen[desc] = true
			parts = append(parts, desc)
		}
	}

	if len(parts) == 0 {
		return "[待确定：核心区别技术特征]"
	}

	return strings.Join(parts, "；")
}

// buildFeatureDescription creates a feature description from a PFE triple.
func buildFeatureDescription(triple PFETriple, featureMap map[string]TechFeature) string {
	for _, fid := range triple.FeatureIDs {
		if f, ok := featureMap[fid]; ok {
			return formatFeatureClaim(f)
		}
	}
	return ""
}

// findOrphanFeatures finds features not linked to any PFE triple.
func findOrphanFeatures(ext *ExtractionResult, featureMap map[string]TechFeature) []TechFeature {
	linked := make(map[string]bool)
	for _, triple := range ext.PFETriples {
		for _, fid := range triple.FeatureIDs {
			linked[fid] = true
		}
	}

	var orphans []TechFeature
	for _, f := range ext.Features {
		if !linked[f.ID] && f.Importance != "low" {
			orphans = append(orphans, f)
		}
	}
	order := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(orphans, func(i, j int) bool {
		return order[orphans[i].Importance] < order[orphans[j].Importance]
	})
	return orphans
}

// formatFeatureClaim formats a TechFeature as a claim element.
func formatFeatureClaim(f TechFeature) string {
	desc := strings.TrimSpace(f.Description)
	if desc == "" {
		return "[特征]"
	}
	// Capitalize first character (for Chinese, this is a no-op convention).
	if f.Category == CatStructure {
		return fmt.Sprintf("所述装置包括%s", desc)
	}
	if f.Category == CatMethod {
		return fmt.Sprintf("所述方法包括%s步骤", desc)
	}
	if f.Function != "" {
		return fmt.Sprintf("%s，用于%s", desc, f.Function)
	}
	return desc
}

// sortFeaturesByImportance sorts features: high → medium → low.
func sortFeaturesByImportance(features []TechFeature) []TechFeature {
	order := map[string]int{"high": 0, "medium": 1, "low": 2}
	sorted := make([]TechFeature, len(features))
	copy(sorted, features)
	sort.Slice(sorted, func(i, j int) bool {
		return order[sorted[i].Importance] < order[sorted[j].Importance]
	})
	return sorted
}
