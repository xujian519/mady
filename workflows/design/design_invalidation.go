package design

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
	"github.com/xujian519/mady/workflows/patent"
)

// State keys used by the design patent invalidation workflow.
const (
	DesignStateInput      = "design_input"        // original design patent description
	DesignStatePatentInfo = "design_patent_info"  // DesignPatentInfo (parsed)
	DesignStateGrounds    = "design_grounds"      // []DesignInvalidationGroundType
	DesignStateComparison = "design_comparison"   // []DesignComparisonResult
	DesignStateRuleCheck  = "design_rule_check"   // rule engine check report
	DesignStateVerdict    = "design_rule_verdict" // aggregate verdict
	DesignStateConclusion = "design_conclusion"   // final conclusion
	DesignStateOutput     = "design_output"       // final output text
)

// =============================================================================
// Design ground identification patterns
// =============================================================================

// designGroundPatterns are the pattern matching rules for identifying
// design invalidation grounds from input text.
var designGroundPatterns = []struct {
	TypeKey  string   // DesignInvalidationGroundType as string
	Article  string   // legal article reference
	Desc     string   // human-readable description
	Patterns []string // keywords to match
}{
	{
		TypeKey:  string(DesignGroundNovelty),
		Article:  "专利法第23条第1款",
		Desc:     "外观设计不属于现有设计",
		Patterns: []string{"23条第1款", "23.1", "现有设计", "不属于现有设计"},
	},
	{
		TypeKey:  string(DesignGroundConflict),
		Article:  "专利法第23条第2款",
		Desc:     "外观设计抵触申请",
		Patterns: []string{"23条第2款", "23.2", "抵触申请", "冲突申请"},
	},
	{
		TypeKey:  string(DesignGroundSubstantive),
		Article:  "专利法第23条第3款",
		Desc:     "外观设计与在先合法权利冲突",
		Patterns: []string{"23条第3款", "23.3", "在先权利", "商标权", "著作权", "合法权利冲突"},
	},
}

// =============================================================================
// Pregel Nodes
// =============================================================================

// parseDesignNode parses the design patent description and extracts the
// patent info, product category, and design elements.
func parseDesignNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(DesignStateInput)
	if input == "" {
		return nil, fmt.Errorf("design: input is empty")
	}

	info := extractDesignInfo(input)
	grounds := identifyDesignGrounds(input)

	return graph.PregelState{
		DesignStateInput:      input,
		DesignStatePatentInfo: info,
		DesignStateGrounds:    grounds,
	}, nil
}

// extractDesignInfo parses the design patent description into a structured info object.
func extractDesignInfo(text string) *DesignPatentInfo {
	info := &DesignPatentInfo{
		DesignElements:   extractDesignElements(text),
		DesignSpaceLevel: inferDesignSpaceLevel(text),
	}

	// Extract product name (first meaningful line).
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 5 {
			info.Name = truncate(line, 100)
			break
		}
	}

	// Default name if nothing found.
	if info.Name == "" {
		info.Name = truncate(text, 100)
	}

	// Extract application number if present.
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "申请号") || strings.Contains(line, "专利号") {
			parts := strings.SplitN(line, "：", 2)
			if len(parts) == 2 {
				info.ApplicationNo = strings.TrimSpace(parts[1])
			} else if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				info.ApplicationNo = strings.TrimSpace(parts[1])
			}
			break
		}
	}

	// Extract Locarno classification.
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "洛迦诺") || strings.Contains(line, "分类号") || strings.Contains(line, "分类") {
			info.ProductCategory = truncate(line, 100)
			break
		}
	}

	return info
}

// extractDesignElements parses the text for design feature descriptions.
func extractDesignElements(text string) []DesignElement {
	var elements []DesignElement
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		// Identify lines that describe design features.
		lower := strings.ToLower(line)
		if strings.Contains(lower, "设计") || strings.Contains(lower, "形状") ||
			strings.Contains(lower, "图案") || strings.Contains(lower, "色彩") ||
			strings.Contains(lower, "外观") {
			isCommon := strings.Contains(lower, "惯常") || strings.Contains(lower, "通用") ||
				strings.Contains(lower, "常见")
			elements = append(elements, DesignElement{
				Name:        inferElementName(line),
				Description: truncate(line, 200),
				IsCommon:    isCommon,
			})
		}
	}
	// If no elements found, create a default one.
	if len(elements) == 0 {
		elements = append(elements, DesignElement{
			Name:        "整体外观",
			Description: truncate(text, 200),
		})
	}
	return elements
}

