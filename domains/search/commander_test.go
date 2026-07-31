package search

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

// mockRetriever 可编程的测试检索器：记录收到的查询，按配置返回结果。
type mockRetriever struct {
	// responses 每次 Search 按序返回；耗尽后返回最后一组。
	responses [][]domain.DomainDocument
	// queries 记录所有收到的查询（Text/Keywords/Filters）。
	queries []domain.DomainQuery
	// totalCounts 每次返回的 TotalCount（与 responses 对齐）。
	totalCounts []int
}

func (m *mockRetriever) Search(_ context.Context, q domain.DomainQuery) (*domain.DomainResults, error) {
	m.queries = append(m.queries, q)
	idx := len(m.queries) - 1
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	total := len(m.responses[idx])
	if idx < len(m.totalCounts) {
		total = m.totalCounts[idx]
	}
	return &domain.DomainResults{
		Query:      q,
		Documents:  m.responses[idx],
		TotalCount: total,
		Source:     "mock",
	}, nil
}

func (m *mockRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	return nil, nil
}

func (m *mockRetriever) SourceName() string { return "mock" }

func doc(id, title, assignee string) domain.DomainDocument {
	return domain.DomainDocument{
		ID:      id,
		Title:   title,
		Snippet: title + " 摘要内容",
		URL:     "https://patents.google.com/patent/" + id,
		Metadata: map[string]string{
			"source":   "Google Patents",
			"assignee": assignee,
		},
		Score: 0.9,
	}
}

func TestNewCommanderNilRetriever(t *testing.T) {
	if c := NewCommander(nil); c != nil {
		t.Fatal("expected nil commander for nil retriever")
	}
}

