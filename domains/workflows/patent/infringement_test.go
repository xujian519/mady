package patent

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// -----------------------------------------------------------------------------
// infParseClaimsNode
// -----------------------------------------------------------------------------

func TestInfParseClaimsNode_EmptyInput(t *testing.T) {
	_, err := infParseClaimsNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Fatal("expected error for empty claims input")
	}
}

func TestInfParseClaimsNode_BasicInput(t *testing.T) {
	claims := "一种图像处理方法，包括采集图像数据、使用卷积神经网络进行特征提取和对图像进行分类识别。"
	out, err := infParseClaimsNode(context.Background(), graph.PregelState{
		InfStatePatentClaims: claims,
	})
	if err != nil {
		t.Fatalf("infParseClaimsNode: %v", err)
	}
	features, ok := out[InfStateClaimFeatures].([]string)
	if !ok || len(features) == 0 {
		t.Fatalf("expected claim features, got %v", out[InfStateClaimFeatures])
	}
}

// -----------------------------------------------------------------------------
// infParseProductNode
// -----------------------------------------------------------------------------

func TestInfParseProductNode_EmptyInput(t *testing.T) {
	_, err := infParseProductNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Fatal("expected error for empty product input")
	}
}

func TestInfParseProductNode_BasicInput(t *testing.T) {
	product := "被控产品包含图像采集模块、CNN特征提取模块和图像分类模块。"
	out, err := infParseProductNode(context.Background(), graph.PregelState{
		InfStateAccusedProduct: product,
		InfStateClaimFeatures:  []string{"采集图像", "特征提取"},
	})
	if err != nil {
		t.Fatalf("infParseProductNode: %v", err)
	}
	features, ok := out[InfStateProductFeatures].([]string)
	if !ok || len(features) == 0 {
		t.Fatalf("expected product features, got %v", out[InfStateProductFeatures])
	}
	// Claim features should be carried forward.
	if cf, ok := out[InfStateClaimFeatures].([]string); !ok || len(cf) != 2 {
		t.Error("claim features should be carried forward")
	}
}

// -----------------------------------------------------------------------------
// fullCoverageNode
// -----------------------------------------------------------------------------

func TestFullCoverageNode_AllMatched(t *testing.T) {
	// Same features in claims and product → all matched.
	features := []string{"采集图像数据", "卷积神经网络特征提取", "图像分类识别"}
	out, err := fullCoverageNode(context.Background(), graph.PregelState{
		InfStateClaimFeatures:   features,
		InfStateProductFeatures: features,
	})
	if err != nil {
		t.Fatalf("fullCoverageNode: %v", err)
	}
	result := out.GetString(InfStateLiteralMatch)
	if !strings.Contains(result, "构成字面侵权") {
		t.Error("should indicate literal infringement when all features match")
	}
}

func TestFullCoverageNode_PartialMatch(t *testing.T) {
	claimFeatures := []string{"采集图像数据", "卷积神经网络特征提取", "图像分类识别", "后处理增强"}
	productFeatures := []string{"采集图像数据", "卷积神经网络特征提取"}
	out, err := fullCoverageNode(context.Background(), graph.PregelState{
		InfStateClaimFeatures:   claimFeatures,
		InfStateProductFeatures: productFeatures,
	})
	if err != nil {
		t.Fatalf("fullCoverageNode: %v", err)
	}
	result := out.GetString(InfStateLiteralMatch)
	if !strings.Contains(result, "未匹配") {
		t.Error("should indicate unmatched features")
	}
	if !strings.Contains(result, "等同侵权分析") {
		t.Error("should suggest equivalence analysis for unmatched features")
	}
}

// -----------------------------------------------------------------------------
// equivalenceNode
// -----------------------------------------------------------------------------

func TestEquivalenceNode_HasUnmatched(t *testing.T) {
	claimFeatures := []string{"采集图像", "特殊滤波处理"}
	productFeatures := []string{"采集图像"} // 滤波 not matched
	out, err := equivalenceNode(context.Background(), graph.PregelState{
		InfStateClaimFeatures:   claimFeatures,
		InfStateProductFeatures: productFeatures,
	})
	if err != nil {
		t.Fatalf("equivalenceNode: %v", err)
	}
	result := out.GetString(InfStateEquivalence)
	if !strings.Contains(result, "等同三要素") {
		t.Error("should mention equivalence three-element test")
	}
	if !strings.Contains(result, "禁止反悔") {
		t.Error("should mention estoppel limitation")
	}
	if !strings.Contains(result, "捐献规则") {
		t.Error("should mention dedication rule")
	}
}

