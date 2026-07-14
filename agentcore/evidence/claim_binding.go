package evidence

import (
	"fmt"
	"sync"
)

// ClaimBinding links a claim (a statement or conclusion produced by the agent)
// to one or more EvidenceSpans that support or contradict it.
//
// A "claim" here is any substantive assertion the agent makes — e.g. "该特征
// 已被CN12345678A公开" — that requires evidentiary backing.
type ClaimBinding struct {
	mu sync.RWMutex

	// claims maps claimID → set of evidence span IDs.
	claims map[string]map[string]struct{}

	// spans holds all registered evidence spans by ID.
	spans map[string]EvidenceSpan
}

// NewClaimBinding creates an empty claim binding registry.
func NewClaimBinding() *ClaimBinding {
	return &ClaimBinding{
		claims: make(map[string]map[string]struct{}),
		spans:  make(map[string]EvidenceSpan),
	}
}

// RegisterSpan registers an evidence span and optionally links it to claims.
func (cb *ClaimBinding) RegisterSpan(span EvidenceSpan) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.spans[span.ID] = span
	for _, claimID := range span.ClaimRefs {
		if cb.claims[claimID] == nil {
			cb.claims[claimID] = make(map[string]struct{})
		}
		cb.claims[claimID][span.ID] = struct{}{}
	}
}

// LinkEvidence associates an existing evidence span with a claim.
func (cb *ClaimBinding) LinkEvidence(claimID, spanID string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if _, ok := cb.spans[spanID]; !ok {
		return fmt.Errorf("evidence span %q not found", spanID)
	}
	if cb.claims[claimID] == nil {
		cb.claims[claimID] = make(map[string]struct{})
	}
	cb.claims[claimID][spanID] = struct{}{}
	return nil
}

// GetEvidence returns all evidence spans linked to the given claim.
func (cb *ClaimBinding) GetEvidence(claimID string) []EvidenceSpan {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	spanIDs := cb.claims[claimID]
	if len(spanIDs) == 0 {
		return nil
	}

	out := make([]EvidenceSpan, 0, len(spanIDs))
	for id := range spanIDs {
		if s, ok := cb.spans[id]; ok {
			// 深拷贝 ClaimRefs 防止调用者修改共享底层数组
			if len(s.ClaimRefs) > 0 {
				s.ClaimRefs = append([]string(nil), s.ClaimRefs...)
			}
			out = append(out, s)
		}
	}
	return out
}

// GetClaims returns all registered claim IDs.
func (cb *ClaimBinding) GetClaims() []string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	out := make([]string, 0, len(cb.claims))
	for id := range cb.claims {
		out = append(out, id)
	}
	return out
}

// SpanCount returns the total number of registered evidence spans.
func (cb *ClaimBinding) SpanCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return len(cb.spans)
}

// UnbackedClaims returns all claims that have no supporting evidence.
func (cb *ClaimBinding) UnbackedClaims() []string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var out []string
	for claimID, spanIDs := range cb.claims {
		hasSupport := false
		for id := range spanIDs {
			if s, ok := cb.spans[id]; ok && s.Direction == DirectionSupporting {
				hasSupport = true
				break
			}
		}
		if !hasSupport {
			out = append(out, claimID)
		}
	}
	return out
}

// Clear resets all bindings and spans.
func (cb *ClaimBinding) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.claims = make(map[string]map[string]struct{})
	cb.spans = make(map[string]EvidenceSpan)
}
