package generate

import (
	"time"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是附图生成工具对模型可见的名称。
const ToolName = "figure_generate"

const toolDesc = `把结构化描述构建为专利附图并渲染为 SVG/PNG/PDF。

kind 决定图的类型：
- flowchart：流程图（steps，决策菱形 diamond 的每个后继必须带 label）；
- block：系统框图（blocks + connections）；
- hierarchy：组件层级图（nodes，可任意嵌套 children）；
- state：状态转移图（states + transitions，initial 为实心小圆且不占标号）；
- template：内置模板（template = simple_flowchart / system_block / method_steps / component_hierarchy）；
- sequence：时序图（participants + messages，直接绘制 SVG，不依赖 Graphviz）；
- section：剖视图（section.parts 轮廓 + 45° 剖面线 + 引线标号，直接绘制 SVG）。

SVG 产物会做渲染复核：文字被线条穿过、文字与图线净距不足（按图型分档）、
字高低于 min_font_mm、墨迹越出画布；复核发现随结果返回，fail_on_findings 为 true
时阻断级发现令工具失败（文件保留供修正）。

附图默认黑白线条（专利附图一般使用黑色墨水）；参考标号按「图号 × 100」系列自动分配
（图1 = 100、102…，图2 = 200…），也可用 numerals 显式指定以实现跨图同件同号。
输出为 SVG 时可用 references 在渲染后追加图外标号（label 子串匹配文本元素）。

只读取与 kind 对应的字段（flowchart 读 steps、block 读 blocks/connections、hierarchy 读 nodes、
state 读 states/transitions、template 读 template），其余字段忽略。

渲染依赖本机 Graphviz（dot）。产物必须落在工作目录内。`

// ToolConfig 配置附图生成工具。
type ToolConfig struct {
	// WorkingDir 是附图输出的沙箱边界。
	WorkingDir string
	// Executable 覆盖 dot 路径；为空时自动探测。
	Executable string
	// Timeout 为单次渲染超时；为零时用 DefaultRenderTimeout。
	Timeout time.Duration
}

type figureTool struct {
	cfg ToolConfig
}

// NewTool 创建 figure_generate 工具。
func NewTool(cfg ToolConfig) *agentcore.Tool {
	t := &figureTool{cfg: cfg}
	return &agentcore.Tool{
		Name:        ToolName,
		Description: toolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"flowchart", "block", "hierarchy", "state", "template", "sequence", "section"},
					"description": "图的类型",
				},
				"template": map[string]any{
					"type":        "string",
					"enum":        templateNames(),
					"description": "内置模板名（kind=template 时必填）",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "输出路径（相对工作目录或工作目录内的绝对路径），扩展名决定格式",
				},
				"figure_number": map[string]any{
					"type":        "integer",
					"description": "图号（1 起），决定标号系列起点，默认 1",
					"minimum":     1,
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"grayscale", "semantic"},
					"description": "色彩策略，默认 grayscale（黑白线条）",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"svg", "png", "pdf"},
					"description": "输出格式，默认按 output 扩展名或 svg",
				},
				"engine": map[string]any{
					"type":        "string",
					"enum":        []string{"dot", "neato", "fdp", "circo", "twopi", "sfdp"},
					"description": "Graphviz 布局引擎，默认 dot",
				},
				"font": map[string]any{
					"type":        "string",
					"description": "字体名；缺省按平台选择中文字体（苹方/雅黑/Noto Sans CJK SC）",
				},
				"font_pt": map[string]any{
					"type":        "integer",
					"description": "正文与标号字号（pt），缺省 node 10 / edge 9",
				},
				"numeral_step": map[string]any{
					"type":        "integer",
					"description": "自动标号步进，默认 2",
					"minimum":     1,
				},
				"numerals": map[string]any{
					"type":                 "object",
					"description":          "显式标号：组件 id → 标号（跨图同件同号）",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"embed_numerals": map[string]any{
					"type":        "boolean",
					"description": "是否把标号内嵌进标签，默认 true；关闭时可用 references 图外标注",
				},
				"page": map[string]any{
					"type":        "object",
					"description": "提交规格页面（可选）",
					"properties": map[string]any{
						"size":        map[string]any{"type": "string", "enum": []string{"a4", "letter"}},
						"orientation": map[string]any{"type": "string", "enum": []string{"portrait", "landscape"}},
						"dpi":         map[string]any{"type": "integer", "minimum": 1},
						"margin_cm":   map[string]any{"type": "number", "minimum": 0},
					},
					"additionalProperties": false,
				},
				"steps":        stepsSchema(),
				"blocks":       blocksSchema(),
				"connections":  connectionsSchema(),
				"nodes":        nodesSchema(),
				"states":       statesSchema(),
				"transitions":  transitionsSchema(),
				"references":   referencesSchema(),
				"participants": participantsSchema(),
				"messages":     messagesSchema(),
				"sequence":     sequenceOptionsSchema(),
				"section":      sectionSchema(),
				"fail_on_findings": map[string]any{
					"type":        "boolean",
					"description": "渲染复核命中阻断级发现时令工具失败（文件保留供修正），默认 false",
				},
				"min_font_mm": map[string]any{
					"type":        "number",
					"description": "字高内控下限（毫米）；0 关闭该判据",
					"minimum":     0,
				},
			},
			"required":             []string{"kind", "output"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type toolArgs struct {
	Kind          string            `json:"kind"`
	Template      string            `json:"template"`
	Output        string            `json:"output"`
	FigureNumber  int               `json:"figure_number"`
	Style         string            `json:"style"`
	Format        string            `json:"format"`
	Engine        string            `json:"engine"`
	Font          string            `json:"font"`
	FontPt        int               `json:"font_pt"`
	NumeralStep   int               `json:"numeral_step"`
	Numerals      map[string]string `json:"numerals"`
	EmbedNumerals *bool             `json:"embed_numerals"`
	Page          *PageOptions      `json:"page"`
	Steps         []Step            `json:"steps"`
	Blocks        []Block           `json:"blocks"`
	Connections   []Connection      `json:"connections"`
	Nodes         []HierarchyNode   `json:"nodes"`
	States        []State           `json:"states"`
	Transitions   []Transition      `json:"transitions"`
	References    []Reference       `json:"references"`
	// 时序图 / 剖视图输入。
	Participants []SequenceParticipant `json:"participants"`
	Messages     []SequenceMessage     `json:"messages"`
	Sequence     SequenceOptions       `json:"sequence"`
	Section      SectionDiagramInput   `json:"section"`
	// 渲染复核：fail_on_findings 为 true 时阻断级发现令工具失败（文件保留）。
	FailOnFindings bool    `json:"fail_on_findings"`
	MinFontMm      float64 `json:"min_font_mm"`
}

// ToolResult 是附图生成结果。
type ToolResult struct {
	DOT      string   `json:"dot"`
	File     string   `json:"file"`
	Format   string   `json:"format"`
	Engine   string   `json:"engine"`
	WidthMm  float64  `json:"width_mm,omitempty"`
	HeightMm float64  `json:"height_mm,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	// CheckFindings 是 SVG 渲染复核的发现（png/pdf 不做复核）。
	CheckFindings []CheckFinding `json:"check_findings,omitempty"`
}
