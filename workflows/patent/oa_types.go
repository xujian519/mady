package patent

// OaRejectionType mirrors rules.OaRejectionType for dependency inversion.
type OaRejectionType string

const (
	OaNovelty       OaRejectionType = "novelty"
	OaInventiveness OaRejectionType = "inventiveness"
	OaClarity       OaRejectionType = "clarity"
	OaSupport       OaRejectionType = "support"
	OaScope         OaRejectionType = "scope"
	OaDisclosure    OaRejectionType = "disclosure"
	OaFormal        OaRejectionType = "formal"
)

// CitedReference represents a document cited in an office action.
type CitedReference struct {
	DocumentNumber string
	Relevancy      string
	ClaimsAffected []int
}

// ParsedOfficeAction represents a parsed OA notification (wraps rules.ParsedOfficeAction).
type ParsedOfficeAction struct {
	RejectionType     string
	Citations         []CitedReference
	AffectedClaims    []int
	ExaminerArguments []string
}
