package deliver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterPasses(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "claims.docx")
	writeFile(t, file, "v1")

	registered := 0
	result, err := Register(RegisterInput{
		TemplateID: "claims-spec",
		Artifacts:  []Artifact{{Role: "权利要求书", CanonicalPath: file, DeliveredPath: file}},
		Requirements: []Requirement{
			{Requirement: "按 CNIPA 体例成文", Evidence: []string{file}},
		},
		SelfCheck: "已按体例核对",
	}, Config{OnRegistered: func(Deliverable) { registered++ }})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !result.Passed || result.Verdict != VerdictPass {
		t.Fatalf("应通过：%+v / %s", result.Issues, result.Verdict)
	}
	if result.SelfCheck != "已按体例核对" {
		t.Fatalf("自述未保留：%q", result.SelfCheck)
	}
	if registered != 1 {
		t.Fatalf("OnRegistered 调用 %d 次", registered)
	}
	checks := map[string]bool{}
	for _, check := range result.Checks {
		checks[check.Name] = check.Passed
	}
	if !checks["template-exists"] || !checks["deliverable-consistency"] {
		t.Fatalf("核验项缺失：%+v", result.Checks)
	}
}

func TestRegisterCatchesStaleCopyAndMissingEvidence(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "main.docx")
	delivered := filepath.Join(dir, "delivered.docx")
	writeFile(t, canonical, "v2")
	writeFile(t, delivered, "v1")

	result, err := Register(RegisterInput{
		Artifacts:    []Artifact{{Role: "权利要求书", CanonicalPath: canonical, DeliveredPath: delivered}},
		Requirements: []Requirement{{Requirement: "未记录证据"}},
	}, Config{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.Passed {
		t.Fatal("应判失败")
	}
	joined := strings.Join(result.Issues, "；")
	if !strings.Contains(joined, "内容不一致") || !strings.Contains(joined, "没有记录证据") {
		t.Fatalf("issues = %v", result.Issues)
	}
}

func TestRegisterRejectsUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.docx")
	writeFile(t, file, "v1")

	result, err := Register(RegisterInput{
		TemplateID: "no-such-template",
		Artifacts:  []Artifact{{Role: "a", CanonicalPath: file, DeliveredPath: file}},
	}, Config{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.Passed || !strings.Contains(strings.Join(result.Issues, "；"), "不在随包清单中") {
		t.Fatalf("未知模板应判失败：%+v", result)
	}
}

// 交付链：渲染 → 落盘 → 合规 + 一致性 → 登记。
func TestRenderAndRegisterChain(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.html")

	clean, err := RenderAndRegister("claims-spec", map[string]string{
		"specification-body": `<section data-paragraph-numbering="[0001]"><p>正文一。</p></section>`,
	}, output, RenderOptions{}, RegisterInput{
		Artifacts: []Artifact{{Role: "交付件", CanonicalPath: output, DeliveredPath: output}},
	}, Config{})
	if err != nil {
		t.Fatalf("RenderAndRegister: %v", err)
	}
	if !clean.Passed {
		t.Fatalf("干净交付应通过：%+v", clean.Issues)
	}

	dirtyOutput := filepath.Join(dir, "dirty.html")
	dirty, err := RenderAndRegister("claims-spec", map[string]string{
		"specification-body": `<h3>待办事项</h3><p>正文。</p>`,
	}, dirtyOutput, RenderOptions{}, RegisterInput{
		Artifacts: []Artifact{{Role: "交付件", CanonicalPath: dirtyOutput, DeliveredPath: dirtyOutput}},
	}, Config{})
	if err != nil {
		t.Fatalf("RenderAndRegister: %v", err)
	}
	if dirty.Passed {
		t.Fatal("含内部章节的对外件应判失败")
	}
	if !strings.Contains(strings.Join(dirty.Issues, "；"), "document-compliance") {
		t.Fatalf("应报合规失败：%+v", dirty.Issues)
	}
}

// 空交付（无 artifacts、无 rendered）必须判失败，不能因为"没有文件可查"而放行。
func TestRegisterRejectsEmptyDelivery(t *testing.T) {
	result, err := Register(RegisterInput{SelfCheck: "我什么都没交"}, Config{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.Passed || result.Verdict != VerdictFail {
		t.Fatalf("空交付不应通过：%+v / %s", result.Issues, result.Verdict)
	}
	if !strings.Contains(strings.Join(result.Issues, "；"), "未登记任何交付物") {
		t.Fatalf("issues = %v", result.Issues)
	}
}

// PDF 交付件无法做 HTML 合规检查：记入未执行项，结论判为 inconclusive，不冒充通过。
func TestRegisterMarksPdfUnverified(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "out.pdf")
	writeFile(t, pdf, "pdf")

	result, err := Register(RegisterInput{
		TemplateID: "claims-spec",
		Artifacts:  []Artifact{{Role: "交付件", CanonicalPath: pdf, DeliveredPath: pdf}},
		Rendered:   pdf,
	}, Config{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.Passed || result.Verdict != VerdictInconclusive {
		t.Fatalf("合规检查未执行不能算通过：%+v / %s", result.Issues, result.Verdict)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("未执行项不是未通过项：%+v", result.Issues)
	}
	if len(result.Unverified) == 0 || !strings.Contains(result.Unverified[0], "document-compliance") {
		t.Fatalf("PDF 应记入未执行项：%+v", result.Unverified)
	}
}

// 只给渲染件（不给 artifacts）是合法交付：应走核验与合规检查并通过。
func TestRegisterRenderedOnly(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "only.html")
	html, err := Render("claims-spec", map[string]string{
		"specification-body": `<section data-paragraph-numbering="[0001]"><p>正文一。</p></section>`,
	}, RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	writeFile(t, output, html)

	result, err := Register(RegisterInput{TemplateID: "claims-spec", Rendered: output}, Config{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !result.Passed || result.Verdict != VerdictPass {
		t.Fatalf("仅渲染件应通过：%+v / %+v / %s", result.Issues, result.Unverified, result.Verdict)
	}
	checks := map[string]bool{}
	for _, check := range result.Checks {
		checks[check.Name] = check.Passed
	}
	if !checks["document-compliance"] {
		t.Fatalf("HTML 渲染件应做合规检查：%+v", result.Checks)
	}
}

func TestRegisterTool(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.docx")
	writeFile(t, file, "v1")

	tool := NewTool(Config{})
	args, err := json.Marshal(map[string]any{
		"template_id": "claims-spec",
		"artifacts": []map[string]string{{
			"role": "a", "canonical_path": file, "delivered_path": file,
		}},
		"self_check": "自述通过",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	result, ok := out.(Deliverable)
	if !ok {
		t.Fatalf("输出类型 = %T", out)
	}
	if !result.Passed || result.SelfCheck != "自述通过" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Manifest) == 0 {
		t.Fatal("清单为空")
	}
}

func TestRegisterToolRejectsBadJSON(t *testing.T) {
	tool := NewTool(Config{})
	if _, err := tool.Func(context.Background(), json.RawMessage(`{`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
