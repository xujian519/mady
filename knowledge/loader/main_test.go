package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	setupTestData()
	code := m.Run()
	os.Exit(code)
}

// setupTestData creates the test wiki data at /tmp/wiki_test if it does not
// already exist. Tests in this package depend on this directory.
func setupTestData() {
	root := testWikiPath // "/tmp/wiki_test"

	// Already set up — nothing to do.
	if _, err := os.Stat(filepath.Join(root, "card-index.json")); err == nil {
		return
	}

	// card-index.json
	ensureDir(root)
	os.WriteFile(filepath.Join(root, "card-index.json"), []byte(`{
  "total_cards": 1,
  "cards": [
    {
      "id": "test-001",
      "title": "什么是全面覆盖原则？",
      "concept": "全面覆盖原则",
      "quality": 0.92,
      "domain": "侵权判定",
      "file_path": "/tmp/wiki_test/Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则.md",
      "related_concepts": [],
      "generated_at": "2026-04-29T10:00:00Z",
      "version": 1
    }
  ]
}`), 0644)

	// cards/test-card.md
	ensureDir(filepath.Join(root, "cards"))
	os.WriteFile(filepath.Join(root, "cards", "test-card.md"), []byte(`# 测试卡片-全面覆盖原则

> **来源：** 专利侵权判定指南
> **核心法条：** 专利法第59条

## 核心要点

全面覆盖原则是专利侵权判定的基础性原则。
`), 0644)

	// Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则.md
	ensureDir(filepath.Join(root, "Wiki", "专利侵权", "侵权判定"))
	os.WriteFile(
		filepath.Join(root, "Wiki", "专利侵权", "侵权判定", "侵权判定-全面覆盖原则.md"),
		[]byte(`# 侵权判定-全面覆盖原则

> **来源：** 《侵权判定指南(2017)理解与适用》第二章，第35条
> **核心法条：** 《专利法》第五十九条第一款；《侵犯专利权司法解释（一）》第七条
> **关联页面：** [[权利保护范围-内部证据与外部证据]]、[[侵权判定-等同侵权的限制]]

## 核心要点

全面覆盖原则要求被控侵权技术方案必须包含权利要求中记载的全部技术特征。
`), 0644)

	// Wiki/专利侵权/index.md — filtered by ShouldImport
	os.WriteFile(filepath.Join(root, "Wiki", "专利侵权", "index.md"), []byte(`# 专利侵权索引`), 0644)

	// Wiki/专利侵权/log.md — filtered by ShouldImport
	os.WriteFile(filepath.Join(root, "Wiki", "专利侵权", "log.md"), []byte(`# Changelog`), 0644)

	// Wiki/专利侵权/CLAUDE.md — filtered by ShouldImport
	os.WriteFile(filepath.Join(root, "Wiki", "专利侵权", "CLAUDE.md"), []byte(`# CLAUDE`), 0644)
}

func ensureDir(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		panic("failed to create test directory: " + path + ": " + err.Error())
	}
}
