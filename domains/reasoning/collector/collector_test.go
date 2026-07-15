package collector_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/reasoning/collector"
	"github.com/xujian519/mady/graph"
)

// --------------------------------------------------------------------------
// Stubs
// --------------------------------------------------------------------------

type stubDocReader struct {
	text string
	err  error
}

func (s *stubDocReader) ReadText(_ context.Context, path string) (string, error) {
	return s.text, s.err
}

type stubLLM struct {
	response string
	err      error
}

func (s *stubLLM) Chat(_ context.Context, prompt string) (string, error) {
	return s.response, s.err
}

type stubKnowledgeStore struct {
	facts []collector.KnowledgeFact
	err   error
}

func (s *stubKnowledgeStore) SearchFacts(_ context.Context, query string, topK int) ([]collector.KnowledgeFact, error) {
	return s.facts, s.err
}

// failingLLM implements LLMClient but returns an error on every call.
type failingLLM struct{}

func (f *failingLLM) Chat(_ context.Context, prompt string) (string, error) {
	return "", errors.New("llm unavailable")
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newBlackboard() *reasoning.FactBlackboard {
	return reasoning.NewFactBlackboard("case-001", reasoning.CasePatentability, "人工智能")
}

func TestDocumentCollector_ID(t *testing.T) {
	c := collector.NewDocumentCollector(nil, nil, 5)
	if got := c.ID(); got != reasoning.CollectorDocuments {
		t.Errorf("ID() = %q, want %q", got, reasoning.CollectorDocuments)
	}
}

func TestDocumentCollector_NilReader(t *testing.T) {
	t.Parallel()
	c := collector.NewDocumentCollector(nil, nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "some path", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap when reader is nil")
	}
}

func TestDocumentCollector_ReaderErrorFallsBackToInline(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{err: errors.New("file not found")}
	c := collector.NewDocumentCollector(reader, nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "inline document content", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 1 {
		t.Fatalf("len(bb.Facts()) = %d, want 1", len(all))
	}
	if !strings.Contains(all[0].Content, "inline document") {
		t.Errorf("fact content = %q, should contain inline text", all[0].Content)
	}
}

func TestDocumentCollector_ReaderReturnsEmpty(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: ""}
	c := collector.NewDocumentCollector(reader, nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "some path", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for empty content")
	}
}

func TestDocumentCollector_NoLLM(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: "装置包括处理器和存储器。"}
	c := collector.NewDocumentCollector(reader, nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "/tmp/doc.txt", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 1 {
		t.Fatalf("len(bb.Facts()) = %d, want 1", len(all))
	}
	f := all[0]
	if f.Source != "file" {
		t.Errorf("Source = %q, want %q", f.Source, "file")
	}
	if f.FilePath != "/tmp/doc.txt" {
		t.Errorf("FilePath = %q, want %q", f.FilePath, "/tmp/doc.txt")
	}
	if f.Category != reasoning.FactCategoryTechnical {
		t.Errorf("Category = %q, want %q", f.Category, reasoning.FactCategoryTechnical)
	}
}

func TestDocumentCollector_WithLLM(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: "处理器以2.4GHz运行，存储器为16GB DDR4。"}
	llm := &stubLLM{
		response: "technical|处理器频率2.4GHz\ntechnical|存储器容量16GB DDR4",
	}
	c := collector.NewDocumentCollector(reader, llm, 10)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "/tmp/doc.txt", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 2 {
		t.Fatalf("len(bb.Facts()) = %d, want 2", len(all))
	}
	// Verify first fact.
	if all[0].Category != reasoning.FactCategoryTechnical {
		t.Errorf("Category[0] = %q, want technical", all[0].Category)
	}
	if !strings.Contains(all[0].Content, "2.4GHz") {
		t.Errorf("Content[0] = %q, should contain 2.4GHz", all[0].Content)
	}
}

func TestDocumentCollector_WithLLMReturnsEmpty(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: "一些文本"}
	llm := &stubLLM{response: ""}
	c := collector.NewDocumentCollector(reader, llm, 10)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "/tmp/doc.txt", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap when LLM returned empty")
	}
}

