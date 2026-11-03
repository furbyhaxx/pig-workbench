package host

import (
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
	if leaf == nil {
		return resetLocation(manager)
	}
	index := make(map[string]storedEntry, len(entries))
	for _, raw := range entries {
		var entry storedEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return errors.New("cannot decode session entry for location replay")
		}
		if entry.ID != "" {
			index[entry.ID] = entry
		}
	}
	seen := make(map[string]bool)
	for id := *leaf; id != ""; {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[id] {
			return errors.New("session entry parent chain contains a cycle")
		}
		seen[id] = true
		entry, ok := index[id]
		if !ok {
			return errors.New("session entry parent chain is incomplete")
		}
		if entry.Type == "custom" && entry.CustomType == "pig-workbench.location" {
			var change location.Change
			if err := json.Unmarshal(entry.Data, &change); err != nil {
				return errors.New("cannot decode stored location")
			}
			if change.SchemaVersion != 1 {
				return errors.New("unsupported stored location version")
			}
			return manager.Restore(change.Current)
		}
		id = entry.ParentID
	}
	return resetLocation(manager)
}

func resetLocation(manager *location.Manager) error {
	current := manager.Current()
	current.Cwd = current.Origin
	current.DisplayCwd = current.Origin
	current.Revision++
	return manager.Restore(current)
}
