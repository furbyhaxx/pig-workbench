package ask

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// StatePending identifies an unresolved questionnaire.
	StatePending = "pending"
	// StateCompleted identifies a submitted questionnaire.
	StateCompleted = "completed"
	// StateCancelled identifies a dismissed questionnaire.
	StateCancelled = "cancelled"
	// StateTimedOut identifies a questionnaire whose dialog expired.
	StateTimedOut = "timedOut"
)

// Record is a persisted questionnaire state; Payload contains the complete form.
type Record struct {
	SchemaVersion int             `json:"schemaVersion"`
	AskID         string          `json:"askId"`
	SessionID     string          `json:"sessionId"`
	State         string          `json:"state"`
	Source        string          `json:"source,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	Result        json.RawMessage `json:"result,omitempty"`
}

// NewRecord creates a pending record for a validated form at the supplied time.
func NewRecord(askID, sessionID, source string, form Form, now time.Time) (Record, error) {
	if strings.TrimSpace(askID) == "" {
		return Record{}, fmt.Errorf("askId: must not be empty")
	}
	if strings.TrimSpace(sessionID) == "" {
		return Record{}, fmt.Errorf("sessionId: must not be empty")
	}
	for _, field := range []struct{ name, value string }{
		{"askId", askID}, {"sessionId", sessionID}, {"source", source},
	} {
		if err := validateText(field.name, field.value, false); err != nil {
			return Record{}, err
		}
	}
	if now.IsZero() {
		return Record{}, fmt.Errorf("createdAt: time is required")
	}
	payload, err := form.Marshal()
	if err != nil {
		return Record{}, err
	}
	now = now.UTC()
	return Record{
		SchemaVersion: 1, AskID: askID, SessionID: sessionID,
		State: StatePending, Source: source, Payload: payload,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Key identifies an ask within a session without delimiter collisions.
func (r Record) Key() string {
	return strconv.Itoa(len(r.SessionID)) + ":" + r.SessionID + r.AskID
}

// LoadPending returns the latest unresolved ask from branch entries in oldest-first order.
// Entries can be records or PiG custom-entry envelopes; invalid entries are ignored.
func LoadPending(entries []json.RawMessage) (Record, bool, error) {
	type seenRecord struct {
		record   Record
		position int
		terminal bool
	}
	seen := make(map[string]seenRecord)
	for i, entry := range entries {
		record, ok := decodeRecord(entry)
		if !ok {
			continue
		}
		key := record.Key()
		if seen[key].terminal {
			continue
		}
		seen[key] = seenRecord{record: record, position: i, terminal: record.State != StatePending}
	}
	var newest seenRecord
	found := false
	for _, item := range seen {
		if !item.terminal && (!found || item.position > newest.position) {
			newest, found = item, true
		}
	}
	return newest.record, found, nil
}

func decodeRecord(raw json.RawMessage) (Record, bool) {
	var entry struct {
		Type       string          `json:"type"`
		CustomType string          `json:"customType"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return Record{}, false
	}
	if entry.Type != "" || entry.CustomType != "" {
		if entry.Type != "custom" || entry.CustomType != "pig-workbench.ask" {
			return Record{}, false
		}
		raw = entry.Data
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil || record.SchemaVersion != 1 {
		return Record{}, false
	}
	if strings.TrimSpace(record.AskID) == "" || strings.TrimSpace(record.SessionID) == "" || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return Record{}, false
	}
	for _, value := range []string{record.AskID, record.SessionID, record.Source} {
		if validateText("record", value, false) != nil {
			return Record{}, false
		}
	}
	switch record.State {
	case StatePending, StateCompleted, StateCancelled, StateTimedOut:
	default:
		return Record{}, false
	}
	if len(record.Payload) == 0 || string(record.Payload) == "null" {
		return record, record.State != StatePending
	}
	if _, err := ParseQuestions(record.Payload); err != nil {
		return Record{}, false
	}
	return record, true
}
