package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (t *figureTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p toolArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	outputPath, format, err := resolveOutput(t.cfg.WorkingDir, p.Output, p.Format)
	if err != nil {
		return nil, err
	}
	if len(p.References) > 0 && format != FormatSVG {
		return nil, newError(ErrInvalidOutput, "references 仅支持 svg 格式，当前为 %s", format)
	}
	// 先做符号链接复核再建目录：先建目录会被工作目录内的符号链接带出边界。
	if err := ensureWithin(t.cfg.WorkingDir, filepath.Dir(outputPath)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	result := ToolResult{Format: string(format)}
	renderedPath := outputPath
	var svg string
	if isVectorKind(p.Kind) {
		svg, err = renderVectorFigure(p, format)
	} else {
		svg, err = t.renderGraphviz(ctx, p, outputPath, format, &result)
	}
	if err != nil {
		return nil, err
	}

	var warnings []string
	if format == FormatSVG {
		if _, warnings, err = t.finalizeSVG(svg, p, renderedPath, &result); err != nil {
			return nil, err
		}
	}

	rel, relErr := filepath.Rel(t.cfg.WorkingDir, renderedPath)
	if relErr != nil {
		rel = renderedPath
	}
	result.File = rel
	result.Warnings = warnings
	return result, nil
}

// renderVectorFigure 构造直接绘制 SVG 的图型（时序图 / 剖视图）。
func renderVectorFigure(p toolArgs, format Format) (string, error) {
	if format != FormatSVG {
		return "", newError(ErrInvalidOutput, "时序图与剖视图仅支持 svg 格式")
	}
	figure, err := buildVectorFigure(p)
	if err != nil {
		return "", err
	}
	return figure.SVG, nil
}

// renderGraphviz 构建 DOT 并渲染；SVG 时读回产物供复核与标注。
func (t *figureTool) renderGraphviz(ctx context.Context, p toolArgs, outputPath string,
	format Format, result *ToolResult) (string, error) {
	opts, err := buildOptions(p)
	if err != nil {
		return "", err
	}
	dot, err := buildDOT(p, opts)
	if err != nil {
		return "", err
	}
	engine := Engine(p.Engine)
	if engine == "" {
		engine = EngineDot
	}
	if !engine.Valid() {
		return "", newError(ErrRenderFailed, "未知布局引擎 %q", p.Engine)
	}
	stem := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
	renderedPath, err := Render(ctx, RenderSpec{
		DOT: dot, Filename: stem, Format: format, Engine: engine, OutputDir: filepath.Dir(outputPath),
	}, RenderConfig{Executable: t.cfg.Executable, Timeout: t.cfg.Timeout})
	if err != nil {
		return "", err
	}
	result.DOT = dot
	result.Engine = string(engine)
	if format != FormatSVG {
		return "", nil
	}
	data, err := os.ReadFile(renderedPath) //nolint:gosec // renderedPath 已限定在工作目录内
	if err != nil {
		return "", newError(ErrInvalidOutput, "读取渲染产物失败：%v", err)
	}
	return string(data), nil
}

// finalizeSVG 追加图外标号、写回文件、补充尺寸并执行渲染复核。
func (t *figureTool) finalizeSVG(svg string, p toolArgs, renderedPath string,
	result *ToolResult) (string, []string, error) {
	var warnings []string
	if len(p.References) > 0 {
		annotated, err := AnnotateSVG(svg, p.References, DefaultSVGMaxBytes)
		if err != nil {
			return "", nil, err
		}
		svg = annotated.SVG
		warnings = append(warnings, annotated.Warnings...)
	}
	// 附图是交付文档，按常规文档权限写回。
	if err := os.WriteFile(renderedPath, []byte(svg), 0o644); err != nil { //nolint:gosec
		return "", nil, newError(ErrInvalidOutput, "写入 SVG 失败：%v", err)
	}
	viewport := ResolveViewport(RootTag(svg))
	if width, height, ok := viewport.DiagramSizeMm(); ok {
		result.WidthMm = width
		result.HeightMm = height
	}
	if viewport.Note != "" {
		warnings = append(warnings, viewport.Note)
	}

	report, err := CheckFigure(svg, CheckOptions{FigureType: p.Kind, MinFontMm: p.MinFontMm})
	if err != nil {
		return "", nil, err
	}
	result.CheckFindings = report.Findings
	for _, finding := range report.Findings {
		warnings = append(warnings, "渲染复核："+finding.Message)
	}
	if !p.FailOnFindings {
		return svg, warnings, nil
	}
	blocking := BlockingFindings(report)
	if len(blocking) == 0 {
		return svg, warnings, nil
	}
	messages := make([]string, len(blocking))
	for i, finding := range blocking {
		messages[i] = "- " + finding.Message
	}
	return "", nil, newError(ErrRenderFailed,
		"渲染复核未通过（%d 项）：\n%s\n文件已写入 %s，但未作为交付件导出；修正后重跑。",
		len(blocking), strings.Join(messages, "\n"), renderedPath)
}

// isVectorKind 报告该 kind 是否走直接绘制 SVG 的矢量通路。
func isVectorKind(kind string) bool {
	return kind == "sequence" || kind == "section"
}

// buildVectorFigure 构造时序图或剖视图。
func buildVectorFigure(p toolArgs) (VectorFigure, error) {
	switch p.Kind {
	case "sequence":
		return BuildSequenceDiagram(p.Participants, p.Messages, p.Sequence)
	case "section":
		return BuildSectionDiagram(p.Section)
	default:
		return VectorFigure{}, newError(ErrInvalidTemplate, "未知的矢量图型 %q", p.Kind)
	}
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
