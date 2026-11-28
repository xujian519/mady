// Package generate 把结构化描述构建为专利附图 DOT，并调用本机 Graphviz 渲染。
//
// 移植自 deepseek-harness packages/patent/patent-tools/src/figure 的 F1 部分
// （dot-builder / graphviz-renderer / svg-viewport / svg-annotate）：附图一律
// 黑白线条（《专利审查指南》第一部分第一章 4.3），参考标号按「图号 × 100」
// 系列分配（图1 = 100–199、图2 = 200–299），标签内嵌标号；决策分支必须带边
// 标签，否则图面歧义。
package generate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Style 是附图色彩策略：grayscale 为黑白线条（默认），semantic 允许按块类型
// 填充颜色，仅当色彩承载技术内容时使用。
type Style string

// Style constants.
const (
	StyleGrayscale Style = "grayscale"
	StyleSemantic  Style = "semantic"
)

// Engine 是 Graphviz 布局引擎（白名单）。
type Engine string

// Layout engine constants.
const (
	EngineDot   Engine = "dot"
	EngineNeato Engine = "neato"
	EngineFDP   Engine = "fdp"
	EngineCirco Engine = "circo"
	EngineTwopi Engine = "twopi"
	EngineSFDP  Engine = "sfdp"
)

// AllEngines 返回全部可用布局引擎。
func AllEngines() []Engine {
	return []Engine{EngineDot, EngineNeato, EngineFDP, EngineCirco, EngineTwopi, EngineSFDP}
}

// Valid reports whether the engine is in the whitelist.
func (e Engine) Valid() bool {
	switch e {
	case EngineDot, EngineNeato, EngineFDP, EngineCirco, EngineTwopi, EngineSFDP:
		return true
	default:
		return false
	}
}

// Format 是渲染输出格式（白名单）。
type Format string

// Output format constants.
const (
	FormatSVG Format = "svg"
	FormatPNG Format = "png"
	FormatPDF Format = "pdf"
)

// AllFormats 返回全部输出格式。
func AllFormats() []Format { return []Format{FormatSVG, FormatPNG, FormatPDF} }

// Valid reports whether the format is in the whitelist.
func (f Format) Valid() bool {
	switch f {
	case FormatSVG, FormatPNG, FormatPDF:
		return true
	default:
		return false
	}
}

// ErrorCode 分类 DOT 构建与渲染错误，供工具层转成可读提示。
type ErrorCode string

// Error codes.
const (
	ErrEmptyInput         ErrorCode = "empty_input"
	ErrUnknownID          ErrorCode = "unknown_id"
	ErrMissingEdgeLabel   ErrorCode = "missing_edge_label"
	ErrConflictingNumeral ErrorCode = "conflicting_numeral"
	ErrInvalidLabel       ErrorCode = "invalid_label"
	ErrInvalidID          ErrorCode = "invalid_id"
	ErrInvalidShape       ErrorCode = "invalid_shape"
	ErrInvalidTemplate    ErrorCode = "invalid_template"
	ErrInvalidPage        ErrorCode = "invalid_page"
	ErrUnsafeSVG          ErrorCode = "unsafe_svg"
	ErrInvalidSVG         ErrorCode = "invalid_svg"
	ErrTooLarge           ErrorCode = "too_large"
	ErrNotInstalled       ErrorCode = "not_installed"
	ErrRenderFailed       ErrorCode = "render_failed"
	ErrInvalidOutput      ErrorCode = "invalid_output"
	ErrPathEscape         ErrorCode = "path_escape"
)

// Error 是附图构建/渲染的分类错误。
type Error struct {
	Code    ErrorCode
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

func newError(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// 标号系列：图 N 从 100 + 100*(N-1) 起，步进默认 2（对照 100/102/104 示例）。
const (
	numeralSeriesBase   = 100
	numeralSeriesStride = 100
	DefaultNumeralStep  = 2
)

// Next 是流程图后继：字符串节点 id，或带边标签的对象。
// 决策节点（diamond）的分支必须用带标签形式。
type Next struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// UnmarshalJSON 接受 "id" 字符串或 {"id":…,"label":…} 对象两种形态。
func (n *Next) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, `"`) {
		return json.Unmarshal(data, &n.ID)
	}
	type nextAlias Next
	var alias nextAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*n = Next(alias)
	return nil
}

// Step 是流程图步骤。
type Step struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Shape 为 box/ellipse/diamond/parallelogram/cylinder，默认 box。
	Shape string `json:"shape,omitempty"`
	Next  []Next `json:"next,omitempty"`
}

// Block 是系统框图的块。
type Block struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Type 为 input/output/process/storage/decision/default，默认 default。
	Type string `json:"type,omitempty"`
}

// Connection 是框图连接。
type Connection struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// HierarchyNode 是层级图节点（任意深度）。
type HierarchyNode struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Children []HierarchyNode `json:"children,omitempty"`
}

// State 是状态图状态；Kind 为 normal/initial/final。
type State struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

// Transition 是状态转移，条件写在箭头上。
type Transition struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// PageOptions 是提交规格页面参数（A4/Letter、方向、分辨率、页边距）。
type PageOptions struct {
	// Size 为 a4 或 letter；为空时不输出页面属性。
	Size string `json:"size,omitempty"`
	// Orientation 为 portrait 或 landscape。
	Orientation string `json:"orientation,omitempty"`
	// DPI 为渲染分辨率（光栅格式生效）。
	DPI int `json:"dpi,omitempty"`
	// MarginCm 为页边距（厘米，四边同值）。
	MarginCm float64 `json:"margin_cm,omitempty"`
}