// inferElementName tries to identify a short name for a design element line.
func inferElementName(line string) string {
	if strings.Contains(line, "形状") {
		if strings.Contains(line, "整体") {
			return "整体形状"
		}
		return "形状"
	}
	if strings.Contains(line, "图案") {
		return "图案"
	}
	if strings.Contains(line, "色彩") {
		return "色彩"
	}
	if strings.Contains(line, "轮廓") {
		return "轮廓"
	}
	if strings.Contains(line, "布局") || strings.Contains(line, "界面") {
		return "布局"
	}
	return "设计特征"
}

// inferDesignSpaceLevel guesses the design space level from the text.
func inferDesignSpaceLevel(text string) DesignSpaceLevel {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "设计空间大") || strings.Contains(lower, "空间大") || strings.Contains(lower, "自由度大") {
		return DesignSpaceLarge
	}
	if strings.Contains(lower, "设计空间小") || strings.Contains(lower, "空间小") || strings.Contains(lower, "自由度小") ||
		strings.Contains(lower, "受限") || strings.Contains(lower, "有限") {
		return DesignSpaceSmall
	}
	// Default: if text mentions functional constraints, infer small space.
	if strings.Contains(lower, "功能决定") || strings.Contains(lower, "标准") || strings.Contains(lower, "尺寸") {
		return DesignSpaceSmall
	}
	return DesignSpaceMedium
}

// identifyDesignGrounds scans the input for design invalidation ground references.
func identifyDesignGrounds(text string) []DesignInvalidationGroundType {
	lower := strings.ToLower(text)
	var grounds []DesignInvalidationGroundType
	seen := make(map[DesignInvalidationGroundType]bool)

	for _, gp := range designGroundPatterns {
		for _, p := range gp.Patterns {
			if strings.Contains(lower, strings.ToLower(p)) {
				gt := DesignInvalidationGroundType(gp.TypeKey)
				if !seen[gt] {
					grounds = append(grounds, gt)
					seen[gt] = true
				}
				break
			}
		}
	}

	// If no specific grounds found, default to all three.
	if len(grounds) == 0 {
		grounds = append(grounds,
			DesignGroundNovelty,
			DesignGroundConflict,
			DesignGroundSubstantive,
		)
	}

	return grounds
}

// identifyDesignGroundsNode refines the design grounds and enriches with context.
func identifyDesignGroundsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[DesignStateGrounds].([]DesignInvalidationGroundType)
	info, _ := state[DesignStatePatentInfo].(*DesignPatentInfo)

	out := graph.PregelState{
		DesignStateInput:      state.GetString(DesignStateInput),
		DesignStatePatentInfo: info,
		DesignStateGrounds:    grounds,
	}

	// Carry forward comparison result if present.
	if cr, ok := state[DesignStateComparison].([]DesignComparisonResult); ok {
		out[DesignStateComparison] = cr
	}

	return out, nil
}

