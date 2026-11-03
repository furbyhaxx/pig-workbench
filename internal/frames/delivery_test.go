package frames

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func testScope() DeliveryScope {
	return DeliveryScope{BranchLeaf: "leaf-1", Epoch: "epoch-1"}
}

func testAdmit(t *testing.T, l *Ledger, key, delivery string) Receipt {
	t.Helper()
	p := testPublish(key)
	p.Source = l.Context().ProducerPrefix
	if delivery != "" {
		p.Delivery = delivery
	}
	r, err := l.Admit(p)
	if err != nil {
		t.Fatalf("admit %q: %v", key, err)
	}
	return r
}

func testFrameIDs(frames []Frame) []string {
	var ids []string
	for _, frame := range frames {
		ids = append(ids, frame.FrameID)
	}
	return ids
}

func TestSelectPendingFiltersByDeliveryBranchAndEpoch(t *testing.T) {
	s := testSession()
	branchA := testLedger(s)
	branchB := s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-2", Epoch: "epoch-1"})
	staleEpoch := s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-1", Epoch: "epoch-2"})
	a := testAdmit(t, branchA, "a", "nextBoundary")
	testAdmit(t, branchA, "b", "beforePrompt")
	c := testAdmit(t, branchB, "c", "nextBoundary")
	testAdmit(t, staleEpoch, "d", "nextBoundary")

	before := s.Snapshot()
	frames, err := s.SelectPending("nextBoundary", testScope())
	if err != nil {
		t.Fatal(err)
	}
	if ids := testFrameIDs(frames); len(ids) != 1 || ids[0] != a.FrameID {
		t.Fatalf("branch A selection = %v, want [%s]", ids, a.FrameID)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("selection changed delivery state")
	}
	frames, err = s.SelectPending("nextBoundary", DeliveryScope{BranchLeaf: "leaf-2", Epoch: "epoch-1"})
	if err != nil || len(frames) != 1 || frames[0].FrameID != c.FrameID {
		t.Fatalf("branch B selection = %#v, %v", testFrameIDs(frames), err)
	}
	frames, err = s.SelectPending("beforePrompt", testScope())
	if err != nil || len(frames) != 1 || frames[0].Sequence != 2 {
		t.Fatalf("beforePrompt selection = %#v, %v", testFrameIDs(frames), err)
	}
	if frames, err := s.SelectPending("nextTurn", testScope()); !errors.Is(err, ErrInvalidDelivery) || frames != nil {
		t.Fatalf("unknown delivery mode = %#v, %v", frames, err)
	}
}

func TestMarkDeliveredSurvivesRestoreWithoutRedelivery(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	scope := testScope()
	first := testAdmit(t, l, "a", "nextPrompt")
	second := testAdmit(t, l, "b", "nextPrompt")
	frames, err := s.SelectPending("nextPrompt", scope)
	if err != nil || len(frames) != 2 {
		t.Fatalf("selection = %#v, %v", testFrameIDs(frames), err)
	}
	if err := s.MarkDelivered(scope, []string{first.FrameID}); err != nil {
		t.Fatal(err)
	}
	if records := snapshotRecords(t, s); records[0].State != "delivered" || records[1].State != "pending" {
		t.Fatalf("states after delivery = %#v", records)
	}
	frames, err = s.SelectPending("nextPrompt", scope)
	if err != nil || len(frames) != 1 || frames[0].FrameID != second.FrameID {
		t.Fatalf("selection after delivery = %#v, %v", testFrameIDs(frames), err)
	}
	snapshot := s.Snapshot()
	replayed := testSession()
	if err := replayed.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, replayed.Snapshot()) {
		t.Fatal("delivery state changed across restore")
	}
	frames, err = replayed.SelectPending("nextPrompt", scope)
	if err != nil || len(frames) != 1 || frames[0].FrameID != second.FrameID {
		t.Fatalf("selection after restore = %#v, %v", testFrameIDs(frames), err)
	}
	if err := replayed.MarkDelivered(scope, []string{first.FrameID}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("redelivery of a restored frame = %v", err)
	}
	duplicate := testPublish("a")
	duplicate.Delivery = "nextPrompt"
	if r, err := testLedger(replayed).Admit(duplicate); err != nil || r != first {
		t.Fatalf("restored duplicate = %#v, %v; want %#v", r, err, first)
	}
	if records := snapshotRecords(t, replayed); records[0].State != "delivered" {
		t.Fatalf("duplicate admission resurrected a delivered frame: %#v", records[0])
	}
}

