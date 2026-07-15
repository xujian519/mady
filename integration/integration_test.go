package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/agentcore/tracing"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/knowledge"
	kgraph "github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/workflows/legal"
	"github.com/xujian519/mady/workflows/patent"
)

// TestLegalSyllogismE2E exercises the full legal reasoning pipeline:
// FactBlackboard + syllogistic reasoning (Week 2) mounted into the legal
// comparison Pregel workflow (Week 3). It verifies that the output contains
// auditable syllogism chains and the blackboard accumulates validated facts.
func TestLegalSyllogismE2E(t *testing.T) {
	compiled, bb, err := legal.BuildComparisonGraphWithReasoning(
		"case-001", reasoning.CaseInvalidation,
	)
	if err != nil {
		t.Fatalf("BuildComparisonGraphWithReasoning: %v", err)
	}
	if bb == nil {
		t.Fatal("FactBlackboard is nil")
	}

	facts := "本案涉及一项发明专利的侵权纠纷。原告主张被告制造的产品侵犯了其权利要求中记载的技术方案，该发明具有新颖性和创造性，被告应停止侵权行为。"

	state, err := compiled.Run(context.Background(), graph.PregelState{
		legal.StateCaseFacts: facts,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := state.GetString(legal.StateOutput)
	if output == "" {
		t.Fatal("output is empty")
	}

	// The output should reference syllogistic reasoning elements.
	for _, want := range []string{"大前提", "小前提", "结论"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}

	// The blackboard should contain validated reasoning chains.
	chains := bb.ReasoningChains()
	if len(chains) == 0 {
		t.Error("FactBlackboard has no reasoning chains")
	}
	for _, c := range chains {
		if c.FactRef == "" {
			t.Errorf("chain %s has empty FactRef", c.ID)
		}
	}
	active := bb.ActiveFacts()
	if len(active) == 0 {
		t.Error("FactBlackboard has no active facts")
	}
	if !bb.Locked {
		t.Error("FactBlackboard should be locked after conclude")
	}
}

// TestPatentRulesGraphCitationE2E exercises the patent rule engine (Week 4),
// knowledge graph construction (Week 5), graph-enhanced retrieval (Week 6),
// and citation tracking (Week 4) in a single end-to-end flow.
func TestPatentRulesGraphCitationE2E(t *testing.T) {
	// --- 1. Build knowledge graph from patent case documents ---
	docs := []*knowledge.Document{
		{
			ID:      "case001",
			Title:   "案例001 创造性判断",
			Domain:  "patent",
			Content: "本案涉及一种数据处理方法的创造性判断，重点考察技术启示。",
			Source:  "inline",
			Metadata: map[string]string{
				"type":     "case",
				"law_refs": "专利法第22条第3款",
				"ipc":      "G06F",
			},
			Searchable: true,
		},
		{
			ID:      "case002",
			Title:   "案例002 创造性分析",
			Domain:  "patent",
			Content: "本案对算法方法的创造性进行三步法分析。",
			Source:  "inline",
			Metadata: map[string]string{
				"type":     "case",
				"law_refs": "专利法第22条第3款",
				"ipc":      "G06F",
			},
			Searchable: true,
		},
	}

	gs := kgraph.NewGraphStore()
	builder := kgraph.NewGraphBuilder(gs)
	parsed := make([]kgraph.ParsedDoc, 0, len(docs))
	for _, d := range docs {
		parsed = append(parsed, kgraph.ParseKnowledgeDocument(d))
	}
	builder.Build(parsed)

	if gs.NodeCount() < 3 {
		t.Errorf("expected >=3 graph nodes (2 cases + 1 law), got %d", gs.NodeCount())
	}

	// --- 2. Graph-enhanced retrieval ---
	chunks := retrieval.ChunkDocument("case001", docs[0].Content, retrieval.DefaultChunkOptions())
	searcher := retrieval.NewKeywordSearcher()
	results := searcher.Search("创造性", chunks, 5)
	if len(results) == 0 {
		t.Fatal("keyword search returned no results")
	}

	enhancer := kgraph.NewGraphEnhancer(gs, kgraph.DefaultEnhanceConfig())
	enhanced := enhancer.Enhance(results).(kgraph.EnhancementResult)
	if enhanced.Context == "" {
		t.Error("enhanced context is empty")
	}
	// The enhancer should discover case002 as similar (shared law citation).
	if !strings.Contains(enhanced.Context, "知识图谱扩展") {
		t.Errorf("enhanced context missing graph expansion section\ngot:\n%s", enhanced.Context)
	}

	// --- 3. Citation tracking ---
	citables := retrieval.ScoredChunksToCitable(results)
	citationText := retrieval.FormatCitations(citables)
	if !strings.Contains(citationText, "参考来源") {
		t.Errorf("citation text missing header\ngot:\n%s", citationText)
	}

	// --- 4. Patent rule engine workflow ---
	compiled, err := patent.BuildNoveltyGraphWithRules()
	if err != nil {
		t.Fatalf("BuildNoveltyGraphWithRules: %v", err)
	}
	state, err := compiled.Run(context.Background(), graph.PregelState{
		patent.StateInput: "一种数据处理方法，其特征在于包括以下步骤：接收用户输入；提取特征；输出结果。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	output := state.GetString(patent.StateOutput)
	if !strings.Contains(output, "检查") {
		t.Errorf("patent output missing rule-check section\ngot:\n%s", output)
	}
}

// TestTracedEvaluationE2E exercises OpenTelemetry tracing (Week 2) wrapping
// the evaluation system (Week 7), verifying that traced evaluation produces
// a structured report and trace spans without errors.
func TestTracedEvaluationE2E(t *testing.T) {
	tracer, shutdown, err := tracing.NewStdoutTracer("integration-test")
	if err != nil {
		t.Fatalf("NewStdoutTracer: %v", err)
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	ev := evaluate.NewEvaluator()
	ev = ev.WithThreshold(0.5)
	traced := evaluate.NewTracedEvaluator(ev, tracer)

	cases := []evaluate.TestCase{
		{ID: "c1", Input: "什么是新颖性", Expected: "新颖性是指发明不属于现有技术"},
		{ID: "c2", Input: "三步法", Expected: "三步法包括确定最接近现有技术"},
	}

	report, err := traced.EvaluateBatch(context.Background(), cases,
		func(ctx context.Context, input string) (string, error) {
			if strings.Contains(input, "新颖性") {
				return "新颖性是指发明不属于现有技术", nil
			}
			return "三步法包括确定最接近现有技术", nil
		},
	)
	if err != nil {
		t.Fatalf("EvaluateBatch: %v", err)
	}
	if report == nil {
		t.Fatal("report is nil")
	}
	if report.TotalCases != 2 {
		t.Errorf("expected 2 cases, got %d", report.TotalCases)
	}

	md := evaluate.FormatReport(report)
	if !strings.Contains(md, "评估") {
		t.Errorf("report markdown missing summary\ngot:\n%s", md)
	}
}