func TestEquivalenceNode_AllMatched(t *testing.T) {
	features := []string{"采集图像", "分类处理"}
	out, err := equivalenceNode(context.Background(), graph.PregelState{
		InfStateClaimFeatures:   features,
		InfStateProductFeatures: features,
	})
	if err != nil {
		t.Fatalf("equivalenceNode: %v", err)
	}
	result := out.GetString(InfStateEquivalence)
	if !strings.Contains(result, "无需等同分析") {
		t.Error("should indicate no equivalence needed when all matched")
	}
}

// -----------------------------------------------------------------------------
// infRuleCheckNode
// -----------------------------------------------------------------------------

func TestInfRuleCheckNode_Basic(t *testing.T) {
	out, err := infRuleCheckNode(context.Background(), graph.PregelState{
		InfStateLiteralMatch: "全面覆盖分析：包含全部技术特征",
		InfStateEquivalence:  "等同分析：手段/功能/效果基本相同",
	})
	if err != nil {
		t.Fatalf("infRuleCheckNode: %v", err)
	}
	ruleCheck := out.GetString(InfStateRuleCheck)
	if ruleCheck == "" {
		t.Error("rule check report should not be empty")
	}
	verdict := out.GetString(InfStateRuleVerdict)
	if verdict == "" {
		t.Error("rule verdict should not be empty")
	}
}

// -----------------------------------------------------------------------------
// BuildInfringementGraph (end-to-end)
// -----------------------------------------------------------------------------

func TestBuildInfringementGraph_EndToEnd(t *testing.T) {
	compiled, err := BuildInfringementGraph()
	if err != nil {
		t.Fatalf("BuildInfringementGraph: %v", err)
	}

	out, err := compiled.Run(context.Background(), graph.PregelState{
		InfStatePatentClaims: `1. 一种基于深度学习的图像识别方法，包括以下步骤：
步骤一：采集图像数据；
步骤二：使用卷积神经网络对图像进行特征提取；
步骤三：基于提取的特征对图像进行分类识别。`,
		InfStateAccusedProduct: `被控产品是一种智能相机系统，包含：
- 图像采集模块，用于采集图像数据；
- AI处理芯片，使用卷积神经网络对采集的图像进行特征提取；
- 分类输出模块，基于提取的特征进行图像分类识别。`,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(InfStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	if !strings.Contains(output, "侵权分析报告") {
		t.Error("output should contain report title")
	}
	if !strings.Contains(output, "全面覆盖") {
		t.Error("output should contain full-coverage analysis")
	}
	if !strings.Contains(output, "等同") {
		t.Error("output should contain equivalence analysis")
	}
}

func TestBuildInfringementGraph_EndToEnd_PartialMatch(t *testing.T) {
	compiled, err := BuildInfringementGraph()
	if err != nil {
		t.Fatalf("BuildInfringementGraph: %v", err)
	}

	// Use completely orthogonal technology terms to ensure no naive substring match.
	out, err := compiled.Run(context.Background(), graph.PregelState{
		InfStatePatentClaims:   `1. 一种红外线传感器装置，包括红外发射模块、信号放大模块和数据显示模块。`,
		InfStateAccusedProduct: `某产品包含机械触点开关、LED指示灯和蜂鸣器报警电路，不含红外相关组件。`,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(InfStateOutput)
	// Should detect unmatched features and suggest equivalence analysis.
	if !strings.Contains(output, "未匹配") {
		t.Errorf("should detect unmatched features.\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "等同") {
		t.Errorf("should suggest equivalence analysis.\nOutput:\n%s", output)
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func TestIsFeatureMatch_DirectContainment(t *testing.T) {
	if !isFeatureMatch("采集图像数据", "采集图像数据") {
		t.Error("identical features should match")
	}
	if !isFeatureMatch("采集图像", "系统采集图像并处理") {
		t.Error("claim feature contained in product should match")
	}
}

func TestIsFeatureMatch_NoMatch(t *testing.T) {
	if isFeatureMatch("超声波清洁", "红外线加热") {
		t.Error("unrelated features should not match")
	}
}

func TestSplitBySeparators_Basic(t *testing.T) {
	text := "采集图像数据，使用卷积神经网络提取特征；对图像进行分类识别。"
	features := splitBySeparators(text)
	if len(features) < 2 {
		t.Fatalf("expected >= 2 features, got %d: %v", len(features), features)
	}
}

func TestSplitBySeparators_Empty(t *testing.T) {
	if features := splitBySeparators(""); features != nil {
		t.Errorf("empty text should return nil, got %v", features)
	}
}
