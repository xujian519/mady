package disclosure

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Stub Provider — 返回预设响应，避免真实 LLM 调用
// =============================================================================

// stubProvider 实现 agentcore.Provider，返回预设结构或回退响应。
type stubProvider struct {
	mu        sync.Mutex
	responses map[string]string // agentName → 预设 JSON 响应
	fallback  string            // 未命中时的回退响应
	requests  []*agentcore.ProviderRequest
}

func (p *stubProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)

	content := extractMessagesContent(req.Messages)
	for key, resp := range p.responses {
		if strings.Contains(content, key) {
			return &agentcore.ProviderResponse{Content: resp}, nil
		}
	}
	if p.fallback != "" {
		return &agentcore.ProviderResponse{Content: p.fallback}, nil
	}
	return &agentcore.ProviderResponse{Content: `{}`}, nil
}

func (p *stubProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Content: resp.Content, Done: true}
	close(ch)
	return ch, nil
}

func extractMessagesContent(msgs []agentcore.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Content)
	}
	return sb.String()
}

// newTestProvider 创建测试用 stub provider，预设三提取+报告的 JSON 响应。
func newTestProvider() *stubProvider {
	return &stubProvider{
		responses: map[string]string{
			"提取所有需要解决的技术问题": mustJSON(map[string]any{
				"problems": []map[string]any{
					{"id": "P1", "text": "现有技术中传感器响应速度慢", "confidence": 0.9},
					{"id": "P2", "text": "高功耗导致电池续航不足", "confidence": 0.85},
				},
			}),
			"提取所有技术特征": mustJSON(map[string]any{
				"features": []map[string]any{
					{"id": "F1", "description": "采用 MEMS 加速度计", "category": "structure", "function": "检测运动状态", "prior_art_status": "known", "importance": "high", "confidence": 0.9, "solves": []string{"P1"}},
					{"id": "F2", "description": "低功耗休眠模式", "category": "method", "function": "降低待机功耗", "prior_art_status": "known", "importance": "medium", "confidence": 0.85, "solves": []string{"P2"}},
					{"id": "F3", "description": "自适应采样率算法", "category": "method", "function": "动态调整数据采集频率", "prior_art_status": "partial", "importance": "high", "confidence": 0.8, "solves": []string{"P1", "P2"}},
				},
			}),
			"提取所有有益技术效果": mustJSON(map[string]any{
				"effects": []map[string]any{
					{"id": "E1", "text": "响应时间从 100ms 缩短到 10ms", "from": []string{"F1", "F3"}, "confidence": 0.9},
					{"id": "E2", "text": "待机功耗降低 80%", "from": []string{"F2"}, "confidence": 0.85},
				},
			}),
			"生成技术交底书分析报告": `{
				"report": "## 技术交底书分析报告\n\n### 文档概况\n...\n\n### 免责声明\n本报告由 AI 辅助生成，不构成正式法律意见。"
			}`,
		},
		fallback: `{}`,
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// =============================================================================
// 示例交底书文本
// =============================================================================

const sampleDisclosure = `
发明名称：一种低功耗运动检测传感器

技术领域
本发明涉及传感器技术领域，具体涉及一种用于可穿戴设备的运动检测传感器。

背景技术
现有的运动检测传感器在连续工作时功耗较高，导致可穿戴设备续航时间不足。
同时，传统传感器在状态切换时存在响应延迟问题，影响用户体验。

发明内容
本发明提供一种低功耗运动检测传感器，通过自适应采样率算法和硬件休眠机制，
在保持检测精度的同时大幅降低功耗。

要解决的技术问题
1. 现有传感器响应速度慢
2. 高功耗导致电池续航不足

技术方案
1. 采用 MEMS 加速度计作为核心检测元件
2. 实现低功耗休眠模式，无运动时自动进入休眠
3. 采用自适应采样率算法，根据运动强度动态调整采样频率

有益效果
1. 响应时间从 100ms 缩短到 10ms
2. 待机功耗降低 80%

具体实施方式
本实施例中，传感器包含 MEMS 加速度计、微控制器和电源管理模块。
参见图 1，微控制器通过 I2C 接口读取加速度计数据...
`

// =============================================================================
// 单元测试 — 各节点独立测试
// =============================================================================

func TestPreprocessNode(t *testing.T) {
	node := preprocessNode()
	state := graph.PregelState{StateKeyInput: sampleDisclosure}

	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("preprocessNode failed: %v", err)
	}

	doc, ok := result[StateKeyDoc].(*DisclosureDoc)
	if !ok {
		t.Fatal("expected *DisclosureDoc in state")
	}

	if doc.Title != "一种低功耗运动检测传感器" {
		t.Errorf("unexpected title: %q", doc.Title)
	}
	if len(doc.Sections) < 5 {
		t.Errorf("expected at least 5 sections, got %d", len(doc.Sections))
	}
	if _, ok := doc.Sections[SecProblem]; !ok {
		t.Error("expected technical_problem section")
	}
	if _, ok := doc.Sections[SecSolution]; !ok {
		t.Error("expected technical_solution section")
	}
	if !doc.HasDrawings {
		t.Error("expected HasDrawings to be true")
	}
}

func TestConsistencyCheckNode_Pass(t *testing.T) {
	ext := &ExtractionResult{
		Problems: []string{"传感器响应慢", "功耗高"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure},
			{ID: "F2", Description: "低功耗休眠", Category: CatMethod},
		},
		Effects: []string{"响应时间缩短", "功耗降低"},
		PFETriples: []PFETriple{
			{ID: "T1", Problem: "传感器响应慢", FeatureIDs: []string{"F1"}, Effect: "响应时间缩短"},
			{ID: "T2", Problem: "功耗高", FeatureIDs: []string{"F2"}, Effect: "功耗降低"},
		},
	}

	state := graph.PregelState{StateKeyExtraction: ext}
	node := consistencyCheckNode()
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("consistencyCheckNode failed: %v", err)
	}

	cr, ok := result[StateKeyConsistency].(*ConsistencyResult)
	if !ok {
		t.Fatal("expected *ConsistencyResult in state")
	}
	if !cr.Pass {
		t.Errorf("expected pass, got issues: %v", cr.Issues)
	}
}

