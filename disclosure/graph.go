package disclosure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// =============================================================================
// Pregel 适配器
// =============================================================================

// pregelAgentNode 将 agentcore.Agent 包装为 PregelNode。
// stateKey 决定 Agent 的原始 JSON 输出写入哪个 PregelState 键。
//   - 提取 Agent 使用 StateKeyExtractProblem / StateKeyExtractFeatures / StateKeyExtractEffects
//   - 报告 Agent 使用 StateKeyReport
//
// 重试时，如果 state 中存在 StateKeyRetryFeedback，会作为额外上下文注入到 Agent 输入中。
func pregelAgentNode(cfg agentcore.Config, stateKey string) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		agent := agentcore.New(cfg)

		// 读取输入：优先使用 state["input"]，回退到 document 文本
		input := state.GetString(StateKeyInput)
		if input == "" {
			if doc, ok := state[StateKeyDoc].(*DisclosureDoc); ok {
				input = doc.RawText
			}
		}

		// 重试反馈机制：将一致性校验的反馈注入到 Agent 输入中
		if feedback := state.GetString(StateKeyRetryFeedback); feedback != "" {
			input = "【上一轮一致性校验反馈，请针对性修正】\n" + feedback + "\n\n【原始交底书】\n" + input
		}

		output, err := agent.Run(ctx, input)
		if err != nil {
			return state, err
		}

		// 将输出存储到指定的 state key
		state[stateKey] = output

		return state, nil
	}
}

// =============================================================================
// merge_extractions — 合并节点
// =============================================================================

// mergeExtractionsNode 将三个并行提取 Agent 的独立输出合并为统一的 ExtractionResult。
// 此节点作为单一节点运行（非并行），因此不存在 last-writer-wins 或数据竞争。
// 合并过程：
//  1. 从三个独立 key 读取各 Agent 的原始输出
//  2. 解析 JSON 并构建 ExtractionResult
//  3. 构建 PFE 三元组的交叉引用（此时所有数据已在同一处）
//  4. 清除独立 key，写入统一的 StateKeyExtraction
func mergeExtractionsNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		result := &ExtractionResult{}

		// 解析问题输出
		mergeProblemsFromState(result, state)
		// 解析特征输出
		mergeFeaturesFromState(result, state)
		// 解析效果输出
		mergeEffectsFromState(result, state)

		// 构建 PFE 三元组的交叉引用
		linkPFETriples(result)

		// 清除独立 key
		delete(state, StateKeyExtractProblem)
		delete(state, StateKeyExtractFeatures)
		delete(state, StateKeyExtractEffects)

		state[StateKeyExtraction] = result
		return state, nil
	}
}

