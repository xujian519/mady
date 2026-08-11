package inventiveness

import "testing"

// =============================================================================
// 技术问题四检验（atomicChecker 移植）
// =============================================================================

func TestCheckAtomicProblem_Compliant(t *testing.T) {
	result := CheckAtomicProblem("如何在保证设备运行效率的同时降低能耗")
	if !result.Pass {
		t.Errorf("合规技术问题应通过校验，got pass=%v checks=%+v diagnostics=%v",
			result.Pass, result.Checks, result.Diagnostics)
	}
	if !result.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 true")
	}
	if !result.Checks.SingleCausality {
		t.Error("SingleCausality 应为 true")
	}
}

func TestCheckAtomicProblem_SolutionBinding(t *testing.T) {
	result := CheckAtomicProblem("如何通过设置限位凸台提高连接强度")
	if result.Pass {
		t.Error("包含解决手段的技术问题不应通过校验")
	}
	if result.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 false")
	}
	if len(result.Diagnostics) == 0 {
		t.Error("应有未通过项的诊断说明")
	}
}

func TestCheckAtomicProblem_ActionStructureBinding(t *testing.T) {
	result := CheckAtomicProblem("设置密封圈以增强防水性能")
	if result.Pass {
		t.Error("动作+结构名词形态的绑方案表述不应通过校验")
	}
	if result.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 false")
	}
}

func TestCheckAtomicProblem_ThroughGenericPrefixExempted(t *testing.T) {
	// "通过+泛指词"直接连写（"通过现有技术手段"）不视为绑定具体方案（Sati 负向前瞻豁免形态）。
	result := CheckAtomicProblem("如何通过现有技术手段降低成本")
	if !result.Pass {
		t.Errorf("通过后紧跟泛指手段词不应视为绑方案，got pass=%v", result.Pass)
	}
	if !result.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 true（通过后紧跟泛指词，未绑定具体方案）")
	}
}

func TestCheckAtomicProblem_VerbPlusGenericMeansBound(t *testing.T) {
	// "通过+动词+泛指词"（"通过采用现有技术手段"）：Sati 负向前瞻位于动词组之前，
	// 该形态视为绑定解决手段（动词已锚定具体动作，泛指词仅修饰宾语）。
	result := CheckAtomicProblem("如何通过采用现有技术手段降低成本")
	if result.Pass {
		t.Error("通过+动词+泛指词形态（通过采用现有技术手段）应视为绑方案")
	}
	if result.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 false")
	}
}

func TestCheckAtomicProblem_MultiCausal(t *testing.T) {
	result := CheckAtomicProblem("如何解决散热问题使得重量增加导致成本上升")
	if result.Pass {
		t.Error("含多个因果连接词的捆绑问题不应通过校验")
	}
	if result.Checks.SingleCausality {
		t.Error("SingleCausality 应为 false")
	}
}

func TestCheckAtomicProblem_MeasurableEffect(t *testing.T) {
	result := CheckAtomicProblem("如何将焊点断裂率从0.1%降至0.01%")
	if !result.Checks.MeasurableEffect {
		t.Error("含量化指标的问题应识别出可测效果")
	}
	if !result.Pass {
		t.Errorf("可测效果不参与合规判定，合规问题仍应通过，got pass=%v", result.Pass)
	}
}

// TestCheckAtomicProblem_MeasurableOnlyNoBlock verifies missing measurable effect
// does not fail the compliance check (it is a quality hint only).
func TestCheckAtomicProblem_MeasurableOnlyNoBlock(t *testing.T) {
	result := CheckAtomicProblem("如何提高设备运行效率")
	if !result.Pass {
		t.Errorf("缺少量化指标不应阻断校验（Pass 应保持 true），got pass=%v", result.Pass)
	}
	if result.Checks.MeasurableEffect {
		t.Error("该问题无量化指标，MeasurableEffect 应为 false")
	}
}

func TestCheckAtomicProblem_MeansReversible(t *testing.T) {
	result := CheckAtomicProblem("如何解决现有技术中设备散热效率低的问题")
	if !result.Checks.MeansReversible {
		t.Error("提及'现有技术'现状锚点应识别出手段可反推")
	}
}

// =============================================================================
// 技术问题提取
// =============================================================================

func TestExtractTechnicalProblem_JSON(t *testing.T) {
	got := ExtractTechnicalProblem(`{"distinguishing_features": ["a"], "actual_technical_problem": "如何提高运行效率"}`)
	if got != "如何提高运行效率" {
		t.Errorf("JSON 形态提取失败，got %q", got)
	}
}

func TestExtractTechnicalProblem_JSONEscaped(t *testing.T) {
	got := ExtractTechnicalProblem(`"actual_technical_problem": "如何\"快速\"运行"`)
	if got != "如何\"快速\"运行" {
		t.Errorf("转义引号处理失败，got %q", got)
	}
}

func TestExtractTechnicalProblem_Flat(t *testing.T) {
	got := ExtractTechnicalProblem("实际解决的技术问题是：如何提高散热效率，从而降低成本")
	// 平铺形态按 Sati 语义捕获到句号/换行前，含逗号。
	if got != "如何提高散热效率，从而降低成本" {
		t.Errorf("文本形态提取失败，got %q", got)
	}
}

func TestExtractTechnicalProblem_None(t *testing.T) {
	if got := ExtractTechnicalProblem("无相关信息"); got != "" {
		t.Errorf("应返回空串，got %q", got)
	}
}

// =============================================================================
// parseStep2 接线
// =============================================================================

func TestParseStep2_AttachesProblemChecks(t *testing.T) {
	output := `{"distinguishing_features": ["限位凸台"], "actual_tech_problem": "如何通过设置限位凸台提高连接强度"}`
	r := parseStep2(output)
	if r.ActualTechProblem != "如何通过设置限位凸台提高连接强度" {
		t.Fatalf("解析失败，got %q", r.ActualTechProblem)
	}
	if r.ProblemChecks.Pass {
		t.Error("绑方案的 technical problem 不应通过四检验")
	}
	if r.ProblemChecks.Checks.NoSolutionBinding {
		t.Error("NoSolutionBinding 应为 false")
	}
}

func TestParseStep2_EmptyProblemNoChecks(t *testing.T) {
	r := parseStep2(`{"distinguishing_features": ["a"], "actual_tech_problem": ""}`)
	if r.ProblemChecks.Pass {
		t.Error("空技术问题不应通过四检验")
	}
}
