package host

import (
	"encoding/json"
	"fmt"

	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

func (a *app) displayChangeEvents(s *session, revision uint64, keys []string) ([]api.Event, *api.Error) {
	var events []api.Event
	for _, registration := range s.contributions.List() {
		if registration.Kind != contribution.KindToolPresentation {
			continue
		}
		prefix := parser.Key(displayNamespace(registration.Owner, registration.Key))
		changed := len(keys) == 0
		for _, key := range keys {
			parts, err := parser.ParseKey(key)
			if err == nil && prefix.IsPrefixOf(parts) {
				changed = true
			}
		}
		if !changed {
			continue
		}
		result, failure := a.displaySettingsGet(s, api.Request{RegistrationID: registration.RegistrationID, From: api.Identity{Owner: registration.Owner, Instance: registration.Instance}, Params: json.RawMessage(`{}`)})
		if failure != nil {
			return nil, failure
		}
		values := map[string]any{}
		for name, field := range result.(api.DisplaySettings).Fields {
			values[name] = field.Value
		}
		events = append(events, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: fmt.Sprintf("%s:display-settings:%d:%s", s.generation, revision, registration.RegistrationID), Type: "settings.changed", RegistrationID: registration.RegistrationID, Data: api.DisplaySettingsChanged{Owner: registration.Owner, Key: registration.Key, ConfigRevision: revision, Values: values}})
	}
	return events, nil
}

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
