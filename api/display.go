package api

// DisplayValue is one effective display-only value with its configuration provenance.
type DisplayValue struct {
	Value       any    `json:"value"`
	Scope       string `json:"scope"`
	File        string `json:"file,omitempty"`
	Application string `json:"application"`
}

// DisplaySettings is the owner-scoped settings.get reply for one presentation registration.
type DisplaySettings struct {
	RegistrationID     string                  `json:"registrationId"`
	Owner              string                  `json:"owner"`
	Key                string                  `json:"key"`
	ConfigRevision     uint64                  `json:"configRevision"`
	AppliesAfterReload bool                    `json:"appliesAfterReload"`
	Fields             map[string]DisplayValue `json:"fields"`
}
