package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandIncludes_SingleLevel(t *testing.T) {
	dir := t.TempDir()

	// Create included file
	includeContent := "### Included Section\nThis content was included.\n"
	writeFile(t, filepath.Join(dir, "included.md"), includeContent)

	// Create main body with include tag
	body := "# Main Skill\n\nSome intro text.\n\n<include ref=\"included.md\" />\n\nSome outro text."
	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	if !strings.Contains(expanded, "### Included Section") {
		t.Errorf("expected included content, got:\n%s", expanded)
	}
	if !strings.Contains(expanded, "# Main Skill") {
		t.Errorf("expected main skill title, got:\n%s", expanded)
	}
	if !strings.Contains(expanded, "Some intro text.") {
		t.Errorf("expected intro text, got:\n%s", expanded)
	}
	if !strings.Contains(expanded, "Some outro text.") {
		t.Errorf("expected outro text, got:\n%s", expanded)
	}
}

func TestExpandIncludes_MultiLevel(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure: A includes B, B includes C
	writeFile(t, filepath.Join(dir, "c.md"), "Content from C.")
	writeFile(t, filepath.Join(dir, "b.md"), "Content from B.\n<include ref=\"c.md\" />")
	body := "# Main\n\n<include ref=\"b.md\" />"

	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	if !strings.Contains(expanded, "Content from B.") {
		t.Errorf("expected B content, got:\n%s", expanded)
	}
	if !strings.Contains(expanded, "Content from C.") {
		t.Errorf("expected C content, got:\n%s", expanded)
	}
}

func TestExpandIncludes_CircularReference(t *testing.T) {
	dir := t.TempDir()

	// A includes B, B includes A (circular)
	writeFile(t, filepath.Join(dir, "a.md"), "Content A.\n<include ref=\"b.md\" />")
	writeFile(t, filepath.Join(dir, "b.md"), "Content B.\n<include ref=\"a.md\" />")

	_, err := ExpandIncludes(dir, "Content A.\n<include ref=\"b.md\" />")
	if err == nil {
		t.Fatal("expected error for circular include, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error, got: %v", err)
	}
}

func TestExpandIncludes_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	body := "<include ref=\"../../../etc/passwd\" />"
	_, err := ExpandIncludes(dir, body)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected 'path traversal' in error, got: %v", err)
	}
}

func TestExpandIncludes_MissingFile(t *testing.T) {
	dir := t.TempDir()

	body := "<include ref=\"nonexistent.md\" />"
	_, err := ExpandIncludes(dir, body)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestExpandIncludes_DepthLimit(t *testing.T) {
	dir := t.TempDir()

	// Create 4 levels: A -> B -> C -> D (depth 3 is ok, depth 4 is not)
	writeFile(t, filepath.Join(dir, "d.md"), "Content D.")
	writeFile(t, filepath.Join(dir, "c.md"), "Content C.\n<include ref=\"d.md\" />")
	writeFile(t, filepath.Join(dir, "b.md"), "Content B.\n<include ref=\"c.md\" />")
	writeFile(t, filepath.Join(dir, "a.md"), "Content A.\n<include ref=\"b.md\" />")

	// Including from body at depth 0 -> A(depth1) -> B(depth2) -> C(depth3) -> D => should fail at D (depth 4)
	_, err := ExpandIncludes(dir, "<include ref=\"a.md\" />")
	if err == nil {
		t.Fatal("expected error for exceeding depth limit, got nil")
	}
	if !strings.Contains(err.Error(), "depth exceeded") {
		t.Errorf("expected 'depth exceeded' in error, got: %v", err)
	}
}

func TestExpandIncludes_WhitespaceFlexibility(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "foo.md"), "Hello world.")

	tests := []struct {
		name string
		body string
	}{
		{"spaces around ref", `<include ref="foo.md" />`},
		{"extra space before ref", `<include  ref="foo.md" />`},
		{"space around equals", `<include ref = "foo.md" />`},
		{"space before closing", `<include ref="foo.md"  />`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := ExpandIncludes(dir, tt.body)
			if err != nil {
				t.Fatalf("ExpandIncludes failed: %v", err)
			}
			if !strings.Contains(expanded, "Hello world.") {
				t.Errorf("expected included content, got: %s", expanded)
			}
		})
	}
}

