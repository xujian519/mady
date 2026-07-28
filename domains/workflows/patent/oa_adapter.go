package patent

import "github.com/xujian519/mady/domains/rules"

// ParseOA wraps rules.ParseOfficeAction to return local types.
func ParseOA(text string) ParsedOfficeAction {
	parsed := rules.ParseOfficeAction(text)
	citations := make([]CitedReference, len(parsed.Citations))
	for i, c := range parsed.Citations {
		citations[i] = CitedReference{
			DocumentNumber: c.DocumentNumber,
			Relevancy:      c.Relevancy,
			ClaimsAffected: c.ClaimsAffected,
		}
	}
	return ParsedOfficeAction{
		RejectionType:     string(parsed.RejectionType),
		Citations:         citations,
		AffectedClaims:    parsed.AffectedClaims,
		ExaminerArguments: parsed.ExaminerArguments,
	}
}

// ToRulesRejectionType converts a local OaRejectionType to the rules package type.
func ToRulesRejectionType(t OaRejectionType) rules.OaRejectionType {
	return rules.OaRejectionType(t)
}

// FormatOaSummary wraps rules.FormatOaSummary for the local ParsedOfficeAction type.
func FormatOaSummary(oa ParsedOfficeAction) string {
	// Convert local type back to rules type for formatting.
	citations := make([]rules.CitedReference, len(oa.Citations))
	for i, c := range oa.Citations {
		citations[i] = rules.CitedReference{
			DocumentNumber: c.DocumentNumber,
			Relevancy:      c.Relevancy,
			ClaimsAffected: c.ClaimsAffected,
		}
	}
	ruleOA := rules.ParsedOfficeAction{
		RejectionType:     rules.OaRejectionType(oa.RejectionType),
		Citations:         citations,
		AffectedClaims:    oa.AffectedClaims,
		ExaminerArguments: oa.ExaminerArguments,
	}
	return rules.FormatOaSummary(ruleOA)
}