// mergeProblemsFromState 从 StateKeyExtractProblem 读取并解析问题列表。
func mergeProblemsFromState(ext *ExtractionResult, state graph.PregelState) {
	raw, _ := state[StateKeyExtractProblem].(string)
	if raw == "" {
		// 兼容 struct 类型（测试中可能直接存储）
		return
	}
	if !json.Valid([]byte(raw)) {
		return
	}

	var parsed struct {
		Problems []struct {
			ID         string  `json:"id"`
			Text       string  `json:"text"`
			Confidence float64 `json:"confidence"`
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return
	}

	for _, p := range parsed.Problems {
		ext.Problems = append(ext.Problems, p.Text)
		ext.PFETriples = append(ext.PFETriples, PFETriple{
			ID:      p.ID,
			Problem: p.Text,
		})
	}
}

// mergeFeaturesFromState 从 StateKeyExtractFeatures 读取并解析特征列表。
func mergeFeaturesFromState(ext *ExtractionResult, state graph.PregelState) {
	raw, _ := state[StateKeyExtractFeatures].(string)
	if raw == "" {
		return
	}
	if !json.Valid([]byte(raw)) {
		return
	}

	var parsed struct {
		Features []struct {
			ID             string   `json:"id"`
			Description    string   `json:"description"`
			Category       string   `json:"category"`
			Function       string   `json:"function"`
			PriorArtStatus string   `json:"prior_art_status"`
			Importance     string   `json:"importance"`
			Confidence     float64  `json:"confidence"`
			Solves         []string `json:"solves"`
		} `json:"features"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return
	}

	for _, f := range parsed.Features {
		feature := TechFeature{
			ID:             f.ID,
			Description:    f.Description,
			Category:       TechFeatureCategory(f.Category),
			Function:       f.Function,
			PriorArtStatus: f.PriorArtStatus,
			Importance:     f.Importance,
			Confidence:     f.Confidence,
		}
		ext.Features = append(ext.Features, feature)

		// 将 feature 关联到对应的 PFE 三元组（索引访问，非值拷贝）
		for i := range ext.PFETriples {
			for _, pid := range f.Solves {
				if ext.PFETriples[i].ID == pid {
					ext.PFETriples[i].FeatureIDs = append(ext.PFETriples[i].FeatureIDs, f.ID)
				}
			}
		}
	}
}

// mergeEffectsFromState 从 StateKeyExtractEffects 读取并解析效果列表。
func mergeEffectsFromState(ext *ExtractionResult, state graph.PregelState) {
	raw, _ := state[StateKeyExtractEffects].(string)
	if raw == "" {
		return
	}
	if !json.Valid([]byte(raw)) {
		return
	}

	var parsed struct {
		Effects []struct {
			ID         string   `json:"id"`
			Text       string   `json:"text"`
			From       []string `json:"from"`
			Confidence float64  `json:"confidence"`
		} `json:"effects"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return
	}

	for _, e := range parsed.Effects {
		ext.Effects = append(ext.Effects, e.Text)

		// 将 effect 关联到对应的 PFE 三元组（索引访问，非值拷贝）
		for i := range ext.PFETriples {
			for _, fid := range e.From {
				for _, tfid := range ext.PFETriples[i].FeatureIDs {
					if fid == tfid {
						// 已有 Effect 则不覆盖，追加描述
						if ext.PFETriples[i].Effect == "" {
							ext.PFETriples[i].Effect = e.Text
						} else if ext.PFETriples[i].Effect != e.Text {
							ext.PFETriples[i].Effect += "；" + e.Text
						}
						// 找到匹配后继续检查此 fid 是否匹配其他 FeatureID
					}
				}
			}
		}
	}

	// 校验：孤立效果
	for _, e := range parsed.Effects {
		found := false
		for _, t := range ext.PFETriples {
			for _, fid := range e.From {
				for _, tfid := range t.FeatureIDs {
					if fid == tfid {
						found = true
						break
					}
				}
			}
		}
		if !found && len(ext.PFETriples) > 0 {
			// 孤立效果（直接 add，不在 PFE 中）
			ext.Effects = append(ext.Effects, e.Text+" [未关联特征]")
		}
	}
}

// linkPFETriples 构建 PFE 三元组的交叉引用。
// 此时所有数据已在同一 ExtractionResult 中，不存在跨 goroutine 的引用问题。
func linkPFETriples(ext *ExtractionResult) {
	// 如果 PFE 三元组为空但有问题和特征，创建基础三元组
	if len(ext.PFETriples) == 0 && len(ext.Problems) > 0 {
		for i, p := range ext.Problems {
			ext.PFETriples = append(ext.PFETriples, PFETriple{
				ID:      fmt.Sprintf("T%d", i+1),
				Problem: p,
			})
		}
	}
}

// =============================================================================
// Pregel 图构建
// =============================================================================

// GraphOption 可选地配置 disclosure 图的依赖（如 retrieve_prior_art 的检索器）。
// 采用 functional option 模式，使无检索器的调用点零破坏，有检索器的调用点注入。
type GraphOption func(*graphConfig)

type graphConfig struct {
	retriever domain.DomainRetriever
}

// WithRetriever 注入专利领域检索器，启用 retrieve_prior_art 节点的真实检索。
// 未注入时该节点标记 evidence_coverage=none，check_novelty 据此降级。
func WithRetriever(r domain.DomainRetriever) GraphOption {
	return func(c *graphConfig) { c.retriever = r }
}

// BuildDisclosureAnalysisGraph 构建技术交底书分析的 Pregel 图（无检索器，兼容旧调用）。
// 返回编译后的图，入口节点为 "preprocess"。
//
// 图拓扑：
//
//	preprocess → extract_problem, extract_features, extract_effects (并行)
//	extract_* → merge_extractions (单一合并节点)
//	merge_extractions → check_consistency
//	check_consistency → [retry: extract_*] or [continue: generate_keywords]
//	generate_keywords → retrieve_prior_art → check_novelty → generate_report → review_gate → __end__
func BuildDisclosureAnalysisGraph(provider agentcore.Provider) (*graph.CompiledPregelGraph, error) {
	return BuildDisclosureAnalysisGraphWithOpts(provider)
}

// BuildDisclosureAnalysisGraphWithOpts 构建带可选依赖的 disclosure 图。
func BuildDisclosureAnalysisGraphWithOpts(provider agentcore.Provider, opts ...GraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &graphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	pg := graph.NewPregelGraph()

	// 注册节点
	if err := pg.AddNode("preprocess", preprocessNode()); err != nil {
		return nil, err
	}
	if err := pg.AddNode("extract_problem", newExtractionAgent(provider, extractProblems, StateKeyExtractProblem)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("extract_features", newExtractionAgent(provider, extractFeatures, StateKeyExtractFeatures)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("extract_effects", newExtractionAgent(provider, extractEffects, StateKeyExtractEffects)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("merge_extractions", mergeExtractionsNode()); err != nil {
		return nil, err
	}
	if err := pg.AddNode("check_consistency", consistencyCheckNode()); err != nil {
		return nil, err
	}
	if err := pg.AddNode("generate_keywords", generateKeywordsNode()); err != nil {
		return nil, err
	}
	if err := pg.AddNode("retrieve_prior_art", retrievePriorArtNode(cfg.retriever)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("check_novelty", noveltyNode(provider)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("generate_report", generateReportNode(provider)); err != nil {
		return nil, err
	}
	if err := pg.AddNode("review_gate", reviewGateNode()); err != nil {
		return nil, err
	}
	if err := pg.AddNode("draft_claims", draftClaimsNode()); err != nil {
		return nil, err
	}

	// 静态边：preprocess → 三提取节点（并发）
	for _, edge := range [][2]string{
		{"preprocess", "extract_problem"},
		{"preprocess", "extract_features"},
		{"preprocess", "extract_effects"},
		{"extract_problem", "merge_extractions"},
		{"extract_features", "merge_extractions"},
		{"extract_effects", "merge_extractions"},
		{"merge_extractions", "check_consistency"},
		{"generate_keywords", "retrieve_prior_art"},
		{"retrieve_prior_art", "check_novelty"},
		{"check_novelty", "generate_report"},
		{"generate_report", "review_gate"},
		{"review_gate", "draft_claims"},
		{"draft_claims", graph.PregelEnd},
	} {
		if err := pg.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	// 条件边：一致性校验 → 前进或回退
	if err := pg.SetConditionalEdge("check_consistency", consistencyRouter); err != nil {
		return nil, err
	}

	return pg.Compile("preprocess", 100)
}
