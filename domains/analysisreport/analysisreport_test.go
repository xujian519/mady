package analysisreport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/domains/checker"
)

func TestExtractTechnicalFeatures(t *testing.T) {
	claims := []string{
		"一种药物组合物的制备方法，包括混合和干燥步骤",
		"如权利要求1所述的方法，真空干燥",
		"一种控制系统，包括控制器和传感器",
		"一种图像识别算法，基于神经网络模型处理图像数据",
		"一种壳体，采用铝合金材料制成",
	}
	features := ExtractTechnicalFeatures(claims)
	if len(features) != len(claims) {
		t.Fatalf("features = %d", len(features))
	}
	wantTypes := []AnalysisFeatureType{FeatureMethod, FeatureMethod, FeatureSystem, FeatureData, FeatureMaterial}
	for i, want := range wantTypes {
		if features[i].Type != want {
			t.Errorf("claim %d type = %q, want %q", i+1, features[i].Type, want)
		}
	}
	if features[0].Importance != ImportanceHigh {
		t.Errorf("claim 1 importance = %q", features[0].Importance)
	}
	if features[1].Importance != ImportanceLow {
		t.Errorf("短从属权项 importance = %q, want low", features[1].Importance)
	}
	if features[0].Confidence != typeHitConfidence {
		t.Errorf("confidence = %v", features[0].Confidence)
	}
}

func TestFeatureStatisticsCountsAllKeys(t *testing.T) {
	features := ExtractTechnicalFeatures([]string{"一种方法", "一种材料"})
	stats := ComputeFeatureStatistics(features)
	if stats.Total != 2 {
		t.Errorf("total = %d", stats.Total)
	}
	if stats.ByType[FeatureMethod] != 1 || stats.ByType[FeatureMaterial] != 1 {
		t.Errorf("by_type = %+v", stats.ByType)
	}
	if _, ok := stats.ByType[FeatureDevice]; !ok {
		t.Error("by_type 应包含全枚举键（含 0 值）")
	}
	if stats.ByImportance[ImportanceHigh] != 1 || stats.ByImportance[ImportanceLow] != 1 {
		t.Errorf("by_importance = %+v", stats.ByImportance)
	}
}

func TestComputeClarityScore(t *testing.T) {
	if score := ComputeClarityScore(nil); score.Score != 0 {
		t.Errorf("empty = %v", score.Score)
	}
	clean := ComputeClarityScore([]string{"一种壳体，包括底板和侧板"})
	if clean.Score != 100 {
		t.Errorf("clean = %v", clean.Score)
	}
	fuzzy := ComputeClarityScore([]string{"一种壳体，厚度约为 3mm，大致呈方形", "优选地包括加强筋"})
	if fuzzy.Score != 90 { // 约 / 优选地
		t.Errorf("fuzzy = %v（应命中「约」「优选地」）", fuzzy.Score)
	}
	if !strings.Contains(fuzzy.Rationale, "约") {
		t.Errorf("rationale = %q", fuzzy.Rationale)
	}
}

func TestComputeCompletenessScore(t *testing.T) {
	short := ComputeCompletenessScore([]string{"一种壳体"}, "")
	if short.Score != 55 { // 1 条扣 20、缺摘要扣 15、独立权项过短扣 10
		t.Errorf("score = %v, want 55（%s）", short.Score, short.Rationale)
	}
	full := ComputeCompletenessScore(
		[]string{
			"一种用于半导体封装的散热壳体，包括底板、侧板以及设置在底板上的若干散热齿",
			"根据权利要求1所述的散热壳体，其特征在于所述散热齿呈阵列排布",
		},
		"本发明公开了一种散热壳体。",
	)
	if full.Score != 100 {
		t.Errorf("full = %v（%s）", full.Score, full.Rationale)
	}
}

func TestMergeScoresOrderAndOverride(t *testing.T) {
	deterministic := []QualityScore{
		ComputeCompletenessScore([]string{"一种壳体"}, ""),
		ComputeClarityScore([]string{"一种壳体"}),
	}
	merged := MergeScores(deterministic, map[QualityDomain]ModelScore{
		DomainNovelty: {Score: 80, Rationale: "与 D1 有区别"},
		DomainClarity: {Score: 60, Rationale: "模型认为表述偏宽"},
	})
	if len(merged) != 3 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].Domain != DomainNovelty || merged[0].Basis != BasisModel {
		t.Errorf("merged[0] = %+v", merged[0])
	}
	if merged[1].Domain != DomainClarity || merged[1].Score != 60 || merged[1].Basis != BasisModel {
		t.Errorf("模型补分应覆盖同维度，merged[1] = %+v", merged[1])
	}
	if merged[2].Domain != DomainCompleteness || merged[2].Basis != BasisDeterministic {
		t.Errorf("merged[2] = %+v", merged[2])
	}
}

