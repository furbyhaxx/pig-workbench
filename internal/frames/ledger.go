package frames

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type storedRecord struct {
	SchemaVersion  int        `json:"schemaVersion"`
	ProducerPrefix string     `json:"producerPrefix"`
	Frame          Frame      `json:"frame"`
	DataTrust     string     `json:"dataTrust"`
	CoalesceKey   string     `json:"coalesceKey"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	State         string     `json:"state"`
	Receipt       Receipt    `json:"receipt"`
}

type eventKey struct {
	producer string
	event    string
}

type coalesceKey struct {
	producer string
	branch   string
	epoch    string
	source   string
	subject  string
	key      string
}

// Session owns the sequence, admission limits and records shared by all producers.
type Session struct {
	mu        sync.Mutex
	sessionID string
	clock     func() time.Time
	sequence  int64
	records   []*storedRecord
	events    map[eventKey]*storedRecord
	pending   map[coalesceKey]*storedRecord
	count     int
	bodySize  int
}

// Ledger admits frames in an immutable producer and branch context.
type Ledger struct {
	session *Session
	context Context
}

// NewSession creates an empty, non-durable session shared by all its producers.
func NewSession(sessionID string) *Session {
	return &Session{
		sessionID: sessionID,
		clock:     time.Now,
		events:    make(map[eventKey]*storedRecord),
		pending:   make(map[coalesceKey]*storedRecord),
	}
}

// Ledger binds a producer and branch to the session, overriding ctx.SessionID.
func (s *Session) Ledger(ctx Context) *Ledger {
	ctx.SessionID = s.sessionID
	return &Ledger{session: s, context: ctx}
}

// NewLedger creates an isolated single-producer session; use Session for shared sessions.
func NewLedger(ctx Context) *Ledger {
	return NewSession(ctx.SessionID).Ledger(ctx)
}

// Context returns the immutable ledger context.
func (l *Ledger) Context() Context { return l.context }

// SetClock sets the session admission clock. A nil clock restores time.Now.
func (s *Session) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if clock == nil {
		clock = time.Now
	}
	s.clock = clock
}

// Admit validates and records a frame or returns the original idempotent receipt.
func (l *Ledger) Admit(p Publish) (Receipt, error) {
	p, err := ValidatePublish(p)
	if err != nil {
		return Receipt{}, err
	}
	if !validContext(l.context) {
		return Receipt{}, ErrInvalidContext
	}
	if !ownsSource(l.context, p.Source) {
		return Receipt{}, &OwnershipError{Source: p.Source, ProducerPrefix: l.context.ProducerPrefix}
	}

	s := l.session
	s.mu.Lock()
	defer s.mu.Unlock()
	event := eventKey{producer: l.context.ProducerPrefix, event: p.EventKey}
	if existing := s.events[event]; existing != nil {
		if !samePublish(existing.publish(), p) || existing.Frame.BranchLeaf != l.context.BranchLeaf || existing.Frame.Epoch != l.context.Epoch {
			return Receipt{}, ErrConflict
		}
		return existing.Receipt, nil
	}

	var previous *storedRecord
	key, eligible := coalesceFor(l.context, p)
	if eligible {
		previous = s.pending[key]
	}
	pending := s.count
	if previous != nil {
		pending--
	}
	if pending >= maxPending {
		return Receipt{}, fmt.Errorf("%w: pending frame capacity", ErrLimitExceeded)
	}
	if len(p.Body) > maxBodyMemory-s.bodySize {
		return Receipt{}, fmt.Errorf("%w: retained body memory", ErrLimitExceeded)
	}
	if s.sequence == math.MaxInt64 {
		return Receipt{}, ErrSequenceExhausted
	}
	created := jsonTime(s.clock())
	if _, err := created.MarshalJSON(); err != nil {
		return Receipt{}, ErrInvalidClock
	}
	frame := makeFrame(l.context, p, s.sequence+1, created)
	receipt := makeReceipt(frame)
	record := &storedRecord{
		SchemaVersion:  schemaVersion,
		ProducerPrefix: l.context.ProducerPrefix,
		Frame:          frame,
		DataTrust:      p.DataTrust,
		CoalesceKey:    p.CoalesceKey,
		ExpiresAt:      p.ExpiresAt,
		State:          statePending,
		Receipt:        receipt,
	}
	if previous != nil {
		previous.State = stateSuperseded
		s.count--
	}
	s.sequence = frame.Sequence
	s.records = append(s.records, record)
	s.events[event] = record
	if eligible {
		s.pending[key] = record
	}
	s.count++
	s.bodySize += len(p.Body)
	return receipt, nil
}

// Snapshot returns independent versioned session records in admission order.
func (s *Session) Snapshot() []json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshot []json.RawMessage
	for _, record := range s.records {
		data, err := json.Marshal(record)
		if err != nil {
			panic("frames: invalid in-memory record: " + err.Error())
		}
		snapshot = append(snapshot, data)
	}
	return snapshot
}

// Restore atomically rebuilds the session from its complete, ordered records.
func (s *Session) Restore(snapshot []json.RawMessage) error {
	if !utf8.ValidString(s.sessionID) {
		return ErrInvalidContext
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]*storedRecord, 0, len(snapshot))
	events := make(map[eventKey]*storedRecord, len(snapshot))
	pending := make(map[coalesceKey]*storedRecord)
	var sequence int64
	var count, bodySize int
	for i, raw := range snapshot {
		if !utf8.Valid(raw) || !json.Valid(raw) {
			return fmt.Errorf("%w: record %d is not valid JSON", ErrInvalidSnapshot, i)
		}
		var record storedRecord
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return fmt.Errorf("%w: record %d cannot be decoded", ErrInvalidSnapshot, i)
		}
		if record.SchemaVersion != schemaVersion || record.Frame.SchemaVersion != schemaVersion || record.Frame.Sequence <= sequence {
			return fmt.Errorf("%w: record %d has an invalid version or sequence", ErrInvalidSnapshot, i)
		}
		ctx := Context{SessionID: s.sessionID, ProducerPrefix: record.ProducerPrefix, BranchLeaf: record.Frame.BranchLeaf, Epoch: record.Frame.Epoch}
		p := record.publish()
		normalized, err := ValidatePublish(p)
		if err != nil || !samePublish(normalized, p) || !validContext(ctx) || !ownsSource(ctx, p.Source) {
			return fmt.Errorf("%w: record %d has an invalid publication", ErrInvalidSnapshot, i)
		}
		if record.Frame != makeFrame(ctx, p, record.Frame.Sequence, record.Frame.CreatedAt) || record.Receipt != makeReceipt(record.Frame) {
			return fmt.Errorf("%w: record %d has inconsistent provenance", ErrInvalidSnapshot, i)
		}
		event := eventKey{producer: ctx.ProducerPrefix, event: p.EventKey}
		if events[event] != nil {
			return fmt.Errorf("%w: record %d repeats an event key", ErrInvalidSnapshot, i)
		}
		if len(p.Body) > maxBodyMemory-bodySize {
			return fmt.Errorf("%w: body memory bound", ErrInvalidSnapshot)
		}
		key, eligible := coalesceFor(ctx, p)
		switch record.State {
		case statePending:
			count++
			if count > maxPending {
				return fmt.Errorf("%w: pending frame bound", ErrInvalidSnapshot)
			}
			if eligible {
				if pending[key] != nil {
					return fmt.Errorf("%w: duplicate pending coalescing key", ErrInvalidSnapshot)
				}
				pending[key] = &record
			}
		case stateSuperseded:
			if !eligible {
				return fmt.Errorf("%w: non-replaceable record superseded", ErrInvalidSnapshot)
			}
		default:
			return fmt.Errorf("%w: record %d has an unsupported state", ErrInvalidSnapshot, i)
		}
		sequence = record.Frame.Sequence
		bodySize += len(p.Body)
		records = append(records, &record)
		events[event] = &record
	}
	if len(records) < len(s.records) || sequence < s.sequence {
		return fmt.Errorf("%w: snapshot would discard admitted records", ErrInvalidSnapshot)
	}
	for i, existing := range s.records {
		oldData, oldErr := json.Marshal(existing)
		newData, newErr := json.Marshal(records[i])
		if oldErr != nil || newErr != nil || !bytes.Equal(oldData, newData) {
			return fmt.Errorf("%w: snapshot would change admitted records", ErrInvalidSnapshot)
		}
	}
	s.records = records
	s.events = events
	s.pending = pending
	s.sequence = sequence
	s.count = count
	s.bodySize = bodySize
	return nil
}

func (r *storedRecord) publish() Publish {
	f := r.Frame
	return Publish{
		EventKey: f.EventKey, Kind: f.Kind, Source: f.Source, Body: f.Body,
		Title: f.Title, Summary: f.Summary, SubjectKey: f.SubjectKey, Resource: f.Resource,
		Delivery: f.Delivery, DataTrust: r.DataTrust, CoalesceKey: r.CoalesceKey, ExpiresAt: r.ExpiresAt,
	}
}

func samePublish(a, b Publish) bool {
	if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) {
		return false
	}
	if a.ExpiresAt != nil && !a.ExpiresAt.Equal(*b.ExpiresAt) {
		return false
	}
	a.ExpiresAt, b.ExpiresAt = nil, nil
	return a == b
}

func coalesceFor(ctx Context, p Publish) (coalesceKey, bool) {
	return coalesceKey{producer: ctx.ProducerPrefix, branch: ctx.BranchLeaf, epoch: ctx.Epoch, source: p.Source, subject: p.SubjectKey, key: p.CoalesceKey},
		p.CoalesceKey != "" && (p.Kind == "info" || p.Kind == "notice")
}

func makeFrame(ctx Context, p Publish, sequence int64, created time.Time) Frame {
	identity, err := json.Marshal(struct {
		Context  Context `json:"context"`
		Sequence int64   `json:"sequence"`
	}{ctx, sequence})
	if err != nil {
		panic("frames: invalid frame identity: " + err.Error())
	}
	idDigest := sha256.Sum256(identity)
	bodyDigest := sha256.Sum256([]byte(p.Body))
	owner, _, _ := strings.Cut(ctx.ProducerPrefix, ".")
	return Frame{
		SchemaVersion: schemaVersion, FrameID: "f-" + hex.EncodeToString(idDigest[:16]), Sequence: sequence,
		SessionID: ctx.SessionID, Source: p.Source, ProducerOwner: owner, Kind: p.Kind, Body: p.Body,
		Title: p.Title, Summary: p.Summary, SubjectKey: p.SubjectKey, Resource: p.Resource,
		Delivery: p.Delivery, Trust: "untrusted", Authority: "data", EventKey: p.EventKey,
		ContentDigest: "sha256:" + hex.EncodeToString(bodyDigest[:]), CreatedAt: created,
		Nonce: strings.ToUpper(hex.EncodeToString(idDigest[16:20])), BranchLeaf: ctx.BranchLeaf, Epoch: ctx.Epoch,
	}
}

func makeReceipt(frame Frame) Receipt {
	return Receipt{
		FrameID: frame.FrameID, Sequence: frame.Sequence, State: statePending,
		Delivery: frame.Delivery, EffectiveAuthority: frame.Authority,
	}
}