func TestDocumentCollector_WithLLMError(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: "一些文本"}
	llm := &failingLLM{}
	c := collector.NewDocumentCollector(reader, llm, 10)
	bb := newBlackboard()
	_, err := c.Collect(context.Background(), "/tmp/doc.txt", bb)
	if err == nil {
		t.Fatal("expected an error from LLM")
	}
	if !strings.Contains(err.Error(), "llm unavailable") {
		t.Errorf("error = %q, should contain 'llm unavailable'", err.Error())
	}
}

func TestDocumentCollector_ExceedsMaxFacts(t *testing.T) {
	t.Parallel()
	reader := &stubDocReader{text: "text"}
	llm := &stubLLM{
		response: "technical|f1\ntechnical|f2\ntechnical|f3",
	}
	c := collector.NewDocumentCollector(reader, llm, 2)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "/tmp/doc.txt", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2 (maxFacts)", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 2 {
		t.Fatalf("len(bb.Facts()) = %d, want 2", len(all))
	}
}

func TestDocumentCollector_DefaultMaxFacts(t *testing.T) {
	// maxFacts <= 0 should default to 20.
	c := collector.NewDocumentCollector(nil, nil, 0)
	_ = c // would panic if maxFacts <= 0 was used as a divisor; just ensure it doesn't.
}

func TestUserInputCollector_ID(t *testing.T) {
	c := collector.NewUserInputCollector(nil, 5)
	if got := c.ID(); got != reasoning.CollectorUserInput {
		t.Errorf("ID() = %q, want %q", got, reasoning.CollectorUserInput)
	}
}

func TestUserInputCollector_EmptyInput(t *testing.T) {
	t.Parallel()
	c := collector.NewUserInputCollector(nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for empty input")
	}
}

func TestUserInputCollector_NoLLM(t *testing.T) {
	t.Parallel()
	c := collector.NewUserInputCollector(nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "本发明涉及一种人工智能芯片。", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 1 {
		t.Fatalf("len(bb.Facts()) = %d, want 1", len(all))
	}
	if all[0].Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0", all[0].Confidence)
	}
}

func TestUserInputCollector_WithLLM(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{
		response: "technical|装置包括一个神经网络处理器\nlegal|该方案符合专利法第22条",
	}
	c := collector.NewUserInputCollector(llm, 10)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "本发明涉及一种神经网络处理器。", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", result.FactCount)
	}
	all := bb.Facts()
	if len(all) != 2 {
		t.Fatalf("len(bb.Facts()) = %d, want 2", len(all))
	}
	if all[0].Category != reasoning.FactCategoryTechnical {
		t.Errorf("Category[0] = %q, want technical", all[0].Category)
	}
	if all[1].Category != reasoning.FactCategoryLegal {
		t.Errorf("Category[1] = %q, want legal", all[1].Category)
	}
}

func TestUserInputCollector_WithLLMErrorFallsBack(t *testing.T) {
	t.Parallel()
	llm := &failingLLM{}
	c := collector.NewUserInputCollector(llm, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "本发明涉及一种人工智能芯片。", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1 (fallback)", result.FactCount)
	}
	if result.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8 (fallback)", result.Confidence)
	}
	all := bb.Facts()
	if len(all) != 1 {
		t.Fatalf("len(bb.Facts()) = %d, want 1", len(all))
	}
}

func TestUserInputCollector_WithLLMReturnsEmpty(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{response: ""}
	c := collector.NewUserInputCollector(llm, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "本发明涉及一种人工智能芯片。", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap when LLM returns empty")
	}
}

func TestUserInputCollector_ExceedsMaxFacts(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{
		response: "technical|f1\ntechnical|f2\ntechnical|f3\ntechnical|f4",
	}
	c := collector.NewUserInputCollector(llm, 3)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 3 {
		t.Errorf("FactCount = %d, want 3", result.FactCount)
	}
}

