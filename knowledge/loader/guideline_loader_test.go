package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge"
)

func TestLoadGuidelineDir(t *testing.T) {
	// 创建临时目录结构模拟 Obsidian wiki 审查指南目录。
	tmpDir := t.TempDir()

	// 第一部分-初步审查/第一章-发明专利申请初步审查/
	part1Dir := filepath.Join(tmpDir, "第一部分-初步审查")
	ch1Dir := filepath.Join(part1Dir, "第一章-发明专利申请初步审查")
	if err := os.MkdirAll(ch1Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// index.md（应跳过）
	os.WriteFile(filepath.Join(ch1Dir, "index.md"), []byte("# 第一章 发明专利申请初步审查"), 0644)
	// 正常内容文件
	os.WriteFile(filepath.Join(ch1Dir, "审查-初步审查-申请文件.md"), []byte(`# 申请文件的形式审查

申请文件应当包括发明专利请求书、说明书（有附图的应当包括附图）、权利要求书、摘要及其摘要附图。
根据专利法第26条第1款的规定。
`), 0644)

	// 第二部分-实质审查/第四章-创造性/
	part2Dir := filepath.Join(tmpDir, "第二部分-实质审查")
	ch4Dir := filepath.Join(part2Dir, "第四章-创造性")
	if err := os.MkdirAll(ch4Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(ch4Dir, "审查-创造性-最接近现有技术.md"), []byte(`# 确定最接近的现有技术

确定最接近的现有技术是三步法的第一步。
最接近的现有技术是指与要求保护的发明最密切相关的一个现有技术方案。
根据专利法第22条第3款的规定。
`), 0644)
	os.WriteFile(filepath.Join(ch4Dir, "审查-创造性-区别特征与技术问题.md"), []byte(`# 区别特征和实际解决的技术问题

在确定最接近的现有技术之后，应当确定区别特征。
发明实际解决的技术问题应当基于区别特征客观确定。
`), 0644)

	// 第二部分-实质审查/第三章-新颖性/
	ch3Dir := filepath.Join(part2Dir, "第三章-新颖性")
	if err := os.MkdirAll(ch3Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(ch3Dir, "审查-新颖性-单独对比.md"), []byte(`# 单独对比原则

根据专利法第22条第2款的规定，新颖性判断应当遵循单独对比原则。
一项权利要求只能与一份现有技术文件进行单独对比。
`), 0644)

	store := knowledge.NewStore()
	stats, err := LoadGuidelineDir(store, tmpDir)
	if err != nil {
		t.Fatalf("LoadGuidelineDir() error = %v", err)
	}

	if stats.Parts != 2 {
		t.Errorf("Parts = %d, want 2", stats.Parts)
	}
	if stats.Chapters != 3 {
		t.Errorf("Chapters = %d, want 3", stats.Chapters)
	}
	if stats.Sections < 3 {
		t.Errorf("Sections = %d, want >= 3", stats.Sections)
	}

	// 验证文档在 store 中且元数据正确。
	tests := []struct {
		docID    string
		wantType string
		wantLaw  string
	}{
		{
			docID:    "guideline_2023/第二部分-实质审查/第四章-创造性/最接近现有技术",
			wantType: "guideline_rule",
			wantLaw:  "专利法第22条第3款",
		},
		{
			docID:    "guideline_2023/第二部分-实质审查/第三章-新颖性/单独对比",
			wantType: "guideline_rule",
			wantLaw:  "专利法第22条第2款",
		},
		{
			docID:    "guideline_2023/第一部分-初步审查/第一章-发明专利申请初步审查/申请文件",
			wantType: "guideline_rule",
			wantLaw:  "专利法第26条第1款",
		},
	}
	for _, tt := range tests {
		doc, ok := store.GetDocument(tt.docID)
		if !ok {
			t.Errorf("document %s not found in store", tt.docID)
			continue
		}
		if doc.Metadata == nil {
			t.Errorf("document %s: metadata is nil", tt.docID)
			continue
		}
		if doc.Metadata["type"] != tt.wantType {
			t.Errorf("document %s: Metadata[type] = %q, want %q", tt.docID, doc.Metadata["type"], tt.wantType)
		}
		if doc.Metadata["law_refs"] != tt.wantLaw {
			t.Errorf("document %s: Metadata[laws] = %q, want %q", tt.docID, doc.Metadata["law_refs"], tt.wantLaw)
		}
		if !doc.Searchable {
			t.Errorf("document %s: Searchable = false, want true", tt.docID)
		}
	}
}

func TestLoadGuidelineDir_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore()
	_, err := LoadGuidelineDir(store, tmpDir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestLoadGuidelineDir_NilStore(t *testing.T) {
	_, err := LoadGuidelineDir(nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil store, got nil")
	}
}

func TestSectionNameFromFile(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"审查-创造性-最接近现有技术.md", "最接近现有技术"},
		{"审查-创造性-区别特征与技术问题.md", "区别特征与技术问题"},
		{"审查-新颖性-单独对比.md", "单独对比"},
		{"审查-初步审查-申请文件.md", "申请文件"},
		{"审查-创造性-最接近现有技术-拆分-01-审查标准.md", "最接近现有技术"},
		{"index.md", "index"},
	}
	for _, tt := range tests {
		got := sectionNameFromFile(tt.filename)
		if got != tt.want {
			t.Errorf("sectionNameFromFile(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		content  string
		fallback string
		want     string
	}{
		{"# 确定最接近的现有技术\n\n内容", "fallback", "确定最接近的现有技术"},
		{"## 次级标题\n\n内容", "fallback", "次级标题"},
		{"无标题内容", "文件推导名", "文件推导名"},
	}
	for _, tt := range tests {
		got := extractTitle(tt.content, tt.fallback)
		if got != tt.want {
			t.Errorf("extractTitle(%q, %q) = %q, want %q", tt.content[:min(len(tt.content), 20)], tt.fallback, got, tt.want)
		}
	}
}

func TestExtractLawRefs(t *testing.T) {
	tests := []struct {
		content string
		wantLen int
	}{
		{"根据专利法第22条第3款的规定", 1},
		{"专利法第22条第2款和专利法第22条第3款", 2},
		{"本领域技术人员能够实现", 0},
	}
	for _, tt := range tests {
		got := extractLawRefs(tt.content)
		if len(got) != tt.wantLen {
			t.Errorf("extractLawRefs(%q) returned %d refs, want %d", tt.content, len(got), tt.wantLen)
		}
	}
}