func TestDeliveryTransitionErrors(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	scope := testScope()
	first := testAdmit(t, l, "a", "")
	second := testAdmit(t, l, "b", "")
	for name, stale := range map[string]DeliveryScope{
		"epoch":  {BranchLeaf: "leaf-1", Epoch: "epoch-2"},
		"branch": {BranchLeaf: "leaf-2", Epoch: "epoch-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := s.MarkDelivered(stale, []string{first.FrameID}); !errors.Is(err, ErrStaleContext) {
				t.Fatalf("stale %s delivery = %v", name, err)
			}
		})
	}
	if err := s.MarkDelivered(scope, []string{first.FrameID, "f-unknown"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("batch with unknown frame = %v", err)
	}
	if records := snapshotRecords(t, s); records[0].State != "pending" || records[1].State != "pending" {
		t.Fatalf("rejected batch changed states: %#v", records)
	}
	if err := s.Cancel(first.FrameID); err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(second.FrameID); err != nil {
		t.Fatal(err)
	}
	for name, attempt := range map[string]func() error{
		"deliver cancelled": func() error { return s.MarkDelivered(scope, []string{first.FrameID}) },
		"cancel cancelled":  func() error { return s.Cancel(first.FrameID) },
		"fail cancelled":    func() error { return s.Fail(first.FrameID) },
		"cancel failed":     func() error { return s.Cancel(second.FrameID) },
		"cancel unknown":    func() error { return s.Cancel("f-unknown") },
		"fail unknown":      func() error { return s.Fail("f-unknown") },
	} {
		if err := attempt(); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	for i := 0; i < 256; i++ {
		if _, err := l.Admit(testPublish(fmt.Sprint(i))); err != nil {
			t.Fatalf("admission %d after retirement: %v", i, err)
		}
	}
	if _, err := l.Admit(testPublish("overflow")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("pending capacity after retirement = %v", err)
	}
}

func TestExpiryAndCancelReleaseCapacityWithoutLosingDedup(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	scope := testScope()
	expires := time.Date(2026, 11, 3, 13, 30, 0, 0, time.UTC)
	p := testPublish("0")
	p.ExpiresAt = &expires
	first, err := l.Admit(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 256; i++ {
		if _, err := l.Admit(testPublish(fmt.Sprint(i))); err != nil {
			t.Fatalf("admission %d: %v", i, err)
		}
	}
	if _, err := l.Admit(testPublish("overflow")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("pending capacity before expiry = %v", err)
	}
	s.SetClock(func() time.Time { return expires })
	frames, err := s.SelectPending("nextBoundary", scope)
	if err != nil || len(frames) != 255 {
		t.Fatalf("selection after expiry = %d frames, %v", len(frames), err)
	}
	if records := snapshotRecords(t, s); records[0].State != "expired" {
		t.Fatalf("expired frame state = %q", records[0].State)
	}
	if err := s.MarkDelivered(scope, []string{first.FrameID}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("delivery of expired frame = %v", err)
	}
	if r, err := l.Admit(p); err != nil || r != first {
		t.Fatalf("expired duplicate = %#v, %v; want %#v", r, err, first)
	}
	if _, err := l.Admit(testPublish("after-expiry")); err != nil {
		t.Fatalf("expiry did not release pending capacity: %v", err)
	}
	if records := snapshotRecords(t, s); len(records) != 257 {
		t.Fatalf("records after expiry = %d, want 257", len(records))
	}
	cancelled := frames[0]
	if err := s.Cancel(cancelled.FrameID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Admit(testPublish("after-cancel")); err != nil {
		t.Fatalf("cancel did not release pending capacity: %v", err)
	}
	if _, err := l.Admit(testPublish("overflow-again")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("pending capacity after cancel = %v", err)
	}
	duplicate := testPublish(cancelled.EventKey)
	if r, err := l.Admit(duplicate); err != nil || r.FrameID != cancelled.FrameID {
		t.Fatalf("cancelled duplicate = %#v, %v; want frame %s", r, err, cancelled.FrameID)
	}
	records := snapshotRecords(t, s)
	if len(records) != 258 || records[1].State != "cancelled" {
		t.Fatalf("cancelled record after duplicate = %#v", records[1])
	}
}

func TestDeliveryStatesSurviveRestore(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	scope := testScope()
	expires := time.Date(2026, 11, 3, 13, 30, 0, 0, time.UTC)
	admit := func(key string, mutate func(*Publish)) Receipt {
		t.Helper()
		p := testPublish(key)
		if mutate != nil {
			mutate(&p)
		}
		r, err := l.Admit(p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	coalesced := func(p *Publish) { p.CoalesceKey, p.SubjectKey = "progress", "build/1" }
	delivered := admit("delivered", nil)
	cancelled := admit("cancelled", nil)
	failed := admit("failed", nil)
	admit("expired", func(p *Publish) { p.ExpiresAt = &expires })
	pending := admit("pending", nil)
	admit("superseded", coalesced)
	latest := admit("latest", coalesced)
	if err := s.MarkDelivered(scope, []string{delivered.FrameID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(cancelled.FrameID); err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(failed.FrameID); err != nil {
		t.Fatal(err)
	}
	s.SetClock(func() time.Time { return expires })
	if frames, err := s.SelectPending("nextBoundary", scope); err != nil || len(frames) != 2 {
		t.Fatalf("selection before snapshot = %d frames, %v", len(frames), err)
	}
	snapshot := s.Snapshot()
	want := []string{"delivered", "cancelled", "failed", "expired", "pending", "superseded", "pending"}
	if got := states(snapshotRecords(t, s)); !reflect.DeepEqual(got, want) {
		t.Fatalf("states = %v, want %v", got, want)
	}

	replayed := testSession()
	replayed.SetClock(func() time.Time { return expires })
	if err := replayed.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, replayed.Snapshot()) {
		t.Fatal("delivery states changed across restore")
	}
	if got := states(snapshotRecords(t, replayed)); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored states = %v, want %v", got, want)
	}
	frames, err := replayed.SelectPending("nextBoundary", scope)
	if err != nil || len(frames) != 2 || frames[0].FrameID != pending.FrameID || frames[1].FrameID != latest.FrameID {
		t.Fatalf("selection after restore = %#v, %v", testFrameIDs(frames), err)
	}
	restored := testLedger(replayed)
	for i := 0; i < 254; i++ {
		if _, err := restored.Admit(testPublish(fmt.Sprint("fill-", i))); err != nil {
			t.Fatalf("admission %d after restore: %v", i, err)
		}
	}
	if _, err := restored.Admit(testPublish("fill-overflow")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("restored pending capacity = %v", err)
	}
	if err := replayed.MarkDelivered(scope, []string{pending.FrameID, latest.FrameID}); err != nil {
		t.Fatal(err)
	}
	frames, err = replayed.SelectPending("nextBoundary", scope)
	if err != nil || len(frames) != 254 {
		t.Fatalf("selection after restore delivery = %d frames, %v", len(frames), err)
	}
	for _, frame := range frames {
		if frame.FrameID == pending.FrameID || frame.FrameID == latest.FrameID {
			t.Fatalf("delivered frame %s selected again", frame.FrameID)
		}
	}
}

func TestRestoreRejectsInvalidDeliveryState(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	frame := testAdmit(t, l, "a", "")
	raw := s.Snapshot()[0]
	rewrite := func(mutate func(map[string]json.RawMessage)) json.RawMessage {
		t.Helper()
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		mutate(record)
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for name, snapshot := range map[string][]json.RawMessage{
		"unknown state":          {rewrite(func(r map[string]json.RawMessage) { r["state"] = json.RawMessage(`"deferred"`) })},
		"expired without expiry": {rewrite(func(r map[string]json.RawMessage) { r["state"] = json.RawMessage(`"expired"`) })},
	} {
		t.Run(name, func(t *testing.T) {
			replayed := testSession()
			if err := replayed.Restore(snapshot); !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("%s restore = %v", name, err)
			}
			if len(replayed.Snapshot()) != 0 {
				t.Fatal("failed restore retained records")
			}
		})
	}
	delivered := rewrite(func(r map[string]json.RawMessage) { r["state"] = json.RawMessage(`"delivered"`) })
	replayed := testSession()
	if err := replayed.Restore([]json.RawMessage{delivered}); err != nil {
		t.Fatalf("delivered restore = %v", err)
	}
	if frames, err := replayed.SelectPending("nextBoundary", testScope()); err != nil || len(frames) != 0 {
		t.Fatalf("restored delivered frame selected = %#v, %v", testFrameIDs(frames), err)
	}
	if err := replayed.MarkDelivered(testScope(), []string{frame.FrameID}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("restored delivered frame redelivered = %v", err)
	}
}

func states(records []testRecord) []string {
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = record.State
	}
	return names
}