// compareOverallVisualNode implements the "整体观察、综合判断" four-step method
// for design patent comparison. It produces a structured comparison result.
func compareOverallVisualNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	info, _ := state[DesignStatePatentInfo].(*DesignPatentInfo)
	grounds, _ := state[DesignStateGrounds].([]DesignInvalidationGroundType)
	input := state.GetString(DesignStateInput)

	if info == nil {
		return nil, fmt.Errorf("design: patent info not found in state")
	}

	// Build the comparison analysis using the four-step method.
	var comparison strings.Builder
	comparison.WriteString("## 外观设计整体视觉效果对比分析\n\n")
	comparison.WriteString("### 分析框架：整体观察、综合判断（四步法）\n\n")

	// Step 1: 确定设计特征
	comparison.WriteString("**第一步：确定设计特征**\n\n")
	comparison.WriteString("根据涉案专利的图片或照片，确定以下设计特征：\n")
	for i, elem := range info.DesignElements {
		commonMark := ""
		if elem.IsCommon {
			commonMark = "（惯常设计，在对比中应排除）"
		}
		fmt.Fprintf(&comparison, "%d. **%s**：%s%s\n", i+1, elem.Name, elem.Description, commonMark)
	}
	comparison.WriteString("\n")

	// Step 2: 对比整体视觉效果
	comparison.WriteString("**第二步：对比整体视觉效果（一般消费者视角）**\n\n")
	comparison.WriteString("以一般消费者的知识水平和认知能力，对涉案专利与对比设计的整体视觉效果进行对比：\n")
	comparison.WriteString("- 一般消费者对在先设计与涉案设计之间的整体视觉效果进行综合判断\n")
	comparison.WriteString("- 关注产品的整体形状、图案、色彩及其结合\n")
	comparison.WriteString("- 对容易观察到的部位和设计要点给予更大权重\n\n")

	// Step 3: 判断是否构成近似
	comparison.WriteString("**第三步：判断是否构成近似**\n\n")
	comparison.WriteString("基于整体观察和综合判断，从一般消费者角度评估：\n")
	comparison.WriteString(fmt.Sprintf("- 设计空间等级：%s\n", info.DesignSpaceLevel))
	switch info.DesignSpaceLevel {
	case DesignSpaceLarge:
		comparison.WriteString("- 该产品设计空间较大，一般消费者对设计差异的敏感度较低，相近似的判断尺度较为宽松\n")
	case DesignSpaceSmall:
		comparison.WriteString("- 该产品设计空间较小，一般消费者对设计差异更为敏感，相近似的判断尺度较为严格\n")
	default:
		comparison.WriteString("- 设计空间适中，需综合考量各设计特征对整体视觉效果的影响\n")
	}
	comparison.WriteString("- 是否构成近似，取决于整体视觉效果是否足以使一般消费者产生混淆\n\n")

	// Step 4: 结论
	comparison.WriteString("**第四步：结论**\n\n")
	comparison.WriteString("综合以上分析：\n")
	comparison.WriteString("- 如整体视觉效果无实质性差异（足以引起混淆）：构成近似\n")
	comparison.WriteString("- 如整体视觉效果存在明显差异（足以区分）：不构成近似\n")
	comparison.WriteString("- 结论须附带推理过程，并标注置信度\n\n")

	// Check for GUI-specific considerations.
	if strings.Contains(strings.ToLower(input), "gui") ||
		strings.Contains(input, "图形用户界面") ||
		strings.Contains(input, "界面") {
		comparison.WriteString("### GUI 外观设计特殊考量\n\n")
		comparison.WriteString("鉴于该外观设计涉及图形用户界面（GUI），还需考虑以下特殊要素：\n")
		comparison.WriteString("- 界面布局和交互方式的整体视觉效果\n")
		comparison.WriteString("- 动态变化过程对整体视觉印象的影响\n")
		comparison.WriteString("- GUI 画面与产品的关联关系\n")
		comparison.WriteString("- GUI 领域的惯常设计特征\n\n")
	}

	// Run rule engine check.
	engine := NewDesignRuleEngine()
	results := engine.EvaluateAll(comparison.String())
	verdict := Aggregate(results)
	ruleReport := FormatRuleResults(results, verdict)

	return graph.PregelState{
		DesignStateInput:      input,
		DesignStatePatentInfo: info,
		DesignStateGrounds:    grounds,
		DesignStateComparison: comparison.String(),
		DesignStateRuleCheck:  ruleReport,
		DesignStateVerdict:    string(verdict),
	}, nil
}