func TestAggregateCheckerVerdict(t *testing.T) {
	if got := AggregateCheckerVerdict(nil); got != checker.StatusPass {
		t.Errorf("no failures = %q", got)
	}
	warnings := []CheckerFailure{
		{RuleID: "A", Severity: checker.SeverityWarning},
		{RuleID: "B", Severity: checker.SeverityWarning},
	}
	if got := AggregateCheckerVerdict(warnings); got != checker.StatusPass {
		t.Errorf("2 warnings = %q, want pass", got)
	}
	warnings = append(warnings, CheckerFailure{RuleID: "C", Severity: checker.SeverityWarning})
	if got := AggregateCheckerVerdict(warnings); got != checker.StatusNeedsRevision {
		t.Errorf("3 warnings = %q, want needs_revision", got)
	}
	blocked := append(warnings, CheckerFailure{RuleID: "D", Severity: checker.SeverityError})
	if got := AggregateCheckerVerdict(blocked); got != checker.StatusBlocked {
		t.Errorf("error = %q, want blocked", got)
	}
	if got := AggregateCheckerVerdict([]CheckerFailure{{RuleID: "E", Severity: checker.SeverityInfo}}); got != checker.StatusPass {
		t.Errorf("info only = %q", got)
	}
}

func TestBuildReportAutoIPC(t *testing.T) {
	report := BuildReport(Inputs{
		Title:    "一种药物组合物",
		Claims:   []string{"一种药物组合物，包括活性成分和药学上可接受的载体"},
		Abstract: "本发明涉及一种药物组合物，用于治疗高血压。",
	})
	if len(report.IPC) != 1 {
		t.Fatalf("ipc = %+v", report.IPC)
	}
	if report.IPC[0].DomainName == "" || report.IPC[0].DomainName == "未知" {
		t.Errorf("ipc = %+v", report.IPC[0])
	}
	if len(report.IPC[0].NoveltyImplications) == 0 {
		t.Error("IPC 摘要应带审查要点")
	}
	if report.CheckerRan {
		t.Error("未提供 checker_failures 时不应有判级")
	}
	if len(report.InnovationInsights) == 0 || len(report.ExpertConsiderations) == 0 {
		t.Errorf("insights=%v considerations=%v", report.InnovationInsights, report.ExpertConsiderations)
	}
}

func TestBuildReportNoIPCMatch(t *testing.T) {
	report := BuildReport(Inputs{Claims: []string{"一种物品"}})
	if len(report.IPC) != 0 {
		t.Errorf("未命中关键词不应写入 IPC，got %+v", report.IPC)
	}
}

func runReportTool(t *testing.T, args string) (ToolResult, error) {
	t.Helper()
	tool := NewTool()
	raw, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return ToolResult{}, err
	}
	result, ok := raw.(ToolResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

func TestReportToolFull(t *testing.T) {
	result, err := runReportTool(t, `{
		"claims": ["一种散热壳体，包括底板、侧板以及设置在底板上的若干散热齿"],
		"abstract": "本发明公开了一种散热壳体。",
		"checker_failures": [
			{"rule_id": "CLA-001", "message": "缺少必要技术特征", "severity": "warning"}
		],
		"model_scores": {"novelty": {"score": 72, "rationale": "与 D1 存在区别"}},
		"evidence_receipt_count": 0
	}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if !result.Report.CheckerRan || result.Report.CheckerVerdict != checker.StatusPass {
		t.Errorf("checker = %q ran=%v", result.Report.CheckerVerdict, result.Report.CheckerRan)
	}
	if len(result.Report.Scores) != 3 {
		t.Errorf("scores = %+v", result.Report.Scores)
	}
	if !strings.Contains(result.Summary, "新颖性评估良好") {
		t.Errorf("summary = %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "尚未记录证据收据") {
		t.Errorf("summary 缺少证据提醒：%q", result.Summary)
	}
}

func TestReportToolCheckerNotRun(t *testing.T) {
	result, err := runReportTool(t, `{"claims": ["一种壳体，包括底板"]}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.Report.CheckerRan || result.Report.CheckerVerdict != "" {
		t.Errorf("checker verdict = %q", result.Report.CheckerVerdict)
	}
	if result.Report.CheckerFailures == nil {
		t.Error("checker_failures 应为空数组而非 null")
	}
}

func TestReportToolRejectsBadArgs(t *testing.T) {
	if _, err := runReportTool(t, `{"claims": []}`); err == nil {
		t.Error("空 claims 应报错")
	}
	if _, err := runReportTool(t, `{"claims": ["x"], "checker_failures": "bad"}`); err == nil {
		t.Error("checker_failures 类型错误应报错")
	}
}
