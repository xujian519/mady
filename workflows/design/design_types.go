// Package design provides Pregel-based design patent analysis workflows.
//
// Design patents (外观设计专利) follow fundamentally different legal logic from
// invention/utility model patents:
//   - Invention/utility: individual comparison, all-elements rule, three-step method
//   - Design: overall observation, comprehensive judgment, ordinary consumer perspective
//
// Chinese Patent Law Article 23 governs design patent validity:
//   - A23.1: Not an existing design (不属于现有设计)
//   - A23.2: Conflict application (抵触申请)
//   - A23.3: Substantive defects (明显实质性缺陷)
//
// The invalidation workflow graph:
//
//	parse_design → identify_design_grounds → compare_overall_visual → conclude → __end__
//
// Each node is deterministic (no LLM calls). The design-specific rule engine validates
// that the analysis follows the five reasoning patterns:
//  1. Overall visual comparison four-step method (整体视觉效果对比四步法)
//  2. Design space determination (设计空间认定)
//  3. Common design exclusion (惯常设计排除)
//  4. GUI special rules (图形用户界面特殊规则)
//  5. Combination & transformation (组合与转用)
package design

import (
	"github.com/xujian519/mady/workflows/patent"
)

// DesignInvalidationGroundType identifies the legal basis for design patent invalidation.
type DesignInvalidationGroundType string

const (
	// DesignGroundNovelty 专利法第23条第1款——不属于现有设计。
	// 与发明/实用新型不同，外观设计的新颖性判断采用"整体观察、综合判断"标准。
	DesignGroundNovelty DesignInvalidationGroundType = "A23.1_existing_design"
	// DesignGroundConflict 专利法第23条第2款——抵触申请。
	// 他人在先申请、在后公布的相同或近似外观设计。
	DesignGroundConflict DesignInvalidationGroundType = "A23.2_conflict"
	// DesignGroundSubstantive 专利法第23条第3款——明显实质性缺陷。
	// 与在先合法权利冲突（商标权、著作权、企业名称权等）。
	DesignGroundSubstantive DesignInvalidationGroundType = "A23.3_substantive"
)

// DesignSpaceLevel describes how much design freedom exists for the product category.
type DesignSpaceLevel string

const (
	DesignSpaceLarge  DesignSpaceLevel = "大" // 设计空间大，近似判断尺度较宽松
	DesignSpaceMedium DesignSpaceLevel = "中" // 设计空间中等
	DesignSpaceSmall  DesignSpaceLevel = "小" // 设计空间小，近似判断尺度较严格
)

// DesignPatentInfo describes the target design patent for analysis.
type DesignPatentInfo struct {
	// Name 外观设计产品名称。
	Name string
	// ApplicationNo 申请号。
	ApplicationNo string
	// ProductCategory 产品类别（洛迦诺分类号）。
	ProductCategory string
	// DesignElements 设计特征要素列表。
	DesignElements []DesignElement
	// DesignSpaceLevel 设计空间等级（大/中/小）。
	DesignSpaceLevel DesignSpaceLevel
}

// DesignElement represents a single design feature.
type DesignElement struct {
	// Name 特征名称，如"整体形状"、"图案"、"色彩"。
	Name string
	// Description 特征描述。
	Description string
	// IsCommon 是否为惯常设计（通用/常见设计）。
	IsCommon bool
}

// DesignComparisonResult records the outcome of a visual comparison.
type DesignComparisonResult struct {
	// OverallImpression 整体视觉效果判断描述。
	OverallImpression string
	// IsSimilar 是否构成近似。
	IsSimilar bool
	// SimilarReasoning 推理过程。
	SimilarReasoning string
	// Confidence 置信度：high/medium/low。
	Confidence string
}

// DesignCheckType is the rule check type constant for design comparison.
const DesignCheckType patent.CheckType = "patent_design_comparison"

// String returns the Chinese description of an invalidation ground type.
func (g DesignInvalidationGroundType) String() string {
	switch g {
	case DesignGroundNovelty:
		return "专利法第23条第1款——不属于现有设计"
	case DesignGroundConflict:
		return "专利法第23条第2款——抵触申请"
	case DesignGroundSubstantive:
		return "专利法第23条第3款——与在先合法权利冲突"
	default:
		return string(g)
	}
}
