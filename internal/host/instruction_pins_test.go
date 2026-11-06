package host

import (
	"encoding/json"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func pinEntry(t *testing.T, id, action, class, path string) json.RawMessage {
	t.Helper()
	data, _ := json.Marshal(pinRecord{Action: action, instructionPin: instructionPin{Class: class, Path: path, Origin: "user"}})
	raw, _ := json.Marshal(map[string]any{"id": id, "type": "custom", "customType": pinEntryType, "data": json.RawMessage(data)})
	return raw
}

func TestPinsFollowTheActiveBranchOnly(t *testing.T) {
	entries := []json.RawMessage{
		pinEntry(t, "1", "pin", "backlog", "/p/BACKLOG.md"),
		pinEntry(t, "2", "pin", "backlog", "/p/sub/BACKLOG.md"),
		pinEntry(t, "3", "unpin", "", "/p/BACKLOG.md"),
		pinEntry(t, "side", "pin", "backlog", "/p/other/BACKLOG.md"),
	}
	main := []sdk.BranchEntry{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	got := pinsFromBranch(entries, main)
	if len(got) != 1 || got[0].Path != "/p/sub/BACKLOG.md" {
		t.Fatalf("pins on the main branch = %+v", got)
	}
	fork := []sdk.BranchEntry{{ID: "1"}, {ID: "side"}}
	if got := pinsFromBranch(entries, fork); len(got) != 2 || got[1].Path != "/p/other/BACKLOG.md" {
		t.Fatalf("a fork before the unpin keeps the earlier pin and its own: %+v", got)
	}
}

func TestPinnableClassRefusesSteeringAndNamesAmbiguity(t *testing.T) {
	if _, err := pinnableClass("/p/AGENTS.md", ""); err == nil {
		t.Fatal("steering must not be pinnable")
	}
	if class, err := pinnableClass("/p/BACKLOG.md", ""); err != nil || class.ID != "backlog" {
		t.Fatalf("backlog = %v %v", class.ID, err)
	}
	if _, err := pinnableClass("/p/NOTES.md", ""); err == nil {
		t.Fatal("an unknown file is not an instruction file")
	}
	if _, err := pinnableClass("/p/BACKLOG.md", "steering"); err == nil {
		t.Fatal("the requested class must own the file name")
	}
}
