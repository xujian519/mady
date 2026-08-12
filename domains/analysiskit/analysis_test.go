package analysiskit

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// stubProvider 是 ArticleFrameworkProvider 的测试替身：按 id 精确命中或 miss。
type stubProvider struct {
	articles map[string]ArticleFrameworkData
}

func (p stubProvider) Article(id string) ArticleFrameworkData {
	return p.articles[id]
}

func TestFormatArticleData(t *testing.T) {
	af := ArticleFrameworkData{
		Name:         "新颖性判断",
		LawRef:       "专利法第22条第2款",
		GuidelineRef: "审查指南第二部分第三章",
		Steps: []ArticleStepData{
			{Order: 1, Name: "特征比对", InputHint: "权利要求技术特征", OutputSchema: map[string]string{"matched": "相同特征"}},
			{Order: 2, Name: "结论判定", OutputSchema: map[string]string{"verdict": "是否具备新颖性"}},
		},
		ConclusionSchema: map[string]string{"novel": "具备新颖性"},
		ApplicableTo:     []string{"发明", "实用新型"},
	}

	out := FormatArticleData(af)

	for _, want := range []string{
		"## 新颖性判断",
		"**法条依据**：专利法第22条第2款",
		"**审查指南依据**：审查指南第二部分第三章",
		"**第 1 步：特征比对**",
		"- 输入：权利要求技术特征",
		"- matched：相同特征",
		"**第 2 步：结论判定**",
		"- verdict：是否具备新颖性",
		"### 结论模式",
		"- novel：具备新颖性",
		"**适用场景**：发明、实用新型",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatArticleData 输出缺少 %q；完整输出:\n%s", want, out)
		}
	}
}

func TestFormatArticleDataPartial(t *testing.T) {
	// 无审查指南依据、无步骤、无适用场景时输出不 panic 且保留核心字段。
	af := ArticleFrameworkData{
		Name:   "最低要求",
		LawRef: "专利法第26条第3款",
	}
	out := FormatArticleData(af)
	if !strings.Contains(out, "## 最低要求") || !strings.Contains(out, "专利法第26条第3款") {
		t.Errorf("部分数据格式化失败:\n%s", out)
	}
	if strings.Contains(out, "审查指南依据") {
		t.Errorf("空 GuidelineRef 不应输出审查指南依据行:\n%s", out)
	}
}

func TestFrameworkGetArticleFramework_ProviderHit(t *testing.T) {
	p := stubProvider{articles: map[string]ArticleFrameworkData{
		"A22.2": {Name: "新颖性", LawRef: "专利法第22条第2款"},
	}}
	f := NewFramework(p, []string{"patent-law-a22.2", "A22.2"}, func() string { return "fallback" })

	out := f.GetArticleFramework()
	if !strings.Contains(out, "## 新颖性") {
		t.Errorf("provider 命中时应返回格式化框架文本，got:\n%s", out)
	}
}

func TestFrameworkGetArticleFramework_IDOrder(t *testing.T) {
	// 第一个 id 未命中时按序尝试第二个。
	p := stubProvider{articles: map[string]ArticleFrameworkData{
		"A22.2": {Name: "新颖性", LawRef: "专利法第22条第2款"},
	}}
	f := NewFramework(p, []string{"missing", "A22.2"}, func() string { return "fallback" })

	if out := f.GetArticleFramework(); !strings.Contains(out, "## 新颖性") {
		t.Errorf("应按序尝试 id，第二个 id 命中时返回框架文本，got:\n%s", out)
	}
}

func TestFrameworkGetArticleFramework_Fallback(t *testing.T) {
	cases := []struct {
		name     string
		provider ArticleFrameworkProvider
	}{
		{"nil provider", nil},
		{"provider 未命中", stubProvider{articles: map[string]ArticleFrameworkData{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFramework(tc.provider, []string{"A22.2"}, func() string { return "fallback-text" })
			if out := f.GetArticleFramework(); out != "fallback-text" {
				t.Errorf("应降级为 fallback 文本，got %q", out)
			}
		})
	}
}

func TestAssemblePregel(t *testing.T) {
	step := func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) { return state, nil }
	pg, err := AssemblePregel(
		map[string]graph.PregelNode{"a": step, "b": step},
		[][2]string{{"a", "b"}, {"b", graph.PregelEnd}},
	)
	if err != nil {
		t.Fatalf("AssemblePregel 应成功，got %v", err)
	}
	if pg == nil {
		t.Fatal("AssemblePregel 返回 nil 图")
	}

	// 装配结果应可继续 Compile（未编译图，由调用方收尾）。
	if _, err := pg.Compile("a"); err != nil {
		t.Fatalf("装配后的图应可编译，got %v", err)
	}
}

func TestAssemblePregel_Error(t *testing.T) {
	step := func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) { return state, nil }

	t.Run("重复节点", func(t *testing.T) {
		// 同一名字出现两次（map 字面量去重后是单节点），改用顺序重复构造：先正常装配，
		// 再直接对图 AddNode 验证底层错误路径的包装。
		pg, err := AssemblePregel(
			map[string]graph.PregelNode{"a": step},
			[][2]string{{"a", graph.PregelEnd}},
		)
		if err != nil {
			t.Fatalf("装配失败: %v", err)
		}
		if err := pg.AddNode("a", step); err == nil {
			t.Error("重复 AddNode 应报错")
		}
	})

	t.Run("边引用未知节点", func(t *testing.T) {
		_, err := AssemblePregel(
			map[string]graph.PregelNode{"a": step},
			[][2]string{{"a", "ghost"}},
		)
		if err == nil || !strings.Contains(err.Error(), "ghost") {
			t.Errorf("未知目标节点应报错，got %v", err)
		}
	})
}