func TestConsistencyCheckNode_Fail(t *testing.T) {
	ext := &ExtractionResult{
		Problems: []string{"传感器响应慢"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure},
			{ID: "F2", Description: "孤立特征无效果", Category: CatMethod},
		},
		Effects: []string{"响应时间缩短"},
		PFETriples: []PFETriple{
			{ID: "T1", Problem: "传感器响应慢", FeatureIDs: []string{"F1"}, Effect: "响应时间缩短"},
		},
	}

	state := graph.PregelState{StateKeyExtraction: ext}
	node := consistencyCheckNode()
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("consistencyCheckNode failed: %v", err)
	}

	cr := result[StateKeyConsistency].(*ConsistencyResult)
	if cr.Pass {
		t.Error("expected fail due to orphan feature")
	}
	if len(cr.Issues) == 0 {
		t.Error("expected at least one issue")
	}
	if cr.Feedback == "" {
		t.Error("expected feedback for retry")
	}
}

func TestMergeExtractionsNode(t *testing.T) {
	// 模拟三个提取 Agent 的独立输出
	state := graph.PregelState{
		StateKeyExtractProblem: mustJSON(map[string]any{
			"problems": []map[string]any{
				{"id": "P1", "text": "传感器响应慢", "confidence": 0.9},
				{"id": "P2", "text": "功耗高", "confidence": 0.85},
			},
		}),
		StateKeyExtractFeatures: mustJSON(map[string]any{
			"features": []map[string]any{
				{"id": "F1", "description": "MEMS 加速度计", "category": "structure", "function": "", "prior_art_status": "known", "importance": "high", "confidence": 0.9, "solves": []string{"P1"}},
				{"id": "F2", "description": "低功耗休眠", "category": "method", "function": "", "prior_art_status": "known", "importance": "medium", "confidence": 0.85, "solves": []string{"P2"}},
			},
		}),
		StateKeyExtractEffects: mustJSON(map[string]any{
			"effects": []map[string]any{
				{"id": "E1", "text": "响应时间缩短", "from": []string{"F1"}, "confidence": 0.9},
				{"id": "E2", "text": "功耗降低", "from": []string{"F2"}, "confidence": 0.85},
			},
		}),
	}

	node := mergeExtractionsNode()
	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("mergeExtractionsNode failed: %v", err)
	}

	ext, ok := result[StateKeyExtraction].(*ExtractionResult)
	if !ok {
		t.Fatal("expected *ExtractionResult in state")
	}

	// 验证三个提取维度的完整性
	if len(ext.Problems) != 2 {
		t.Errorf("expected 2 problems, got %d", len(ext.Problems))
	}
	if len(ext.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(ext.Features))
	}
	if len(ext.Effects) == 0 {
		t.Error("expected effects to be non-empty")
	}
	// 验证 PFE 三元组交叉引用
	if len(ext.PFETriples) != 2 {
		t.Errorf("expected 2 PFE triples, got %d", len(ext.PFETriples))
	}
	for _, triple := range ext.PFETriples {
		if len(triple.FeatureIDs) == 0 {
			t.Errorf("PFE triple %s has no FeatureIDs", triple.ID)
		}
		if triple.Effect == "" {
			t.Errorf("PFE triple %s has no Effect", triple.ID)
		}
	}

	// 验证独立 key 已被清除
	if _, exists := result[StateKeyExtractProblem]; exists {
		t.Error("expected StateKeyExtractProblem to be deleted")
	}
	if _, exists := result[StateKeyExtractFeatures]; exists {
		t.Error("expected StateKeyExtractFeatures to be deleted")
	}
	if _, exists := result[StateKeyExtractEffects]; exists {
		t.Error("expected StateKeyExtractEffects to be deleted")
	}
}

