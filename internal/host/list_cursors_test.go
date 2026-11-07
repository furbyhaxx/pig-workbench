package host

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestListingCursorRetentionIsBoundedByBytesAndCount(t *testing.T) {
	for _, size := range []int{1, 16384} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var store listingCursorStore
			binding := cursorBinding{Registration: "reg", Descriptor: "rev", Generation: "generation", Instance: "instance"}
			now := time.Now()
			first, last := "", ""
			count := 300
			if size > 1 {
				count = 100
			}
			for i := 0; i < count; i++ {
				token, err := store.put(listingCursor{Binding: binding, Requested: "kb://root", Canonical: "kb://root", Raw: fmt.Sprintf("%d-%s", i, strings.Repeat("x", size))}, now)
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = token
				}
				last = token
			}
			if _, err := store.get(first, binding, "kb://root", now); err == nil {
				t.Fatal("oldest cursor was not evicted at the retention cap")
			}
			if _, err := store.get(last, binding, "kb://root", now); err != nil {
				t.Fatalf("new cursor was lost: %v", err)
			}
		})
	}
}

func TestListingCursorBindsIdentityResourceAndSnapshotWithoutExposingPayload(t *testing.T) {
	var store listingCursorStore
	now := time.Now()
	binding := cursorBinding{Registration: "reg", Descriptor: "rev", Generation: "generation", Instance: "instance"}
	entry := listingCursor{Binding: binding, Requested: "kb://root", Canonical: "kb://root/", Raw: "secret-backend-cursor", Revision: "catalog-1"}
	token, err := store.put(entry, now)
	if err != nil || strings.Contains(token, "secret-backend-cursor") || strings.Contains(token, "kb://") {
		t.Fatalf("unsafe token %q %v", token, err)
	}
	if same, err := store.put(entry, now); err != nil || same != token {
		t.Fatalf("wrapping hid a repeated backend cursor: %q %v", same, err)
	}
	got, err := store.get(token, binding, "kb://root/", now)
	if err != nil || got.Raw != entry.Raw || got.Revision != "catalog-1" {
		t.Fatalf("cursor did not roundtrip: %+v %v", got, err)
	}
	if _, err := store.get(token, binding, "kb://other/", now); err == nil {
		t.Fatal("cursor reused against a different root")
	}
	changed := binding
	changed.Descriptor = "new"
	if _, err := store.get(token, changed, "kb://root/", now); err == nil {
		t.Fatal("cursor reused after descriptor replacement")
	}
	changed = binding
	changed.Generation = "new"
	if _, err := store.get(token, changed, "kb://root/", now); err == nil {
		t.Fatal("cursor reused after reload")
	}
	if _, err := store.get(token, binding, "kb://root/", now.Add(6*time.Minute)); err == nil {
		t.Fatal("expired cursor accepted")
	}
}
