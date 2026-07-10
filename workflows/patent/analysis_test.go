package patent

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestExtractFeatures(t *testing.T) {
	text := "一种基于深度学习的图像识别方法，包括以下步骤：步骤一，采集图像数据；步骤二，使用卷积神经网络提取特征；步骤三，基于所述特征进行分类识别。"
	features := extractFeatures(text)

	if len(features) == 0 {
		t.Fatal("expected at least one feature")
	}

	// Should find features after "包括" marker.
	found := false
	for _, f := range features {
		if strings.Contains(f, "采集图像") || strings.Contains(f, "卷积神经网络") || strings.Contains(f, "特征") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("features should contain technical content: %v", features)
	}
}

func TestExtractFeatures_SimpleText(t *testing.T) {
	text := "本发明涉及一种茶杯，主要由杯体和杯盖组成。杯体采用双层结构保温。"
	features := extractFeatures(text)

	if len(features) == 0 {
		t.Fatal("expected at least one feature from simple text")
	}
	t.Logf("features: %v", features)
}

func TestBuildSearchQuery(t *testing.T) {
	features := []string{"深度学习", "图像识别", "卷积神经网络", "数据增强", "模型优化"}
	query := buildSearchQuery(features)

	if query == "" {
		t.Fatal("query should not be empty")
	}
	// Should use first 3 features.
	if strings.Count(query, " ") > 2 {
		t.Errorf("query should use at most 3 terms: %q", query)
	}
}

func TestParseNode(t *testing.T) {
	input := "一种自动清洁窗户的装置，包括清洁模块、驱动模块和控制模块，其特征在于所述清洁模块采用超声波清洁技术。"
	state := graph.PregelState{StateInput: input}

	out, err := parseNode(context.Background(), state)
	if err != nil {
		t.Fatalf("parseNode: %v", err)
	}

	features, ok := out[StateFeatures].([]string)
	if !ok || len(features) == 0 {
		t.Fatal("expected features")
	}
	_ = features // satisfies compile check.

	query := out.GetString(StateSearchQuery)
	if query == "" {
		t.Error("expected search query")
	}

	if out.GetString(StateInput) != input {
		t.Error("input should be preserved in state")
	}
}

func TestParseNode_EmptyInput(t *testing.T) {
	_, err := parseNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestSearchNode(t *testing.T) {
	state := graph.PregelState{
		StateSearchQuery: "超声波 清洁 窗户",
	}
	out, err := searchNode(context.Background(), state)
	if err != nil {
		t.Fatalf("searchNode: %v", err)
	}

	priorArt, ok := out[StatePriorArt].([]string)
	if !ok || len(priorArt) == 0 {
		t.Fatal("expected prior art results")
	}
}

func TestAnalyzeNode(t *testing.T) {
	features := []string{"清洁模块", "驱动模块", "控制模块", "超声波清洁技术"}
	priorArt := []string{"现有技术A: 机械清洁装置", "现有技术B: 水压清洁窗户"}

	state := graph.PregelState{
		StateFeatures: features,
		StatePriorArt: priorArt,
	}

	out, err := analyzeNode(context.Background(), state)
	if err != nil {
		t.Fatalf("analyzeNode: %v", err)
	}

	comparison := out.GetString(StateComparison)
	if !strings.Contains(comparison, "技术特征比对分析") {
		t.Error("comparison should have analysis header")
	}
	if !strings.Contains(comparison, "清洁模块") {
		t.Error("comparison should list features")
	}
	if !strings.Contains(comparison, "现有技术") {
		t.Error("comparison should reference prior art")
	}
}

func TestConcludeNode(t *testing.T) {
	state := graph.PregelState{
		StateInput:      "一种自动清洁窗户的装置，包括超声波清洁模块。",
		StateComparison: "## 技术特征比对分析\n\n1. 超声波清洁技术",
	}

	out, err := concludeNode(context.Background(), state)
	if err != nil {
		t.Fatalf("concludeNode: %v", err)
	}

	conclusion := out.GetString(StateConclusion)
	if !strings.Contains(conclusion, "专利分析报告") {
		t.Error("conclusion should be a report")
	}
	if !strings.Contains(conclusion, "不构成正式法律意见") {
		t.Error("conclusion must include legal disclaimer")
	}

	output := out.GetString(StateOutput)
	if output == "" {
		t.Error("output should not be empty")
	}
}

func TestBuildNoveltyGraph(t *testing.T) {
	g, err := BuildNoveltyGraph()
	if err != nil {
		t.Fatalf("BuildNoveltyGraph: %v", err)
	}
	if g == nil {
		t.Fatal("graph should not be nil")
	}

	// Run the graph end-to-end.
	state := graph.PregelState{
		StateInput: "一种基于深度学习的图像识别系统，包括图像采集模块、特征提取模块和分类模块，其特征在于所述特征提取模块使用改进的卷积神经网络结构。",
	}

	finalState, err := g.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify end-to-end output.
	output := finalState.GetString(StateOutput)
	if output == "" {
		t.Error("final output should not be empty")
	}
	if !strings.Contains(output, "专利分析报告") {
		t.Error("output should be a patent analysis report")
	}

	// Verify intermediate states are populated.
	if finalState.GetString(StateComparison) == "" {
		t.Error("comparison should be populated")
	}

	t.Logf("Output: %s", output[:min(len(output), 300)])
}
