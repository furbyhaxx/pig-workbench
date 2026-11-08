package frames

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func testSession() *Session {
	s := NewSession("session-1")
	s.SetClock(func() time.Time { return time.Date(2026, 11, 3, 12, 30, 0, 123, time.FixedZone("CET", 3600)) })
	return s
}

func testLedger(s *Session) *Ledger {
	return s.Ledger(Context{SessionID: "ignored", ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-1", Epoch: "epoch-1"})
}

// repeated is the receipt a repeated publication of the same frame returns.
func repeated(r Receipt) Receipt { r.Duplicate = true; return r }

func testPublish(key string) Publish {
	return Publish{EventKey: key, Source: "build-tools.build", Kind: "notice", Body: "build complete"}
}

type testRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	Frame         Frame  `json:"frame"`
	State         string `json:"state"`
}

func snapshotRecords(t *testing.T, s *Session) []testRecord {
	t.Helper()
	var records []testRecord
	for _, raw := range s.Snapshot() {
		var record testRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestAdmissionDedupAndSnapshotReplay(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	p := testPublish("build-1")
	p.Title, p.Summary, p.SubjectKey, p.Resource = "title", "summary", "build/1", "build://1"
	p.DataTrust, p.Delivery = "trusted", "nextPrompt"
	expires := time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
	p.ExpiresAt = &expires
	first, err := l.Admit(p)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := l.Admit(p)
	if err != nil || duplicate != repeated(first) || first.Duplicate {
		t.Fatalf("duplicate receipt = %#v, %v; want %#v", duplicate, err, repeated(first))
	}
	changed := p
	changed.Body = "different body"
	if _, err := l.Admit(changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed duplicate error = %v, want conflict", err)
	}
	before := s.Snapshot()
	if len(before) != 1 {
		t.Fatalf("duplicate added a record: %d", len(before))
	}
	records := snapshotRecords(t, s)
	f := records[0].Frame
	if records[0].SchemaVersion != 1 || f.SchemaVersion != 1 || f.FrameID != first.FrameID || f.Sequence != 1 ||
		f.SessionID != "session-1" || f.BranchLeaf != "leaf-1" || f.Epoch != "epoch-1" ||
		f.Source != p.Source || f.ProducerOwner != "build-tools" || f.Summary != p.Summary ||
		f.Trust != "untrusted" || f.Authority != "data" || f.CreatedAt.Format(time.RFC3339Nano) != "2026-11-03T12:30:00.000000123+01:00" {
		t.Fatalf("unexpected frame: %#v", f)
	}
	replayed := testSession()
	restored := testLedger(replayed)
	if err := replayed.Restore(before); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, replayed.Snapshot()) {
		t.Fatal("snapshot changed across restore")
	}
	again, err := restored.Admit(p)
	if err != nil || again != repeated(first) {
		t.Fatalf("restored duplicate = %#v, %v; want %#v", again, err, repeated(first))
	}
	if _, err := restored.Admit(changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("restored changed duplicate error = %v", err)
	}
	next, err := restored.Admit(testPublish("build-2"))
	if err != nil || next.Sequence != 2 || next.FrameID == first.FrameID {
		t.Fatalf("next receipt = %#v, %v", next, err)
	}
}

func TestValidatePublishAndOwnership(t *testing.T) {
	p := testPublish("e")
	normalized, err := ValidatePublish(p)
	if err != nil || normalized.Delivery != "nextBoundary" || normalized.DataTrust != "untrusted" {
		t.Fatalf("defaults = %#v, %v", normalized, err)
	}
	for _, kind := range []string{"instruction", "reminder", "warning", "error", "notice", "info", "message"} {
		valid := p
		valid.Kind = kind
		if _, err := ValidatePublish(valid); err != nil {
			t.Fatalf("kind %q rejected: %v", kind, err)
		}
	}
	for _, delivery := range []string{"nextBoundary", "nextPrompt", "beforePrompt"} {
		valid := p
		valid.Delivery = delivery
		if _, err := ValidatePublish(valid); err != nil {
			t.Fatalf("delivery %q rejected: %v", delivery, err)
		}
	}
	for name, change := range map[string]func(*Publish){
		"empty event":      func(p *Publish) { p.EventKey = " " },
		"empty source":     func(p *Publish) { p.Source = "" },
		"malformed source": func(p *Publish) { p.Source = "build..test" },
		"unknown kind":     func(p *Publish) { p.Kind = "urgent" },
		"unknown delivery": func(p *Publish) { p.Delivery = "immediately" },
		"retired followUp": func(p *Publish) { p.Delivery = "followUp" },
		"wake on held":     func(p *Publish) { p.Delivery, p.Wake = "nextPrompt", true },
		"unknown trust":    func(p *Publish) { p.DataTrust = "self-authorized" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := p
			change(&invalid)
			if _, err := ValidatePublish(invalid); !errors.Is(err, ErrInvalidPublish) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
	p.Body = strings.Repeat("x", 32*1024)
	if _, err := ValidatePublish(p); err != nil {
		t.Fatalf("maximum body rejected: %v", err)
	}
	p.Body += "x"
	if _, err := ValidatePublish(p); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("oversized body error = %v", err)
	}
	s := testSession()
	l := testLedger(s)
	for _, source := range []string{"build-tools.builder", "build-tools", "other.build"} {
		invalid := testPublish(source)
		invalid.Source = source
		_, err := l.Admit(invalid)
		var ownership *OwnershipError
		if !errors.As(err, &ownership) || !errors.Is(err, ErrOwnership) {
			t.Fatalf("source %q error = %v", source, err)
		}
	}
	child := testPublish("child")
	child.Source += ".runner"
	if _, err := l.Admit(child); err != nil {
		t.Fatalf("dotted child rejected: %v", err)
	}
}

func TestSessionRequiresIdentity(t *testing.T) {
	s := NewSession("")
	l := s.Ledger(Context{ProducerPrefix: "build-tools.build"})
	if _, err := l.Admit(testPublish("event")); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty session identity admission = %v", err)
	}
	if err := s.Restore(nil); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("empty session identity restore = %v", err)
	}
}

func TestPendingBoundNeverDrops(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	for i := 0; i < 256; i++ {
		if _, err := l.Admit(testPublish(fmt.Sprint(i))); err != nil {
			t.Fatalf("admission %d: %v", i, err)
		}
	}
	before := s.Snapshot()
	if _, err := l.Admit(testPublish("overflow")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("pending overflow error = %v", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("overflow mutated retained frames")
	}
}

func TestBodyMemoryBoundIncludesExactLimit(t *testing.T) {
	s := testSession()
	ledgers := []*Ledger{testLedger(s), s.Ledger(Context{ProducerPrefix: "deploy.release", BranchLeaf: "leaf-1", Epoch: "epoch-1"})}
	for i := 0; i < 64; i++ {
		p := testPublish(fmt.Sprint(i))
		p.Source = ledgers[i%2].Context().ProducerPrefix
		p.Body = strings.Repeat("x", 32*1024)
		if _, err := ledgers[i%2].Admit(p); err != nil {
			t.Fatalf("admission at %d bytes: %v", (i+1)*len(p.Body), err)
		}
	}
	before := s.Snapshot()
	if _, err := ledgers[0].Admit(testPublish("overflow")); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("body memory overflow error = %v", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("body memory overflow mutated retained frames")
	}
}

func TestCoalescePendingNoticesButPreserveErrors(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	for _, item := range []struct{ key, kind, body string }{
		{"error", "error", "failed"}, {"old", "notice", "old"}, {"new", "info", "new"},
	} {
		p := testPublish(item.key)
		p.Kind, p.Body, p.CoalesceKey, p.SubjectKey = item.kind, item.body, "progress", "build/1"
		if _, err := l.Admit(p); err != nil {
			t.Fatal(err)
		}
	}
	records := snapshotRecords(t, s)
	if len(records) != 3 || records[0].State != "pending" || records[1].State != "superseded" || records[2].State != "pending" {
		t.Fatalf("coalesced states = %#v", records)
	}
	p := testPublish("other-subject")
	p.CoalesceKey, p.SubjectKey = "progress", "build/2"
	if _, err := l.Admit(p); err != nil {
		t.Fatal(err)
	}
	if records = snapshotRecords(t, s); records[2].State != "pending" || records[3].State != "pending" {
		t.Fatalf("different subject coalesced: %#v", records)
	}
	replayed := testSession()
	restored := testLedger(replayed)
	if err := replayed.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	p = testPublish("latest")
	p.Kind, p.CoalesceKey, p.SubjectKey = "notice", "progress", "build/1"
	if _, err := restored.Admit(p); err != nil {
		t.Fatal(err)
	}
	if records = snapshotRecords(t, replayed); records[0].State != "pending" || records[2].State != "superseded" || records[4].State != "pending" {
		t.Fatalf("restored coalescing states = %#v", records)
	}
}

func TestCoalescingRespectsBranchAndEpoch(t *testing.T) {
	s := testSession()
	ledgers := []*Ledger{
		testLedger(s),
		s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "other-leaf", Epoch: "epoch-1"}),
		s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-1", Epoch: "epoch-2"}),
	}
	for i, l := range ledgers {
		p := testPublish(fmt.Sprintf("event-%d", i))
		p.CoalesceKey, p.SubjectKey = "progress", "build/1"
		if _, err := l.Admit(p); err != nil {
			t.Fatal(err)
		}
	}
	duplicate := testPublish("event-0")
	duplicate.CoalesceKey, duplicate.SubjectKey = "progress", "build/1"
	if _, err := ledgers[1].Admit(duplicate); !errors.Is(err, ErrConflict) {
		t.Fatalf("event key reused across branches: %v", err)
	}
	for _, record := range snapshotRecords(t, s) {
		if record.State != "pending" {
			t.Fatalf("different scope coalesced: %#v", record)
		}
	}
	replayed := testSession()
	if err := replayed.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	p := testPublish("replacement")
	p.CoalesceKey, p.SubjectKey = "progress", "build/1"
	if _, err := testLedger(replayed).Admit(p); err != nil {
		t.Fatal(err)
	}
	records := snapshotRecords(t, replayed)
	if records[0].State != "superseded" || records[1].State != "pending" || records[2].State != "pending" || records[3].State != "pending" {
		t.Fatalf("replacement crossed scope: %#v", records)
	}
}

