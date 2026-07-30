// Package iface 为 domain 层定义基础设施契约。
// domain 子包通过此包调用 disclosure 等功能，不直接依赖具体实现包。
package iface

import "github.com/xujian519/mady/agentcore"

// =============================================================================
// TechFeature — 最小技术单元
// =============================================================================

// TechFeature 是最小技术单元——不可再分的原子技术手段。
// 这是 disclosure.TechFeature 的领域安全版本，仅含 domain 层实际使用的字段。
type TechFeature struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	Category       string `json:"category"` // structure / method / parameter / material
	Function       string `json:"function"`
	Importance     string `json:"importance"`       // high / medium / low
	PriorArtStatus string `json:"prior_art_status"` // known / unknown / partial
}

// =============================================================================
// PFETriple — 问题-特征-效果因果链
// =============================================================================

// PFETriple 是问题-特征-效果的因果关系链。
// 这是 disclosure.PFETriple 的领域安全版本。
type PFETriple struct {
	ID         string   `json:"id"`
	Problem    string   `json:"problem"`
	FeatureIDs []string `json:"feature_ids"`
	Effect     string   `json:"effect"`
}

// =============================================================================
// ExtractionResult — 提取结果
// =============================================================================

// ExtractionResult 聚合三个提取节点的输出。
// 这是 disclosure.ExtractionResult 的领域安全版本。
type ExtractionResult struct {
	Problems   []string      `json:"problems"`
	Features   []TechFeature `json:"features"`
	Effects    []string      `json:"effects"`
	PFETriples []PFETriple   `json:"pfe_triples"`
}

// =============================================================================
// DisclosureDoc — 交底书文档元信息
// =============================================================================

// DisclosureDoc 表示输入文档的元信息（domain 层关心的字段子集）。
type DisclosureDoc struct {
	HasDrawings bool              `json:"has_drawings"`
	Sections    map[string]string `json:"sections"`
}

// =============================================================================
// NoveltyResult — 新颖性初判结论
// =============================================================================

// NoveltyResult 携带新颖性初判结论（domain 层关心的字段子集）。
type NoveltyResult struct {
	Conclusion string `json:"conclusion"`
}

// =============================================================================
// AnalysisReport — 交底书全部分析报告
// =============================================================================

// AnalysisReport 汇总 disclosure 管线输出中 domain 层需要的数据。
// 这是 disclosure.AnalysisReport 的领域安全子集。
type AnalysisReport struct {
	Document   *DisclosureDoc    `json:"document"`
	Extraction *ExtractionResult `json:"extraction"`
	Novelty    *NoveltyResult    `json:"novelty"`
}

// =============================================================================
// EvidenceChunk — 现有技术证据片段
// =============================================================================

// EvidenceChunk 是检索到的现有技术证据片段。
// 这是 disclosure.EvidenceChunk 的领域安全版本。
type EvidenceChunk struct {
	DocID   string  `json:"doc_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// =============================================================================
// DisclosureToolFactory — 工具工厂
// =============================================================================

// DisclosureToolFactory 是 analyze_disclosure Agent 工具的构造函数类型。
// 具体实现由 bootstrap 层注入（见 bootstrap/disclosure_adapter.go），
// domain 装配代码通过此类型引用而非直接导入 disclosure 包。
type DisclosureToolFactory func(provider agentcore.Provider) *agentcore.Tool
