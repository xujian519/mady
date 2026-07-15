package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "mady-wiki-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp wiki dir: %v\n", err)
		os.Exit(1)
	}
	testWikiPath = root
	setupTestData(root)
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}

// setupTestData creates the test wiki data under root. Tests in this package
// depend on the directory existing before they run.
func setupTestData(root string) {
	// card-index.json
	ensureDir(root)
	cardPath := filepath.Join(root, "Wiki", "专利侵权", "侵权判定", "侵权判定-全面覆盖原则.md")
	os.WriteFile(filepath.Join(root, "card-index.json"), []byte(fmt.Sprintf(`{
  "total_cards": 1,
  "cards": [
    {
      "id": "test-001",
      "title": "什么是全面覆盖原则？",
      "concept": "全面覆盖原则",
      "quality": 0.92,
      "domain": "侵权判定",
      "file_path": %q,
      "related_concepts": [],
      "generated_at": "2026-04-29T10:00:00Z",
      "version": 1
    }
  ]
}`, cardPath)), 0644)

	// cards/test-card.md
	ensureDir(filepath.Join(root, "cards"))
	os.WriteFile(filepath.Join(root, "cards", "test-card.md"), []byte(`# 测试卡片-全面覆盖原则

> **来源：** 专利侵权判定指南
> **核心法条：** 专利法第59条

## 核心要点

全面覆盖原则是专利侵权判定的基础性原则，也是专利侵权诉讼中最常用的判断方法。该原则要求将被控侵权技术方案与专利权利要求进行逐一比对。

## 详细分析

全面覆盖原则的核心在于技术特征的逐一比对，不允许将被控侵权技术方案作为整体与权利要求进行比对，而必须将权利要求分解为各个技术特征。

### 具体适用步骤

第一步，确定专利权的保护范围。根据专利法第五十九条第一款的规定，发明专利或者实用新型专利权的保护范围以其权利要求的内容为准，说明书及附图可以用于解释权利要求的内容。

第二步，分解技术特征。将权利人主张的权利要求所记载的全部技术特征逐一分解，确定每一项技术特征的具体含义和范围。

第三步，确定被控侵权技术方案的相应技术特征。根据被控侵权产品或者方法的具体情况，找出与权利要求中各项技术特征相对应的技术特征。

第四步，进行逐一比对。将被控侵权技术方案的技术特征与权利要求记载的全部技术特征进行逐一比对，判断是否构成相同或者等同。

第五步，得出结论。如果被控侵权技术方案包含与权利要求记载的全部技术特征相同或者等同的技术特征，则认定其落入专利权的保护范围，构成侵权。
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

全面覆盖原则要求被控侵权技术方案必须包含权利要求中记载的全部技术特征，才能认定落入专利权的保护范围。缺少任何一个技术特征，均不构成侵权。这是专利侵权判定中最基本也是最重要的原则。

## 第35条 全面覆盖原则

### 条文原文

> 全面覆盖原则是专利侵权判定的最基本规则。判断被控侵权技术方案是否落入专利权的保护范围，应当审查权利人主张的权利要求所记载的全部技术特征。

### 理解与适用

全面覆盖原则的法律依据来源于《专利法》第五十九条第一款关于专利权保护范围的规定。该条规定，发明或者实用新型专利权的保护范围以其权利要求的内容为准，说明书及附图可以用于解释权利要求的内容。

在具体适用中，应当将被控侵权技术方案的技术特征与权利要求中记载的全部技术特征进行逐一比对。如果被控侵权技术方案包含了权利要求中记载的全部技术特征，则落入专利权的保护范围。如果被控侵权技术方案的技术特征与权利要求记载的全部技术特征相比，缺少权利要求记载的一项或多项技术特征，则没有落入专利权的保护范围。

#### 审查要素

| 审查要素 | 说明 |
|----------|------|
| 技术特征分解 | 将权利要求分解为若干技术特征 |
| 逐一比对 | 将被控侵权技术方案与每项技术特征逐一比对 |
| 全部覆盖 | 全部技术特征均相同或等同时，构成侵权 |
| 缺少特征 | 缺少任何一项技术特征，均不构成侵权 |

需要注意的是，全面覆盖原则并不要求被控侵权技术方案的技术特征与权利要求中记载的技术特征在文字上完全一致。如果被控侵权技术方案的某个技术特征与权利要求中记载的相应技术特征相比，虽然表述不同，但属于相同的技术特征，仍然应当认为被控侵权技术方案包含了权利要求中记载的该项技术特征。
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
