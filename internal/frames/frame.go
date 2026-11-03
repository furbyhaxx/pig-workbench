// Package frames owns attributed frame admission and replay.
package frames

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxBodyBytes    = 32 * 1024
	maxPending      = 256
	maxBodyMemory   = 2 * 1024 * 1024
	schemaVersion   = 1
	statePending    = "pending"
	stateSuperseded = "superseded"
)

var (
	// ErrInvalidPublish indicates a publication violates the frame contract.
	ErrInvalidPublish = errors.New("invalid frame publication")
	// ErrOwnership indicates that the source is outside the producer prefix.
	ErrOwnership = errors.New("frame source is not owned by the producer")
	// ErrInvalidContext indicates a ledger context cannot safely identify a producer.
	ErrInvalidContext = errors.New("invalid frame ledger context")
	// ErrConflict indicates an event key was reused with a different payload.
	ErrConflict = errors.New("frame event key conflicts with an existing publication")
	// ErrLimitExceeded indicates a frame or ledger capacity limit was reached.
	ErrLimitExceeded = errors.New("frame limit exceeded")
	// ErrSequenceExhausted indicates the sequence cannot be incremented.
	ErrSequenceExhausted = errors.New("frame sequence exhausted")
	// ErrInvalidSnapshot indicates stored records cannot safely reconstruct this ledger.
	ErrInvalidSnapshot = errors.New("invalid frame snapshot")
	// ErrInvalidClock indicates the clock returned a time outside the JSON timestamp range.
	ErrInvalidClock = errors.New("invalid frame clock time")
)

// OwnershipError identifies a source outside the ledger's producer prefix.
type OwnershipError struct {
	Source         string
	ProducerPrefix string
}

// Error implements error without including producer-controlled values.
func (e *OwnershipError) Error() string { return ErrOwnership.Error() }

// Unwrap permits matching ErrOwnership.
func (e *OwnershipError) Unwrap() error { return ErrOwnership }

// Context binds a ledger to a session, producer, branch and context epoch.
type Context struct {
	SessionID      string `json:"sessionId"`
	ProducerPrefix string `json:"producerPrefix"`
	BranchLeaf     string `json:"branchLeaf"`
	Epoch          string `json:"epoch"`
}

// Frame is the immutable attributed fact admitted by a ledger.
type Frame struct {
	SchemaVersion int       `json:"schemaVersion"`
	FrameID       string    `json:"frameId"`
	Sequence      int64     `json:"sequence"`
	SessionID     string    `json:"sessionId"`
	Source        string    `json:"source"`
	ProducerOwner string    `json:"producerOwner"`
	Kind          string    `json:"kind"`
	Body          string    `json:"body"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	SubjectKey    string    `json:"subjectKey"`
	Resource      string    `json:"resource"`
	Delivery      string    `json:"delivery"`
	Trust         string    `json:"trust"`
	Authority     string    `json:"authority"`
	EventKey      string    `json:"eventKey"`
	ContentDigest string    `json:"contentDigest"`
	CreatedAt     time.Time `json:"createdAt"`
	Nonce         string    `json:"nonce"`
	BranchLeaf    string    `json:"branchLeaf"`
	Epoch         string    `json:"epoch"`
}

// Publish describes one producer's requested frame.
type Publish struct {
	EventKey    string     `json:"eventKey"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	Body        string     `json:"body"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	SubjectKey  string     `json:"subjectKey"`
	Resource    string     `json:"resource"`
	Delivery    string     `json:"delivery"`
	DataTrust   string     `json:"dataTrust"`
	CoalesceKey string     `json:"coalesceKey"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// Receipt is the original, non-durable in-memory admission acknowledgement.
type Receipt struct {
	FrameID            string `json:"frameId"`
	Sequence           int64  `json:"sequence"`
	State              string `json:"state"`
	Delivery           string `json:"delivery"`
	EffectiveAuthority string `json:"effectiveAuthority"`
	Durable            bool   `json:"durable"`
}

// ValidatePublish checks and normalizes a publication without applying ownership policy.
func ValidatePublish(p Publish) (Publish, error) {
	if len(p.Body) > maxBodyBytes {
		return Publish{}, fmt.Errorf("%w: body exceeds 32 KiB", ErrLimitExceeded)
	}
	for _, value := range []string{p.EventKey, p.Kind, p.Source, p.Body, p.Title, p.Summary, p.SubjectKey, p.Resource, p.Delivery, p.DataTrust, p.CoalesceKey} {
		if !utf8.ValidString(value) {
			return Publish{}, fmt.Errorf("%w: text must be UTF-8", ErrInvalidPublish)
		}
	}
	if strings.TrimSpace(p.EventKey) == "" || !validSource(p.Source) {
		return Publish{}, fmt.Errorf("%w: event key and dotted source are required", ErrInvalidPublish)
	}
	switch p.Kind {
	case "instruction", "reminder", "warning", "error", "notice", "info", "message":
	default:
		return Publish{}, fmt.Errorf("%w: unsupported kind", ErrInvalidPublish)
	}
	if p.Delivery == "" {
		p.Delivery = "nextBoundary"
	}
	switch p.Delivery {
	case "nextBoundary", "nextPrompt", "beforePrompt", "followUp":
	default:
		return Publish{}, fmt.Errorf("%w: unsupported delivery", ErrInvalidPublish)
	}
	if p.DataTrust == "" {
		p.DataTrust = "untrusted"
	}
	if p.DataTrust != "trusted" && p.DataTrust != "untrusted" {
		return Publish{}, fmt.Errorf("%w: unsupported data trust", ErrInvalidPublish)
	}
	if p.ExpiresAt != nil {
		expires := jsonTime(*p.ExpiresAt)
		if _, err := expires.MarshalJSON(); err != nil {
			return Publish{}, fmt.Errorf("%w: invalid expiry timestamp", ErrInvalidPublish)
		}
		p.ExpiresAt = &expires
	}
	return p, nil
}

// RFC3339 JSON timestamps encode offsets only to minute precision.
func jsonTime(t time.Time) time.Time {
	t = t.Round(0)
	if _, offset := t.Zone(); offset%60 != 0 {
		return t.UTC()
	}
	return t
}

func validSource(source string) bool {
	if !utf8.ValidString(source) || source == "" {
		return false
	}
	for _, part := range strings.Split(source, ".") {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return false
		}
	}
	return true
}

func validSessionID(sessionID string) bool {
	return utf8.ValidString(sessionID) && strings.TrimSpace(sessionID) != ""
}

func validContext(ctx Context) bool {
	return validSessionID(ctx.SessionID) && validSource(ctx.ProducerPrefix) && utf8.ValidString(ctx.BranchLeaf) && utf8.ValidString(ctx.Epoch)
}

func ownsSource(ctx Context, source string) bool {
	return source == ctx.ProducerPrefix || strings.HasPrefix(source, ctx.ProducerPrefix+".")
}