func TestExpandIncludes_StripFrontmatter(t *testing.T) {
	dir := t.TempDir()

	// Included file has YAML frontmatter - body should be extracted
	writeFile(t, filepath.Join(dir, "with-fm.md"), `---
title: Some Title
description: Some Description
---
# Actual Content
This is the real body content.
`)

	body := "<include ref=\"with-fm.md\" />"
	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	if strings.Contains(expanded, "title:") || strings.Contains(expanded, "---") {
		t.Errorf("frontmatter should have been stripped, got:\n%s", expanded)
	}
	if !strings.Contains(expanded, "# Actual Content") {
		t.Errorf("expected body content, got:\n%s", expanded)
	}
}

func TestExpandIncludes_CacheReuse(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "shared.md"), "Shared content.")

	// Same file included twice
	body := "<include ref=\"shared.md\" />\n<include ref=\"shared.md\" />"
	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	// Should appear twice (cache only prevents re-reading, not re-insertion)
	count := strings.Count(expanded, "Shared content.")
	if count != 2 {
		t.Errorf("expected Shared content to appear 2 times, got %d", count)
	}
}

func TestExpandIncludes_NoTags(t *testing.T) {
	dir := t.TempDir()

	body := "# No includes here\nJust plain markdown."
	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	if expanded != body {
		t.Errorf("expected body unchanged, got:\n%s", expanded)
	}
}

func TestExpandIncludes_AbsolutePath(t *testing.T) {
	dir := t.TempDir()

	body := "<include ref=\"/etc/passwd\" />"
	_, err := ExpandIncludes(dir, body)
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("expected 'absolute' in error, got: %v", err)
	}
}

func TestExpandIncludes_EmptyRef(t *testing.T) {
	dir := t.TempDir()

	body := "<include ref=\"\" />"
	_, err := ExpandIncludes(dir, body)
	if err == nil {
		t.Fatal("expected error for empty ref, got nil")
	}
	if !strings.Contains(err.Error(), "empty ref") {
		t.Errorf("expected 'empty ref' in error, got: %v", err)
	}
}

func TestExpandIncludes_NestedDir(t *testing.T) {
	dir := t.TempDir()

	// Create subdirectory structure
	subDir := filepath.Join(dir, "references")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(subDir, "checklist.md"), "### Checklist\n- Item 1\n- Item 2")

	body := "# Main\n\n<include ref=\"references/checklist.md\" />"
	expanded, err := ExpandIncludes(dir, body)
	if err != nil {
		t.Fatalf("ExpandIncludes failed: %v", err)
	}

	if !strings.Contains(expanded, "### Checklist") {
		t.Errorf("expected checklist content, got:\n%s", expanded)
	}
}

func TestExpandIncludes_IntegrationWithSkills(t *testing.T) {
	// Test that skills with <include> tags load correctly
	dir := t.TempDir()

	// Create a reference file
	refDir := filepath.Join(dir, "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(refDir, "checklist.md"), "## 检查清单\n1. 确认权利要求格式\n2. 确认引用基础")

	// Create SKILL.md that uses include
	writeFile(t, filepath.Join(dir, "SKILL.md"), `---
name: test-skill
description: A test skill with includes
domain: patent
---

# 测试技能

## 参考文档
<include ref="references/checklist.md" />
`)

	skills, diags, err := LoadPath(dir)
	if err != nil {
		t.Fatalf("LoadPath failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	skill := skills[0]
	if !strings.Contains(skill.Body, "## 检查清单") {
		t.Errorf("skill body should contain included checklist, got:\n%s", skill.Body)
	}
	if !strings.Contains(skill.Body, "1. 确认权利要求格式") {
		t.Errorf("skill body should contain checklist items, got:\n%s", skill.Body)
	}

	// Should have no diagnostics for valid includes
	for _, d := range diags {
		if strings.Contains(d.Message, "include") {
			t.Errorf("unexpected include diagnostic: %s", d.Message)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
