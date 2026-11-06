package host

import (
	"encoding/json"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
)

// frameBaseline returns the last-seen digest per source/resource that the active
// branch's model context already carries, so resume and branch switches do not
// resend it. Entries before the newest compaction's first kept entry are
// summarized away and therefore excluded.
func frameBaseline(entries []json.RawMessage, branch []sdk.BranchEntry) (map[string]string, error) {
	start := 0
	for i, entry := range branch {
		if entry.Type != "compaction" {
			continue
		}
		start = i + 1
		for j := 0; j < i; j++ {
			if branch[j].ID == entry.FirstKeptEntryID && entry.FirstKeptEntryID != "" {
				start = j
				break
			}
		}
	}
	onBranch := make(map[string]bool, len(branch))
	for _, entry := range branch[start:] {
		onBranch[entry.ID] = true
	}
	type record struct {
		key, digest string
		live        bool
	}
	latest := make(map[string]record)
	var order []string
	for _, raw := range entries {
		var entry struct {
			ID         string          `json:"id"`
			Type       string          `json:"type"`
			CustomType string          `json:"customType"`
			Data       json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if !onBranch[entry.ID] || entry.Type != "custom" || entry.CustomType != "pig-workbench.frame" {
			continue
		}
		var stored struct {
			State string       `json:"state"`
			Frame frames.Frame `json:"frame"`
		}
		if err := json.Unmarshal(entry.Data, &stored); err != nil {
			return nil, err
		}
		if _, seen := latest[stored.Frame.FrameID]; !seen {
			order = append(order, stored.Frame.FrameID)
		}
		latest[stored.Frame.FrameID] = record{
			key:    stored.Frame.Source + "/" + stored.Frame.Resource,
			digest: hashline.SourceRevision([]byte(stored.Frame.Body)),
			live:   stored.State == "pending" || stored.State == "delivered",
		}
	}
	baseline := make(map[string]string)
	for _, id := range order {
		if r := latest[id]; r.live {
			baseline[r.key] = r.digest
		}
	}
	return baseline, nil
}
