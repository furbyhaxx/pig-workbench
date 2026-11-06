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

func TestShortIDsOfSessionsStartedTogetherDiffer(t *testing.T) {
	// UUIDv7 IDs created within the same minute share their first 8 characters.
	a, b := "01a1b2a6-8c2f-7d11-9a33-4f0e5c1d2b77", "01a1b2a6-91d4-7e02-8b44-a1b2c3d4e5f6"
	if shortID(a) == shortID(b) {
		t.Fatalf("short IDs collide: %s", shortID(a))
	}
	for _, query := range []string{shortID(a), a[:8+1], a} {
		if !idMatches(a, query) {
			t.Errorf("%q must name %s", query, a)
		}
	}
	if idMatches(a, shortID(b)) || idMatches(a, "01a1b") {
		t.Error("a different short ID and a query under 6 characters must not match")
	}
}
