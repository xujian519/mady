package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
- template：内置模板（template = simple_flowchart / system_block / method_steps / component_hierarchy）。

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
					"enum":        []string{"flowchart", "block", "hierarchy", "state", "template"},
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
				"steps":       stepsSchema(),
				"blocks":      blocksSchema(),
				"connections": connectionsSchema(),
				"nodes":       nodesSchema(),
				"states":      statesSchema(),
				"transitions": transitionsSchema(),
				"references":  referencesSchema(),
			},
			"required":             []string{"kind", "output"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

func stepsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "流程图步骤（kind=flowchart）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"shape": map[string]any{"type": "string", "enum": []string{"box", "ellipse", "diamond", "parallelogram", "cylinder"}},
				"next": map[string]any{
					"type":        "array",
					"description": "后继：字符串 id，或 {\"id\":…,\"label\":…}（决策分支必须带标签）",
					"items":       map[string]any{"type": []string{"string", "object"}},
				},
			},
			"required": []string{"id", "label"},
		},
	}
}

func blocksSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "框图块（kind=block）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"type":  map[string]any{"type": "string", "enum": []string{"input", "output", "process", "storage", "decision", "default"}},
			},
			"required": []string{"id", "label"},
		},
	}
}

func connectionsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "框图连接（kind=block）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":  map[string]any{"type": "string"},
				"to":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
			"required": []string{"from", "to"},
		},
	}
}

func nodesSchema() map[string]any {
	node := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":    map[string]any{"type": "string"},
			"label": map[string]any{"type": "string"},
		},
		"required": []string{"id", "label"},
	}
	node["properties"].(map[string]any)["children"] = map[string]any{
		"type":        "array",
		"description": "子节点（任意深度）",
		"items":       node,
	}
	return map[string]any{
		"type":        "array",
		"description": "层级图根节点（kind=hierarchy）",
		"items":       node,
	}
}

func statesSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "状态（kind=state）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
				"kind":  map[string]any{"type": "string", "enum": []string{"normal", "initial", "final"}},
			},
			"required": []string{"id", "label"},
		},
	}
}

func transitionsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "状态转移（kind=state）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":  map[string]any{"type": "string"},
				"to":    map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
			"required": []string{"from", "to"},
		},
	}
}

func referencesSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "渲染后追加的图外标号（仅 svg）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label":   map[string]any{"type": "string", "description": "匹配图内文本（子串，大小写不敏感）"},
				"numeral": map[string]any{"type": "string", "description": "追加的标号"},
			},
			"required": []string{"label", "numeral"},
		},
	}
}

func templateNames() []string {
	out := make([]string, 0, len(AllTemplates()))
	for _, name := range AllTemplates() {
		out = append(out, string(name))
	}
	return out
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
}

func (t *figureTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p toolArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	opts, err := buildOptions(p)
	if err != nil {
		return nil, err
	}
	dot, err := buildDOT(p, opts)
	if err != nil {
		return nil, err
	}

	outputPath, format, err := resolveOutput(t.cfg.WorkingDir, p.Output, p.Format)
	if err != nil {
		return nil, err
	}
	if len(p.References) > 0 && format != FormatSVG {
		return nil, newError(ErrInvalidOutput, "references 仅支持 svg 格式，当前为 %s", format)
	}

	engine := Engine(p.Engine)
	if engine == "" {
		engine = EngineDot
	}
	if !engine.Valid() {
		return nil, newError(ErrRenderFailed, "未知布局引擎 %q", p.Engine)
	}

	// 先做符号链接复核再建目录：先建目录会被工作目录内的符号链接带出边界。
	if err := ensureWithin(t.cfg.WorkingDir, filepath.Dir(outputPath)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}
	dir := filepath.Dir(outputPath)
	stem := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
	renderedPath, err := Render(ctx, RenderSpec{
		DOT: dot, Filename: stem, Format: format, Engine: engine, OutputDir: dir,
	}, RenderConfig{Executable: t.cfg.Executable, Timeout: t.cfg.Timeout})
	if err != nil {
		return nil, err
	}

	var warnings []string
	result := ToolResult{
		DOT: dot, Format: string(format), Engine: string(engine),
	}
	if format == FormatSVG {
		svgData, readErr := os.ReadFile(renderedPath) //nolint:gosec // renderedPath 已限定在工作目录内
		if readErr != nil {
			return nil, newError(ErrInvalidOutput, "读取渲染产物失败：%v", readErr)
		}
		svg := string(svgData)
		if len(p.References) > 0 {
			annotated, annotateErr := AnnotateSVG(svg, p.References, DefaultSVGMaxBytes)
			if annotateErr != nil {
				return nil, annotateErr
			}
			svg = annotated.SVG
			warnings = append(warnings, annotated.Warnings...)
			// 附图是交付文档，按常规文档权限写回。
			if err := os.WriteFile(renderedPath, []byte(svg), 0o644); err != nil { //nolint:gosec
				return nil, newError(ErrInvalidOutput, "写回标注后的 SVG 失败：%v", err)
			}
		}
		viewport := ResolveViewport(RootTag(svg))
		if width, height, ok := viewport.DiagramSizeMm(); ok {
			result.WidthMm = width
			result.HeightMm = height
		}
		if viewport.Note != "" {
			warnings = append(warnings, viewport.Note)
		}
	}

	rel, err := filepath.Rel(t.cfg.WorkingDir, renderedPath)
	if err != nil {
		rel = renderedPath
	}
	result.File = rel
	result.Warnings = warnings
	return result, nil
}