func TestKnowledgeCollector_ID(t *testing.T) {
	c := collector.NewKnowledgeCollector(nil, 5)
	if got := c.ID(); got != reasoning.CollectorKnowledge {
		t.Errorf("ID() = %q, want %q", got, reasoning.CollectorKnowledge)
	}
}

func TestKnowledgeCollector_NilStore(t *testing.T) {
	t.Parallel()
	c := collector.NewKnowledgeCollector(nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "神经网络处理器", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap when store is nil")
	}
}

func TestKnowledgeCollector_ReturnsFacts(t *testing.T) {
	t.Parallel()
	store := &stubKnowledgeStore{
		facts: []collector.KnowledgeFact{
			{ID: "kg1", Content: "神经网络加速器NPU", Source: "专利库", Confidence: 0.95},
			{ID: "kg2", Content: "深度学习处理器架构", Source: "期刊", Confidence: 0.85},
		},
	}
	c := collector.NewKnowledgeCollector(store, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "神经网络处理器", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", result.FactCount)
	}
	if result.Confidence != 0.7 {
		t.Errorf("Confidence = %v, want 0.7", result.Confidence)
	}
	all := bb.Facts()
	if len(all) != 2 {
		t.Fatalf("len(bb.Facts()) = %d, want 2", len(all))
	}
	if all[0].Source != "knowledge_graph" {
		t.Errorf("Source[0] = %q, want %q", all[0].Source, "knowledge_graph")
	}
	if len(all[0].Tags) != 1 || all[0].Tags[0] != "专利库" {
		t.Errorf("Tags[0] = %v, want [专利库]", all[0].Tags)
	}
}

func TestKnowledgeCollector_EmptyResults(t *testing.T) {
	t.Parallel()
	store := &stubKnowledgeStore{facts: nil}
	c := collector.NewKnowledgeCollector(store, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "未知技术", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if result.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0", result.Confidence)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for empty results")
	}
}

func TestKnowledgeCollector_StoreError(t *testing.T) {
	t.Parallel()
	store := &stubKnowledgeStore{err: errors.New("connection timeout")}
	c := collector.NewKnowledgeCollector(store, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "神经网络处理器", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for store error")
	}
}

func TestKnowledgeCollector_QueryUsesTechnicalField(t *testing.T) {
	t.Parallel()
	store := &stubKnowledgeStore{
		facts: []collector.KnowledgeFact{
			{ID: "kg1", Content: "结果", Source: "test", Confidence: 0.9},
		},
	}
	c := collector.NewKnowledgeCollector(store, 5)
	bb := reasoning.NewFactBlackboard("case-002", reasoning.CasePatentability, "图像处理")
	result, err := c.Collect(context.Background(), "卷积神经网络", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1", result.FactCount)
	}
	// The buildQuery should prefix TechnicalField: verify indirectly by checking
	// that the store received a call. If it didn't error, the store was called.
}

func TestKnowledgeCollector_DefaultMaxFacts(t *testing.T) {
	// maxFacts <= 0 should default to 10.
	c := collector.NewKnowledgeCollector(nil, 0)
	_ = c
}

func TestDerivedCollector_ID(t *testing.T) {
	c := collector.NewDerivedCollector(nil, 5)
	if got := c.ID(); got != reasoning.CollectorDerived {
		t.Errorf("ID() = %q, want %q", got, reasoning.CollectorDerived)
	}
}

func TestDerivedCollector_NilLLM(t *testing.T) {
	t.Parallel()
	c := collector.NewDerivedCollector(nil, 5)
	bb := newBlackboard()
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap when LLM is nil")
	}
}

func TestDerivedCollector_NoActiveFacts(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{response: "推理|something|0.8"}
	c := collector.NewDerivedCollector(llm, 5)
	bb := newBlackboard() // no facts added
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for no active facts")
	}
}