func TestNoveltyStubNode(t *testing.T) {
	node := noveltyStubNode()
	state := graph.PregelState{}

	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("noveltyStubNode failed: %v", err)
	}

	nr, ok := result[StateKeyNovelty].(*NoveltyResult)
	if !ok {
		t.Fatal("expected *NoveltyResult in state")
	}
	if !nr.Assessed {
		t.Error("expected novelty to be assessed (even with empty state)")
	}
	if nr.Notes == "" {
		t.Error("expected non-empty novelty notes")
	}
	if nr.Conclusion == "" {
		t.Error("expected non-empty novelty conclusion")
	}
}

func TestReviewGateNode(t *testing.T) {
	node := reviewGateNode()
	state := graph.PregelState{
		StateKeyReport: &AnalysisReport{ID: "test"},
	}

	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("reviewGateNode failed: %v", err)
	}

	ready, ok := result["_gate_ready"].(bool)
	if !ok || !ready {
		t.Error("expected _gate_ready to be true")
	}
}

func TestReviewGateNode_NilReport(t *testing.T) {
	// 验证 nil report 不会 panic
	node := reviewGateNode()
	state := graph.PregelState{
		StateKeyReport: (*AnalysisReport)(nil),
	}

	result, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("reviewGateNode with nil report failed: %v", err)
	}

	if _, ok := result["_gate_ready"].(bool); !ok {
		t.Error("expected _gate_ready even with nil report")
	}
}

// =============================================================================
// 集成测试 — Pregel 图全流程
// =============================================================================

func TestDisclosureAnalysisGraph_FullFlow(t *testing.T) {
	provider := newTestProvider()
	cpg, err := BuildDisclosureAnalysisGraph(provider)
	if err != nil {
		t.Fatalf("BuildDisclosureAnalysisGraph failed: %v", err)
	}

	initial := graph.PregelState{StateKeyInput: sampleDisclosure}
	final, err := cpg.Run(context.Background(), initial)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}

	// 验证核心输出
	if doc, ok := final[StateKeyDoc].(*DisclosureDoc); !ok || doc == nil {
		t.Error("expected document in final state")
	}
	if ext, ok := final[StateKeyExtraction].(*ExtractionResult); !ok || ext == nil {
		t.Error("expected extraction_result in final state")
	} else {
		// 验证提取完整性（修复后三个维度均应有数据）
		if len(ext.Problems) == 0 {
			t.Error("expected Problems to be non-empty")
		}
		if len(ext.Features) == 0 {
			t.Error("expected Features to be non-empty")
		}
		if len(ext.Effects) == 0 {
			t.Error("expected Effects to be non-empty")
		}
		t.Logf("Extraction: %d problems, %d features, %d effects, %d triples",
			len(ext.Problems), len(ext.Features),
			len(ext.Effects), len(ext.PFETriples))
	}
	if cr, ok := final[StateKeyConsistency].(*ConsistencyResult); !ok || cr == nil {
		t.Error("expected consistency_result in final state")
	}
	if report, ok := final[StateKeyReport].(*AnalysisReport); !ok || report == nil {
		t.Error("expected report in final state")
	}
	if _, ok := final[StateKeySearchKeywords].([]string); !ok {
		t.Error("expected search_keywords in final state")
	}
	if _, ok := final[StateKeyNovelty].(*NoveltyResult); !ok {
		t.Error("expected novelty_result in final state")
	}

	// 验证 report 结构完整
	report := final[StateKeyReport].(*AnalysisReport)
	if report.ReportText == "" {
		t.Error("expected report text")
	}
	if report.Extraction == nil {
		t.Error("expected extraction in report")
	}
	if report.Consistency == nil {
		t.Error("expected consistency in report")
	}

	// 验证输出
	output := final.GetString(StateKeyOutput)
	if output == "" {
		t.Error("expected output string")
	}

	t.Logf("Report text: %s", report.ReportText[:min(len(report.ReportText), 200)])
}