// Options 是四类构建器共用的选项。
type Options struct {
	// FigureNumber 为图号（1 起），决定标号系列起点。
	FigureNumber int
	// Style 为色彩策略，默认 grayscale。
	Style Style
	// FontName 为字体名；含中文时调用方应传平台字体（如 PingFang SC）。
	FontName string
	// FontPt 为正文与标号字号（pt）；缺省 node 10 / edge 9。
	FontPt int
	// Numerals 为显式标号（组件 id → 标号），用于跨图同件同号。
	Numerals map[string]string
	// NumeralStep 为同图自动标号步进，默认 2。
	NumeralStep int
	// Page 为提交规格页面参数；nil 时不输出布局属性。
	Page *PageOptions
	// EmbedNumerals 控制标号是否内嵌进标签；nil 表示内嵌。
	EmbedNumerals *bool
}

func (o Options) embedNumerals() bool {
	if o.EmbedNumerals == nil {
		return true
	}
	return *o.EmbedNumerals
}

// NumeralOptions 是标号分配参数。
type NumeralOptions struct {
	FigureNumber int
	Start        int
	Step         int
	Explicit     map[string]string
	Reserved     []string
}

// NumeralAssignment 是一条标号分配结果。
type NumeralAssignment struct {
	ID      string `json:"id"`
	Numeral string `json:"numeral"`
}

// NumeralSeriesStart 返回图号的标号系列起点（图1 = 100、图2 = 200）。
func NumeralSeriesStart(figureNumber int) int {
	if figureNumber < 1 {
		figureNumber = 1
	}
	return numeralSeriesBase + numeralSeriesStride*(figureNumber-1)
}

// AssignNumerals 按 ids 顺序分配参考标号：显式标号优先，其余从系列起点按
// step 递增并跳过已占用号。
func AssignNumerals(ids []string, opts NumeralOptions) ([]NumeralAssignment, error) {
	start := opts.Start
	if start == 0 {
		start = NumeralSeriesStart(opts.FigureNumber)
	}
	step := opts.Step
	if step == 0 {
		step = DefaultNumeralStep
	}
	if start < 1 || step < 1 {
		return nil, newError(ErrInvalidLabel, "标号起点/步进必须为正整数：start=%d, step=%d", start, step)
	}

	used := make(map[string]string, len(ids))
	assigned := make(map[string]string, len(ids))
	for id, numeral := range opts.Explicit {
		value := strings.TrimSpace(numeral)
		if value == "" {
			return nil, newError(ErrConflictingNumeral, "显式标号不能为空：%s", id)
		}
		if owner, ok := used[value]; ok && owner != id {
			return nil, newError(ErrConflictingNumeral, "标号 %s 被组件 %s 与 %s 重复占用", value, owner, id)
		}
		used[value] = id
		assigned[id] = value
	}
	reserved := make(map[string]bool, len(opts.Reserved))
	for _, numeral := range opts.Reserved {
		reserved[numeral] = true
	}

	candidate := start
	for _, id := range ids {
		if _, ok := assigned[id]; ok {
			continue
		}
		numeral := fmt.Sprintf("%d", candidate)
		for used[numeral] != "" || reserved[numeral] {
			candidate += step
			numeral = fmt.Sprintf("%d", candidate)
		}
		used[numeral] = id
		assigned[id] = numeral
		candidate += step
	}

	out := make([]NumeralAssignment, len(ids))
	for i, id := range ids {
		out[i] = NumeralAssignment{ID: id, Numeral: assigned[id]}
	}
	return out, nil
}

var invalidIDChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// SanitizeID 清洗 DOT 节点/边 id（非法字符替换为下划线）。
func SanitizeID(id string) (string, error) {
	cleaned := invalidIDChars.ReplaceAllString(id, "_")
	if cleaned == "" {
		return "", newError(ErrInvalidID, "节点 id 为空或不合法：%q", id)
	}
	return cleaned, nil
}

var controlChars = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)

// EscapeLabel 转义 DOT 双引号字符串：控制字符换空格，反斜杠与引号加转义。
func EscapeLabel(text string) string {
	escaped := controlChars.ReplaceAllString(text, " ")
	escaped = strings.ReplaceAll(escaped, `\`, `\\`)
	return strings.ReplaceAll(escaped, `"`, `\"`)
}

const cmPerInch = 2.54

// 流程图节点形状白名单。
var flowchartShapes = map[string]bool{
	"box": true, "ellipse": true, "diamond": true, "parallelogram": true, "cylinder": true,
}

// indexSteps 清洗 id、检查重复，并返回 id 顺序与原步骤索引。
func indexSteps(steps []Step) ([]string, map[string]bool, error) {
	ids := make([]string, len(steps))
	known := make(map[string]bool, len(steps))
	for i, step := range steps {
		id, err := SanitizeID(step.ID)
		if err != nil {
			return nil, nil, err
		}
		if known[id] {
			return nil, nil, newError(ErrUnknownID, "节点 id 重复：%s", id)
		}
		ids[i] = id
		known[id] = true
	}
	return ids, known, nil
}

func assignFor(ids []string, opts Options) (map[string]string, error) {
	assignments, err := AssignNumerals(ids, NumeralOptions{
		FigureNumber: opts.FigureNumber,
		Step:         opts.NumeralStep,
		Explicit:     opts.Numerals,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		out[assignment.ID] = assignment.Numeral
	}
	return out, nil
}

func fontOf(opts Options) string {
	if opts.FontName == "" {
		return "Helvetica"
	}
	return opts.FontName
}
