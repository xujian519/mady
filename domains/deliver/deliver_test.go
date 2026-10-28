package deliver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTemplatesLoaded(t *testing.T) {
	ids, err := Templates()
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	if len(ids) != 13 {
		t.Fatalf("模板数 = %d, want 13（%v）", len(ids), ids)
	}
	if !IsInternalRecord("invalidation-analysis") {
		t.Fatal("invalidation-analysis 应为内部记录模板")
	}
	if IsInternalRecord("claims-spec") {
		t.Fatal("claims-spec 不应是内部记录模板")
	}
}

func TestRenderInjectsSlotsBrandAndNumbering(t *testing.T) {
	htmlOut, err := Render("claims-spec", map[string]string{
		"specification-body": `<section data-paragraph-numbering="[0001]"><p>第一段。</p><p>第二段。</p></section>`,
	}, RenderOptions{Brand: Brand{Firm: "测试事务所", Accent: "#123456"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(htmlOut, "第一段") || !strings.Contains(htmlOut, "第二段") {
		t.Fatal("注入内容缺失")
	}
	if !strings.Contains(htmlOut, "测试事务所") || !strings.Contains(htmlOut, "#123456") {
		t.Fatal("品牌变量未注入")
	}
	if !strings.Contains(htmlOut, "[0001] 第一段") || !strings.Contains(htmlOut, "[0002] 第二段") {
		t.Fatalf("段落编号未写入：%s", snippet(htmlOut))
	}
}

func TestRenderRejectsUnknownSlot(t *testing.T) {
	if _, err := Render("claims-spec", map[string]string{"nonexistent": "<p>x</p>"}, RenderOptions{}); err == nil {
		t.Fatal("未知槽位应报错")
	}
}

func TestRenderRejectsUnknownTemplate(t *testing.T) {
	if _, err := Render("no-such-template", nil, RenderOptions{}); err == nil {
		t.Fatal("未知模板应报错")
	}
}

func TestComplianceSectionNumbering(t *testing.T) {
	htmlDoc := `<html><body>
<h2>一、总体立场</h2><p>a</p>
<h2>三、证据</h2><p>b</p>
<h2>二、理由</h2><p>c</p>
</body></html>`
	issues, err := CheckCompliance(htmlDoc, ScopeExternalFiling)
	if err != nil {
		t.Fatalf("CheckCompliance: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("章节编号乱序应报出")
	}
	if !strings.Contains(issues[0].Message, "递增") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestComplianceInternalRecords(t *testing.T) {
	htmlDoc := `<html><body><h2>一、总体立场</h2><h3>待办事项</h3><p>成功率为 60%。</p></body></html>`
	issues, _ := CheckCompliance(htmlDoc, ScopeExternalFiling)
	rules := map[string]bool{}
	for _, issue := range issues {
		rules[issue.Rule] = true
	}
	if !rules["internal-section"] || !rules["internal-body"] {
		t.Fatalf("应同时报出内部标题与内部正文：%+v", issues)
	}

	// 内部记录件不查内部工作记录，只查编号与证据。
	internalIssues, _ := CheckCompliance(htmlDoc, ScopeInternalRecord)
	for _, issue := range internalIssues {
		if issue.Rule == "internal-section" || issue.Rule == "internal-body" {
			t.Fatalf("内部记录件不应报内部记录规则：%+v", internalIssues)
		}
	}
}

func TestComplianceEvidenceCrossReference(t *testing.T) {
	htmlDoc := `<html><body><p>如上所述，证据1 与证据2 支持该主张。</p>
<table><thead><tr><th>编号</th><th>标识</th><th>证据名称</th></tr></thead>
<tbody><tr><td>证据1</td><td>CN1</td><td>对比文件</td></tr>
<tr><td>证据3</td><td>CN2</td><td>教科书</td></tr></tbody></table></body></html>`
	issues, err := CheckCompliance(htmlDoc, ScopeExternalFiling)
	if err != nil {
		t.Fatalf("CheckCompliance: %v", err)
	}
	var unregistered, uncited bool
	for _, issue := range issues {
		if strings.Contains(issue.Message, "未登记在证据清单") {
			unregistered = true
		}
		if strings.Contains(issue.Message, "未被引用") {
			uncited = true
		}
	}
	if !uncited || !unregistered {
		t.Fatalf("应同时报出未引用与未登记：%+v", issues)
	}
}

func TestVerifyDeliverablePasses(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "main.docx")
	delivered := filepath.Join(dir, "05-交付件.docx")
	writeFile(t, canonical, "v1")
	writeFile(t, delivered, "v1")
	rendered := filepath.Join(dir, "out.pdf")
	writeFile(t, rendered, "pdf")
	future(t, rendered, canonical)

	result, err := VerifyDeliverable(VerifyInput{
		Artifacts: []Artifact{{Role: "权利要求书", CanonicalPath: canonical, DeliveredPath: delivered}},
		Rendered:  rendered,
		Requirements: []Requirement{
			{Requirement: "按 CNIPA 体例成文", Evidence: []string{canonical}},
		},
	})
	if err != nil {
		t.Fatalf("VerifyDeliverable: %v", err)
	}
	if !result.Passed {
		t.Fatalf("应通过：%+v", result.Issues)
	}
	if len(result.Manifest) < 3 {
		t.Fatalf("清单条目 = %d", len(result.Manifest))
	}
}

func TestVerifyDeliverableCatchesStaleCopy(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "main.docx")
	delivered := filepath.Join(dir, "delivered.docx")
	writeFile(t, canonical, "v2")
	writeFile(t, delivered, "v1")

	result, _ := VerifyDeliverable(VerifyInput{
		Artifacts: []Artifact{{Role: "权利要求书", CanonicalPath: canonical, DeliveredPath: delivered}},
	})
	if result.Passed || !containsIssue(result.Issues, "内容不一致") {
		t.Fatalf("应报内容不一致：%+v", result.Issues)
	}
}

func TestVerifyDeliverableCatchesEarlyRendered(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "main.docx")
	rendered := filepath.Join(dir, "out.pdf")
	writeFile(t, canonical, "v1")
	writeFile(t, rendered, "pdf")
	past(t, rendered, canonical)

	result, _ := VerifyDeliverable(VerifyInput{
		Artifacts: []Artifact{{Role: "权利要求书", CanonicalPath: canonical, DeliveredPath: canonical}},
		Rendered:  rendered,
	})
	if result.Passed || !containsIssue(result.Issues, "早于其输入") {
		t.Fatalf("应报出件早于改稿：%+v", result.Issues)
	}
}

func TestVerifyDeliverableCatchesMissingEvidence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.docx")
	writeFile(t, file, "v1")

	result, _ := VerifyDeliverable(VerifyInput{
		Artifacts:    []Artifact{{Role: "a", CanonicalPath: file, DeliveredPath: file}},
		Requirements: []Requirement{{Requirement: "未记录证据的要求"}},
	})
	if result.Passed || !containsIssue(result.Issues, "没有记录证据") {
		t.Fatalf("应报缺少证据：%+v", result.Issues)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入 %s: %v", path, err)
	}
}

// future 让 path 的修改时间晚于 reference。
func future(t *testing.T, path, reference string) {
	t.Helper()
	info, err := os.Stat(reference)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	when := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// past 让 path 的修改时间早于 reference。
func past(t *testing.T, path, reference string) {
	t.Helper()
	info, err := os.Stat(reference)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	when := info.ModTime().Add(-2 * time.Second)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func containsIssue(issues []string, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, fragment) {
			return true
		}
	}
	return false
}

func snippet(text string) string {
	if len(text) > 200 {
		return text[:200]
	}
	return text
}