func TestRunEmptyQuery(t *testing.T) {
	c := NewCommander(&mockRetriever{})
	if _, err := c.Run(context.Background(), Request{Query: "  "}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestDetectScene(t *testing.T) {
	cases := []struct {
		query string
		want  Scene
	}{
		{"无效宣告的证据收集", SceneInvalidation},
		{"侵权风险排查", SceneInfringement},
		{"做一份FTO报告", SceneFTO},
		{"学术论文和专利调研", SceneAcademic},
		{"骨髓腔输液装置现有技术", SceneOA},
	}
	for _, tc := range cases {
		if got := detectScene(tc.query, SceneAuto); got != tc.want {
			t.Errorf("detectScene(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
	// 显式场景优先于自动识别。
	if got := detectScene("无效宣告", SceneFTO); got != SceneFTO {
		t.Errorf("explicit scene override = %q, want %q", got, SceneFTO)
	}
}

func TestRunMultiRoundProgression(t *testing.T) {
	// Round 1：宽搜 2 篇；Round 2：IPC 过滤 1 篇；Round 3：扩展 1 篇。
	m := &mockRetriever{
		responses: [][]domain.DomainDocument{
			{doc("CN100A", "深度学习图像识别装置", "华为"), doc("CN200B", "神经网络加速器", "华为")},
			{doc("CN300C", "图像识别专用芯片", "清华大学")},
			{doc("US400D", "Deep learning vision system", "MIT")},
		},
		totalCounts: []int{15, 6, 8},
	}
	c := NewCommander(m)
	rep, err := c.Run(context.Background(), Request{
		Query: "深度学习图像识别",
		IPCs:  []string{"G06F 17/30"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(rep.Rounds) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(rep.Rounds))
	}
	// Round 1 应是宽语义检索，无 IPC 约束。
	if rep.Rounds[0].Phase != PhaseBroad {
		t.Errorf("round1 phase = %q, want %q", rep.Rounds[0].Phase, PhaseBroad)
	}
	if _, ok := m.queries[0].Filters["ipc"]; ok {
		t.Error("round1 should not carry IPC filter")
	}
	// Round 2 应有 IPC 约束。
	if v, ok := m.queries[1].Filters["ipc"]; !ok || !strings.Contains(v, "G06F") {
		t.Errorf("round2 filters = %v, want ipc containing G06F", m.queries[1].Filters)
	}
	// Round 3 应扩展关键词（含 Round 2 的申请人/术语）。
	if len(m.queries[2].Keywords) == 0 && len(m.queries[2].Filters["applicant"]) == 0 {
		t.Error("round3 should expand with keywords or applicant")
	}

	// 总表应去重（每篇一次）且按轮次排序。
	if len(rep.Table) != 4 {
		t.Fatalf("expected 4 unique docs in table, got %d", len(rep.Table))
	}
	if rep.Table[0].Round != 1 {
		t.Errorf("table should be sorted by round, first round = %d", rep.Table[0].Round)
	}
	// Markdown 应含关键节。
	md := rep.Markdown()
	for _, want := range []string{"# 检索指挥官报告", "## 检索目标", "## 对比文件总表", "## 结论与建议"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing section %q", want)
		}
	}
}

func TestRunConvergesEarly(t *testing.T) {
	// Round 1 命中 3 条（偏窄但稳定），提取到新申请人；Round 2 无新增 → 提前停止。
	m := &mockRetriever{
		responses: [][]domain.DomainDocument{
			{doc("CN100A", "装置", "华为")},
		},
		totalCounts: []int{3},
	}
	c := NewCommander(m)
	rep, err := c.Run(context.Background(), Request{Query: "装置"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Rounds) != 2 {
		t.Fatalf("expected stop after 2 rounds (round1 initial, round2 stable), got %d", len(rep.Rounds))
	}
	if !rep.Rounds[1].Stop {
		t.Error("round2 should stop when no new findings relative to round1")
	}
	if rep.Conclusion == "" {
		t.Error("expected conclusion")
	}
}

func TestRunZeroHitsStops(t *testing.T) {
	m := &mockRetriever{
		responses:   [][]domain.DomainDocument{{}},
		totalCounts: []int{0},
	}
	c := NewCommander(m)
	rep, err := c.Run(context.Background(), Request{Query: "不存在的技术"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Rounds) != 1 || !rep.Rounds[0].Stop {
		t.Fatalf("expected stop on zero hits, rounds=%d", len(rep.Rounds))
	}
	// Gap 应标注未检索到对比文件。
	found := false
	for _, g := range rep.Gaps {
		if strings.Contains(g, "未检索到任何对比文件") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gap about no documents, got %v", rep.Gaps)
	}
}

func TestRunMaxRoundsLimit(t *testing.T) {
	m := &mockRetriever{
		responses: [][]domain.DomainDocument{
			{doc("CN100A", "甲装置", "华为")},
			{doc("CN100B", "乙装置", "华为")},
			{doc("CN100C", "丙装置", "华为")},
			{doc("CN100D", "丁装置", "华为")},
			{doc("CN100E", "戊装置", "华为")},
		},
		totalCounts: []int{10, 10, 10, 10, 10},
	}
	c := NewCommander(m)
	rep, err := c.Run(context.Background(), Request{Query: "装置", MaxRounds: 2})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Rounds) > 2 {
		t.Fatalf("maxRounds=2 exceeded: %d rounds", len(rep.Rounds))
	}
}

func TestRunCountryFilterPropagated(t *testing.T) {
	m := &mockRetriever{
		responses: [][]domain.DomainDocument{
			{doc("CN100A", "装置", "华为")},
		},
		totalCounts: []int{10},
	}
	c := NewCommander(m)
	_, err := c.Run(context.Background(), Request{
		Query:   "装置",
		Country: "cn",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(m.queries) == 0 {
		t.Fatal("no queries recorded")
	}
	// 每轮都应透传 country 过滤。
	for i, q := range m.queries {
		if v, ok := q.Filters["country"]; !ok || v != "cn" {
			t.Errorf("round %d filters missing country=cn: %v", i+1, q.Filters)
		}
	}
}

func TestRunAllRoundsFailReturnsError(t *testing.T) {
	m := &failingRetriever{err: fmt.Errorf("ego-browser 不可用")}
	c := NewCommander(m)
	_, err := c.Run(context.Background(), Request{Query: "装置"})
	if err == nil {
		t.Fatal("expected error when all rounds fail")
	}
	if !strings.Contains(err.Error(), "所有检索轮次均失败") {
		t.Errorf("error = %v, want all-rounds-failed message", err)
	}
}

func TestRunPartialRoundFailureContinues(t *testing.T) {
	// Round 1 失败、Round 2 成功 → 不返回 error，失败记入 Gap。
	m := &partialFailRetriever{responses: [][]domain.DomainDocument{
		{doc("CN100A", "甲装置", "华为")},
	}}
	c := NewCommander(m)
	rep, err := c.Run(context.Background(), Request{Query: "装置"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Rounds) == 0 {
		t.Fatal("expected at least one successful round")
	}
	found := false
	for _, g := range rep.Gaps {
		if strings.Contains(g, "执行失败") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected round-failure gap, got %v", rep.Gaps)
	}
}

func TestMdCell(t *testing.T) {
	if got := mdCell("a|b\nc"); got != "a\\|b c" {
		t.Errorf("mdCell = %q, want %q", got, "a\\|b c")
	}
	long := strings.Repeat("x", 100)
	if got := mdCell(long); len([]rune(got)) != 81 {
		t.Errorf("mdCell truncation = %d runes, want 81", len([]rune(got)))
	}
}

// failingRetriever 每次 Search 都返回错误。
type failingRetriever struct {
	err error
}

func (f *failingRetriever) Search(_ context.Context, _ domain.DomainQuery) (*domain.DomainResults, error) {
	return nil, f.err
}

func (f *failingRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	return nil, nil
}

func (f *failingRetriever) SourceName() string { return "failing" }

// partialFailRetriever 第一次 Search 失败，之后成功。
type partialFailRetriever struct {
	responses [][]domain.DomainDocument
	calls     int
}

func (p *partialFailRetriever) Search(_ context.Context, q domain.DomainQuery) (*domain.DomainResults, error) {
	p.calls++
	if p.calls == 1 {
		return nil, fmt.Errorf("mock round failure")
	}
	idx := p.calls - 2
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	return &domain.DomainResults{
		Query:      q,
		Documents:  p.responses[idx],
		TotalCount: len(p.responses[idx]),
		Source:     "partial",
	}, nil
}

func (p *partialFailRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	return nil, nil
}

func (p *partialFailRetriever) SourceName() string { return "partial" }

func TestExtractApplicantsDedup(t *testing.T) {
	docs := []domain.DomainDocument{
		doc("CN1", "甲", "华为"),
		doc("CN2", "乙", "华为"),
		doc("CN3", "丙", "清华大学"),
	}
	got := extractApplicants(docs, []string{"华为"})
	if len(got) != 1 || got[0] != "清华大学" {
		t.Errorf("extractApplicants = %v, want [清华大学]", got)
	}
}

func TestBuildTableSorted(t *testing.T) {
	seen := map[string]CompareDoc{
		"CN1": {Number: "CN1", Round: 2},
		"CN2": {Number: "CN2", Round: 1},
	}
	table := buildTable(seen)
	if table[0].Number != "CN2" || table[1].Number != "CN1" {
		t.Errorf("table not sorted by round: %+v", table)
	}
}

func TestGapsText(t *testing.T) {
	r := &Report{Gaps: []string{"a", "b"}}
	if !strings.Contains(r.GapsText(), "a") {
		t.Error("GapsText should contain gap items")
	}
	empty := &Report{}
	if !strings.Contains(empty.GapsText(), "未发现明显遗漏") {
		t.Error("GapsText empty case wrong")
	}
}
