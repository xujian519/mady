package component

// domain.go 定义面向专业卡片渲染的结构化领域消息类型。
//
// 这些类型原本位于 agentcore/message_domain.go，但其唯一消费者是 TUI 的
// 专业卡片渲染器（evidence_card / conclusion_card / approval_prompt）。
// 为遵循 tui/LAYERS.md 的分层约束（tui/component 不依赖 agentcore），
// 类型定义下沉到本包，agentcore 不再持有 TUI 专用数据模型。
//
// DomainMessage 目前由上层调用方直接构造并注入 chat.ChatMessage.DomainMsg；
// agentcore.Message.Metadata["domain"] 的 JSON 解码链路尚未实现。

// DomainMessageType 标记结构化专业消息的渲染类型。
type DomainMessageType string

const (
	// DomainMsgTypeEvidenceCard 渲染为带来源/方向/摘录的证据卡。
	DomainMsgTypeEvidenceCard DomainMessageType = "evidence_card"
	// DomainMsgTypeConclusionCard 渲染为带置信度/证据计数的结论卡。
	DomainMsgTypeConclusionCard DomainMessageType = "conclusion_card"
	// DomainMsgTypeApprovalPrompt 渲染为带操作按钮的审批门卡。
	DomainMsgTypeApprovalPrompt DomainMessageType = "approval_prompt"
)

// EvidenceDirection 描述证据与主张之间的关系方向。
type EvidenceDirection string

const (
	DirectionSupporting    EvidenceDirection = "supporting"
	DirectionContradicting EvidenceDirection = "contradicting"
	DirectionNeutral       EvidenceDirection = "neutral"
)

// EvidenceRef 是对单条证据的轻量引用，避免与 agentcore/evidence 子包形成循环依赖。
type EvidenceRef struct {
	Snippet   string            `json:"snippet,omitempty"`
	SourceURI string            `json:"source_uri,omitempty"`
	PageRange string            `json:"page_range,omitempty"`
	Direction EvidenceDirection `json:"direction"`
}

// DomainAction 描述用户可对领域消息执行的操作。
type DomainAction struct {
	Label   string `json:"label"`   // 例如 "采纳"、"拒绝"
	Command string `json:"command"` // 例如 "/approve"、"/reject"
}

// DomainMessage 承载面向专业卡片渲染的结构化内容。
// 在 agentcore.Message.Metadata 中以 "domain" 为键携带，由 TUI 适配层解码。
type DomainMessage struct {
	Type       DomainMessageType `json:"type"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Confidence float64           `json:"confidence,omitempty"` // 0.0–1.0
	Spans      []EvidenceRef     `json:"spans,omitempty"`
	Actions    []DomainAction    `json:"actions,omitempty"`
	// Extra 承载类型特定的附加字段（例如分类标签）。
	Extra map[string]string `json:"extra,omitempty"`
}

// SupportingSpans 返回支持方向的证据条数。
func (d *DomainMessage) SupportingSpans() int {
	n := 0
	for _, s := range d.Spans {
		if s.Direction == DirectionSupporting {
			n++
		}
	}
	return n
}

// ContradictingSpans 返回反对方向的证据条数。
func (d *DomainMessage) ContradictingSpans() int {
	n := 0
	for _, s := range d.Spans {
		if s.Direction == DirectionContradicting {
			n++
		}
	}
	return n
}

// ConfidencePct 返回 0–100 的整数百分比。
func (d *DomainMessage) ConfidencePct() int {
	return int(d.Confidence * 100)
}

// ConfidenceLevel 返回 "low"、"medium" 或 "high"。
func (d *DomainMessage) ConfidenceLevel() string {
	switch {
	case d.Confidence >= 0.67:
		return "high"
	case d.Confidence >= 0.34:
		return "medium"
	default:
		return "low"
	}
}
