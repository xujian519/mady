package lawcite

import (
	"strings"
	"testing"
)

// 单条引用断言辅助：提取结果须恰好一条且字段全对。
func expectOne(t *testing.T, text string, statute Statute, article, paragraph, item, suffix int) Citation {
	t.Helper()
	cs := Extract(text)
	if len(cs) != 1 {
		t.Fatalf("Extract(%q) = %d 条引用, 期望 1 条: %+v", text, len(cs), cs)
	}
	c := cs[0]
	if c.Statute != statute || c.Article != article || c.Paragraph != paragraph || c.Item != item || c.Suffix != suffix {
		t.Errorf("Extract(%q) = {statute:%v art:%d para:%d item:%d suffix:%d}, 期望 {%v %d %d %d %d}",
			text, c.Statute, c.Article, c.Paragraph, c.Item, c.Suffix, statute, article, paragraph, item, suffix)
	}
	return c
}

func TestExtractBasicArabic(t *testing.T) {
	expectOne(t, "根据专利法第22条第3款的规定，权利要求1不具备新颖性。",
		StatutePatentLaw, 22, 3, 0, 0)
}

func TestExtractChineseNumerals(t *testing.T) {
	expectOne(t, "专利法第二十二条所称现有技术，是指申请日以前在国内外为公众所知的技术。",
		StatutePatentLaw, 22, 0, 0, 0)
}

func TestExtractImplementingRulesFullName(t *testing.T) {
	expectOne(t, "《专利法实施细则》第四十二条第一款规定，一件专利申请包括两项以上发明的，申请人可以提出分案申请。",
		StatuteImplementingRules, 42, 1, 0, 0)
}

func TestExtractImplementingRulesAlias(t *testing.T) {
	expectOne(t, "该修改符合细则第68条关于无效宣告期间权利要求修改的规定。",
		StatuteImplementingRules, 68, 0, 0, 0)
}

func TestPatentLawNotStealingRulesPrefix(t *testing.T) {
	// "专利法实施细则"整体命中细则，不得被"专利法"抢占。
	expectOne(t, "专利法实施细则第42条允许分案。",
		StatuteImplementingRules, 42, 0, 0, 0)
}

func TestExtractStatuteCarryOver(t *testing.T) {
	text := "依据专利法第22条第3款，权利要求1不具备创造性。此外，第26条第4款要求权利要求以说明书为依据。"
	cs := Extract(text)
	if len(cs) != 2 {
		t.Fatalf("Extract = %d 条, 期望 2: %+v", len(cs), cs)
	}
	if cs[1].Statute != StatutePatentLaw || cs[1].Article != 26 || cs[1].Paragraph != 4 {
		t.Errorf("承接语境归一失败: cs[1] = %+v", cs[1])
	}
}

func TestExtractStatuteWindowExpiry(t *testing.T) {
	// 法律名称与引用之间超过 statuteWindow(120) 字 → 不承接。
	gap := strings.Repeat("分析", 70) // 140 字
	text := "专利法" + gap + "第22条"
	c := expectOne(t, text, StatuteUnknown, 22, 0, 0, 0)
	if c.Statute != StatuteUnknown {
		t.Errorf("窗口外引用应为 StatuteUnknown, 实际 %v", c.Statute)
	}
}

func TestExtractSuffix(t *testing.T) {
	expectOne(t, "专利法第十条之一规定了分案申请不得超出原申请记载的范围。",
		StatutePatentLaw, 10, 0, 0, 1)
	expectOne(t, "专利法第10条之二的情形。",
		StatutePatentLaw, 10, 0, 0, 2)
}

func TestExtractItem(t *testing.T) {
	expectOne(t, "专利法第22条第3款第2项所述情形。",
		StatutePatentLaw, 22, 3, 2, 0)
}

func TestExtractUnknownStatute(t *testing.T) {
	expectOne(t, "综上所述，第33条的要求已经满足。",
		StatuteUnknown, 33, 0, 0, 0)
}

func TestExtractNearestMentionWins(t *testing.T) {
	// 两次法律名称，引用归属于最近的一次。
	text := "专利法第22条分析如上。再依据实施细则第42条，可分案处理。"
	cs := Extract(text)
	if len(cs) != 2 {
		t.Fatalf("Extract = %d 条, 期望 2", len(cs))
	}
	if cs[0].Statute != StatutePatentLaw {
		t.Errorf("cs[0].Statute = %v, 期望 专利法", cs[0].Statute)
	}
	if cs[1].Statute != StatuteImplementingRules {
		t.Errorf("cs[1].Statute = %v, 期望 实施细则", cs[1].Statute)
	}
}

