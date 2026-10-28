package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// 附图交付工具：落版（figure_submission）与跨图一致性核验（figure_consistency）。

// 工具名。
const (
	SubmissionToolName  = "figure_submission"
	ConsistencyToolName = "figure_consistency"
)

const submissionToolDesc = `把一张附图 SVG 落版到目标法域的 A4 附图页。

- 幅面与页边距按法域固定档案（cnipa / pct / uspto），图号画在图形正下方、页码画在版心底部；
- 图号写法：cnipa「图1」、pct「Fig. 1」、uspto「FIG. 1」；仅一幅附图时按法域规则不编号；
- 图案按可用绘图区等比缩放（放大上限 4 倍），可选 0/90/180/270 旋转；
- 返回落版缩放比、图形占位尺寸与字高核算（给出 body_font_size 时）。

落版页会做渲染复核：线条穿字、墨迹越界；fail_on_findings 为 true 时阻断级发现令工具失败（文件保留）。`

const consistencyToolDesc = `核验同一案件多张附图的图号、标记与文本引用是否一致。

- 图号：重复与缺号（1..最大图号）；
- 标记：同一标记在不同图中名称不一致；
- 文本引用：从权利要求书/说明书抽取纯数字附图标记（壳体(10)、盖板20），列出图中未识别的标记；
- 摘要附图：指定图号是否在附图集合中。

只做纯文本/标记层面的核验，不替代对图面的视觉审查。`

