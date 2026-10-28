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
	"math"
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

// 页面物理尺寸（英寸，纵向）。
var pageSizeInches = map[string][2]float64{
	"a4":     {8.27, 11.69},
	"letter": {8.5, 11},
}

const cmPerInch = 2.54

// validatePage 校验页面参数并返回可输出的属性行。
func pageAttributeLines(page *PageOptions) ([]string, error) {
	if page == nil {
		return nil, nil
	}
	if page.DPI != 0 && page.DPI < 1 {
		return nil, newError(ErrInvalidPage, "dpi 必须为正整数：%d", page.DPI)
	}
	if page.MarginCm < 0 {
		return nil, newError(ErrInvalidPage, "页边距必须为正（厘米）：%v", page.MarginCm)
	}
	if page.Size == "" && page.Orientation == "" && page.DPI == 0 && page.MarginCm == 0 {
		return nil, nil
	}
	var lines []string
	if page.Size != "" {
		size, ok := pageSizeInches[page.Size]
		if !ok {
			return nil, newError(ErrInvalidPage, "未知页面尺寸 %q（可选 a4/letter）", page.Size)
		}
		width, height := size[0], size[1]
		if page.Orientation == "landscape" {
			width, height = height, width
		}
		lines = append(lines, fmt.Sprintf(`    page="%s,%s";`, formatInches(width), formatInches(height)))
		if page.MarginCm > 0 {
			marginIn := page.MarginCm / cmPerInch
			lines = append(lines,
				fmt.Sprintf(`    size="%s,%s";`, formatInches(width-2*marginIn), formatInches(height-2*marginIn)),
				fmt.Sprintf(`    margin="%s,%s";`, formatInches(marginIn), formatInches(marginIn)))
		}
	} else if page.MarginCm > 0 {
		marginIn := formatInches(page.MarginCm / cmPerInch)
		lines = append(lines, fmt.Sprintf(`    margin="%s,%s";`, marginIn, marginIn))
	}
	if page.DPI > 0 {
		lines = append(lines, fmt.Sprintf("    dpi=%d;", page.DPI))
	}
	if page.Orientation == "landscape" {
		lines = append(lines, "    orientation=landscape;")
	}
	return lines, nil
}

func formatInches(value float64) string {
	return fmt.Sprintf("%g", math.Round(value*100)/100)
}

// buildHeader 生成 DOT 头：图形名、方向、页面属性、node/edge 默认样式。
func buildHeader(graphName, rankdir, fontName string, filled bool, page *PageOptions, fontPt int) ([]string, error) {
	nodeFontPt := 10
	edgeFontPt := 9
	if fontPt > 0 {
		nodeFontPt, edgeFontPt = fontPt, fontPt
	}
	nodeAttrs := []string{
		fmt.Sprintf(`fontname="%s"`, EscapeLabel(fontName)),
		fmt.Sprintf("fontsize=%d", nodeFontPt),
	}
	if filled {
		nodeAttrs = append(nodeAttrs, "style=filled")
	}
	pageLines, err := pageAttributeLines(page)
	if err != nil {
		return nil, err
	}
	lines := []string{
		fmt.Sprintf("digraph %s {", graphName),
		fmt.Sprintf("    rankdir=%s;", rankdir),
	}
	lines = append(lines, pageLines...)
	lines = append(lines,
		fmt.Sprintf("    node [%s];", strings.Join(nodeAttrs, ", ")),
		fmt.Sprintf(`    edge [fontname="%s", fontsize=%d];`, EscapeLabel(fontName), edgeFontPt),
		"",
	)
	return lines, nil
}

// 流程图节点形状白名单。
var flowchartShapes = map[string]bool{
	"box": true, "ellipse": true, "diamond": true, "parallelogram": true, "cylinder": true,
}

