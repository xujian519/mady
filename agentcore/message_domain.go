package agentcore

// message_domain.go defines structured message types for professional domain
// outputs (patent analysis, legal reasoning). These types bridge the gap between
// generic LLM text and TUI-rendered professional cards (evidence, conclusion,
// approval). They are carried via Message.Metadata["domain"] and rendered
// by tui/component/*_card.go components.

// DomainMessageType tags a message as a structured professional artifact.
type DomainMessageType string

const (
	// DomainMsgTypeEvidenceCard renders as an evidence card with source/direction/snippet.
	DomainMsgTypeEvidenceCard DomainMessageType = "evidence_card"
	// DomainMsgTypeConclusionCard renders as a conclusion card with confidence/evidence count.
	DomainMsgTypeConclusionCard DomainMessageType = "conclusion_card"
	// DomainMsgTypeApprovalPrompt renders as an approval gate card with actions.
	DomainMsgTypeApprovalPrompt DomainMessageType = "approval_prompt"
)

// EvidenceDirection describes the relationship between evidence and a claim.
type EvidenceDirection string

const (
	DirectionSupporting    EvidenceDirection = "supporting"
	DirectionContradicting EvidenceDirection = "contradicting"
	DirectionNeutral       EvidenceDirection = "neutral"
)

// EvidenceRef is a lightweight reference to a piece of evidence, avoiding
// an import cycle with the evidence sub-package.
type EvidenceRef struct {
	Snippet   string            `json:"snippet,omitempty"`
	SourceURI string            `json:"source_uri,omitempty"`
	PageRange string            `json:"page_range,omitempty"`
	Direction EvidenceDirection `json:"direction"`
}

// DomainAction describes an action the user can take on a domain message.
type DomainAction struct {
	Label   string `json:"label"`   // e.g. "采纳", "拒绝"
	Command string `json:"command"` // e.g. "/approve", "/reject"
}

// DomainMessage carries structured content for professional card rendering.
// It is embedded in Message.Metadata under the key "domain".
type DomainMessage struct {
	Type       DomainMessageType `json:"type"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Confidence float64           `json:"confidence,omitempty"` // 0.0–1.0
	Spans      []EvidenceRef     `json:"spans,omitempty"`
	Actions    []DomainAction    `json:"actions,omitempty"`
	// Extra carries type-specific fields (e.g. classification label).
	Extra map[string]string `json:"extra,omitempty"`
}

// SupportingSpans returns the count of supporting evidence refs.
func (d *DomainMessage) SupportingSpans() int {
	n := 0
	for _, s := range d.Spans {
		if s.Direction == DirectionSupporting {
			n++
		}
	}
	return n
}

// ContradictingSpans returns the count of contradicting evidence refs.
func (d *DomainMessage) ContradictingSpans() int {
	n := 0
	for _, s := range d.Spans {
		if s.Direction == DirectionContradicting {
			n++
		}
	}
	return n
}

// ConfidencePct returns a simple 0-100 integer percentage.
func (d *DomainMessage) ConfidencePct() int {
	return int(d.Confidence * 100)
}

// ConfidenceLevel returns "low", "medium", or "high".
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