// =============================================================================
// 一致性校验 Retry 循环测试
// =============================================================================

func TestConsistencyRouter_Retry(t *testing.T) {
	ext := &ExtractionResult{
		Problems: []string{"传感器响应慢"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure},
		},
		Effects: []string{},
		PFETriples: []PFETriple{
			{ID: "T1", Problem: "传感器响应慢", FeatureIDs: []string{"F1"}},
		},
	}

	cr := &ConsistencyResult{
		Pass: false,
		Issues: []ConsistencyIssue{
			{Type: "orphan_feature", Description: "F1 无对应效果", Severity: "warning"},
		},
	}

	t.Run("first retry should go back to extraction", func(t *testing.T) {
		state := graph.PregelState{
			StateKeyExtraction:  ext,
			StateKeyConsistency: cr,
			StateKeyRetryCount:  0,
		}
		targets := consistencyRouter(context.Background(), state)
		if len(targets) != 3 {
			t.Fatalf("expected 3 targets (extract_*), got %d", len(targets))
		}
		if targets[0] != "extract_problem" || targets[1] != "extract_features" || targets[2] != "extract_effects" {
			t.Errorf("unexpected targets: %v", targets)
		}
		if state[StateKeyRetryCount].(int) != 1 {
			t.Errorf("expected retry_count=1, got %d", state[StateKeyRetryCount])
		}
		// 验证旧提取结果已清除
		if _, ok := state[StateKeyExtraction]; ok {
			t.Error("expected old ExtractionResult to be deleted on retry")
		}
		// 验证重试反馈已设置
		if fb := state[StateKeyRetryFeedback]; fb == nil {
			t.Error("expected retry feedback to be set")
		}
	})

	t.Run("max retries exceeded should fail-open", func(t *testing.T) {
		state := graph.PregelState{
			StateKeyExtraction:  ext,
			StateKeyConsistency: cr,
			StateKeyRetryCount:  2,
		}
		targets := consistencyRouter(context.Background(), state)
		if len(targets) != 1 || targets[0] != "generate_keywords" {
			t.Errorf("expected fail-open to generate_keywords, got %v", targets)
		}
		// 验证 RetriesExhausted 已设置
		updatedCR := state[StateKeyConsistency].(*ConsistencyResult)
		if !updatedCR.RetriesExhausted {
			t.Error("expected RetriesExhausted to be true")
		}
	})

	t.Run("pass should go to generate_keywords", func(t *testing.T) {
		passCR := &ConsistencyResult{Pass: true}
		state := graph.PregelState{
			StateKeyExtraction:  ext,
			StateKeyConsistency: passCR,
			StateKeyRetryCount:  0,
		}
		targets := consistencyRouter(context.Background(), state)
		if len(targets) != 1 || targets[0] != "generate_keywords" {
			t.Errorf("expected generate_keywords, got %v", targets)
		}
	})
}

func TestConsistencyRouter_FailOpen(t *testing.T) {
	ext := &ExtractionResult{}
	cr := &ConsistencyResult{
		Pass: false,
		Issues: []ConsistencyIssue{
			{Type: "orphan_feature", Description: "F1 无对应效果", Severity: "warning"},
		},
	}

	state := graph.PregelState{
		StateKeyExtraction:  ext,
		StateKeyConsistency: cr,
		StateKeyRetryCount:  2,
	}

	targets := consistencyRouter(context.Background(), state)
	if len(targets) != 1 || targets[0] != "generate_keywords" {
		t.Errorf("expected generate_keywords, got %v", targets)
	}

	updatedCR := state[StateKeyConsistency].(*ConsistencyResult)
	if !updatedCR.RetriesExhausted {
		t.Error("expected fail-open to set RetriesExhausted=true")
	}
	if updatedCR.Pass {
		t.Error("expected Pass to remain false after fail-open")
	}
	if len(updatedCR.Issues) == 0 {
		t.Error("expected issues to be preserved on fail-open")
	}
}

