package inventiveness

import (
	"regexp"
	"strings"
)

// =============================================================================
// 技术问题四检验（原子化合规校验，纯确定性、零依赖）
//
// 面向创造性分析（A22.3 三步法）第 2 步"实际解决的技术问题"表述的合规校验，
// 对应审查指南第二部分第四章 3.2.1.1：问题表述不得包含解决手段、不得捆绑多个
// 因果链、应落到可测效果。移植自 Sati src/patent/problem/atomicChecker.ts。
//
// 四检验：
//  1. NoSolutionBinding 不绑方案——问题文本不得包含解决手段。参与合规判定。
//  2. SingleCausality  单一因果——不得捆绑/复合多个因果链。参与合规判定。
//  3. MeasurableEffect 可测效果——缺少量化指标为质量提示项，不参与合规判定。
//  4. MeansReversible  手段可反推——现状锚点的弱启发式，仅信息性，不参与判定。
// =============================================================================

// causalConnectors 因果连接词（用于单一因果检验）。
// 注意剔除"产生"——"产生热量/噪音"为正常结果宾语表述而非因果桥，计入会显著误报。
var causalConnectors = []string{"导致", "使得", "造成", "引起", "引发", "致使", "源于", "归因于"}

// genericMeans 泛指手段词。仅"通过"后紧跟泛指手段词（如"通过现有技术手段降低成本"）
// 的形态不视为绑定具体方案——对应 Sati 负向前瞻 (?!GENERIC_MEANS) 位于"通过"之后、
// 动词组之前的豁免语义。
var genericMeans = []string{"技术手段", "现有技术", "常规手段", "通常做法", "已有手段", "公知手段"}

// solutionBindingThroughRe "通过设置X/利用液冷泵/借助弹性垫"等：动词前缀 + 具体内容。
// 注意：Sati 负向前瞻位于动词组之前，故"通过采用现有技术手段…"（动词后接泛指词）
// 仍视为绑方案，仅"通过+泛指词"直接连写的形态豁免（见 checkNoSolutionBinding）。
var solutionBindingThroughRe = regexp.MustCompile(`通过(设置|增设|加装|引入|配置|利用|采用|借助|使用|依靠)([^，。；]{1,16})`)

// throughPosRe 定位"通过"出现位置，用于逐位置复现 Sati 负向前瞻的豁免语义。
var throughPosRe = regexp.MustCompile(`通过`)

// solutionBindingActionRe "设置限位凸台/增设密封圈/引入闭环控制器"等：动作 + 具体结构名词。
var solutionBindingActionRe = regexp.MustCompile(`(设置|增设|加装|引入|配置|利用|采用)[^，。；]{1,12}(机构|装置|组件|模块|系统|结构|单元|片|件|阀|泵|块|器|机|圈|垫|座|罩|盖|台|板|管|杆|轮|轴|簧|塞)`)

// measurableEffectPatterns 可测指标模式（单一可测效果检验）。命中任一即认为有量化支撑。
var measurableEffectPatterns = []*regexp.Regexp{
	// "15°C" / "58dB" / "23%" / "20000h" / "8mm" 等
	regexp.MustCompile(`\d+(?:\.\d+)?\s*(?:%|％|℃|°C|度|dB|mm|cm|m|kg|h|小时|天|次|倍|ppm|MPa|kPa|V|A|W)`),
	// "降低至42dB" / "提升到60%" / "缩短了3天" 等
	regexp.MustCompile(`(?:提升|降低|减少|增加|升高|下降|缩短|延长)[^。]{0,10}\d`),
	// "从95°C降至78°C" 等对比句式
	regexp.MustCompile(`从\s*\d`),
}

// reversibilityAnchors 现状锚点（手段可反推检验）：问题表述提及现有技术/传统方案等
// 现状，即可反推出一个"现有手段不能解决"的场景。"现有"为通配锚点。
var reversibilityAnchors = []string{"现有", "传统", "目前", "常规", "背景技术"}

// AtomicChecks 单一检验结果。
type AtomicChecks struct {
	SingleCausality   bool `json:"single_causality"`    // 单一因果
	MeasurableEffect  bool `json:"measurable_effect"`   // 可测效果
	MeansReversible   bool `json:"means_reversible"`    // 手段可反推
	NoSolutionBinding bool `json:"no_solution_binding"` // 不绑方案
}

