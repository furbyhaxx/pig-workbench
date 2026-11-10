package host

import (
	"fmt"
	"testing"
)

func TestRecentDropsOnlyTheOldestEntriesWhenFull(t *testing.T) {
	calls := newRecent[string](3)
	for i := range 3 {
		calls.put(fmt.Sprint("call-", i), "read")
	}
	calls.put("call-1", "edit")
	calls.put("call-3", "read")
	if _, ok := calls.get("call-0"); ok {
		t.Fatal("the oldest entry survived a full map")
	}
	for _, id := range []string{"call-1", "call-2", "call-3"} {
		if _, ok := calls.get(id); !ok {
			t.Fatalf("%s, still in flight, was dropped", id)
		}
	}
	if name, _ := calls.get("call-1"); name != "edit" {
		t.Fatalf("an updated entry kept its old value %q", name)
	}
}

func TestRecentStaysBoundedThroughAddAndRemoveCycles(t *testing.T) {
	calls := newRecent[int](4)
	for i := range 1000 {
		id := fmt.Sprint(i)
		calls.put(id, i)
		if i%2 == 0 {
			calls.remove(id)
		}
	}
	if calls.len() > 4 || len(calls.order) > 8 {
		t.Fatalf("map holds %d entries and %d order slots, want at most 4 and 8", calls.len(), len(calls.order))
	}
	if _, ok := calls.get("999"); !ok {
		t.Fatal("the newest entry is missing")
	}
}