func TestCoalescingAtPendingCapacity(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	first := testPublish("old")
	first.CoalesceKey = "progress"
	firstReceipt, err := l.Admit(first)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 255; i++ {
		if _, err := l.Admit(testPublish(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	newer := testPublish("new")
	newer.CoalesceKey = "progress"
	if _, err := l.Admit(newer); err != nil {
		t.Fatalf("coalescing at capacity: %v", err)
	}
	if receipt, err := l.Admit(first); err != nil || receipt != repeated(firstReceipt) {
		t.Fatalf("superseded duplicate = %#v, %v", receipt, err)
	}
	if records := snapshotRecords(t, s); len(records) != 257 || records[0].State != "superseded" || records[256].State != "pending" {
		t.Fatalf("unexpected records after coalescing: %#v", records)
	}
}

func TestSequencesAndExhaustion(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	for i := int64(1); i <= 100; i++ {
		r, err := l.Admit(testPublish(fmt.Sprint(i)))
		if err != nil || r.Sequence != i {
			t.Fatalf("sequence %d: %#v, %v", i, r, err)
		}
	}
	s.sequence = math.MaxInt64
	before := s.Snapshot()
	if _, err := l.Admit(testPublish("exhausted")); !errors.Is(err, ErrSequenceExhausted) {
		t.Fatalf("exhausted error = %v", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("exhaustion mutated frames")
	}
}

func TestSessionSharesSequenceAndPendingLimitAcrossProducers(t *testing.T) {
	s := testSession()
	ledgers := []*Ledger{
		testLedger(s),
		s.Ledger(Context{SessionID: "ignored", ProducerPrefix: "deploy.release", BranchLeaf: "leaf-1", Epoch: "epoch-1"}),
	}
	start := make(chan struct{})
	receipts := make(chan Receipt, 2)
	failures := make(chan error, 2)
	for _, l := range ledgers {
		go func(l *Ledger) {
			<-start
			p := testPublish("shared-event")
			p.Source = l.Context().ProducerPrefix
			r, err := l.Admit(p)
			if err != nil {
				failures <- err
				return
			}
			receipts <- r
		}(l)
	}
	close(start)
	sequences := make(map[int64]bool)
	for i := 0; i < 2; i++ {
		select {
		case err := <-failures:
			t.Fatal(err)
		case receipt := <-receipts:
			sequences[receipt.Sequence] = true
		}
	}
	if !sequences[1] || !sequences[2] || len(sequences) != 2 {
		t.Fatalf("producer sequences = %v", sequences)
	}
	if ledgers[1].Context().SessionID != "session-1" {
		t.Fatalf("session context not stamped: %#v", ledgers[1].Context())
	}
	replayed := testSession()
	if err := replayed.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	for _, l := range ledgers {
		p := testPublish("shared-event")
		p.Source = l.Context().ProducerPrefix
		r, err := replayed.Ledger(l.Context()).Admit(p)
		if err != nil || !sequences[r.Sequence] {
			t.Fatalf("producer-scoped replay = %#v, %v", r, err)
		}
	}
	for i := 0; i < 254; i++ {
		l := ledgers[i%2]
		p := testPublish(fmt.Sprintf("event-%d", i))
		p.Source = l.Context().ProducerPrefix
		if _, err := l.Admit(p); err != nil {
			t.Fatalf("shared admission %d: %v", i, err)
		}
	}
	before := s.Snapshot()
	p := testPublish("overflow")
	p.Source = ledgers[1].Context().ProducerPrefix
	if _, err := ledgers[1].Admit(p); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("shared pending overflow error = %v", err)
	}
	if len(before) != 256 || !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("shared bound dropped or changed records")
	}
}

func TestConcurrentAdmissionsHaveUniqueSequences(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	var wg sync.WaitGroup
	results := make(chan Receipt, 100)
	failures := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := l.Admit(testPublish(fmt.Sprint(i)))
			if err != nil {
				failures <- err
				return
			}
			results <- r
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := make(map[int64]bool)
	for r := range results {
		if r.Sequence < 1 || r.Sequence > 100 || seen[r.Sequence] {
			t.Fatalf("duplicate or invalid sequence: %d", r.Sequence)
		}
		seen[r.Sequence] = true
	}
	if len(seen) != 100 {
		t.Fatalf("got %d distinct sequences", len(seen))
	}
}

func TestExpiryOwnershipAndTimestampReplay(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	zone := time.FixedZone("historical", 59)
	created := time.Date(1900, 1, 2, 12, 0, 0, 0, zone)
	s.SetClock(func() time.Time { return created })
	expires := created.Add(24 * time.Hour)
	originalExpiry := expires
	p := testPublish("expiry")
	p.ExpiresAt = &expires
	first, err := l.Admit(p)
	if err != nil {
		t.Fatal(err)
	}
	expires = expires.Add(time.Hour)
	if _, err := l.Admit(p); !errors.Is(err, ErrConflict) {
		t.Fatalf("mutated expiry error = %v", err)
	}
	p.ExpiresAt = &originalExpiry
	before := s.Snapshot()
	if got := snapshotRecords(t, s)[0].Frame.CreatedAt; !got.Equal(created) {
		t.Fatalf("creation instant changed: got %v, want %v", got, created)
	}
	replayed := testSession()
	if err := replayed.Restore(before); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, replayed.Snapshot()) {
		t.Fatal("timestamp snapshot changed across restore")
	}
	if r, err := testLedger(replayed).Admit(p); err != nil || r != repeated(first) {
		t.Fatalf("expiry duplicate after restore = %#v, %v", r, err)
	}
}

func TestRestoreFailureIsAtomic(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	if _, err := l.Admit(testPublish("original")); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	invalid := append(append([]json.RawMessage(nil), before...), json.RawMessage(`{"schemaVersion":2}`))
	if err := s.Restore(invalid); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("invalid snapshot error = %v", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed restore mutated ledger")
	}
	if err := s.Restore(before); err != nil {
		t.Fatalf("idempotent restore: %v", err)
	}
	if _, err := l.Admit(testPublish("newer")); err != nil {
		t.Fatal(err)
	}
	latest := s.Snapshot()
	if err := s.Restore(before); !errors.Is(err, ErrInvalidSnapshot) || !reflect.DeepEqual(latest, s.Snapshot()) {
		t.Fatalf("stale snapshot overwrote records: %v", err)
	}
	other := NewSession("other")
	if err := other.Restore(before); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("cross-session restore error = %v", err)
	}
}

