package hashline

import (
	"bytes"
	"strings"
	"testing"
)

func TestStoreRequiresDeliveredTaggedSnapshotInCurrentEpoch(t *testing.T) {
	store, err := NewStore(StoreLimits{}, "epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("one\ntwo\nthree\n")
	snap, err := store.Retain("file:///a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(snap.URI, snap.Tag); err == nil {
		t.Fatal("retention granted model visibility")
	}
	if err := store.Delivered(snap.URI, snap.Revision, "epoch-1", []LineSpan{{1, 1}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(snap.URI, snap.Tag); err == nil {
		t.Fatal("raw read granted tagged handle")
	}
	if err := store.Delivered(snap.URI, snap.Revision, "epoch-1", []LineSpan{{3, 3}}, true); err != nil {
		t.Fatal(err)
	}
	seen, err := store.Resolve(snap.URI, snap.Tag)
	if err != nil {
		t.Fatal(err)
	}
	source, err := Normalize(seen.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckSeen(source, SeenView{Epoch: seen.Epoch, Ranges: seen.Seen}, "epoch-1", []Op{{Kind: PUT_RANGE, Line: 1, End: 3, Payload: []string{"new"}}}); err == nil {
		t.Fatal("unseen interior authorized")
	}
	seen.Raw[0] = 'X'
	seen.Seen[0].End = 3
	unchanged, err := store.Resolve(snap.URI, snap.Tag)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged.Raw, raw) || unchanged.Seen[0].End != 1 {
		t.Fatal("caller mutated cache")
	}
	if err := store.SetEpoch("epoch-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(snap.URI, snap.Tag); err == nil {
		t.Fatal("old context authorized edit")
	}
	if err := store.Delivered(snap.URI, snap.Revision, "epoch-1", []LineSpan{{1, 3}}, true); err == nil {
		t.Fatal("late delivery crossed epoch")
	}
}

func TestStoreLimitsAndCanonicalResourceBinding(t *testing.T) {
	store, err := NewStore(StoreLimits{Resources: 1, Versions: 1, Bytes: 32, FileBytes: 16}, "e")
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.Retain("file:///a", []byte("same\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delivered(a.URI, a.Revision, "e", []LineSpan{{1, 1}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve("file:///b", a.Tag); err == nil {
		t.Fatal("cross-resource tag accepted")
	}
	if _, err := store.Retain("file:///b", []byte("same\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(a.URI, a.Tag); err == nil {
		t.Fatal("resource limit not enforced")
	}
	if _, err := store.Retain("file:///c", bytes.Repeat([]byte{'x'}, 17)); err == nil {
		t.Fatal("file ceiling ignored")
	}
}

func TestStoreChoosesNewestVersionWithNormalizedTag(t *testing.T) {
	store, err := NewStore(StoreLimits{}, "e")
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.Retain("file:///a", []byte("value\n"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Retain("file:///a", []byte("value  \r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Tag != b.Tag || a.Revision == b.Revision {
		t.Fatal("fixture did not produce tag collision")
	}
	if err := store.Delivered(b.URI, b.Revision, "e", []LineSpan{{1, 1}}, true); err != nil {
		t.Fatal(err)
	}
	chosen, err := store.Resolve(a.URI, a.Tag)
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Revision != b.Revision {
		t.Fatal("did not select newest exact-byte version")
	}
}

func TestResolveNamesTheRetainedTagsWhenTheRequestedOneIsGone(t *testing.T) {
	store, err := NewStore(StoreLimits{}, "epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve("file:///a", "ABCD"); err == nil || !strings.Contains(err.Error(), "read it before editing") {
		t.Fatalf("an unread resource must say so, got %v", err)
	}
	snap, err := store.Retain("file:///a", []byte("one\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Resolve("file:///a", "BEEF")
	if err == nil || !strings.Contains(err.Error(), "BEEF") || !strings.Contains(err.Error(), snap.Tag) {
		t.Fatalf("the error must name the requested and the retained tag, got %v", err)
	}
}

func TestSetLimitsEvictsDownToTheNewBounds(t *testing.T) {
	store, err := NewStore(StoreLimits{}, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"file:///a", "file:///b", "file:///c"} {
		for _, text := range []string{"one\n", "two\n"} {
			if _, err := store.Retain(uri, []byte(text)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.SetLimits(StoreLimits{Resources: 2, Versions: 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Latest("file:///a"); ok {
		t.Fatal("the least recently used resource survived a lower resource limit")
	}
	for _, uri := range []string{"file:///b", "file:///c"} {
		latest, ok := store.Latest(uri)
		if !ok || string(latest.Raw) != "two\n" || len(store.entries[uri]) != 1 {
			t.Fatalf("%s must keep only its newest version: ok=%v versions=%d", uri, ok, len(store.entries[uri]))
		}
	}
	if err := store.SetLimits(StoreLimits{FileBytes: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Retain("file:///d", []byte("12345")); err == nil {
		t.Fatal("a source above the new file limit was retained")
	}
	if err := store.SetLimits(StoreLimits{Bytes: 8}); err != nil {
		t.Fatal(err)
	}
	if store.bytes > 8 || len(store.entries) != 1 {
		t.Fatalf("byte limit not enforced: bytes=%d resources=%d", store.bytes, len(store.entries))
	}
	if err := store.SetLimits(StoreLimits{Versions: -1}); err == nil {
		t.Fatal("a negative limit was accepted")
	}
}
