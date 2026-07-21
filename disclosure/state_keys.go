package disclosure

import "github.com/xujian519/mady/graph"

// =============================================================================
// 类型安全 StateKey 访问器
// =============================================================================
//
// 本文件为 disclosure 管线的 PregelState 提供类型安全的键值访问函数。
// 底层存储仍是 graph.PregelState (map[string]any)，但通过这些访问器
// 可在编译时捕获类型不匹配，代替手动的类型断言。
//
// 使用示例：
//
//	var state graph.PregelState
//	doc, ok := GetExtraction(state)
//	SetSearchKeywords(state, []string{"关键词1", "关键词2"})

// GetDoc 从 state 中读取 DisclosureDoc。
func GetDoc(state graph.PregelState) (*DisclosureDoc, bool) {
	raw, ok := state[StateKeyDoc]
	if !ok {
		return nil, false
	}
	doc, ok := raw.(*DisclosureDoc)
	return doc, ok
}

// SetDoc 将 DisclosureDoc 写入 state。
func SetDoc(state graph.PregelState, doc *DisclosureDoc) {
	state[StateKeyDoc] = doc
}

// GetExtraction 从 state 中读取 ExtractionResult。
func GetExtraction(state graph.PregelState) (*ExtractionResult, bool) {
	raw, ok := state[StateKeyExtraction]
	if !ok {
		return nil, false
	}
	ext, ok := raw.(*ExtractionResult)
	return ext, ok
}

// SetExtraction 将 ExtractionResult 写入 state。
func SetExtraction(state graph.PregelState, ext *ExtractionResult) {
	state[StateKeyExtraction] = ext
}

// GetConsistency 从 state 中读取 ConsistencyResult。
func GetConsistency(state graph.PregelState) (*ConsistencyResult, bool) {
	raw, ok := state[StateKeyConsistency]
	if !ok {
		return nil, false
	}
	cr, ok := raw.(*ConsistencyResult)
	return cr, ok
}

// SetConsistency 将 ConsistencyResult 写入 state。
func SetConsistency(state graph.PregelState, cr *ConsistencyResult) {
	state[StateKeyConsistency] = cr
}

// GetSearchKeywords 从 state 中读取关键词列表。
func GetSearchKeywords(state graph.PregelState) ([]string, bool) {
	raw, ok := state[StateKeySearchKeywords]
	if !ok {
		return nil, false
	}
	kw, ok := raw.([]string)
	return kw, ok
}

// SetSearchKeywords 将关键词列表写入 state。
func SetSearchKeywords(state graph.PregelState, keywords []string) {
	state[StateKeySearchKeywords] = keywords
}

// GetNovelty 从 state 中读取 NoveltyResult。
func GetNovelty(state graph.PregelState) (*NoveltyResult, bool) {
	raw, ok := state[StateKeyNovelty]
	if !ok {
		return nil, false
	}
	nr, ok := raw.(*NoveltyResult)
	return nr, ok
}

// SetNovelty 将 NoveltyResult 写入 state。
func SetNovelty(state graph.PregelState, nr *NoveltyResult) {
	state[StateKeyNovelty] = nr
}

// GetReport 从 state 中读取 AnalysisReport。
func GetReport(state graph.PregelState) (*AnalysisReport, bool) {
	raw, ok := state[StateKeyReport]
	if !ok {
		return nil, false
	}
	rpt, ok := raw.(*AnalysisReport)
	return rpt, ok
}

// SetReport 将 AnalysisReport 写入 state。
func SetReport(state graph.PregelState, rpt *AnalysisReport) {
	state[StateKeyReport] = rpt
}

// GetOutput 从 state 中读取输出文本。
func GetOutput(state graph.PregelState) string {
	return state.GetString(StateKeyOutput)
}

// SetOutput 将输出文本写入 state。
func SetOutput(state graph.PregelState, output string) {
	state[StateKeyOutput] = output
}

// GetEvidence 从 state 中读取证据片段列表。
func GetEvidence(state graph.PregelState) ([]EvidenceChunk, bool) {
	raw, ok := state[StateKeyEvidence]
	if !ok {
		return nil, false
	}
	chunks, ok := raw.([]EvidenceChunk)
	return chunks, ok
}

// SetEvidence 将证据片段列表写入 state。
func SetEvidence(state graph.PregelState, chunks []EvidenceChunk) {
	state[StateKeyEvidence] = chunks
}

// GetEvidenceCoverage 从 state 中读取证据覆盖度。
func GetEvidenceCoverage(state graph.PregelState) string {
	v, _ := state[StateKeyEvidenceCoverage].(string)
	return v
}

// SetEvidenceCoverage 将证据覆盖度写入 state。
func SetEvidenceCoverage(state graph.PregelState, coverage string) {
	state[StateKeyEvidenceCoverage] = coverage
}

// GetDraftClaims 从 state 中读取权利要求草稿。
func GetDraftClaims(state graph.PregelState) (string, bool) {
	raw, ok := state[StateKeyDraftClaims]
	if !ok {
		return "", false
	}
	claims, ok := raw.(string)
	return claims, ok
}

// SetDraftClaims 将权利要求草稿写入 state。
func SetDraftClaims(state graph.PregelState, claims string) {
	state[StateKeyDraftClaims] = claims
}

// GetRetryCount 从 state 中读取重试计数。
func GetRetryCount(state graph.PregelState) int {
	v, _ := state[StateKeyRetryCount].(int)
	return v
}

// SetRetryCount 将重试计数写入 state。
func SetRetryCount(state graph.PregelState, count int) {
	state[StateKeyRetryCount] = count
}

// GetRetryFeedback 从 state 中读取重试反馈。
func GetRetryFeedback(state graph.PregelState) string {
	return state.GetString(StateKeyRetryFeedback)
}

// SetRetryFeedback 将重试反馈写入 state。
func SetRetryFeedback(state graph.PregelState, feedback string) {
	state[StateKeyRetryFeedback] = feedback
}
