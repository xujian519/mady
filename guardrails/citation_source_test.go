package guardrails

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/lawcite"
)

// stubSource 是测试用知识源：固定返回给定主题表与上限。
type stubSource struct {
	topics map[int][]string
	max    int
}

func (s stubSource) Topics(_ lawcite.Statute, article int) ([]string, bool) {
	kw, ok := s.topics[article]
	return kw, ok
}

func (s stubSource) MaxArticle(lawcite.Statute) int { return s.max }

// TestCompositeTopicsMerge 验证复合源关键词并集去重（primary 在前）、
// 单边未覆盖时另一源仍可作答。
func TestCompositeTopicsMerge(t *testing.T) {
	primary := stubSource{topics: map[int][]string{2: {"甲", "乙"}}, max: 82}
	secondary := stubSource{topics: map[int][]string{2: {"乙", "丙"}, 3: {"丁"}}}
	src := CompositeCitationSource(primary, secondary)

	kw, ok := src.Topics(lawcite.StatutePatentLaw, 2)
	if !ok {
		t.Fatal("复合源 Topics(2) 未覆盖")
	}
	want := []string{"甲", "乙", "丙"}
	if strings.Join(kw, ",") != strings.Join(want, ",") {
		t.Errorf("Topics(2) = %v，期望并集去重 %v（primary 在前）", kw, want)
	}

	kw3, ok := src.Topics(lawcite.StatutePatentLaw, 3)
	if !ok || len(kw3) != 1 || kw3[0] != "丁" {
		t.Errorf("Topics(3) = %v，期望仅 secondary 作答 [丁]", kw3)
	}

	if _, ok := src.Topics(lawcite.StatutePatentLaw, 99); ok {
		t.Error("Topics(99) 两源均未覆盖，应为 not ok")
	}
}

// TestCompositeMaxArticle 验证存在性上限取 primary 非零优先，否则 secondary。
func TestCompositeMaxArticle(t *testing.T) {
	p := stubSource{max: 82}
	s := stubSource{max: 100}
	if got := CompositeCitationSource(p, s).MaxArticle(lawcite.StatutePatentLaw); got != 82 {
		t.Errorf("primary 非零时应取 primary，得到 %d", got)
	}
	if got := CompositeCitationSource(stubSource{}, s).MaxArticle(lawcite.StatutePatentLaw); got != 100 {
		t.Errorf("primary 为零时应回退 secondary，得到 %d", got)
	}
}

// TestCompositeNilPassthrough 验证 nil 源直通（装配侧可无条件组合）：
// 直通语义 = 不再包装 compositeSource（包装后调用 nil 端会 panic）。
func TestCompositeNilPassthrough(t *testing.T) {
	s := stubSource{topics: map[int][]string{1: {"甲"}}}

	got := CompositeCitationSource(nil, s)
	if _, wrapped := got.(compositeSource); wrapped {
		t.Error("primary 为 nil 时不应再包装 compositeSource")
	}
	kw, ok := got.Topics(lawcite.StatutePatentLaw, 1)
	if !ok || len(kw) != 1 || kw[0] != "甲" {
		t.Errorf("primary 为 nil 时应直通 secondary 行为，得到 %v", kw)
	}

	got2 := CompositeCitationSource(s, nil)
	if _, wrapped := got2.(compositeSource); wrapped {
		t.Error("secondary 为 nil 时不应再包装 compositeSource")
	}
	if kw2, ok := got2.Topics(lawcite.StatutePatentLaw, 1); !ok || kw2[0] != "甲" {
		t.Errorf("secondary 为 nil 时应直通 primary 行为，得到 %v", kw2)
	}
}

// TestCitationSourceFuncsNilSafe 验证函数适配器 nil 字段安全（装配侧可只接其一）。
func TestCitationSourceFuncsNilSafe(t *testing.T) {
	var f CitationSourceFuncs
	if _, ok := f.Topics(lawcite.StatutePatentLaw, 1); ok {
		t.Error("TopicsFunc 为 nil 时应返回 not ok")
	}
	if got := f.MaxArticle(lawcite.StatutePatentLaw); got != 0 {
		t.Errorf("MaxArticleFunc 为 nil 时应返回 0，得到 %d", got)
	}
}

// TestVerifyWithSourceExpandsCoverage 验证注入复合源后，S1 未覆盖条目
// （专利法第 3 条）从 Unknown 变为可核验——S2 索引的核心价值。
func TestVerifyWithSourceExpandsCoverage(t *testing.T) {
	// 语境含「专利行政管理部门」，与 S2 第 3 条标题词一致。
	text := "根据专利法第3条，国务院专利行政管理部门负责管理全国的专利工作。"

	base := VerifyCitations(text)
	if base.Unknown != 1 || base.Valid != 0 {
		t.Fatalf("S1 默认源下第 3 条应落 Unknown：%+v", base)
	}

	s2 := stubSource{topics: map[int][]string{3: {"专利行政管理部门", "管理专利工作"}}}
	composite := CompositeCitationSource(DefaultCitationSource(), s2)
	report := VerifyCitationsWithSource(text, composite)
	if report.Valid != 1 || len(report.Flagged) != 0 {
		t.Errorf("复合源下第 3 条应自证通过（Valid=1 且无标记）：%+v", report)
	}
}

// TestVerifyWithSourcePreservesSuspect 验证复合源不改变 S1 的张冠李戴判定：
// 分案申请错引第 47 条在 S1+S2 下仍必须命中（stub 词与用途无关时）。
func TestVerifyWithSourcePreservesSuspect(t *testing.T) {
	text := "3. **专利法第四十七条第一款（分案申请）**：申请人可以在审查过程中提出分案申请。"

	s2 := stubSource{topics: map[int][]string{47: {"视为自始不存在"}}}
	composite := CompositeCitationSource(DefaultCitationSource(), s2)
	report := VerifyCitationsWithSource(text, composite)
	if len(report.Flagged) != 1 || report.Flagged[0].Verdict != VerdictSuspect {
		t.Fatalf("复合源下张冠李戴仍必须命中 Suspect：%+v", report.Flagged)
	}
}

// TestGateUsesInjectedSource 验证 Gate 热路径消费注入源：
// S1 未覆盖条目在注入 stub 源后从放行变为标记（R1 存在性）。
func TestGateUsesInjectedSource(t *testing.T) {
	// 条号超出 stub 上限（stub 声称专利法仅 10 条），S1 默认源不会触发该判定
	// ——仅用于证明 Gate 确实消费了注入源，而非真实法条上限。
	s2 := stubSource{topics: map[int][]string{3: {"专利行政管理部门"}}, max: 10}
	var recorded CitationReport
	hook := NewCitationGate(
		WithCitationGateLevel(LevelStandard),
		WithCitationRecorder(func(r CitationReport, _ string) { recorded = r }),
		WithCitationSource(s2),
	)
	resp := callHook(t, hook, &agentcore.ProviderResponse{
		Content: "根据专利法第99条，这是一个不存在的条号。",
	})
	if resp == nil || len(recorded.Flagged) != 1 || recorded.Flagged[0].Verdict != VerdictInvalid {
		t.Fatalf("注入源后第 99 条应判 Invalid 并留痕：resp=%+v recorded=%+v", resp, recorded)
	}
	if !strings.Contains(resp.Content, "引用核验提示") {
		t.Error("响应应追加引用核验提示")
	}
}
