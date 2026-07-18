package doctmpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDocTemplates(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "claims"), 0o755)
	os.WriteFile(filepath.Join(root, "claims", "method.md"), []byte(`---
name: method-claim
title: 方法权利要求
category: claims
description: 方法类权利要求模板
domain: patent
---
# {{title}}

一种{{method_name}}方法，其特征在于，包括以下步骤：
步骤1：{{step_1}}；
步骤2：{{step_2}}。
`), 0o644)

	os.MkdirAll(filepath.Join(root, "spec"), 0o755)
	os.WriteFile(filepath.Join(root, "spec", "mechanical.md"), []byte(`---
name: mechanical-spec
title: 机械领域说明书
category: specification
description: 机械领域说明书模板
domain: patent
---
# 说明书

## 技术领域
本发明涉及{{technical_field}}。

## 背景技术
{{background}}
`), 0o644)

	templates, err := LoadDocTemplates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2, got %d", len(templates))
	}
}

func TestLoadDocTemplatesFromFS_Embedded(t *testing.T) {
	// Verify that the embedded templates load correctly via LoadDocTemplatesFromFS.
	templates, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		t.Fatalf("LoadDocTemplatesFromFS embedded: %v", err)
	}
	if len(templates) < 8 {
		t.Fatalf("expected at least 8 embedded templates, got %d", len(templates))
	}
	// Verify all categories are present.
	cats := map[string]bool{}
	for _, tmpl := range templates {
		cats[tmpl.Category] = true
	}
	expected := []string{"claims", "specification", "oa-response", "disclosure"}
	for _, cat := range expected {
		if !cats[cat] {
			t.Errorf("missing category: %s", cat)
		}
	}
}

func TestResolveDoc(t *testing.T) {
	tmpl := DocTemplate{
		Name: "test",
		Body: "# {{title}}\n\n发明名称：{{invention}}\n技术效果：{{effect}}",
	}
	result := ResolveDoc(tmpl, map[string]string{
		"title":     "测试专利",
		"invention": "一种智能装置",
		"effect":    "提高效率",
	})
	if !strings.Contains(result, "测试专利") {
		t.Fatal("missing title")
	}
	if !strings.Contains(result, "智能装置") {
		t.Fatal("missing invention")
	}
	if strings.Contains(result, "{{") {
		t.Fatal("unresolved variable")
	}
}

func TestFindDocByCategory(t *testing.T) {
	templates := []DocTemplate{
		{Name: "a", Category: "claims"},
		{Name: "b", Category: "claims"},
		{Name: "c", Category: "specification"},
	}
	claims := FindDocByCategory(templates, "claims")
	if len(claims) != 2 {
		t.Fatalf("len = %d", len(claims))
	}
	none := FindDocByCategory(templates, "nonexistent")
	if len(none) != 0 {
		t.Fatal("expected empty")
	}
}

func TestDocIndex(t *testing.T) {
	templates := []DocTemplate{
		{Name: "method", Category: "claims", Description: "方法权利要求"},
		{Name: "mech", Category: "specification", Description: "机械说明书"},
	}
	idx := DocIndex(templates)
	if !strings.Contains(idx, "method") || !strings.Contains(idx, "mech") {
		t.Fatalf("index = %s", idx)
	}
	if DocIndex(nil) != "" {
		t.Fatal("expected empty")
	}
}
