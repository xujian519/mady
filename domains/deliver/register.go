package deliver

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RegisterInput 是一次交付登记。
type RegisterInput struct {
	// TemplateID 是渲染使用的模板；为空表示本次交付不来自模板。
	TemplateID string
	// Artifacts 是交付物的主路径与交付副本。
	Artifacts []Artifact
	// Figures 是本次交付的附图。
	Figures []string
	// Rendered 是渲染出的交付件（html/pdf），可为空。
	Rendered string
	// Requirements 是本次交付要交代的要求及证据。
	Requirements []Requirement
	// SelfCheck 是模型自述的质量门结论。
	SelfCheck string
}

// Check 是一项确定性核验的结论。
type Check struct {
	// Name 是核验项名称。
	Name string `json:"name"`
	// Passed 表示通过。
	Passed bool `json:"passed"`
	// Details 是未通过时的说明。
	Details string `json:"details,omitempty"`
}

// 交付结论三值：查不了不能算通过，因此「通过」之外还要区分「有项没查」。
const (
	// VerdictPass 表示全部应做的核验都已执行且通过。
	VerdictPass = "pass"
	// VerdictFail 表示存在未通过项。
	VerdictFail = "fail"
	// VerdictInconclusive 表示没有未通过项，但有核验未能执行。
	VerdictInconclusive = "inconclusive"
)

// Deliverable 是一次交付的登记结果。
//
// 结构性字段（清单、确定性核验）与自述字段（SelfCheck）分开保存：两列并列呈现，
// 读者能直接看到「文件是什么」与「声称做了什么」。
type Deliverable struct {
	// TemplateID 是模板标识。
	TemplateID string `json:"template_id,omitempty"`
	// RegisteredAt 是登记时间。
	RegisteredAt time.Time `json:"registered_at"`
	// Manifest 是文件清单（路径、摘要、大小、时间）。
	Manifest []ManifestEntry `json:"manifest"`
	// Checks 是确定性核验结论。
	Checks []Check `json:"checks"`
	// SelfCheck 是模型自述。
	SelfCheck string `json:"self_check,omitempty"`
	// Unverified 是本次未能执行的核验项（如 PDF 无法做 HTML 合规检查）。
	Unverified []string `json:"unverified,omitempty"`
	// Verdict 是本次交付的结论（pass / fail / inconclusive）。
	Verdict string `json:"verdict"`
	// Passed 表示全部应做的核验既已执行又已通过，即 Verdict == pass。
	Passed bool `json:"passed"`
	// Issues 是未通过项的汇总。
	Issues []string `json:"issues,omitempty"`
}

// Config 是登记配置。
type Config struct {
	// OnRegistered 在登记完成后调用，供装配层写会话日志或审计。
	//
	// 只在核验链跑完（无论通过与否）时调用；参数缺失导致的提前错误不触发。
	// 回调同步执行，不另开协程，可能因此阻塞 Register 返回。
	OnRegistered func(Deliverable)
}