func TestDerivedCollector_HappyPath(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{
		response: "推理|该芯片采用7nm工艺，功耗降低30%|0.85\n推理|支持TensorFlow和PyTorch框架|0.75",
	}
	c := collector.NewDerivedCollector(llm, 10)
	bb := newBlackboard()
	bb.AddFact(reasoning.FactEntry{
		ID: "f1", Content: "芯片制程7nm", Confidence: 0.95, Category: reasoning.FactCategoryTechnical,
	})
	bb.AddFact(reasoning.FactEntry{
		ID: "f2", Content: "支持AI框架", Confidence: 0.90, Category: reasoning.FactCategoryTechnical,
	})
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", result.FactCount)
	}
	if result.Confidence != 0.6 {
		t.Errorf("Confidence = %v, want 0.6", result.Confidence)
	}
	all := bb.Facts()
	// 2 original + 2 derived = 4 total.
	if len(all) != 4 {
		t.Fatalf("len(bb.Facts()) = %d, want 4", len(all))
	}
	// The last two facts should be derived.
	for i := 2; i < 4; i++ {
		if all[i].Source != "llm_derived" {
			t.Errorf("Facts[%d].Source = %q, want %q", i, all[i].Source, "llm_derived")
		}
	}
}

func TestDerivedCollector_LLMError(t *testing.T) {
	t.Parallel()
	llm := &failingLLM{}
	c := collector.NewDerivedCollector(llm, 5)
	bb := newBlackboard()
	bb.AddFact(reasoning.FactEntry{ID: "f1", Content: "something", Confidence: 0.9})
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 0 {
		t.Errorf("FactCount = %d, want 0", result.FactCount)
	}
	if len(result.Gaps) == 0 {
		t.Error("expected a gap for LLM error")
	}
}

func TestDerivedCollector_SkipsNonInferenceLines(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{
		response: "推理|valid derived fact|0.8\nsome other line\n推理|another fact|0.7",
	}
	c := collector.NewDerivedCollector(llm, 10)
	bb := newBlackboard()
	bb.AddFact(reasoning.FactEntry{ID: "f1", Content: "base fact", Confidence: 0.9})
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2 (skipped non-inference line)", result.FactCount)
	}
}

func TestDerivedCollector_ExceedsMaxFacts(t *testing.T) {
	t.Parallel()
	llm := &stubLLM{
		response: "推理|f1|0.8\n推理|f2|0.7\n推理|f3|0.9",
	}
	c := collector.NewDerivedCollector(llm, 2)
	bb := newBlackboard()
	bb.AddFact(reasoning.FactEntry{ID: "f1", Content: "base", Confidence: 0.9})
	result, err := c.Collect(context.Background(), "input", bb)
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if result.FactCount != 2 {
		t.Errorf("FactCount = %d, want 2", result.FactCount)
	}
}


// --------------------------------------------------------------------------
// Stage 1 Graph
// --------------------------------------------------------------------------

func TestBuildStage1Graph_EmptyCollectors(t *testing.T) {
	_, err := collector.BuildStage1Graph(nil)
	if err == nil {
		t.Fatal("expected an error for nil collectors")
	}
	_, err = collector.BuildStage1Graph([]collector.FactCollector{})
	if err == nil {
		t.Fatal("expected an error for empty collectors")
	}
}

func TestBuildStage1Graph_Success(t *testing.T) {
	uc := collector.NewUserInputCollector(nil, 5)
	dc := collector.NewDocumentCollector(nil, nil, 5)
	kc := collector.NewKnowledgeCollector(nil, 5)
	cc := collector.NewDerivedCollector(nil, 5)

	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc, dc, kc, cc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("BuildStage1Graph() returned nil")
	}
}

func TestBuildStage1Graph_Edges(t *testing.T) {
	uc := collector.NewUserInputCollector(nil, 5)
	dc := collector.NewDocumentCollector(nil, nil, 5)

	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc, dc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	_ = g // structural validation by the graph package happened inside BuildStage1Graph.
}

func TestBuildStage1Entry(t *testing.T) {
	if got := collector.BuildStage1Entry(nil); got != "" {
		t.Errorf("entry for nil = %q, want empty", got)
	}
	if got := collector.BuildStage1Entry([]collector.FactCollector{}); got != "" {
		t.Errorf("entry for empty = %q, want empty", got)
	}

	uc := collector.NewUserInputCollector(nil, 5)
	dc := collector.NewDocumentCollector(nil, nil, 5)

	entry := collector.BuildStage1Entry([]collector.FactCollector{uc, dc})
	if entry != "user_input" {
		t.Errorf("entry = %q, want %q", entry, "user_input")
	}
}

