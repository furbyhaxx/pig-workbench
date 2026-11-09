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