func buildOptions(p toolArgs) (Options, error) {
	style := Style(p.Style)
	if style == "" {
		style = StyleGrayscale
	}
	if style != StyleGrayscale && style != StyleSemantic {
		return Options{}, newError(ErrInvalidPage, "未知色彩策略 %q（可选 grayscale/semantic）", p.Style)
	}
	font := p.Font
	if font == "" {
		font = defaultFont()
	}
	return Options{
		FigureNumber:  p.FigureNumber,
		Style:         style,
		FontName:      font,
		FontPt:        p.FontPt,
		Numerals:      p.Numerals,
		NumeralStep:   p.NumeralStep,
		Page:          p.Page,
		EmbedNumerals: p.EmbedNumerals,
	}, nil
}

func buildDOT(p toolArgs, opts Options) (string, error) {
	switch p.Kind {
	case "flowchart":
		return BuildFlowchart(p.Steps, opts)
	case "block":
		return BuildBlockDiagram(p.Blocks, p.Connections, opts)
	case "hierarchy":
		return BuildHierarchy(p.Nodes, opts)
	case "state":
		return BuildStateDiagram(p.States, p.Transitions, opts)
	case "template":
		if p.Template == "" {
			return "", newError(ErrInvalidTemplate, "kind=template 时必须提供 template")
		}
		return Template(TemplateName(p.Template), opts)
	default:
		return "", newError(ErrInvalidTemplate,
			"未知的 kind %q（可选 flowchart/block/hierarchy/state/template）", p.Kind)
	}
}

// resolveOutput 把输出路径限制在工作目录内，并确定渲染格式。
func resolveOutput(workDir, output, formatArg string) (string, Format, error) {
	if strings.TrimSpace(output) == "" {
		return "", "", newError(ErrPathEscape, "缺少必填字段 'output'")
	}
	if workDir == "" {
		return "", "", newError(ErrPathEscape, "未配置工作目录，无法写入附图")
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败: %w", err)
	}
	absOut := output
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(absWork, output)
	}
	absOut = filepath.Clean(absOut)
	rel, err := filepath.Rel(absWork, absOut)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", newError(ErrPathEscape, "输出路径必须位于工作目录 %s 内：%s", absWork, output)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(absOut), "."))
	format := Format(strings.ToLower(formatArg))
	if format == "" {
		if ext != "" {
			format = Format(ext)
		} else {
			format = FormatSVG
		}
	}
	if !format.Valid() {
		return "", "", newError(ErrInvalidOutput, "未知输出格式 %q（可选 svg/png/pdf）", format)
	}
	if ext != "" && ext != string(format) {
		return "", "", newError(ErrInvalidOutput,
			"输出扩展名 .%s 与 format=%s 不一致", ext, format)
	}
	if ext == "" {
		absOut += "." + string(format)
	}
	return absOut, format, nil
}

// ensureWithin 解析已存在祖先的符号链接后，确认目录仍在工作目录内，
// 防止工作目录内的符号链接把产物写到边界之外。
func ensureWithin(workDir, dir string) error {
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("解析工作目录失败: %w", err)
	}
	resolvedWork, err := filepath.EvalSymlinks(absWork)
	if err != nil {
		return fmt.Errorf("解析工作目录符号链接失败: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("解析输出目录失败: %w", err)
	}
	resolvedDir, err := resolveExistingPrefix(absDir)
	if err != nil {
		return newError(ErrPathEscape, "解析输出目录失败：%v", err)
	}
	rel, err := filepath.Rel(resolvedWork, resolvedDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return newError(ErrPathEscape, "输出路径经符号链接后越出工作目录：%s", resolvedDir)
	}
	return nil
}

// resolveExistingPrefix 从 dir 向上找到第一个存在的路径并解析其符号链接，
// 再把尚不存在的尾部拼回；用于目录尚未创建时的越界判定。
func resolveExistingPrefix(dir string) (string, error) {
	current := dir
	var tail []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{resolved}, tail...)...), nil
}

// defaultFont 返回平台常见的中文字体，避免中文标签在 Graphviz 中丢字。
func defaultFont() string {
	switch runtime.GOOS {
	case "darwin":
		return "PingFang SC"
	case "windows":
		return "Microsoft YaHei"
	default:
		return "Noto Sans CJK SC"
	}
}
