package host

import (
	"context"
	"encoding/json"
	"errors"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/location"
)

type storedEntry struct {
	ID         string          `json:"id"`
	ParentID   string          `json:"parentId"`
	Type       string          `json:"type"`
	CustomType string          `json:"customType"`
	Data       json.RawMessage `json:"data"`
}

func restoreLocation(ctx sdk.Context, manager *location.Manager) error {
	entries, err := ctx.GetEntries()
	if err != nil {
		return err
	}
	leaf, err := ctx.GetLeafID()
	if err != nil {
		return err
	}
	chain, err := storedBranch(requestContext{ctx}, entries, leaf)
	if err != nil {
		return err
	}
	for _, entry := range chain {
		if entry.Type == "custom" && entry.CustomType == "pig-workbench.location" {
			var change location.Change
			if err := json.Unmarshal(entry.Data, &change); err != nil {
				return errors.New("cannot decode stored location")
			}
			if change.SchemaVersion != 1 {
				return errors.New("unsupported stored location version")
			}
			return manager.RestoreContext(requestContext{ctx}, change.Current)
		}
	}
	return resetLocation(ctx, manager)
}

func storedBranch(ctx context.Context, entries []json.RawMessage, leaf *string) ([]storedEntry, error) {
	if leaf == nil {
		return nil, nil
	}
	index := make(map[string]storedEntry, len(entries))
	for _, raw := range entries {
		var entry storedEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, errors.New("cannot decode stored session entry")
		}
		if entry.ID != "" {
			index[entry.ID] = entry
		}
	}
	seen := make(map[string]bool)
	var chain []storedEntry
	for id := *leaf; id != ""; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, errors.New("session entry parent chain contains a cycle")
		}
		seen[id] = true
		entry, ok := index[id]
		if !ok {
			return nil, errors.New("session entry parent chain is incomplete")
		}
		chain = append(chain, entry)
		id = entry.ParentID
	}
	return chain, nil
}

func resetLocation(ctx sdk.Context, manager *location.Manager) error {
	current := manager.Current()
	current.Cwd = current.Origin
	current.DisplayCwd = current.Origin
	current.Revision++
	return manager.RestoreContext(requestContext{ctx}, current)
}