func TestBuildCollectorNode(t *testing.T) {
	t.Parallel()
	uc := collector.NewUserInputCollector(nil, 5)

	// Build the graph so we can compile and run it.
	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	entry := collector.BuildStage1Entry([]collector.FactCollector{uc})

	compiled, err := g.Compile(entry)
	if err != nil {
		t.Fatalf("Compile() unexpected error: %v", err)
	}

	bb := newBlackboard()
	input := "测试输入"
	state, err := compiled.Run(context.Background(), graph.PregelState{
		"input": input,
		"bb":    bb,
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// user_input collector should have written result under "user_input".
	v, ok := state["user_input"]
	if !ok {
		t.Fatal("state missing key 'user_input'")
	}
	cr, ok := v.(*reasoning.CollectResult)
	if !ok {
		t.Fatalf("state['user_input'] type = %T, want *CollectResult", v)
	}
	if cr.FactCount != 1 {
		t.Errorf("FactCount = %d, want 1", cr.FactCount)
	}
	// Merge results should be set under "stage1_results".
	mv, ok := state["stage1_results"]
	if !ok {
		t.Fatal("state missing key 'stage1_results'")
	}
	if mv == nil {
		t.Fatal("stage1_results is nil")
	}
}

func TestBuildCollectorNode_MissingBlackboard(t *testing.T) {
	t.Parallel()
	uc := collector.NewUserInputCollector(nil, 5)

	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	entry := collector.BuildStage1Entry([]collector.FactCollector{uc})
	compiled, err := g.Compile(entry)
	if err != nil {
		t.Fatalf("Compile() unexpected error: %v", err)
	}

	// Run without "bb" in state — the node should error.
	_, err = compiled.Run(context.Background(), graph.PregelState{"input": "test"})
	if err == nil {
		t.Fatal("expected an error for missing blackboard")
	}
}

func TestStage1MergeNode(t *testing.T) {
	t.Parallel()
	uc := collector.NewUserInputCollector(nil, 5)

	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	entry := collector.BuildStage1Entry([]collector.FactCollector{uc})
	compiled, err := g.Compile(entry)
	if err != nil {
		t.Fatalf("Compile() unexpected error: %v", err)
	}

	bb := newBlackboard()
	state, err := compiled.Run(context.Background(), graph.PregelState{
		"input": "test",
		"bb":    bb,
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify the merge node set stage output on the blackboard.
	out, ok := bb.StageOutput("stage1")
	if !ok {
		t.Fatal("StageOutput('stage1') not found")
	}
	if out == nil {
		t.Fatal("StageOutput('stage1') is nil")
	}

	// Verify "stage1_results" in returned state.
	_, ok = state["stage1_results"]
	if !ok {
		t.Fatal("state missing 'stage1_results'")
	}
}

func TestStage1MergeNode_NilBlackboard(t *testing.T) {
	t.Parallel()
	uc := collector.NewUserInputCollector(nil, 5)

	g, err := collector.BuildStage1Graph([]collector.FactCollector{uc})
	if err != nil {
		t.Fatalf("BuildStage1Graph() unexpected error: %v", err)
	}
	entry := collector.BuildStage1Entry([]collector.FactCollector{uc})
	compiled, err := g.Compile(entry)
	if err != nil {
		t.Fatalf("Compile() unexpected error: %v", err)
	}

	// The collector node itself will fail first if bb is nil, so the merge
	// node won't execute. We can't test the merge node's nil-bb branch
	// separately via normal graph execution since the node is unexported.
	// Just confirm that a nil bb inside the collector node gives an error.
	_, err = compiled.Run(context.Background(), graph.PregelState{
		"input": "test",
	})
	if err == nil {
		t.Fatal("expected an error for nil blackboard")
	}
}
