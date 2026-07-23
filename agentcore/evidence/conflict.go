package evidence

// Conflict represents a detected contradiction between two or more evidence spans.
// Conflicts must be surfaced to the human reviewer before a conclusion can be
// accepted, ensuring that contradictory evidence is not silently ignored.
type Conflict struct {
	// Description is a human-readable explanation of the conflict.
	Description string `json:"description"`

	// SpanIDs are the evidence span IDs involved in the conflict.
	SpanIDs []string `json:"span_ids"`

	// Type classifies the nature of the conflict.
	Type ConflictType `json:"type"`
}

// ConflictType describes the category of evidence conflict.
type ConflictType string

const (
	// ConflictDirection occurs when two evidence spans have opposite
	// directions for the same claim (one supporting, one contradicting).
	ConflictDirection ConflictType = "direction"

	// ConflictSource occurs when two evidence spans cite the same source
	// document but with contradictory snippets or facts.
	ConflictSource ConflictType = "source"

	// ConflictTimeBoundary occurs when evidence references information
	// outside its valid temporal range (e.g. a prior-art reference that
	// post-dates the filing date).
	//
	// Note: This conflict type is defined for API completeness but is not
	// currently produced by Detect(). It requires a date-aware evidence
	// pipeline that is not yet implemented.
	ConflictTimeBoundary ConflictType = "time_boundary"
)

// ConflictDetector examines evidence spans and claim bindings for
// contradictions. It is used after evidence collection to flag issues
// that require human review.
type ConflictDetector struct {
	binding *ClaimBinding
}

// NewConflictDetector creates a conflict detector backed by the given binding.
func NewConflictDetector(binding *ClaimBinding) *ConflictDetector {
	return &ConflictDetector{binding: binding}
}

// Detect runs all conflict checks and returns any detected conflicts.
func (cd *ConflictDetector) Detect() []Conflict {
	if cd.binding == nil {
		return nil
	}
	var conflicts []Conflict
	conflicts = append(conflicts, cd.checkDirectionConflicts()...)
	conflicts = append(conflicts, cd.checkSourceConflicts()...)
	return conflicts
}

// checkDirectionConflicts finds claims backed by both supporting and
// contradicting evidence.
func (cd *ConflictDetector) checkDirectionConflicts() []Conflict {
	var conflicts []Conflict
	for _, claimID := range cd.binding.GetClaims() {
		spans := cd.binding.GetEvidence(claimID)
		var supporting, contradicting []string
		for _, s := range spans {
			switch s.Direction {
			case DirectionSupporting:
				supporting = append(supporting, s.ID)
			case DirectionContradicting:
				contradicting = append(contradicting, s.ID)
			}
		}
		if len(supporting) > 0 && len(contradicting) > 0 {
			allIDs := append(append([]string{}, supporting...), contradicting...)
			conflicts = append(conflicts, Conflict{
				Description: "主张「" + claimID + "」同时有支持性和矛盾性证据",
				SpanIDs:     allIDs,
				Type:        ConflictDirection,
			})
		}
	}
	return conflicts
}

// checkSourceConflicts finds evidence spans from the same source that
// have contradictory content.
func (cd *ConflictDetector) checkSourceConflicts() []Conflict {
	// Group spans by source URI, then check for direction conflicts
	// within the same source.
	type sourceGroup struct {
		supporting    []string
		contradicting []string
	}
	sources := make(map[string]*sourceGroup)

	for _, claimID := range cd.binding.GetClaims() {
		for _, span := range cd.binding.GetEvidence(claimID) {
			if span.SourceURI == "" {
				continue
			}
			if sources[span.SourceURI] == nil {
				sources[span.SourceURI] = &sourceGroup{}
			}
			sg := sources[span.SourceURI]
			switch span.Direction {
			case DirectionSupporting:
				sg.supporting = append(sg.supporting, span.ID)
			case DirectionContradicting:
				sg.contradicting = append(sg.contradicting, span.ID)
			}
		}
	}

	var conflicts []Conflict
	for uri, sg := range sources {
		if len(sg.supporting) > 0 && len(sg.contradicting) > 0 {
			allIDs := append(append([]string{}, sg.supporting...), sg.contradicting...)
			conflicts = append(conflicts, Conflict{
				Description: "同一来源「" + uri + "」同时包含支持和矛盾证据",
				SpanIDs:     allIDs,
				Type:        ConflictSource,
			})
		}
	}
	return conflicts
}
