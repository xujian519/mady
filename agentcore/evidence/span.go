package evidence

import "time"

// EvidenceSpan is a locatable piece of evidence sourced from a tool call,
// document read, or retrieval operation. It provides the provenance chain
// from a claim back to its original source.
//
// Every professional conclusion produced by the agent should be backed by
// one or more EvidenceSpans. An unbacked conclusion is explicitly flagged
// as "无证据支持" rather than presented as fact.
type EvidenceSpan struct {
	// ID is a unique identifier for this evidence span.
	ID string `json:"id"`

	// TurnID is the agent turn in which this evidence was collected.
	TurnID string `json:"turn_id,omitempty"`

	// ReceiptID is the receipt that produced this evidence (if created by a tool call).
	ReceiptID string `json:"receipt_id,omitempty"`

	// DocVersion identifies the document version from which this evidence was extracted.
	// Examples: "v1.0", "2026-07-14", content hash prefix.
	DocVersion string `json:"doc_version,omitempty"`

	// PageRange locates the evidence within the document, e.g. "第3页第15-20行".
	PageRange string `json:"page_range,omitempty"`

	// CharRange is the character offset range, e.g. "1200-1250".
	CharRange string `json:"char_range,omitempty"`

	// ContentHash is a hash of the original snippet for integrity verification.
	ContentHash string `json:"content_hash,omitempty"`

	// Snippet is the original text excerpt that supports or contradicts a claim.
	Snippet string `json:"snippet,omitempty"`

	// SourceURI is the URI from which the evidence was retrieved.
	// Examples: "file:///path/to/doc.pdf", "patent:CN12345678A", "web:https://..."
	SourceURI string `json:"source_uri,omitempty"`

	// RetrievalAt is when the evidence was collected.
	// Stored as RFC3339 string via custom marshaling; omitempty works for zero time.
	RetrievalAt time.Time `json:"retrieval_at,omitempty"`

	// Direction indicates whether this evidence supports, contradicts, or is
	// neutral toward the referenced claim.
	Direction EvidenceDirection `json:"direction"`

	// ClaimRefs are the claim IDs that this evidence supports or contradicts.
	ClaimRefs []string `json:"claim_refs,omitempty"`
}

// EvidenceDirection describes the relationship between evidence and a claim.
type EvidenceDirection string

const (
	DirectionSupporting    EvidenceDirection = "supporting"
	DirectionContradicting EvidenceDirection = "contradicting"
	DirectionNeutral       EvidenceDirection = "neutral"
)

// Valid returns true if the direction is a known value.
func (d EvidenceDirection) Valid() bool {
	switch d {
	case DirectionSupporting, DirectionContradicting, DirectionNeutral:
		return true
	default:
		return false
	}
}
