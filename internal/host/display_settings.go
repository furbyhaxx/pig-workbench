package host

import (
	"encoding/json"

	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

func (a *app) displaySettingsGet(s *session, request api.Request) (any, *api.Error) {
	if request.RegistrationID == "" {
		return nil, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "settings.get requires a presentation registration"}
	}
	var params struct{}
	if api.Decode(request.Params, &params) != nil {
		return nil, &api.Error{Code: "invalidRequest", Message: "settings.get takes empty params"}
	}
	registration, err := s.contributions.Get(request.From, request.RegistrationID)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if registration.Kind != contribution.KindToolPresentation {
		return nil, &api.Error{Code: "unsupported", Message: "settings.get exposes presentation display values only"}
	}
	descriptor, err := presentation.DecodeDescriptor(registration.Descriptor)
	if err != nil {
		return nil, protocolError(err)
	}
	definitions, err := displayDefinitions(registration.Owner, registration.Key, descriptor)
	if err != nil {
		return nil, protocolError(err)
	}
	snapshot, err := s.settings.Current()
	if err != nil {
		return nil, protocolError(err)
	}
	result := api.DisplaySettings{RegistrationID: registration.RegistrationID, Owner: registration.Owner, Key: registration.Key, ConfigRevision: snapshot.Revision(), AppliesAfterReload: s.presentations.appliesAfterReload(registration.RegistrationID), Fields: make(map[string]api.DisplayValue)}
	for _, definition := range definitions {
		entry, ok := snapshot.Get(definition.Key)
		if !ok {
			return nil, &api.Error{Code: "unavailable", Message: "display schema is unavailable; reconcile the registration"}
		}
		parts, _ := parser.ParseKey(definition.Key)
		result.Fields[parts[3]] = api.DisplayValue{Value: entry.Value, Scope: string(entry.Scope), File: entry.File, Application: entry.Application}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 48<<10 {
		return nil, &api.Error{Code: "limitExceeded", Message: "display settings exceed the 48 KiB reply budget"}
	}
	return result, nil
}
