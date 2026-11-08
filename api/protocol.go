// Package api defines the optional Workbench v1 extension protocol.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// RequestChannel carries client control requests.
const RequestChannel = "workbench:v1:request"

// ReplyChannel carries addressed Workbench replies.
const ReplyChannel = "workbench:v1:reply"

// CancelChannel carries `cancel` requests for a caller's own in-flight control request.
// It is separate from RequestChannel because the host serializes a listener per channel,
// so a cancel queued behind a busy handler on RequestChannel would never be seen.
const CancelChannel = "workbench:v1:cancel"

// EventChannel carries generation-scoped Workbench notifications.
const EventChannel = "workbench:v1:event"

// ControlBytes is the maximum encoded control envelope size.
const ControlBytes = 65536

// Identity identifies one runtime incarnation of a stable extension owner.
type Identity struct {
	Owner    string `json:"owner"`
	Instance string `json:"instance"`
}

// Request is the versioned control envelope; Params has a method-specific schema.
type Request struct {
	Version        int             `json:"v"`
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId"`
	Generation     string          `json:"generation,omitempty"`
	From           Identity        `json:"from"`
	Method         string          `json:"method"`
	Params         json.RawMessage `json:"params"`
	TimeoutMS      int             `json:"timeoutMs,omitempty"`
	RegistrationID string          `json:"registrationId,omitempty"`
}

// Error is an actionable protocol failure, distinct from a transport error.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Field     string `json:"field,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
}

// Error returns the protocol error code and diagnostic.
func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Reply is an addressed result or error, never both.
type Reply struct {
	Version    int      `json:"v"`
	ID         string   `json:"id"`
	SessionID  string   `json:"sessionId"`
	Generation string   `json:"generation"`
	To         Identity `json:"to"`
	OK         bool     `json:"ok"`
	Result     any      `json:"result,omitempty"`
	Error      *Error   `json:"error,omitempty"`
}

// Event is a generation-scoped notification, not a transactional acknowledgement.
type Event struct {
	Version        int    `json:"v"`
	SessionID      string `json:"sessionId"`
	Generation     string `json:"generation"`
	EventID        string `json:"eventId"`
	Type           string `json:"type"`
	RegistrationID string `json:"registrationId,omitempty"`
	Data           any    `json:"data"`
}

// DiscoveryRequest specifies supported major versions and desired capabilities.
type DiscoveryRequest struct {
	Majors       []int    `json:"majors"`
	Capabilities []string `json:"capabilities"`
}

// Limits are the negotiated Workbench resource ceilings.
type Limits struct {
	ControlBytes     int `json:"controlBytes"`
	PageBytes        int `json:"pageBytes"`
	PageLines        int `json:"pageLines"`
	InFlightPerOwner int `json:"inFlightPerOwner"`
}

// Discovery is one live Workbench instance's version and capability declaration.
type Discovery struct {
	Major            int      `json:"major"`
	Minor            int      `json:"minor"`
	WorkbenchVersion string   `json:"workbenchVersion"`
	Instance         string   `json:"instance"`
	SessionID        string   `json:"sessionId"`
	Generation       string   `json:"generation"`
	Capabilities     []string `json:"capabilities"`
	Limits           Limits   `json:"limits"`
	Location         any      `json:"location,omitempty"`
}

// DecodeReply validates one bounded reply without interpreting method-specific result data.
func DecodeReply(value any) (Reply, error) {
	var reply Reply
	invalid := func() (Reply, error) {
		return Reply{}, &Error{Code: "invalidRequest", Message: "invalid reply envelope"}
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > ControlBytes {
		return invalid()
	}
	if Decode(encoded, &reply) != nil {
		return invalid()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(encoded, &fields) != nil {
		return invalid()
	}
	if !bytes.Equal(fields["ok"], []byte("true")) && !bytes.Equal(fields["ok"], []byte("false")) {
		return invalid()
	}
	if reply.Version != 1 || !validWireID(reply.ID) || !validWireID(reply.SessionID) || !validWireID(reply.Generation) || !validWireID(reply.To.Owner) || !validWireID(reply.To.Instance) {
		return invalid()
	}
	if reply.OK {
		if reply.Result == nil || reply.Error != nil {
			return invalid()
		}
	} else {
		if reply.Result != nil || reply.Error == nil || !validWireID(reply.Error.Code) || strings.TrimSpace(reply.Error.Message) == "" {
			return invalid()
		}
		if (reply.Error.Outcome == "unknown" || reply.Error.Code == "outcomeUnknown") && reply.Error.Retryable {
			return invalid()
		}
		if reply.Error.Code == "outcomeUnknown" && reply.Error.Outcome != "" && reply.Error.Outcome != "unknown" {
			return invalid()
		}
		switch reply.Error.Outcome {
		case "", "notApplied", "applied", "unknown":
		default:
			return invalid()
		}
	}
	return reply, nil
}

// DecodeRequest validates the common shape without interpreting method parameters.
func DecodeRequest(value any) (Request, error) {
	var request Request
	encoded, err := json.Marshal(value)
	if err != nil {
		return request, &Error{Code: "invalidRequest", Message: "control payload is not JSON"}
	}
	if len(encoded) > ControlBytes {
		if object, ok := value.(map[string]any); ok {
			request.ID, _ = object["id"].(string)
			request.SessionID, _ = object["sessionId"].(string)
			if from, ok := object["from"].(map[string]any); ok {
				request.From.Owner, _ = from["owner"].(string)
				request.From.Instance, _ = from["instance"].(string)
			}
			for _, id := range []string{request.ID, request.SessionID, request.From.Owner, request.From.Instance} {
				if id == "" || len(id) > 128 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
					request = Request{}
					break
				}
			}
		}
		return request, &Error{Code: "limitExceeded", Message: "control envelope exceeds 65536 bytes"}
	}
	if err := Decode(encoded, &request); err != nil {
		return request, &Error{Code: "invalidRequest", Message: "invalid control envelope"}
	}
	if request.Version != 1 {
		return request, &Error{Code: "incompatibleVersion", Message: "Workbench requires protocol v1"}
	}
	for field, value := range map[string]string{"id": request.ID, "sessionId": request.SessionID, "from.owner": request.From.Owner, "from.instance": request.From.Instance, "method": request.Method} {
		if value == "" || len(value) > 128 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return request, &Error{Code: "invalidRequest", Field: field, Message: "identifier must contain 1–128 UTF-8 bytes without controls"}
		}
	}
	if request.Method != "discover" && request.Generation == "" {
		return request, &Error{Code: "invalidRequest", Field: "generation", Message: "discover Workbench before making service requests"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return request, &Error{Code: "invalidRequest", Message: "invalid control envelope"}
	}
	_, timeoutSupplied := fields["timeoutMs"]
	if timeoutSupplied && request.TimeoutMS <= 0 || request.TimeoutMS > 300000 {
		return request, &Error{Code: "invalidRequest", Field: "timeoutMs", Message: "execution budget must be 1–300000 milliseconds when supplied"}
	}
	if len(request.Params) == 0 || bytes.Equal(request.Params, []byte("null")) {
		return request, &Error{Code: "invalidRequest", Field: "params", Message: "params must be an object"}
	}
	if request.Params[0] != '{' {
		return request, &Error{Code: "invalidRequest", Field: "params", Message: "params must be an object"}
	}
	return request, nil
}

// Decode rejects unknown fields and trailing JSON rather than silently accepting a different contract.
func Decode(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
