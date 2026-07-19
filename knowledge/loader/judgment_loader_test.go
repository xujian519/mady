package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge"
)

func TestLoadJudgmentDir(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建复审无效/ 目录结构。
	reexamDir := filepath.Join(tmpDir, "复审无效", "创造性")
	if err := os.MkdirAll(reexamDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(reexamDir, "index.md"), []byte("# index"), 0644)
	os.WriteFile(filepath.Join(reexamDir, "审查标准-医药.md"), []byte(`# 创造性审查标准 -- 医药领域

> **标签：** 主题=创造性；子主题=审查标准；知识点=医药
> **覆盖决定数：** 11 件

## 核心要点

医药领域的创造性审查需要特别关注。

## 决定要点

### 要点1：区别技术特征未被公开，具备创造性

*引用：* "区别技术特征未被公开"（第566088号决定）
`), 0644)

	// 创建专利侵权/ 目录结构。
	infringeDir := filepath.Join(tmpDir, "专利侵权", "侵权判定")
	if err := os.MkdirAll(infringeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(infringeDir, "等同侵权.md"), []byte(`# 侵权判定-等同侵权

> **来源：** 侵权判定指南
> **核心法条：** 《专利法》第59条第1款

## 核心要点

等同侵权是指被诉侵权技术方案有一个或一个以上技术特征与权利要求中的相应技术特征从字面上看不相同，但属于等同特征。
`), 0644)

	store := knowledge.NewStore()
	stats, err := LoadJudgmentDir(store, tmpDir)
	if err != nil {
		t.Fatalf("LoadJudgmentDir() error = %v", err)
	}

	if stats.Imported < 2 {
		t.Errorf("Imported = %d, want >= 2", stats.Imported)
	}
	if stats.Skipped < 1 {
		t.Errorf("Skipped = %d, want >= 1 (index.md)", stats.Skipped)
	}

	// 验证文档存在于 store 中。
	docIDs := store.AllDocIDs()
	if len(docIDs) == 0 {
		t.Fatal("no documents in store")
	}
	for _, id := range docIDs {
		doc, ok := store.GetDocument(id)
		if !ok {
			t.Errorf("document %s not found", id)
			continue
		}
		if doc.Metadata == nil || doc.Metadata["type"] == "" {
			t.Errorf("document %s: missing type metadata", id)
		}
		t.Logf("  %s → type=%s domain=%s", id, doc.Metadata["type"], doc.Metadata["domain"])
	}
}

func TestInferDocTypeFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"复审无效/创造性/医药.md", "reexam"},
		{"专利判决/侵权判定/案例.md", "judgment"},
		{"专利侵权/侵权判定/等同侵权.md", "judgment"},
		{"侵权判定/等同侵权.md", "judgment"},
		{"其他/概念.md", "case"},
	}
	for _, tt := range tests {
		got := inferDocTypeFromPath(tt.path)
		if got != tt.want {
			t.Errorf("inferDocTypeFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
