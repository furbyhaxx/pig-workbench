package ask

import (
	"bytes"
	"encoding/json"
	"slices"
)

// Answered contains only the final answer returned for a question.
type Answered struct {
	Values      []string `json:"values,omitempty"`
	Custom      string   `json:"custom,omitempty"`
	Note        string   `json:"note,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

// Result is a submitted, cancelled, timed-out or unavailable interaction.
// Order is for Go callers; JSON object member order is not a consumer contract.
type Result struct {
	Answers     map[string]Answered `json:"answers,omitempty"`
	Cancelled   bool                `json:"cancelled,omitempty"`
	TimedOut    bool                `json:"timedOut,omitempty"`
	Unavailable string              `json:"-"`
	Reason      string              `json:"reason,omitempty"`
	Order       []string            `json:"-"`
}

// MarshalJSON emits the result contract and deterministically orders answer members.
func (r Result) MarshalJSON() ([]byte, error) {
	if r.Cancelled {
		return json.Marshal(struct {
			Cancelled bool   `json:"cancelled"`
			Reason    string `json:"reason,omitempty"`
		}{true, r.Reason})
	}
	if r.TimedOut {
		return json.Marshal(struct {
			TimedOut bool   `json:"timedOut"`
			Reason   string `json:"reason,omitempty"`
		}{true, r.Reason})
	}
	if r.Unavailable != "" {
		reason := r.Reason
		if reason == "" {
			reason = r.Unavailable
		}
		return json.Marshal(struct {
			Unavailable bool   `json:"interactionUnavailable"`
			Reason      string `json:"reason"`
		}{true, reason})
	}

	var buf bytes.Buffer
	buf.WriteString(`{"answers":{`)
	seen := make(map[string]bool, len(r.Answers))
	write := func(id string) error {
		answer, ok := r.Answers[id]
		if !ok || seen[id] {
			return nil
		}
		if len(seen) != 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(id)
		if err != nil {
			return err
		}
		value, err := json.Marshal(answer)
		if err != nil {
			return err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
		seen[id] = true
		return nil
	}
	for _, id := range r.Order {
		if err := write(id); err != nil {
			return nil, err
		}
	}
	remaining := make([]string, 0, len(r.Answers)-len(seen))
	for id := range r.Answers {
		if !seen[id] {
			remaining = append(remaining, id)
		}
	}
	slices.Sort(remaining)
	for _, id := range remaining {
		if err := write(id); err != nil {
			return nil, err
		}
	}
	buf.WriteString(`}}`)
	return buf.Bytes(), nil
}
