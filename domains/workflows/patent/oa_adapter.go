package patent

import "github.com/xujian519/mady/domains/rules"

// ParseOA wraps rules.ParseOfficeAction to return local types.
func ParseOA(text string) ParsedOfficeAction {
	return toLocalParsed(rules.ParseOfficeAction(text))
}

// ToRulesRejectionType converts a local OaRejectionType to the rules package type.
func ToRulesRejectionType(t OaRejectionType) rules.OaRejectionType {
	return rules.OaRejectionType(t)
}

// FormatOaSummary wraps rules.FormatOaSummary for the local ParsedOfficeAction type.
func FormatOaSummary(oa ParsedOfficeAction) string {
	return rules.FormatOaSummary(toRulesParsed(oa))
}

// toLocalParsed converts the rules-level parse result into the local (decoupled)
// ParsedOfficeAction, mirroring the types field by field.
func toLocalParsed(parsed rules.ParsedOfficeAction) ParsedOfficeAction {
	citations := make([]CitedReference, len(parsed.Citations))
	for i, c := range parsed.Citations {
		citations[i] = CitedReference{
			DocumentNumber: c.DocumentNumber,
			Relevancy:      c.Relevancy,
			ClaimsAffected: c.ClaimsAffected,
		}
	}
	rejectionTypes := make([]OaRejectionType, len(parsed.RejectionTypes))
	for i, rt := range parsed.RejectionTypes {
		rejectionTypes[i] = OaRejectionType(rt)
	}
	return ParsedOfficeAction{
		RejectionType:     string(parsed.RejectionType),
		RejectionTypes:    rejectionTypes,
		Citations:         citations,
		AffectedClaims:    parsed.AffectedClaims,
		ExaminerArguments: parsed.ExaminerArguments,
	}
}

// toRulesParsed converts the local ParsedOfficeAction back to the rules-level
// type for formatting and other rules-package entry points.
func toRulesParsed(oa ParsedOfficeAction) rules.ParsedOfficeAction {
	citations := make([]rules.CitedReference, len(oa.Citations))
	for i, c := range oa.Citations {
		citations[i] = rules.CitedReference{
			DocumentNumber: c.DocumentNumber,
			Relevancy:      c.Relevancy,
			ClaimsAffected: c.ClaimsAffected,
		}
	}
	rejectionTypes := make([]rules.OaRejectionType, len(oa.RejectionTypes))
	for i, rt := range oa.RejectionTypes {
		rejectionTypes[i] = rules.OaRejectionType(rt)
	}
	return rules.ParsedOfficeAction{
		RejectionType:     rules.OaRejectionType(oa.RejectionType),
		RejectionTypes:    rejectionTypes,
		Citations:         citations,
		AffectedClaims:    oa.AffectedClaims,
		ExaminerArguments: oa.ExaminerArguments,
	}
}
