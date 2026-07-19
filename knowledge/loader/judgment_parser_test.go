package loader

import (
	"testing"
)

const sampleJudgmentDoc = `# 创造性审查标准 -- 医药领域

> **标签：** 主题=创造性；子主题=审查标准；知识点=医药
> **来源：** 39,496份复审/无效决定元数据深度分析
> **法律依据：** 专利法第22条第3款
> **技术领域：** 医药
> **覆盖决定数：** 11 件
> **时间跨度：** 2013-2024

---

## 核心要点

医药领域的创造性审查在适用"三步法"基本框架的基础上，需要特别关注医药技术的特殊性质。
根据专利法第22条第3款的规定，创造性判断应当基于现有技术整体进行。

---

## 决定要点

### 要点1：区别技术特征未被公开且非公知常识，具备创造性

当权利要求的技术方案与最接近现有技术存在区别技术特征，且该区别技术特征既未被其他对比文件公开，也不属于本领域公知常识时，应当认定该权利要求具备创造性。

*引用：* "如果一项权利要求请求保护的技术方案与最接近的对比文件所公开的技术内容相比，存在某些区别技术特征"（第566088号决定）

### 要点2：现有技术给出结合启示则不具备创造性

如果区别技术特征被其他对比文件公开且作用相同，或者属于本领域公知常识，则不具备创造性。

*引用：* "如果一项权利要求要求保护的技术方案与最接近的对比文件公开的技术方案相比存在区别特征"（第580287号决定）
`

const sampleSimpleJudgment = `# 侵权判定-等同侵权

> **来源：** 《侵权判定指南(2017)理解与适用》第二章第三节
> **核心法条：** 《专利法》第59条第1款

等同侵权制度源自美国判例法。
`

func TestParseJudgmentDoc_Full(t *testing.T) {
	jd, err := ParseJudgmentDoc("reexam/inventiveness-pharma", sampleJudgmentDoc)
	if err != nil {
		t.Fatalf("ParseJudgmentDoc() error = %v", err)
	}

	if jd.Title != "创造性审查标准 -- 医药领域" {
		t.Errorf("Title = %q, want %q", jd.Title, "创造性审查标准 -- 医药领域")
	}
	if len(jd.Tags) < 3 {
		t.Errorf("got %d tags, want >= 3", len(jd.Tags))
	}
	if jd.Source == "" {
		t.Error("Source should not be empty")
	}
	if jd.DecisionCount != 11 {
		t.Errorf("DecisionCount = %d, want 11", jd.DecisionCount)
	}
	if jd.TimeSpan != "2013-2024" {
		t.Errorf("TimeSpan = %q, want %q", jd.TimeSpan, "2013-2024")
	}
	if jd.CoreSummary == "" {
		t.Error("CoreSummary should not be empty")
	}
	if len(jd.DecPoints) != 2 {
		t.Fatalf("got %d decision points, want 2", len(jd.DecPoints))
	}

	// 验证第1个决定要点。
	dp1 := jd.DecPoints[0]
	if dp1.Index != 1 {
		t.Errorf("DecPoints[0].Index = %d, want 1", dp1.Index)
	}
	if dp1.Title != "区别技术特征未被公开且非公知常识，具备创造性" {
		t.Errorf("DecPoints[0].Title = %q, want %q", dp1.Title, "区别技术特征未被公开且非公知常识，具备创造性")
	}
	if len(dp1.Citations) != 1 {
		t.Fatalf("DecPoints[0] has %d citations, want 1", len(dp1.Citations))
	}
	if dp1.Citations[0].DecNumber != 566088 {
		t.Errorf("Citation DecNumber = %d, want 566088", dp1.Citations[0].DecNumber)
	}

	// 验证法条引用。
	if len(jd.LawRefs) == 0 {
		t.Error("LawRefs should not be empty")
	}
}

func TestParseJudgmentDoc_Simple(t *testing.T) {
	jd, err := ParseJudgmentDoc("infringement/doctrine-equivalents", sampleSimpleJudgment)
	if err != nil {
		t.Fatalf("ParseJudgmentDoc() error = %v", err)
	}
	if jd.Title != "侵权判定-等同侵权" {
		t.Errorf("Title = %q, want %q", jd.Title, "侵权判定-等同侵权")
	}
	if len(jd.DecPoints) != 0 {
		t.Errorf("expected 0 decision points for simple doc, got %d", len(jd.DecPoints))
	}
}

func TestExtractMetadataTags(t *testing.T) {
	content := "> **标签：** 主题=创造性；子主题=审查标准；知识点=医药"
	tags := extractMetadataTags(content)
	if len(tags) != 3 {
		t.Fatalf("got %d tags, want 3: %v", len(tags), tags)
	}
	if tags[0] != "创造性" {
		t.Errorf("tags[0] = %q, want %q", tags[0], "创造性")
	}
}

func TestExtractDecNumbers(t *testing.T) {
	content := "参见第566088号决定和第580287号决定以及第566088号决定（重复）"
	nums := ExtractDecNumbers(content)
	if len(nums) != 2 {
		t.Fatalf("got %d unique numbers, want 2: %v", len(nums), nums)
	}
	if nums[0] != 566088 {
		t.Errorf("nums[0] = %d, want 566088", nums[0])
	}
}

func TestExtractCaseNumbers(t *testing.T) {
	content := "参见（2023）最高法知民终123号案和（2022）京73民初456号案"
	cases := ExtractCaseNumbers(content)
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2: %v", len(cases), cases)
	}
}
