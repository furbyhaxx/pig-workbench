package host

import (
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func TestDedupeByIDKeepsTheFirstOfEachSession(t *testing.T) {
	all := []sdk.SessionInfo{{ID: "a", Path: "/bucket-one/a.jsonl"}, {ID: "b", Path: "/bucket-one/b.jsonl"}, {ID: "a", Path: "/bucket-two/a.jsonl"}}
	got := dedupeByID(all)
	if len(got) != 2 || got[0].Path != "/bucket-one/a.jsonl" || got[1].ID != "b" {
		t.Fatalf("got %+v", got)
	}
}
