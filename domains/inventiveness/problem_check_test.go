package inventiveness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func violationCodes(result ProblemComplianceResult) []string {
	codes := make([]string, 0, len(result.Violations))
	for _, violation := range result.Violations {
		codes = append(codes, violation.Code)
	}
	return codes
}

func hasViolation(result ProblemComplianceResult, code string) bool {
	for _, violation := range result.Violations {
		if violation.Code == code {
			return true
		}
	}
	return false
}

func TestCheckProblemComplianceClean(t *testing.T) {
	result := CheckProblemCompliance(
		"如何在保证强度的前提下把换热效率由 0.82 提升至 0.95",
		[]string{"壳体上设有环形加强筋"},
	)
	if !result.Passed {
		t.Fatalf("expected pass, violations = %+v", result.Violations)
	}
	if len(result.Violations) != 0 {
		t.Errorf("violations = %+v", result.Violations)
	}
	if !strings.Contains(result.Report, "通过") {
		t.Errorf("report = %q", result.Report)
	}
}

func TestCheckProblemEqualsFeature(t *testing.T) {
	result := CheckProblemCompliance(
		"一种带有环形加强筋的壳体的技术问题",
		[]string{"一种带有环形加强筋的壳体"},
	)
	if result.Passed {
		t.Fatal("问题复述区别特征应阻断")
	}
	if !hasViolation(result, CodeProblemEqualsFeature) {
		t.Errorf("violations = %v", violationCodes(result))
	}
	if !strings.Contains(result.Report, "复述") {
		t.Errorf("report = %q", result.Report)
	}
}

func TestCheckProblemNotEqualFeature(t *testing.T) {
	// 负控制：问题落到效果，与特征文本不同，不判 PROBLEM-EQUALS-FEATURE。
	result := CheckProblemCompliance(
		"如何在保证强度的前提下把换热效率由 0.82 提升至 0.95",
		[]string{"一种带有环形加强筋的壳体"},
	)
	if hasViolation(result, CodeProblemEqualsFeature) {
		t.Error("不应判为特征复述")
	}
}

func TestCheckProblemSolutionBinding(t *testing.T) {
	result := CheckProblemCompliance("如何通过设置限位凸台降低装配误差", nil)
	if result.Passed || !hasViolation(result, CodeProblemSolutionBinding) {
		t.Errorf("passed=%v violations=%v", result.Passed, violationCodes(result))
	}
}

func TestCheckProblemSolutionBindingNotRestatement(t *testing.T) {
	// 问题自带手段动词且与特征同文时，只能定性为 SOLUTION-BINDING：
	// 规范化会剥动词，若再比「问题=特征」会把手段绑定误标成特征复述。
	result := CheckProblemCompliance(
		"如何通过液冷泵提高散热效率",
		[]string{"通过液冷泵提高散热效率"},
	)
	if !hasViolation(result, CodeProblemSolutionBinding) {
		t.Errorf("violations = %v, want SOLUTION-BINDING", violationCodes(result))
	}
	if hasViolation(result, CodeProblemEqualsFeature) {
		t.Errorf("手段绑定不应同时判为特征复述：%v", violationCodes(result))
	}
}

func TestExtractTechnicalProblemArrayForm(t *testing.T) {
	text := `{"distinguishing_features":["a"],"actual_technical_problem":["如何提高散热效率","并降低噪音"]}`
	if got := ExtractTechnicalProblem(text); got != "如何提高散热效率" {
		t.Errorf("array form = %q", got)
	}
}

func TestCheckProblemMultiCausal(t *testing.T) {
	result := CheckProblemCompliance("体积增大导致散热变差，并造成噪音升高", nil)
	if result.Passed || !hasViolation(result, CodeProblemMultiCausal) {
		t.Errorf("passed=%v violations=%v", result.Passed, violationCodes(result))
	}
}

func TestCheckProblemUnmeasuredIsQualityOnly(t *testing.T) {
	result := CheckProblemCompliance("提高散热效率", nil)
	if !result.Passed {
		t.Error("缺可测指标只是提示级，不应阻断")
	}
	if !hasViolation(result, CodeProblemUnmeasured) {
		t.Errorf("violations = %v", violationCodes(result))
	}
	for _, violation := range result.Violations {
		if violation.Code == CodeProblemUnmeasured && violation.Severity != "quality" {
			t.Errorf("severity = %q, want quality", violation.Severity)
		}
	}
}

func TestNormalizeProblemCore(t *testing.T) {
	cases := map[string]string{
		"如何提高换热效率的问题":  "提高换热效率",
		"通过设置限位凸台降低误差": "限位凸台降低误差",
		"一种壳体（改进）":     "一种壳体改进",
	}
	for input, want := range cases {
		if got := NormalizeProblemCore(input); got != want {
			t.Errorf("NormalizeProblemCore(%q) = %q, want %q", input, got, want)
		}
	}
}

func runProblemTool(t *testing.T, args string) (ProblemCheckResult, error) {
	t.Helper()
	tool := NewProblemCheckTool()
	raw, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return ProblemCheckResult{}, err
	}
	result, ok := raw.(ProblemCheckResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

func TestProblemToolExtractsFromJSON(t *testing.T) {
	result, err := runProblemTool(t, `{
		"text": "{\"distinguishing_features\":[\"环形加强筋\"],\"actual_technical_problem\":\"如何通过设置环形加强筋提高散热效率\"}",
		"features": ["环形加强筋"]
	}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if result.Skipped {
		t.Fatal("应提取到技术问题")
	}
	if result.Passed || !hasViolation(result.ProblemComplianceResult, CodeProblemSolutionBinding) {
		t.Errorf("violations = %v", violationCodes(result.ProblemComplianceResult))
	}
}

func TestProblemToolSkipsWithoutProblem(t *testing.T) {
	result, err := runProblemTool(t, `{"text":"创造性分析文本，未给出技术问题表述。"}`)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if !result.Skipped || !result.Passed {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Report, "不阻断") {
		t.Errorf("report = %q", result.Report)
	}
}

func TestProblemToolRejectsBadArgs(t *testing.T) {
	if _, err := runProblemTool(t, `{"features":"not-an-array"}`); err == nil {
		t.Error("参数类型错误应报错")
	}
}