// BuildFlowchart 构建流程图 DOT。
// 决策节点（diamond）的每个后继必须带非空边标签。
func BuildFlowchart(steps []Step, opts Options) (string, error) {
	if len(steps) == 0 {
		return "", newError(ErrEmptyInput, "流程图至少需要一个步骤")
	}
	ids, known, err := indexSteps(steps)
	if err != nil {
		return "", err
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("Flowchart", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for i, step := range steps {
		id := ids[i]
		shape := step.Shape
		if shape == "" {
			shape = "box"
		}
		if !flowchartShapes[shape] {
			return "", newError(ErrInvalidShape, "未知节点形状：%s", shape)
		}
		label := EscapeLabel(step.Label)
		if opts.embedNumerals() {
			label = assignments[id] + ". " + label
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=%s];`, id, label, shape))
	}
	lines = append(lines, "")
	for i, step := range steps {
		id := ids[i]
		isDecision := step.Shape == "diamond"
		for _, next := range step.Next {
			target, err := SanitizeID(next.ID)
			if err != nil {
				return "", err
			}
			if !known[target] {
				return "", newError(ErrUnknownID, "步骤 %q 的后继不存在：%s", id, target)
			}
			label := strings.TrimSpace(next.Label)
			if label == "" {
				if isDecision {
					return "", newError(ErrMissingEdgeLabel,
						"决策节点 %q 的分支必须带边标签（next：%s）", id, target)
				}
				lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, id, target))
				continue
			}
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s" [label="%s"];`, id, target, EscapeLabel(label)))
		}
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
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

// 框图的块类型 → 形状 / 填充色。
var blockShapes = map[string]string{
	"input": "invhouse", "output": "house", "process": "box",
	"storage": "cylinder", "decision": "diamond", "default": "box",
}

var blockFillColors = map[string]string{
	"input": "lightblue", "output": "lightgreen", "process": "lightyellow",
	"storage": "lightgray", "decision": "lightcoral", "default": "white",
}

// graphEdge 是框图连接与状态转移的共用形态。
type graphEdge struct{ from, to, label string }

// appendEdges 追加边；端点必须已存在，空标签输出无边标签的边。
func appendEdges(lines []string, edges []graphEdge, known map[string]bool) ([]string, error) {
	for _, edge := range edges {
		from, err := SanitizeID(edge.from)
		if err != nil {
			return nil, err
		}
		to, err := SanitizeID(edge.to)
		if err != nil {
			return nil, err
		}
		if !known[from] {
			return nil, newError(ErrUnknownID, "边的起点不存在：%s", edge.from)
		}
		if !known[to] {
			return nil, newError(ErrUnknownID, "边的终点不存在：%s", edge.to)
		}
		if label := strings.TrimSpace(edge.label); label != "" {
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s" [label="%s"];`, from, to, EscapeLabel(label)))
			continue
		}
		lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, from, to))
	}
	return lines, nil
}

