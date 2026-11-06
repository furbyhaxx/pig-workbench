package api

// SecretsRedactParams is the closed request of secrets.redact.
type SecretsRedactParams struct {
	Text string `json:"text"`
}

// SecretsRedactResult is the redacted text; Enabled is always true on success.
type SecretsRedactResult struct {
	Text    string `json:"text"`
	Enabled bool   `json:"enabled"`
}
