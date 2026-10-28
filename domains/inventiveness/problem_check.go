package inventiveness

import (
	"fmt"
	"regexp"
	"strings"
)

// 实际解决的技术问题的确定性合规检查（A22.3 三步法第二步）。
//
// 对应审查指南第二部分第四章 3.2.1.1：问题应根据区别特征相对于最接近的现有技术
// 所能达到的技术效果客观确定。本文件在既有四检验（problem.go）之上补两类判定：
//   - PROBLEM-EQUALS-FEATURE：问题文本规范化后与某项区别技术特征相同；
//   - 把四检验映射为带代码的违规条目（阻断/提示两档）。
//
// 不判定技术启示或显而易见性——那属于三步法第三步，需要语义判断。

// 违规代码。
const (
	// CodeProblemEqualsFeature 表示问题只是区别特征的复述。
	CodeProblemEqualsFeature = "PROBLEM-EQUALS-FEATURE"
	// CodeProblemSolutionBinding 表示问题包含解决手段。
	CodeProblemSolutionBinding = "PROBLEM-SOLUTION-BINDING"
	// CodeProblemMultiCausal 表示问题捆绑多个因果链。
	CodeProblemMultiCausal = "PROBLEM-MULTI-CAUSAL"
	// CodeProblemUnmeasured 表示问题缺少可测指标（提示级）。
	CodeProblemUnmeasured = "PROBLEM-UNMEASURED"
)

// ProblemViolation 是一条违规。
type ProblemViolation struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // blocking | quality
	Detail   string `json:"detail"`
}

// ProblemComplianceResult 是技术问题合规检查结果。
type ProblemComplianceResult struct {
	Passed     bool               `json:"passed"`
	Problem    string             `json:"problem,omitempty"`
	Features   []string           `json:"features,omitempty"`
	Violations []ProblemViolation `json:"violations,omitempty"`
	Report     string             `json:"report"`
}

var (
	problemPunctuationRE  = regexp.MustCompile(`[\s，。；、：:,.!?！？"'“”‘’（）()\[\]]`)
	problemQuestionRE     = regexp.MustCompile(`(?:如何|怎样|怎么样|怎么)`)
	problemTailRE         = regexp.MustCompile(`(?:的)?(?:技术)?问题$`)
	problemSolutionVerbRE = regexp.MustCompile(`(?:通过|设置|增设|布置|安装|配置|采用|使用|引入|加装|利用|依靠|借助)`)
	problemGenericTailRE  = regexp.MustCompile(`(?:的)?(?:技术)?(?:方案|方法|结构|装置|布局)?$`)
)

// NormalizeProblemCore 规范化技术问题/特征文本，用于「问题是否等于特征本身」比对：
// 去掉疑问词、手段动词、泛化尾词与标点，只留核心文本。
func NormalizeProblemCore(text string) string {
	normalized := problemPunctuationRE.ReplaceAllString(text, "")
	normalized = problemQuestionRE.ReplaceAllString(normalized, "")
	normalized = problemTailRE.ReplaceAllString(normalized, "")
	normalized = problemSolutionVerbRE.ReplaceAllString(normalized, "")
	normalized = problemGenericTailRE.ReplaceAllString(normalized, "")
	return normalized
}

// FindFeatureRestatement 判断问题文本是否只是某项区别特征的复述，
// 返回命中的原始特征文本。
func FindFeatureRestatement(problem string, features []string) (string, bool) {
	normalizedProblem := NormalizeProblemCore(problem)
	if len([]rune(normalizedProblem)) < 2 {
		return "", false
	}
	for _, feature := range features {
		normalizedFeature := NormalizeProblemCore(feature)
		if len([]rune(normalizedFeature)) < 2 {
			continue
		}
		if normalizedProblem == normalizedFeature {
			return feature, true
		}
	}
	return "", false
}

// CheckProblemCompliance 对「实际解决的技术问题」做合规检查：
// 问题不得只是区别特征的复述、不得包含解决手段、不得捆绑多个因果链；
// 缺可测指标为提示级，不影响 Passed。
func CheckProblemCompliance(problem string, features []string) ProblemComplianceResult {
	result := ProblemComplianceResult{
		Problem:  problem,
		Features: features,
	}
	atomic := CheckAtomicProblem(problem)
	// 含解决手段的问题先按 SOLUTION-BINDING 定性：规范化会剥掉手段动词，
	// 若同时做「问题=特征」比对，「如何通过液冷泵散热」这类文本会被误标成
	// 特征复述，与 SOLUTION-BINDING 自相矛盾。
	if atomic.Checks.NoSolutionBinding {
		if restated, ok := FindFeatureRestatement(problem, features); ok {
			result.Violations = append(result.Violations, ProblemViolation{
				Code: CodeProblemEqualsFeature, Severity: "blocking",
				Detail: fmt.Sprintf("问题表述是区别特征自身的复述：“%s”；技术问题应落到区别特征带来的技术效果", restated),
			})
		}
	}
	if !atomic.Checks.NoSolutionBinding {
		result.Violations = append(result.Violations, ProblemViolation{
			Code: CodeProblemSolutionBinding, Severity: "blocking",
			Detail: "问题表述包含解决手段；技术问题不得包含任何具体手段（指南第二部分第四章 3.2.1.1）",
		})
	}
	if !atomic.Checks.SingleCausality {
		result.Violations = append(result.Violations, ProblemViolation{
			Code: CodeProblemMultiCausal, Severity: "blocking",
			Detail: "问题表述含多个因果连接词，疑似捆绑或复合问题；请拆分或明确主因",
		})
	}
	if !atomic.Checks.MeasurableEffect {
		result.Violations = append(result.Violations, ProblemViolation{
			Code: CodeProblemUnmeasured, Severity: "quality",
			Detail: "问题表述缺少可测指标；建议落到量化效果（如“断裂率从 0.1% 升至 3%”）",
		})
	}
	result.Passed = true
	for _, violation := range result.Violations {
		if violation.Severity == "blocking" {
			result.Passed = false
			break
		}
	}
	result.Report = buildProblemReport(result)
	return result
}

func buildProblemReport(result ProblemComplianceResult) string {
	head := "实际解决的技术问题合规检查（A22.3 第二步）：通过"
	if !result.Passed {
		head = "实际解决的技术问题合规检查（A22.3 第二步）：未通过"
	}
	lines := []string{
		head,
		"- 技术问题：" + result.Problem,
		fmt.Sprintf("- 区别技术特征：%d 项", len(result.Features)),
	}
	if len(result.Features) == 0 {
		lines = append(lines, "- 未提供区别技术特征，跳过“问题是区别特征本身”检查")
	}
	for _, violation := range result.Violations {
		tag := "提示"
		if violation.Severity == "blocking" {
			tag = "阻断"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", tag, violation.Detail))
	}
	return strings.Join(lines, "\n")
}