// Register 对待交付的文件做确定性核验并形成登记。
func Register(input RegisterInput, cfg Config) (Deliverable, error) {
	deliverable := Deliverable{
		TemplateID:   input.TemplateID,
		RegisteredAt: time.Now().UTC(),
		SelfCheck:    input.SelfCheck,
	}

	if input.TemplateID != "" {
		templates, err := Templates()
		if err != nil {
			return Deliverable{}, fmt.Errorf("deliver: 读取模板清单: %w", err)
		}
		if !slices.Contains(templates, input.TemplateID) {
			deliverable.Checks = append(deliverable.Checks, Check{
				Name: "template-exists", Passed: false,
				Details: fmt.Sprintf("模板 %s 不在随包清单中", input.TemplateID),
			})
		} else {
			deliverable.Checks = append(deliverable.Checks, Check{Name: "template-exists", Passed: true})
		}
	}

	if len(input.Artifacts) == 0 && strings.TrimSpace(input.Rendered) == "" {
		// 空交付不能因为「没有文件可查」而被判通过：质量门对「什么都没交」必须报错。
		deliverable.Checks = append(deliverable.Checks, Check{
			Name: "non-empty-delivery", Passed: false,
			Details: "未登记任何交付物（artifacts 与 rendered 均为空）",
		})
	} else {
		verification, err := VerifyDeliverable(VerifyInput{
			Artifacts:    input.Artifacts,
			Figures:      input.Figures,
			Rendered:     input.Rendered,
			Requirements: input.Requirements,
		})
		if err != nil {
			return Deliverable{}, err
		}
		deliverable.Manifest = verification.Manifest
		deliverable.Checks = append(deliverable.Checks, Check{
			Name:    "deliverable-consistency",
			Passed:  verification.Passed,
			Details: strings.Join(verification.Issues, "；"),
		})

		if input.Rendered != "" {
			if strings.EqualFold(filepath.Ext(input.Rendered), ".html") {
				compliance, err := complianceForRender(input.TemplateID, input.Rendered)
				if err != nil {
					return Deliverable{}, err
				}
				deliverable.Checks = append(deliverable.Checks, compliance...)
			} else {
				// PDF 等非 HTML 交付件无法做 DOM 合规检查；记入未执行项，不冒充通过。
				deliverable.Unverified = append(deliverable.Unverified,
					"document-compliance：非 HTML 交付件无法做 HTML 合规检查")
			}
		}
	}

	for _, check := range deliverable.Checks {
		if !check.Passed {
			deliverable.Issues = append(deliverable.Issues, check.Name+"："+check.Details)
		}
	}
	switch {
	case len(deliverable.Issues) > 0:
		deliverable.Verdict = VerdictFail
	case len(deliverable.Unverified) > 0:
		deliverable.Verdict = VerdictInconclusive
	default:
		deliverable.Verdict = VerdictPass
	}
	deliverable.Passed = deliverable.Verdict == VerdictPass
	if cfg.OnRegistered != nil {
		cfg.OnRegistered(deliverable)
	}
	return deliverable, nil
}

// complianceForRender 对渲染出的 HTML 做合规检查，范围由模板性质决定。
func complianceForRender(templateID, path string) ([]Check, error) {
	//nolint:gosec // path 由调用方给出，是本次交付的渲染件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("deliver: 读取渲染件: %w", err)
	}
	scope := ScopeExternalFiling
	if templateID != "" && IsInternalRecord(templateID) {
		scope = ScopeInternalRecord
	}
	issues, err := CheckCompliance(string(data), scope)
	if err != nil {
		return nil, err
	}
	details := make([]string, 0, len(issues))
	for _, issue := range issues {
		details = append(details, issue.Rule+"："+issue.Message)
	}
	return []Check{{
		Name:    "document-compliance",
		Passed:  len(issues) == 0,
		Details: strings.Join(details, "；"),
	}}, nil
}

// RenderAndRegister 渲染交付物、落盘，并完成登记与核验。
//
// 这是「渲染 → 落盘 → 核对 → 登记」的一条链：调用方只给模板与内容，不再分别
// 记得跑合规与一致性检查。
func RenderAndRegister(
	templateID string,
	slots map[string]string,
	outputPath string,
	renderOptions RenderOptions,
	input RegisterInput,
	cfg Config,
) (Deliverable, error) {
	if strings.TrimSpace(outputPath) == "" {
		return Deliverable{}, fmt.Errorf("deliver: 未指定渲染件输出路径")
	}
	html, err := Render(templateID, slots, renderOptions)
	if err != nil {
		return Deliverable{}, err
	}
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return Deliverable{}, fmt.Errorf("deliver: 创建输出目录: %w", err)
		}
	}
	//nolint:gosec // outputPath 由调用方给出，是本次交付的落盘位置
	if err := os.WriteFile(outputPath, []byte(html), 0o600); err != nil {
		return Deliverable{}, fmt.Errorf("deliver: 写入渲染件: %w", err)
	}

	input.TemplateID = templateID
	input.Rendered = outputPath
	return Register(input, cfg)
}