// NewSubmissionTool 创建附图落版工具。
func NewSubmissionTool(cfg ToolConfig) *agentcore.Tool {
	t := &submissionTool{cfg: cfg}
	return &agentcore.Tool{
		Name:        SubmissionToolName,
		Description: submissionToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"drawing": map[string]any{
					"type":        "string",
					"description": "图形 SVG 路径（相对工作目录或工作目录内的绝对路径）",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "落版页输出路径（工作目录内，.svg）",
				},
				"office": map[string]any{
					"type":        "string",
					"enum":        officeNames(),
					"description": "目标法域，默认 cnipa",
				},
				"figure_number": map[string]any{"type": "integer", "minimum": 1, "description": "本幅图号（1 起）"},
				"figure_count":  map[string]any{"type": "integer", "minimum": 1, "description": "本案附图总数"},
				"caption": map[string]any{
					"type":        "string",
					"description": "覆盖自动图号（如已按法域写好）；为空时按法域生成",
				},
				"sheet_index": map[string]any{"type": "integer", "minimum": 1, "description": "附图页序号（1 起）"},
				"sheet_total": map[string]any{"type": "integer", "minimum": 1, "description": "附图页总数"},
				"rotate_deg": map[string]any{
					"type":        "integer",
					"enum":        []int{0, 90, 180, 270},
					"description": "绕绘图区中心顺时针旋转，默认 0",
				},
				"body_font_size": map[string]any{
					"type":        "number",
					"description": "图形正文数字/字母字号（用户单位），用于核算落版字高",
				},
				"caption_font_mm": map[string]any{"type": "number", "description": "图号字高（毫米，默认 4）"},
				"sheet_font_mm":   map[string]any{"type": "number", "description": "页码字高（毫米，默认 3）"},
				"fail_on_findings": map[string]any{
					"type":        "boolean",
					"description": "渲染复核命中阻断级发现时令工具失败（文件保留），默认 false",
				},
			},
			"required":             []string{"drawing", "output"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type submissionTool struct{ cfg ToolConfig }

type submissionArgs struct {
	Drawing        string  `json:"drawing"`
	Output         string  `json:"output"`
	Office         string  `json:"office"`
	FigureNumber   int     `json:"figure_number"`
	FigureCount    int     `json:"figure_count"`
	Caption        string  `json:"caption"`
	SheetIndex     int     `json:"sheet_index"`
	SheetTotal     int     `json:"sheet_total"`
	RotateDeg      int     `json:"rotate_deg"`
	BodyFontSize   float64 `json:"body_font_size"`
	CaptionFontMm  float64 `json:"caption_font_mm"`
	SheetFontMm    float64 `json:"sheet_font_mm"`
	FailOnFindings bool    `json:"fail_on_findings"`
}

// SubmissionToolResult 是落版工具的结果。
type SubmissionToolResult struct {
	File          string            `json:"file"`
	Office        string            `json:"office"`
	Caption       string            `json:"caption,omitempty"`
	SheetNumber   string            `json:"sheet_number,omitempty"`
	Metrics       SubmissionMetrics `json:"metrics"`
	Warnings      []string          `json:"warnings,omitempty"`
	CheckFindings []CheckFinding    `json:"check_findings,omitempty"`
}

func (t *submissionTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p submissionArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	office := Office(strings.ToLower(strings.TrimSpace(p.Office)))
	if office == "" {
		office = OfficeCNIPA
	}
	drawingPath, err := resolveWorkFile(t.cfg.WorkingDir, p.Drawing)
	if err != nil {
		return nil, err
	}
	drawing, err := os.ReadFile(drawingPath) //nolint:gosec // 路径已限定在工作目录内
	if err != nil {
		return nil, newError(ErrInvalidSVG, "读取图形失败：%v", err)
	}
	outputPath, format, err := resolveOutput(t.cfg.WorkingDir, p.Output, string(FormatSVG))
	if err != nil {
		return nil, err
	}
	if format != FormatSVG {
		return nil, newError(ErrInvalidOutput, "落版页只支持 svg 输出")
	}
	if err := ensureWithin(t.cfg.WorkingDir, filepath.Dir(outputPath)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	page, err := BuildSubmissionPage(SubmissionInput{
		DrawingSVG: string(drawing), Office: office,
		FigureNumber: p.FigureNumber, FigureCount: p.FigureCount, Caption: p.Caption,
		SheetIndex: p.SheetIndex, SheetTotal: p.SheetTotal,
		BodyFontSize: p.BodyFontSize, CaptionFontMm: p.CaptionFontMm, SheetFontMm: p.SheetFontMm,
		RotateDeg: p.RotateDeg,
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(outputPath, []byte(page.SVG), 0o644); err != nil { //nolint:gosec
		return nil, newError(ErrInvalidOutput, "写入落版页失败：%v", err)
	}

	report, err := CheckFigure(page.SVG, CheckOptions{})
	if err != nil {
		return nil, err
	}
	var warnings []string
	warnings = append(warnings, page.Warnings...)
	for _, finding := range report.Findings {
		warnings = append(warnings, "渲染复核："+finding.Message)
	}
	if p.FailOnFindings {
		if blocking := BlockingFindings(report); len(blocking) > 0 {
			messages := make([]string, len(blocking))
			for i, finding := range blocking {
				messages[i] = "- " + finding.Message
			}
			return nil, newError(ErrRenderFailed,
				"落版页渲染复核未通过（%d 项）：\n%s\n文件已写入 %s，但未作为交付件导出；修正后重跑。",
				len(blocking), strings.Join(messages, "\n"), outputPath)
		}
	}
	rel, relErr := filepath.Rel(t.cfg.WorkingDir, outputPath)
	if relErr != nil {
		rel = outputPath
	}
	return SubmissionToolResult{
		File: rel, Office: string(office), Caption: page.Caption, SheetNumber: page.SheetNumber,
		Metrics: page.Metrics, Warnings: warnings, CheckFindings: report.Findings,
	}, nil
}

// NewConsistencyTool 创建跨图一致性核验工具。
func NewConsistencyTool() *agentcore.Tool {
	t := &consistencyTool{}
	return &agentcore.Tool{
		Name:        ConsistencyToolName,
		Description: consistencyToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"figures": map[string]any{
					"type":        "array",
					"description": "各图的标记（图号 + 标记/名称）",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"number": map[string]any{"type": "integer", "minimum": 1},
							"refs": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"ref":  map[string]any{"type": "string"},
										"name": map[string]any{"type": "string"},
									},
									"required": []string{"ref"},
								},
							},
						},
						"required": []string{"number"},
					},
				},
				"text": map[string]any{
					"type":        "string",
					"description": "权利要求书或说明书正文，用于抽取附图标记引用",
				},
				"abstract_figure": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "指定作为摘要附图的图号",
				},
				"ref_limit": map[string]any{"type": "integer", "minimum": 1, "description": "文本抽取上限，默认 40"},
			},
			"required":             []string{"figures"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type consistencyTool struct{}

func (t *consistencyTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p SetConsistencyInput
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	return CheckFigureSet(p)
}

// resolveWorkFile 把输入文件路径限制在工作目录内（含符号链接复核）。
func resolveWorkFile(workDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", newError(ErrPathEscape, "缺少文件路径")
	}
	if workDir == "" {
		return "", newError(ErrPathEscape, "未配置工作目录，无法读取附图")
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("解析工作目录失败: %w", err)
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(absWork, path)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(absWork, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newError(ErrPathEscape, "文件路径必须位于工作目录 %s 内：%s", absWork, path)
	}
	if err := ensureWithin(absWork, filepath.Dir(resolved)); err != nil {
		return "", err
	}
	return resolved, nil
}

func officeNames() []string {
	names := make([]string, 0, len(AllOffices()))
	for _, office := range AllOffices() {
		names = append(names, string(office))
	}
	return names
}
