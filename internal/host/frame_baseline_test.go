package host

import (
	"encoding/json"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
)

func frameEntry(t *testing.T, id, state, source, resource, body string) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]any{"state": state, "frame": frames.Frame{FrameID: "frm-" + frameOf(id), Source: source, Resource: resource, Body: body}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"id": id, "type": "custom", "customType": "pig-workbench.frame", "data": json.RawMessage(data)})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// frameOf maps entry ids that record the same frame to one frame id.
func frameOf(id string) string {
	if id == "e2s" {
		return "e1"
	}
	return id
}

func branchOf(ids ...string) []sdk.BranchEntry {
	branch := make([]sdk.BranchEntry, len(ids))
	for i, id := range ids {
		branch[i] = sdk.BranchEntry{ID: id, Type: "message"}
	}
	return branch
}

func TestFrameBaselineResumeKeepsDeliveredFactsOnTheBranch(t *testing.T) {
	entries := []json.RawMessage{
		frameEntry(t, "e1", "delivered", "session.date", "", "2026-11-06"),
		frameEntry(t, "e2", "delivered", "session.model", "", "m"),
		frameEntry(t, "e3", "delivered", "session.date", "", "other branch"),
	}
	baseline, err := frameBaseline(entries, branchOf("e1", "e2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 2 || baseline["session.date/"] != hashline.SourceRevision([]byte("2026-11-06")) {
		t.Fatalf("baseline = %v", baseline)
	}
}

func TestFrameBaselineOmitsFramesFromAnotherBranch(t *testing.T) {
	entries := []json.RawMessage{frameEntry(t, "e1", "delivered", "session.date", "", "d")}
	baseline, err := frameBaseline(entries, branchOf("root"))
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 0 {
		t.Fatalf("frame from another branch counted as delivered: %v", baseline)
	}
}

func TestFrameBaselineIgnoresCancelledAndFollowsLatestState(t *testing.T) {
	entries := []json.RawMessage{
		frameEntry(t, "e1", "pending", "session.date", "", "d"),
		frameEntry(t, "e2s", "cancelled", "session.date", "", "d"),
	}
	baseline, err := frameBaseline(entries, branchOf("e1", "e2s"))
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 0 {
		t.Fatalf("cancelled frame in baseline: %v", baseline)
	}
}

func TestFrameBaselineDropsFramesSummarizedByCompaction(t *testing.T) {
	entries := []json.RawMessage{
		frameEntry(t, "e1", "delivered", "session.date", "", "old"),
		frameEntry(t, "e3", "delivered", "session.model", "", "kept"),
	}
	branch := branchOf("e1", "e2", "e3")
	branch = append(branch, sdk.BranchEntry{ID: "c1", Type: "compaction", FirstKeptEntryID: "e2"})
	baseline, err := frameBaseline(entries, branch)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := baseline["session.date/"]; ok {
		t.Fatalf("summarized frame stayed in baseline: %v", baseline)
	}
	if _, ok := baseline["session.model/"]; !ok {
		t.Fatalf("kept frame missing from baseline: %v", baseline)
	}
}
