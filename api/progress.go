package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// ProviderProgressChannel carries bounded progress notes from a provider to Workbench while it serves one request.
// It is separate from the reply channel because the host serializes a listener per channel and the request is still running.
const ProviderProgressChannel = "workbench:v1:provider-progress"

// MaxProgressMessage bounds a progress note in Unicode characters.
const MaxProgressMessage = 200

// ProviderProgress is one progress note about an in-flight provider request. It is a notification:
// Workbench never replies, never re-enters the provider, and keeps only the newest note per request.
type ProviderProgress struct {
	Version    int      `json:"v"`
	SessionID  string   `json:"sessionId"`
	Generation string   `json:"generation"`
	From       Identity `json:"from"`
	To         Identity `json:"to"`
	RequestID  string   `json:"requestId"`
	// Seq increases by one per note; a note whose Seq is not newer than the last accepted one is dropped.
	Seq       int    `json:"seq"`
	Message   string `json:"message,omitempty"`
	Completed int64  `json:"completed,omitempty"`
	// Total is zero while unknown.
	Total int64 `json:"total,omitempty"`
}

// DecodeProgress validates one bounded progress envelope.
func DecodeProgress(value any) (ProviderProgress, error) {
	var progress ProviderProgress
	invalid := func(reason string) (ProviderProgress, error) {
		return ProviderProgress{}, &Error{Code: "invalidRequest", Message: "invalid progress envelope: " + reason}
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > ControlBytes {
		return invalid("size")
	}
	if Decode(encoded, &progress) != nil {
		return invalid("shape")
	}
	if progress.Version != 1 || !validWireID(progress.SessionID) || !validWireID(progress.Generation) || !validWireID(progress.RequestID) ||
		!validWireID(progress.From.Owner) || !validWireID(progress.From.Instance) || !validWireID(progress.To.Owner) || !validWireID(progress.To.Instance) {
		return invalid("identity")
	}
	if progress.Seq < 1 {
		return invalid("seq")
	}
	if progress.Completed < 0 || progress.Total < 0 || progress.Total > 0 && progress.Completed > progress.Total {
		return invalid("counts")
	}
	if utf8.RuneCountInString(progress.Message) > MaxProgressMessage || strings.IndexFunc(progress.Message, unicode.IsControl) >= 0 {
		return invalid("message")
	}
	return progress, nil
}

// ReportProgress tells Workbench how far a request has come. Call it from the handler with an increasing
// seq; the note is advisory and its loss never changes the reply. It must stay inside the request's budget.
func (r ProviderRequest) ReportProgress(ctx sdk.Context, seq int, message string, completed, total int64) error {
	note := ProviderProgress{Version: 1, SessionID: r.SessionID, Generation: r.Generation, From: r.To, To: r.From, RequestID: r.ID, Seq: seq, Message: message, Completed: completed, Total: total}
	if _, err := DecodeProgress(note); err != nil {
		return fmt.Errorf("progress note refused before sending: %w", err)
	}
	return ctx.Events().Emit(ProviderProgressChannel, note)
}
