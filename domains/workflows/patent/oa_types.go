package patent

// OaRejectionType mirrors rules.OaRejectionType for dependency inversion.
type OaRejectionType string

const (
	// OaNovelty indicates a novelty rejection (Article 22.2).
	OaNovelty OaRejectionType = "novelty"
	// OaInventiveness indicates an inventiveness rejection (Article 22.3).
	OaInventiveness OaRejectionType = "inventiveness"
	// OaClarity indicates a clarity rejection (Article 26.4).
	OaClarity OaRejectionType = "clarity"
	// OaSupport indicates a support rejection (Article 26.4).
	OaSupport OaRejectionType = "support"
	// OaScope indicates a scope rejection (Article 26.4).
	OaScope OaRejectionType = "scope"
	// OaDisclosure indicates an insufficient disclosure rejection (Article 26.3).
	OaDisclosure OaRejectionType = "disclosure"
	// OaFormal indicates a formal matters rejection.
	OaFormal OaRejectionType = "formal"
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