// AtomicCheckResult 四检验结果。Pass 由合规性检验决定（不绑方案 + 单一因果）；
// MeasurableEffect（质量提示）与 MeansReversible（信息性）不参与 Pass——
// 缺量化指标或无法判定可反推都不是"确定不合规"。
type AtomicCheckResult struct {
	Pass        bool         `json:"pass"`
	Checks      AtomicChecks `json:"checks"`
	Diagnostics []string     `json:"diagnostics,omitempty"` // 未通过项的具体原因
}

func countCausalConnections(text string) int {
	count := 0
	for _, connector := range causalConnectors {
		count += strings.Count(text, connector)
	}
	return count
}

func startsWithGenericMeans(s string) bool {
	for _, m := range genericMeans {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}

func checkNoSolutionBinding(text string) bool {
	// 动作+结构名词模式（"设置限位凸台/增设密封圈"）：全局检查，无前缀豁免。
	if solutionBindingActionRe.MatchString(text) {
		return false
	}
	// 通过+动词+内容模式：Sati 的负向前瞻 (?!泛指词) 位于"通过"之后、动词组之前，
	// 即仅"通过"后紧跟泛指手段词的位置豁免；RE2 不支持负向前瞻，故逐"通过"位置判定。
	for _, m := range throughPosRe.FindAllStringIndex(text, -1) {
		if startsWithGenericMeans(text[m[1]:]) {
			continue // "通过现有技术手段…"：未绑定具体方案
		}
		if solutionBindingThroughRe.MatchString(text[m[0]:]) {
			return false
		}
	}
	return true
}

func checkSingleCausality(text string) bool {
	return countCausalConnections(text) < 2
}

func checkMeasurableEffect(text string) bool {
	for _, re := range measurableEffectPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func checkMeansReversible(text string) bool {
	for _, anchor := range reversibilityAnchors {
		if strings.Contains(text, anchor) {
			return true
		}
	}
	return false
}

// CheckAtomicProblem 对单个技术问题文本执行四检验。
func CheckAtomicProblem(problem string) AtomicCheckResult {
	checks := AtomicChecks{
		SingleCausality:   checkSingleCausality(problem),
		MeasurableEffect:  checkMeasurableEffect(problem),
		MeansReversible:   checkMeansReversible(problem),
		NoSolutionBinding: checkNoSolutionBinding(problem),
	}
	var diagnostics []string
	if !checks.NoSolutionBinding {
		diagnostics = append(diagnostics,
			"问题表述包含解决手段（如'通过设置X'/'利用X装置'），技术问题不得包含任何具体手段，请改写为不绑定方案的表述")
	}
	if !checks.SingleCausality {
		diagnostics = append(diagnostics, "问题表述含多个因果连接词，疑似捆绑/复合问题，请拆分或明确主因")
	}
	if !checks.MeasurableEffect {
		diagnostics = append(diagnostics, "问题表述缺少可测指标，建议落到量化效果（如'焊点断裂率从 0.1% 升至 3%'）")
	}
	return AtomicCheckResult{
		Pass:        checks.NoSolutionBinding && checks.SingleCausality,
		Checks:      checks,
		Diagnostics: diagnostics,
	}
}

// ExtractTechnicalProblem 从评估文本中提取"实际解决的技术问题"片段（兼容两种形态）：
//   - Graph 形态：inventiveness_diff JSON 的 "actual_technical_problem" 字段；
//   - 文本形态："实际解决的技术问题：/为/是 ..."句。
//
// 提取不到返回空串。
func ExtractTechnicalProblem(text string) string {
	jsonRe := regexp.MustCompile(`"actual_technical_problem"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	if m := jsonRe.FindStringSubmatch(text); m != nil {
		// 先解义转义的反斜杠，再解义转义的引号（对齐 Sati JSON.parse 语义）。
		s := strings.ReplaceAll(m[1], `\\`, `\`)
		s = strings.ReplaceAll(s, `\"`, `"`)
		return s
	}
	flatRe := regexp.MustCompile(`实际解决的技术问题[是为：:]+([^。\n]{4,120})`)
	if m := flatRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}
