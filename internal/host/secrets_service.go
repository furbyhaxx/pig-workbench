package host

import (
	"encoding/json"
	"slices"

	"github.com/furbyhaxx/pig-workbench/api"
)

// redactOwners reads the user-only `secrets.redact-owners` grant.
func (b *secretsBoundary) redactOwners() []string {
	snapshot, err := b.settings.Current()
	if err != nil {
		return nil
	}
	entry, ok := snapshot.Get("secrets.redact-owners")
	owners, _ := entry.Value.([]string)
	if !ok {
		return nil
	}
	return owners
}

// granted reports whether owner may call secrets.redact; the bus cannot authenticate the claim.
func (b *secretsBoundary) granted(owner string) bool {
	return owner != "" && slices.Contains(b.redactOwners(), owner)
}

// secretsRedact serves secrets.redact for a granted owner. Failure never returns the input.
func (a *app) secretsRedact(s *session, request api.Request) (any, *api.Error) {
	if !s.secrets.granted(request.From.Owner) {
		return nil, &api.Error{Code: "permissionDenied", Message: "secrets.redact is not granted to this owner", Outcome: "notApplied"}
	}
	var params api.SecretsRedactParams
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "secrets.redact expects {text}", Outcome: "notApplied"}
	}
	redacted, active, err := s.secrets.redactText(params.Text)
	switch {
	case err != nil:
		return nil, &api.Error{Code: "internal", Message: "the secrets boundary is unavailable; withhold the request", Outcome: "notApplied"}
	case !active:
		return nil, &api.Error{Code: "unsupported", Message: "secret detection is disabled; withhold the request or choose an unprotected mode", Outcome: "notApplied"}
	case encodedLen(redacted) > api.ControlBytes-envelopeSlack:
		return nil, &api.Error{Code: "limitExceeded", Field: "params.text", Message: "the redacted text does not fit the control envelope", Outcome: "notApplied"}
	}
	return api.SecretsRedactResult{Text: redacted, Enabled: true}, nil
}

// envelopeSlack leaves room for the response envelope around the text.
const envelopeSlack = 2048

func encodedLen(text string) int {
	encoded, _ := json.Marshal(text)
	return len(encoded)
}
