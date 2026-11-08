package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

const displaySchemaEntry = "pig-workbench.display-schema"

type storedDisplaySchema struct {
	Version    int             `json:"version"`
	Owner      string          `json:"owner"`
	Key        string          `json:"key"`
	Descriptor json.RawMessage `json:"descriptor"`
}

func retainedDisplaySchema(guard secrets.TextGuard, enabled bool, record storedDisplaySchema) (any, error) {
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded) > api.ControlBytes {
		return nil, errors.New("display schema replay metadata exceeds its budget")
	}
	var data any
	if json.Unmarshal(encoded, &data) != nil {
		return nil, errors.New("invalid display replay metadata")
	}
	if enabled {
		protected, err := secrets.RedactData(guard, data)
		if err != nil {
			return nil, err
		}
		before, _ := json.Marshal(data)
		after, _ := json.Marshal(protected)
		if !bytes.Equal(before, after) {
			return nil, errors.New("display replay metadata contains protected values and was not retained")
		}
	}
	return data, nil
}

func (a *app) retainDisplaySchema(ctx sdk.Context, s *session, request api.Request, result any) error {
	receipt, ok := result.(map[string]any)
	if !ok || receipt["idempotent"] == true {
		return nil
	}
	id, _ := receipt["registrationId"].(string)
	registration, err := s.contributions.Get(request.From, id)
	if err != nil {
		return err
	}
	if registration.Kind != contribution.KindToolPresentation {
		return nil
	}
	guard, enabled, err := s.secrets.Guard()
	if err != nil {
		return err
	}
	data, err := retainedDisplaySchema(guard, enabled, storedDisplaySchema{Version: 1, Owner: registration.Owner, Key: registration.Key, Descriptor: registration.Descriptor})
	if err != nil {
		return err
	}
	return ctx.AppendEntry(displaySchemaEntry, data)
}

func storedDisplaySchemas(ctx context.Context, entries []json.RawMessage, leaf *string) ([]storedDisplaySchema, error) {
	chain, err := storedBranch(ctx, entries, leaf)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var records []storedDisplaySchema
	for _, entry := range chain {
		if entry.Type != "custom" || entry.CustomType != displaySchemaEntry {
			continue
		}
		var record storedDisplaySchema
		if len(entry.Data) > api.ControlBytes || json.Unmarshal(entry.Data, &record) != nil || record.Version != 1 {
			return nil, errors.New("unsupported or invalid retained display schema")
		}
		for _, id := range []string{record.Owner, record.Key} {
			if id == "" || len(id) > 128 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
				return nil, errors.New("invalid retained display schema identity")
			}
		}
		identity := parser.Key(displayNamespace(record.Owner, record.Key)).String()
		if seen[identity] {
			continue
		}
		seen[identity] = true
		if _, err := presentation.DecodeDescriptor(record.Descriptor); err != nil {
			return nil, errors.New("invalid retained display descriptor")
		}
		records = append(records, record)
	}
	return records, nil
}

func restoreDisplaySchemas(ctx sdk.Context, s *session) error {
	entries, err := ctx.GetEntries()
	if err != nil {
		return err
	}
	leaf, err := ctx.GetLeafID()
	if err != nil {
		return err
	}
	records, err := storedDisplaySchemas(requestContext{ctx}, entries, leaf)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	trusted, err := ctx.IsProjectTrusted()
	if err != nil {
		return err
	}
	for _, record := range records {
		descriptor, err := presentation.DecodeDescriptor(record.Descriptor)
		if err != nil {
			return err
		}
		definitions, err := displayDefinitions(record.Owner, record.Key, descriptor)
		if err != nil {
			return err
		}
		if err := s.settings.UpdateDynamic(requestContext{ctx}, displayNamespace(record.Owner, record.Key), definitions, false, trusted); err != nil {
			return err
		}
	}
	return s.settings.Load(requestContext{ctx}, trusted)
}