// designConcludeNode generates the final design invalidation report.
func designConcludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(DesignStateComparison)
	ruleCheck := state.GetString(DesignStateRuleCheck)
	ruleVerdict := state.GetString(DesignStateVerdict)
	grounds, _ := state[DesignStateGrounds].([]DesignInvalidationGroundType)
	info, _ := state[DesignStatePatentInfo].(*DesignPatentInfo)

	var report strings.Builder

	// If rules blocked, prepend warning.
	if ruleVerdict == string(patent.VerdictBlocked) {
		report.WriteString("> ⛔ **规则引擎检查未通过**：外观设计分析存在严重缺陷，结论不宜直接采用。\n\n")
	}

	report.WriteString("# 外观设计专利无效宣告分析报告\n\n")

	// Target patent info.
	report.WriteString("## 目标专利信息\n\n")
	if info != nil {
		report.WriteString(fmt.Sprintf("- **产品名称**：%s\n", info.Name))
		if info.ApplicationNo != "" {
			report.WriteString(fmt.Sprintf("- **申请号**：%s\n", info.ApplicationNo))
		}
		if info.ProductCategory != "" {
			report.WriteString(fmt.Sprintf("- **产品类别**：%s\n", info.ProductCategory))
		}
		report.WriteString(fmt.Sprintf("- **设计空间等级**：%s\n", info.DesignSpaceLevel))
	}
	report.WriteString("\n")

	// Legal grounds.
	report.WriteString("## 无效宣告理由\n\n")
	if len(grounds) > 0 {
		for i, g := range grounds {
			report.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, g.String()))
		}
	} else {
		report.WriteString("未指定具体无效宣告理由，默认按专利法第23条进行全面分析。\n")
	}
	report.WriteString("\n")

	// Comparison analysis.
	if comparison != "" {
		report.WriteString(comparison)
		report.WriteString("\n")
	}

	// Rule check report.
	if ruleCheck != "" {
		report.WriteString(ruleCheck)
		report.WriteString("\n")
	}

	// Conclusion.
	report.WriteString("## 审查结论\n\n")
	if len(grounds) > 0 {
		report.WriteString(fmt.Sprintf("本报告分析了 %d 项外观设计无效宣告理由，基于整体观察、综合判断原则：\n\n", len(grounds)))
		for i, g := range grounds {
			report.WriteString(fmt.Sprintf("%d. **%s**：须结合对比设计进行逐项判断。\n", i+1, g.String()))
		}
	}
	report.WriteString("\n")
	report.WriteString("各无效理由是否成立，取决于整体视觉效果是否足以使一般消费者产生混淆。\n")
	report.WriteString("对于设计空间较大/较小的产品类别，判断尺度应相应调整。\n\n")

	// Disclaimer.
	report.WriteString("---\n\n")
	report.WriteString("> ⚠️ **人工审核提醒**\n")
	report.WriteString(">\n")
	report.WriteString("> 本分析由 AI 辅助生成骨架，以下内容必须由专利代理师/律师逐项核实后定稿：\n")
	report.WriteString("> 1. 每项无效理由的独立论证是否完整\n")
	report.WriteString("> 2. 对比设计的公开日是否已核实\n")
	report.WriteString("> 3. 惯常设计的认定是否准确\n")
	report.WriteString("> 4. 设计空间的判断是否合理\n")
	report.WriteString("> 5. 整体视觉效果差异的认定是否基于一般消费者视角\n")
	report.WriteString(">\n")
	report.WriteString("> 本分析不构成正式法律意见。\n")

	final := report.String()
	return graph.PregelState{
		DesignStateConclusion: final,
		DesignStateOutput:     final,
		DesignStateComparison: comparison,
		DesignStateRuleCheck:  ruleCheck,
		DesignStateVerdict:    ruleVerdict,
		DesignStateInput:      state.GetString(DesignStateInput),
		DesignStatePatentInfo: info,
		DesignStateGrounds:    grounds,
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// DesignGraphOption optionally configures the design invalidation graph.
type DesignGraphOption func(*designGraphConfig)

type designGraphConfig struct {
	retriever domain.DomainRetriever
}

// WithDesignRetriever injects a domain retriever for design evidence search.
// When not injected, evidence gathering returns degraded results.
func WithDesignRetriever(r domain.DomainRetriever) DesignGraphOption {
	return func(c *designGraphConfig) { c.retriever = r }
}

// BuildDesignInvalidationGraph constructs a Pregel graph for design patent
// invalidation analysis.
//
// Graph structure:
//
//	parse_design → identify_design_grounds → compare_overall_visual → conclude → __end__
func BuildDesignInvalidationGraph() (*graph.CompiledPregelGraph, error) {
	return BuildDesignInvalidationGraphWithOpts()
}

// BuildDesignInvalidationGraphWithOpts constructs the design invalidation
// Pregel graph with optional dependency injection.
func BuildDesignInvalidationGraphWithOpts(opts ...DesignGraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &designGraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_design", parseDesignNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("identify_design_grounds", identifyDesignGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("compare_overall_visual", compareOverallVisualNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", designConcludeNode); err != nil {
		return nil, err
	}

	// Build edges.
	edges := [][2]string{
		{"parse_design", "identify_design_grounds"},
		{"identify_design_grounds", "compare_overall_visual"},
		{"compare_overall_visual", "conclude"},
		{"conclude", graph.PregelEnd},
	}
	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_design", 15)
}

// truncate returns at most n runes of s.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
