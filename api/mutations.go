package api

// ResourceMutation binds a mutation to router-issued context and exact preconditions.
type ResourceMutation struct {
	IdempotencyKey          string  `json:"idempotencyKey"`
	AuthorizationRef        string  `json:"authorizationRef"`
	LocationRevision        uint64  `json:"locationRevision"`
	ExpectedRevision        string  `json:"expectedRevision,omitempty"`
	MustNotExist            bool    `json:"mustNotExist,omitempty"`
	DestinationURI          string  `json:"destinationUri,omitempty"`
	DestinationMustNotExist bool    `json:"destinationMustNotExist,omitempty"`
	Content                 *string `json:"content,omitempty"`
}

// MutationEffect records one known target outcome without implying rollback.
type MutationEffect struct {
	URI            string `json:"uri"`
	Status         string `json:"status"`
	BeforeRevision string `json:"beforeRevision,omitempty"`
	AfterRevision  string `json:"afterRevision,omitempty"`
}

// MutationReceipt reports committed, notApplied, partial or unknown mutation effects.
type MutationReceipt struct {
	OperationID      string           `json:"operationId"`
	IdempotencyKey   string           `json:"idempotencyKey"`
	Status           string           `json:"status"`
	CanonicalURI     string           `json:"canonicalUri"`
	Effects          []MutationEffect `json:"effects"`
	ChangedResources []string         `json:"changedResources"`
	Warnings         []string         `json:"warnings,omitempty"`
}