func TestExtractExamGuideline(t *testing.T) {
	expectOne(t, "审查指南第二部分第八章第5节（注：此处假设条号）中引用的第3条标准。",
		StatuteExamGuideline, 3, 0, 0, 0)
}

func TestNoCitation(t *testing.T) {
	if cs := Extract("本回答不涉及任何法条定位，只做流程说明。"); len(cs) != 0 {
		t.Errorf("无引用文本应返回空, 实际 %+v", cs)
	}
}

func TestOrdinaryChineseNumeralsUntouched(t *testing.T) {
	// "第三个""第五天" 等不在归一范围内，不得抽出引用。
	if cs := Extract("第三个申请人第五天提交了材料。"); len(cs) != 0 {
		t.Errorf("普通中文数字不得误抽, 实际 %+v", cs)
	}
}

func TestChineseHundreds(t *testing.T) {
	expectOne(t, "专利法第一百二十三条规定了法律责任。",
		StatutePatentLaw, 123, 0, 0, 0)
}

func TestExtractContextCaptured(t *testing.T) {
	text := "前序分析略。" + strings.Repeat("填充", 30) + "根据专利法第47条（分案申请）可以提出。" + strings.Repeat("后缀", 30)
	cs := Extract(text)
	if len(cs) != 1 {
		t.Fatalf("Extract = %d 条, 期望 1", len(cs))
	}
	if !strings.Contains(cs[0].Context, "（分案申请）") {
		t.Errorf("Context 未捕获用途声明: %q", cs[0].Context)
	}
	// Context 长度受限（前后各 40 字 + 引用本体）。
	if n := len([]rune(cs[0].Context)); n > 90 {
		t.Errorf("Context 超长: %d 字", n)
	}
}

func TestUnique(t *testing.T) {
	text := "专利法第22条第3款。再次强调专利法第22条第3款。另见第22条。"
	cs := Unique(Extract(text))
	// 前两条同键去重；第 22 条（无款）独立键。
	if len(cs) != 2 {
		t.Fatalf("Unique = %d 条, 期望 2: %+v", len(cs), cs)
	}
	if cs[0].Paragraph != 3 || cs[1].Paragraph != 0 {
		t.Errorf("Unique 保留顺序/键有误: %+v", cs)
	}
}

func TestCitationString(t *testing.T) {
	c := Citation{Statute: StatuteImplementingRules, Article: 42, Paragraph: 1}
	if got := c.String(); got != "专利法实施细则第42条第1款" {
		t.Errorf("String() = %q", got)
	}
	c2 := Citation{Statute: StatuteUnknown, Article: 10, Suffix: 1}
	if got := c2.String(); got != "第10条之一" {
		t.Errorf("String() = %q", got)
	}
}

// 真实幻觉案例（v0.8 基线 2008_a31_02 模型实际输出节选）：
// 必须正确识别「专利法第47条（分案申请）」与「专利法实施细则第21条」。
func TestRealHallucinatedAnswer(t *testing.T) {
	text := `3. **专利法第四十七条第一款（分案申请）**：申请人可以在审查过程中提出分案申请。
	同时，专利法实施细则第二十一条规定分案申请的内容应当属于原申请的内容。
	依据专利法第三十一条第一款（单一性）判断。`
	cs := Unique(Extract(text))
	if len(cs) != 3 {
		t.Fatalf("Unique = %d 条, 期望 3: %+v", len(cs), cs)
	}
	want := []struct {
		statute Statute
		article int
	}{
		{StatutePatentLaw, 47},
		{StatuteImplementingRules, 21},
		{StatutePatentLaw, 31},
	}
	for i, w := range want {
		if cs[i].Statute != w.statute || cs[i].Article != w.article {
			t.Errorf("cs[%d] = {%v 第%d条}, 期望 {%v 第%d条}",
				i, cs[i].Statute, cs[i].Article, w.statute, w.article)
		}
	}
	if !strings.Contains(cs[0].Context, "分案申请") {
		t.Errorf("幻觉题的用途声明应进入 Context: %q", cs[0].Context)
	}
}

// BenchmarkExtract 保证热路径性能（设计预算：单答案 < 1ms）。
func BenchmarkExtract(b *testing.B) {
	text := strings.Repeat("根据专利法第22条第3款、实施细则第42条第1款分析，", 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Extract(text)
	}
}
