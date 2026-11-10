package host

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"sort"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
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

// injectedEnvironment returns NAME=value pairs for vault secrets bound to an environment name in
// the session's project or global scope, or none while secrets.env-injection is off. The host process environment is never changed.
func (b *secretsBoundary) injectedEnvironment(projectID string) ([]string, error) {
	if _, enabled, err := b.Guard(); err != nil || !enabled {
		return nil, err
	}
	if snapshot, err := b.settings.Current(); err == nil {
		if entry, ok := snapshot.Get("secrets.env-injection"); ok && entry.Value == false {
			return nil, nil
		}
	}
	b.mu.Lock()
	vault := b.vault
	b.mu.Unlock()
	records, err := vault.Load()
	if err != nil {
		return nil, err
	}
	values, _, err := vault.ReadableValues()
	if err != nil {
		return nil, err
	}
	chosen := secrets.EnvInjection(records, values, projectID)
	names := make([]string, 0, len(chosen))
	for name := range chosen {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+chosen[name])
	}
	return environment, nil
}

// jobEnvironment is the full environment of a bash call: the host's plus injected secrets.
func (s *session) jobEnvironment(ctx context.Context) ([]string, error) {
	projectID := ""
	if identity := s.projectIdentity(ctx); identity != nil {
		projectID = identity.ID
	}
	injected, err := s.secrets.injectedEnvironment(projectID)
	if err != nil || len(injected) == 0 {
		return nil, err
	}
	return append(os.Environ(), injected...), nil
}
