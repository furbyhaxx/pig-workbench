package ask

import (
	"encoding/json"
	"testing"
	"time"
)

func testRecord(t *testing.T, id string) Record {
	t.Helper()
	r, err := NewRecord(id, "session-1", "tool-call", validForm(), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func testEntry(t *testing.T, r Record) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(struct {
		Type       string `json:"type"`
		CustomType string `json:"customType"`
		Data       Record `json:"data"`
	}{Type: "custom", CustomType: "pig-workbench.ask", Data: r})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLoadPendingIgnoresMalformedAndCompleted(t *testing.T) {
	old := testRecord(t, "old")
	invalid := testRecord(t, "invalid")
	invalid.Payload = json.RawMessage(`{"questions":[{"id":"invalid","label":"Invalid","type":"single"}]}`)
	completed := testRecord(t, "new")
	completed.State = StateCompleted

	got, ok, err := LoadPending([]json.RawMessage{
		testEntry(t, old),
		json.RawMessage(`{"broken":`),
		testEntry(t, invalid),
		testEntry(t, completed),
	})
	if err != nil || !ok || got.AskID != old.AskID {
		t.Fatalf("expected old pending record; got %#v, %t, %v", got, ok, err)
	}
	form, err := ParseQuestions(got.Payload)
	if err != nil || form.Title != "Storage policy" {
		t.Fatalf("recovered payload lost form: %#v, %v", form, err)
	}
}

func TestLoadPendingResolvesOnlyMatchingAskAndSession(t *testing.T) {
	first := testRecord(t, "first")
	newer := testRecord(t, "newer")
	completed := newer
	completed.State = StateCompleted
	completed.Payload = nil

	entries := []json.RawMessage{testEntry(t, first), testEntry(t, newer), testEntry(t, completed)}
	got, ok, err := LoadPending(entries)
	if err != nil || !ok || got.AskID != first.AskID {
		t.Fatalf("wrong unresolved ask: %#v, %t, %v", got, ok, err)
	}
	completed.SessionID = "other-session"
	got, ok, err = LoadPending([]json.RawMessage{testEntry(t, newer), testEntry(t, completed)})
	if err != nil || !ok || got.AskID != newer.AskID {
		t.Fatalf("other session resolved pending ask: %#v, %t, %v", got, ok, err)
	}
	completed.SessionID = newer.SessionID
	_, ok, err = LoadPending([]json.RawMessage{testEntry(t, newer), testEntry(t, completed)})
	if err != nil || ok {
		t.Fatalf("completed record returned as pending: %t, %v", ok, err)
	}
}

func TestLoadPendingRejectsInvalidLatestPayloadAndTerminalRegression(t *testing.T) {
	pending := testRecord(t, "ask")
	badUpdate := pending
	badUpdate.Payload = json.RawMessage(`{"questions":[],"unexpected":true}`)
	got, ok, err := LoadPending([]json.RawMessage{testEntry(t, pending), testEntry(t, badUpdate)})
	if err != nil || !ok || got.AskID != pending.AskID {
		t.Fatalf("invalid update hid valid ask: %#v, %t, %v", got, ok, err)
	}
	completed := pending
	completed.State = StateCompleted
	_, ok, err = LoadPending([]json.RawMessage{testEntry(t, pending), testEntry(t, completed), testEntry(t, pending)})
	if err != nil || ok {
		t.Fatalf("pending update resurrected a completed ask: %t, %v", ok, err)
	}
}

func TestLoadPendingUsesBranchOrderAndIgnoresInvalidTerminal(t *testing.T) {
	first := testRecord(t, "first")
	newer := testRecord(t, "newer")
	newer.CreatedAt = first.CreatedAt.Add(-time.Hour)
	newer.UpdatedAt = newer.CreatedAt
	got, ok, err := LoadPending([]json.RawMessage{testEntry(t, first), testEntry(t, newer)})
	if err != nil || !ok || got.AskID != newer.AskID {
		t.Fatalf("branch order did not determine newest ask: %#v, %t, %v", got, ok, err)
	}
	incomplete := newer
	incomplete.State = StateCompleted
	incomplete.CreatedAt = time.Time{}
	got, ok, err = LoadPending([]json.RawMessage{testEntry(t, newer), testEntry(t, incomplete)})
	if err != nil || !ok || got.AskID != newer.AskID {
		t.Fatalf("incomplete terminal record hid a pending ask: %#v, %t, %v", got, ok, err)
	}
}

func TestRecordWireAndKey(t *testing.T) {
	record := testRecord(t, "ask-1")
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["payload"]; !ok {
		t.Fatalf("record has no payload field: %s", raw)
	}
	if record.SchemaVersion != 1 || record.State != StatePending || record.CreatedAt.IsZero() || !record.CreatedAt.Equal(record.UpdatedAt) {
		t.Fatalf("invalid initial record: %#v", record)
	}
	a := Record{SessionID: "a:b", AskID: "c"}
	b := Record{SessionID: "a", AskID: "b:c"}
	if a.Key() == b.Key() {
		t.Fatalf("record keys collide: %q", a.Key())
	}
}