// =============================================================================
// 辅助测试
// =============================================================================

func TestParseDisclosure(t *testing.T) {
	doc := parseDisclosure(sampleDisclosure)

	if doc.Title == "" {
		t.Error("expected title to be extracted")
	}
	if doc.Format != "txt" {
		t.Errorf("expected format=txt, got %s", doc.Format)
	}

	// 验证关键词匹配的章节
	if _, ok := doc.Sections[SecTechField]; !ok {
		t.Log("warning: technical_field section not found")
	}
	if _, ok := doc.Sections[SecProblem]; !ok {
		t.Log("warning: technical_problem section not found")
	}
	if _, ok := doc.Sections[SecSolution]; !ok {
		t.Log("warning: technical_solution section not found")
	}

	if len(doc.FigureRefs) == 0 {
		t.Log("warning: no figure references extracted")
	}
	t.Logf("Figure refs: %v", doc.FigureRefs)
	t.Logf("Sections found: %d", len(doc.Sections))
	for k := range doc.Sections {
		t.Logf("  - %s", k)
	}
}

func TestCollectKeywordsFromExtraction(t *testing.T) {
	ext := &ExtractionResult{
		Problems: []string{"传感器响应速度慢", "电池续航不足"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure},
			{ID: "F2", Description: "低功耗休眠模式", Category: CatMethod},
		},
	}

	kw := collectKeywordsFromExtraction(ext)
	if len(kw) == 0 {
		t.Error("expected keywords to be collected")
	}
	t.Logf("Keywords: %v", kw)
}

func TestBuildReportInput(t *testing.T) {
	doc := parseDisclosure(sampleDisclosure)
	ext := &ExtractionResult{
		Problems: []string{"传感器响应慢"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure, Importance: "high"},
		},
		Effects: []string{"响应时间缩短"},
		PFETriples: []PFETriple{
			{ID: "T1", Problem: "传感器响应慢", FeatureIDs: []string{"F1"}, Effect: "响应时间缩短"},
		},
	}
	cr := &ConsistencyResult{Pass: true, OverallScore: 1.0}

	state := graph.PregelState{
		StateKeyDoc:            doc,
		StateKeyExtraction:     ext,
		StateKeyConsistency:    cr,
		StateKeySearchKeywords: []string{"MEMS", "加速度计", "低功耗"},
	}

	input := buildReportInput(state)
	if input == "" {
		t.Error("expected report input to be non-empty")
	}
	if !strings.Contains(input, "传感器响应慢") {
		t.Error("expected problem in report input")
	}
	if !strings.Contains(input, "MEMS 加速度计") {
		t.Error("expected feature in report input")
	}
	t.Logf("Report input:\n%s", input)
}

func TestBuildReportInput_RetriesExhausted(t *testing.T) {
	// 验证 RetriesExhausted 标志正确显示在报告输入中
	ext := &ExtractionResult{
		Problems: []string{"传感器响应慢"},
		Features: []TechFeature{
			{ID: "F1", Description: "MEMS 加速度计", Category: CatStructure, Importance: "high"},
		},
		Effects:    []string{},
		PFETriples: []PFETriple{},
	}
	cr := &ConsistencyResult{
		Pass:             false,
		RetriesExhausted: true,
		OverallScore:     0.0,
		Issues: []ConsistencyIssue{
			{Type: "orphan_feature", Description: "F1 无对应效果 [未消解]", Severity: "warning"},
		},
	}

	state := graph.PregelState{
		StateKeyDoc:         parseDisclosure(sampleDisclosure),
		StateKeyExtraction:  ext,
		StateKeyConsistency: cr,
	}

	input := buildReportInput(state)
	if !strings.Contains(input, "已达最大重试次数") {
		t.Error("expected RetriesExhausted warning in report input")
	}
}

func TestTruncate(t *testing.T) {
	// 验证 UTF-8 安全截断
	short := truncate("hello", 50)
	if short != "hello" {
		t.Errorf("expected 'hello', got %q", short)
	}

	// 中文字符截断
	chinese := truncate("你好世界你好世界", 3)
	if !strings.HasSuffix(chinese, "...") {
		t.Errorf("expected truncation suffix, got %q", chinese)
	}
	// 不应包含非法 UTF-8
	if !utf8.ValidString(chinese) {
		t.Error("truncated string should be valid UTF-8")
	}
}