func TestRetryDeduplicatesAfterTheBranchLeafAdvances(t *testing.T) {
	s := testSession()
	first := s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-1", Epoch: "epoch-1", Ancestors: map[string]bool{"": true, "leaf-1": true}})
	receipt, err := first.Admit(testPublish("a"))
	if err != nil {
		t.Fatal(err)
	}
	advanced := s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-2", Epoch: "epoch-1", Ancestors: map[string]bool{"": true, "leaf-1": true, "leaf-2": true}})
	again, err := advanced.Admit(testPublish("a"))
	if err != nil || again != repeated(receipt) {
		t.Fatalf("retry on the same branch = %+v, %v; want %+v", again, err, receipt)
	}
	sibling := s.Ledger(Context{ProducerPrefix: "build-tools.build", BranchLeaf: "leaf-3", Epoch: "epoch-1", Ancestors: map[string]bool{"": true, "leaf-3": true}})
	if _, err := sibling.Admit(testPublish("a")); !errors.Is(err, ErrConflict) {
		t.Fatalf("same event key on a sibling branch = %v, want ErrConflict", err)
	}
}

func TestWakeIsPartOfThePublicationIdentity(t *testing.T) {
	s := testSession()
	l := testLedger(s)
	p := testPublish("job-1")
	p.Wake = true
	first, err := l.Admit(p)
	if err != nil || first.Duplicate {
		t.Fatalf("first = %#v, %v", first, err)
	}
	if again, err := l.Admit(p); err != nil || !again.Duplicate || again.FrameID != first.FrameID {
		t.Fatalf("identical wake publication = %#v, %v", again, err)
	}
	quiet := p
	quiet.Wake = false
	if _, err := l.Admit(quiet); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key with a different wake = %v, want conflict", err)
	}
	restored := testSession()
	if err := restored.Restore(s.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if again, err := testLedger(restored).Admit(p); err != nil || !again.Duplicate {
		t.Fatalf("wake lost across restore: %#v, %v", again, err)
	}
}
