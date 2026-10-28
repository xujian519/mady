package triz

// TRIZ 矛盾分析的类型定义。
//
// 用途边界：产物服务于交底书理解、替代方案方向与补强清单，**不是**创造性三步法
// 第二步「实际解决的技术问题」的表述依据——矛盾对刻画的是发明构思层面的参数取舍，
// 带手段色彩，直接进入法定论证段会与「不绑方案」检验冲突。

// Parameter 是一个工程参数引用（编号 + 随包参数表名称）。
type Parameter struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// Principle 是一条发明原理。
type Principle struct {
	Number      int    `json:"number"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MatrixStatus 是一条矛盾对在 39x39 经典矩阵中的落格状态。
type MatrixStatus = string

// Matrix status constants.
const (
	MatrixRecommended MatrixStatus = "recommended" // 有推荐原理
	MatrixPhysical    MatrixStatus = "physical"    // 对角格：物理矛盾，无条目
	MatrixGap         MatrixStatus = "gap"         // 转录空缺：无推荐原理
)

// GapKind 是参数完备性缺口的维度。
type GapKind = string

// Gap kind constants.
const (
	GapCurrentValue GapKind = "current-value" // 缺现状值
	GapTargetValue  GapKind = "target-value"  // 缺目标值
	GapUnit         GapKind = "unit"          // 缺单位
	GapTestMethod   GapKind = "test-method"   // 缺测量/试验口径
	GapOther        GapKind = "other"         // 其它缺口
)

// GapKinds 是缺口维度的全集，按报告展示顺序排列。
var GapKinds = []GapKind{GapCurrentValue, GapTargetValue, GapUnit, GapTestMethod, GapOther}

// Contradiction 是一条已核验的技术矛盾。
type Contradiction struct {
	ID                 string       `json:"id"`
	Improving          Parameter    `json:"improving"`
	Worsening          Parameter    `json:"worsening"`
	Statement          string       `json:"statement"`
	Evidence           string       `json:"evidence"`
	Principles         []Principle  `json:"principles"`
	MatrixStatus       MatrixStatus `json:"matrix_status"`
	SolutionDirections []string     `json:"solution_directions,omitempty"`
}

// ParameterGap 是一条参数完备性缺口。
type ParameterGap struct {
	Parameter Parameter `json:"parameter"`
	Missing   []GapKind `json:"missing"`
	Detail    string    `json:"detail,omitempty"`
}

// UnmappedItem 是未采纳的表述。
type UnmappedItem struct {
	Statement string `json:"statement"`
	Reason    string `json:"reason"`
}

// Analysis 是一次交底书 TRIZ 分析的完整产物。
type Analysis struct {
	Contradictions     []Contradiction `json:"contradictions"`
	ParameterGaps      []ParameterGap  `json:"parameter_gaps"`
	Unmapped           []UnmappedItem  `json:"unmapped"`
	DroppedForEvidence int             `json:"dropped_for_evidence"`
}

// ContradictionInput 是模型抽取的一条矛盾候选。
type ContradictionInput struct {
	Improving          int      `json:"improving"`
	Worsening          int      `json:"worsening"`
	Statement          string   `json:"statement"`
	Evidence           string   `json:"evidence"`
	SolutionDirections []string `json:"solution_directions,omitempty"`
}

// ParameterGapInput 是模型抽取的一条参数缺口候选。
type ParameterGapInput struct {
	Parameter int       `json:"parameter"`
	Missing   []GapKind `json:"missing"`
	Detail    string    `json:"detail,omitempty"`
}

// UnmappedInput 是模型自报的未映射表述。
type UnmappedInput struct {
	Statement string `json:"statement"`
	Reason    string `json:"reason,omitempty"`
}

// Extraction 是模型抽取的原始结果：字段在 BuildAnalysis 中逐项校验。
type Extraction struct {
	Contradictions []ContradictionInput `json:"contradictions"`
	ParameterGaps  []ParameterGapInput  `json:"parameter_gaps,omitempty"`
	Unmapped       []UnmappedInput      `json:"unmapped,omitempty"`
}
