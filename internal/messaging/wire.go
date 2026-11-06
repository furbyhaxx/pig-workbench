// Package messaging is the SDK-free transport and policy for messages between live sessions of one user.
package messaging

import (
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
	"unicode/utf8"
)

const (
	// ProtocolVersion is the wire major version.
	ProtocolVersion = 1
	// MaxFrame is the largest accepted frame body.
	MaxFrame = 256 << 10
	// MaxBody is the largest message body.
	MaxBody = 32 << 10
	// MaxAttachments is the largest number of resource references in one message.
	MaxAttachments = 16
	// CallTimeout bounds one request and its reply.
	CallTimeout = 5 * time.Second
)

// Receipt statuses and rejection reasons (SPEC-0011 §4).
const (
	Accepted        = "accepted"
	Rejected        = "rejected"
	NotDiscoverable = "notDiscoverable"
	SessionChanged  = "sessionChanged"
	PolicyDenied    = "policyDenied"
	RateLimited     = "rateLimited"
	TooLarge        = "tooLarge"
	Expired         = "expired"
	Busy            = "busy"
)

// Delivery dispositions reported with an accepted receipt.
const (
	Delivered = "delivered"
	Woken     = "woken"
	Steered   = "steered"
	Queued    = "queued"
)

// Peer identifies the sender of a message.
type Peer struct {
	EndpointID  string `json:"endpointId"`
	SessionID   string `json:"sessionId"`
	SessionName string `json:"sessionName,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectSlug string `json:"projectSlug,omitempty"`
}

// Envelope is one message between sessions.
type Envelope struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	InReplyTo      string    `json:"inReplyTo,omitempty"`
	Hop            int       `json:"hop"`
	From           Peer      `json:"from"`
	ToSessionID    string    `json:"toSessionId"`
	Subject        string    `json:"subject,omitempty"`
	Body           string    `json:"body"`
	Attachments    []string  `json:"attachments,omitempty"`
	Wake           string    `json:"wake,omitempty"`
	Progress       bool      `json:"progress,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
}

// Validate checks the limits that hold regardless of who receives the envelope.
func (e Envelope) Validate() error {
	switch {
	case e.ID == "" || e.ConversationID == "" || e.ToSessionID == "" || e.From.SessionID == "":
		return errors.New("message needs id, conversation, sender and recipient")
	case e.From.SessionID == e.ToSessionID:
		return errors.New("a session cannot message itself")
	case !utf8.ValidString(e.Body) || !utf8.ValidString(e.Subject):
		return errors.New("message text must be valid UTF-8")
	case len(e.Attachments) > MaxAttachments:
		return fmt.Errorf("a message carries at most %d attachments", MaxAttachments)
	case e.Hop < 0:
		return errors.New("hop must not be negative")
	}
	return nil
}

// Info describes a live session without its token.
type Info struct {
	SessionID   string    `json:"sessionId"`
	SessionName string    `json:"sessionName,omitempty"`
	ProjectID   string    `json:"projectId,omitempty"`
	ProjectSlug string    `json:"projectSlug,omitempty"`
	Cwd         string    `json:"cwd,omitempty"`
	State       string    `json:"state"`
	StateSince  time.Time `json:"stateSince,omitempty"`
	Model       string    `json:"model,omitempty"`
}

// Receipt is the synchronous answer to a message.
type Receipt struct {
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Delivery       string `json:"delivery,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Current        *Info  `json:"current,omitempty"`
}

type hello struct {
	V     int    `json:"v"`
	Type  string `json:"type"`
	Token string `json:"token"`
	From  string `json:"from"`
}

// Request is the single request of a connection: "info" or "message".
type Request struct {
	V       int       `json:"v"`
	Type    string    `json:"type"`
	Message *Envelope `json:"message,omitempty"`
}

// Reply answers a request.
type Reply struct {
	Info    *Info    `json:"info,omitempty"`
	Receipt *Receipt `json:"receipt,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func writeFrame(conn net.Conn, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > MaxFrame {
		return errors.New("frame exceeds the maximum size")
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(body)))
	_, err = conn.Write(append(header, body...))
	return err
}

func readFrame(conn net.Conn, value any) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header)
	if size == 0 || size > MaxFrame {
		return errors.New("frame size out of range")
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}

func sameToken(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