// BuildBlockDiagram 构建系统框图 DOT。
func BuildBlockDiagram(blocks []Block, connections []Connection, opts Options) (string, error) {
	if len(blocks) == 0 {
		return "", newError(ErrEmptyInput, "框图至少需要一个块")
	}
	ids := make([]string, len(blocks))
	known := make(map[string]bool, len(blocks))
	for i, block := range blocks {
		id, err := SanitizeID(block.ID)
		if err != nil {
			return "", err
		}
		if known[id] {
			return "", newError(ErrUnknownID, "块 id 重复：%s", id)
		}
		ids[i] = id
		known[id] = true
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	semantic := opts.Style == StyleSemantic
	lines, err := buildHeader("BlockDiagram", "LR", fontOf(opts), semantic, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for i, block := range blocks {
		id := ids[i]
		kind := block.Type
		if kind == "" {
			kind = "default"
		}
		shape, ok := blockShapes[kind]
		if !ok {
			return "", newError(ErrInvalidShape, "未知块类型：%s", kind)
		}
		label := EscapeLabel(block.Label)
		if opts.embedNumerals() {
			label = label + " (" + assignments[id] + ")"
		}
		stylePart := ", shape=" + shape
		if semantic {
			stylePart += ", fillcolor=" + blockFillColors[kind]
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s"%s];`, id, label, stylePart))
	}
	lines = append(lines, "")
	edges := make([]graphEdge, len(connections))
	for i, conn := range connections {
		edges[i] = graphEdge{from: conn.From, to: conn.To, label: conn.Label}
	}
	lines, err = appendEdges(lines, edges, known)
	if err != nil {
		return "", err
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// BuildHierarchy 构建组件层级图 DOT（先序展平，标号按出现顺序分配）。
func BuildHierarchy(tree []HierarchyNode, opts Options) (string, error) {
	if len(tree) == 0 {
		return "", newError(ErrEmptyInput, "层级图至少需要一个根节点")
	}
	var ids []string
	labels := make(map[string]string)
	parents := make(map[string]string)
	seen := make(map[string]bool)
	var visit func(node HierarchyNode, parent string) error
	visit = func(node HierarchyNode, parent string) error {
		id, err := SanitizeID(node.ID)
		if err != nil {
			return err
		}
		if seen[id] {
			return newError(ErrUnknownID, "节点 id 重复：%s", node.ID)
		}
		seen[id] = true
		ids = append(ids, id)
		labels[id] = node.Label
		if parent != "" {
			parents[id] = parent
		}
		for _, child := range node.Children {
			if err := visit(child, id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range tree {
		if err := visit(root, ""); err != nil {
			return "", err
		}
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("ComponentHierarchy", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		label := EscapeLabel(labels[id])
		if opts.embedNumerals() {
			label = label + " (" + assignments[id] + ")"
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s"];`, id, label))
	}
	lines = append(lines, "")
	for _, id := range ids {
		if parent, ok := parents[id]; ok {
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, parent, id))
		}
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// BuildStateDiagram 构建状态转移图 DOT。initial 伪状态画实心小圆且不分配标号。
func BuildStateDiagram(states []State, transitions []Transition, opts Options) (string, error) {
	if len(states) == 0 {
		return "", newError(ErrEmptyInput, "状态图至少需要一个状态")
	}
	known := make(map[string]bool, len(states))
	var labeled []string
	for _, state := range states {
		id, err := SanitizeID(state.ID)
		if err != nil {
			return "", err
		}
		if known[id] {
			return "", newError(ErrUnknownID, "状态 id 重复：%s", id)
		}
		known[id] = true
		if strings.TrimSpace(state.Label) != "" {
			labeled = append(labeled, id)
		}
	}
	assignments, err := assignFor(labeled, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("StateDiagram", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for _, state := range states {
		id, _ := SanitizeID(state.ID)
		kind := state.Kind
		if kind == "initial" {
			lines = append(lines,
				fmt.Sprintf(`    "%s" [label="", shape=circle, style=filled, fillcolor=black, width=0.25, fixedsize=true];`, id))
			continue
		}
		label := EscapeLabel(state.Label)
		if numeral, ok := assignments[id]; ok && opts.embedNumerals() {
			label = numeral + ". " + label
		}
		if kind == "final" {
			lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=doublecircle];`, id, label))
			continue
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=box, style=rounded];`, id, label))
	}
	lines = append(lines, "")
	edges := make([]graphEdge, len(transitions))
	for i, transition := range transitions {
		edges[i] = graphEdge{from: transition.From, to: transition.To, label: transition.Label}
	}
	lines, err = appendEdges(lines, edges, known)
	if err != nil {
		return "", err
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// TemplateName 是内置模板名。
type TemplateName string

// Template constants.
const (
	TemplateSimpleFlowchart    TemplateName = "simple_flowchart"
	TemplateSystemBlock        TemplateName = "system_block"
	TemplateMethodSteps        TemplateName = "method_steps"
	TemplateComponentHierarchy TemplateName = "component_hierarchy"
)

// AllTemplates 返回全部内置模板名。
func AllTemplates() []TemplateName {
	return []TemplateName{
		TemplateSimpleFlowchart, TemplateSystemBlock,
		TemplateMethodSteps, TemplateComponentHierarchy,
	}
}

// Template 构建内置模板 DOT。
func Template(name TemplateName, opts Options) (string, error) {
	switch name {
	case TemplateSimpleFlowchart:
		return BuildFlowchart([]Step{
			{ID: "start", Label: "开始", Shape: "ellipse", Next: []Next{{ID: "step1"}}},
			{ID: "step1", Label: "初始化", Next: []Next{{ID: "decision"}}},
			{ID: "decision", Label: "成功？", Shape: "diamond", Next: []Next{
				{ID: "step3", Label: "是"}, {ID: "step1", Label: "否"},
			}},
			{ID: "step3", Label: "处理", Next: []Next{{ID: "end"}}},
			{ID: "end", Label: "结束", Shape: "ellipse"},
		}, opts)
	case TemplateSystemBlock:
		return BuildBlockDiagram([]Block{
			{ID: "input", Label: "输入设备", Type: "input"},
			{ID: "controller", Label: "控制器", Type: "process"},
			{ID: "processor", Label: "处理模块", Type: "process"},
			{ID: "memory", Label: "存储", Type: "storage"},
			{ID: "output", Label: "输出设备", Type: "output"},
		}, []Connection{
			{From: "input", To: "controller", Label: "信号"},
			{From: "controller", To: "processor", Label: "指令"},
			{From: "processor", To: "memory", Label: "数据"},
			{From: "memory", To: "processor", Label: "数据"},
			{From: "processor", To: "output", Label: "结果"},
		}, opts)
	case TemplateMethodSteps:
		fixed := Options{
			FigureNumber:  opts.FigureNumber,
			Style:         opts.Style,
			FontName:      opts.FontName,
			FontPt:        opts.FontPt,
			NumeralStep:   opts.NumeralStep,
			Page:          opts.Page,
			EmbedNumerals: opts.EmbedNumerals,
			Numerals: map[string]string{
				"step101": "101", "step102": "102", "step103": "103",
				"step104": "104", "step105": "105",
			},
		}
		return BuildFlowchart([]Step{
			{ID: "step101", Label: "接收输入数据", Next: []Next{{ID: "step102"}}},
			{ID: "step102", Label: "校验数据格式", Next: []Next{{ID: "step103"}}},
			{ID: "step103", Label: "处理数据", Next: []Next{{ID: "step104"}}},
			{ID: "step104", Label: "生成输出", Next: []Next{{ID: "step105"}}},
			{ID: "step105", Label: "发送结果"},
		}, fixed)
	case TemplateComponentHierarchy:
		return BuildHierarchy([]HierarchyNode{
			{
				ID: "system", Label: "系统",
				Children: []HierarchyNode{
					{ID: "sub1", Label: "子系统", Children: []HierarchyNode{
						{ID: "c1", Label: "组件A"}, {ID: "c2", Label: "组件B"},
					}},
					{ID: "sub2", Label: "子系统"},
				},
			},
		}, opts)
	default:
		return "", newError(ErrInvalidTemplate, "未知模板：%s", name)
	}
}
