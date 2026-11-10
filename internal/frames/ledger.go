package frames

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/endpoint"
)

type storedRecord struct {
	Trusted        bool       `json:"trusted,omitempty"`
	Authority      string     `json:"authority,omitempty"`
	SchemaVersion  int        `json:"schemaVersion"`
	ProducerPrefix string     `json:"producerPrefix"`
	Frame          Frame      `json:"frame"`
	DataTrust      string     `json:"dataTrust"`
	CoalesceKey    string     `json:"coalesceKey"`
	Wake           bool       `json:"wake,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	State          string     `json:"state"`
	Receipt        Receipt    `json:"receipt"`
	// BodyDropped marks a terminal record whose body left memory; the digest still identifies it.
	BodyDropped bool `json:"bodyDropped,omitempty"`
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
	nonce     string
	clock     func() time.Time
	sequence  int64
	records   []*storedRecord
	events    map[eventKey]*storedRecord
	pending   map[coalesceKey]*storedRecord
	byID      map[string]*storedRecord
	count     int
	// bodySize counts pending bodies, historySize the bodies of records that left pending.
	bodySize    int
	historySize int
	trimmed     int
}

// Ledger admits frames in an immutable producer and branch context.
type Ledger struct {
	session *Session
	context Context
}

// NewSession creates an empty, non-durable session shared by all its producers.
func NewSession(sessionID string) *Session {
	var token [4]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic("frames: no randomness for the session nonce: " + err.Error())
	}
	return &Session{
		sessionID: sessionID,
		nonce:     strings.ToUpper(hex.EncodeToString(token[:])),
		clock:     time.Now,
		events:    make(map[eventKey]*storedRecord),
		pending:   make(map[coalesceKey]*storedRecord),
		byID:      make(map[string]*storedRecord),
	}
}

// Nonce returns the provenance cue every frame admitted by this session carries.
func (s *Session) Nonce() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nonce
}

// SetNonce replaces the nonce of frames admitted from now on, so the host can keep one nonce for a
// whole session lineage. Frames already admitted or restored keep the nonce they were stored with.
func (s *Session) SetNonce(nonce string) error {
	if !ValidNonce(nonce) {
		return fmt.Errorf("%w: nonce must be eight uppercase hexadecimal digits", ErrInvalidContext)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nonce = nonce
	return nil
}

// ValidNonce reports whether nonce has the stored form: eight uppercase hexadecimal digits.
func ValidNonce(nonce string) bool {
	if len(nonce) != 8 {
		return false
	}
	for _, r := range nonce {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
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

// SetClock sets the admission clock. A nil clock restores time.Now; the clock must not reenter the session.
func (s *Session) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if clock == nil {
		clock = time.Now
	}
	s.clock = clock
}

func (l *Ledger) sameBranch(leaf string) bool {
	if l.context.Ancestors != nil {
		return l.context.Ancestors[leaf]
	}
	return leaf == l.context.BranchLeaf
}

// Admit validates and records a frame or returns the original idempotent receipt.
func (l *Ledger) Admit(p Publish) (Receipt, error) {
	p, err := ValidatePublish(p)
	if err != nil {
		return Receipt{}, err
	}
	if !validContext(l.context) || l.context.Authority == "instruction" && p.DataTrust != "trusted" {
		return Receipt{}, ErrInvalidContext
	}
	if !ownsSource(l.context, p.Source) {
		return Receipt{}, &OwnershipError{Source: p.Source, ProducerPrefix: l.context.ProducerPrefix}
	}

	s := l.session
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	s.expireLocked(now)
	event := eventKey{producer: l.context.ProducerPrefix, event: p.EventKey}
	if existing := s.events[event]; existing != nil {
		if !existing.matches(p) || !l.sameBranch(existing.Frame.BranchLeaf) || existing.Frame.Epoch != l.context.Epoch || existing.Trusted != l.context.Trusted || existing.Authority != l.context.Authority {
			return Receipt{}, ErrConflict
		}
		duplicate := existing.Receipt
		duplicate.Duplicate = true
		return duplicate, nil
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
		return Receipt{}, fmt.Errorf("%w: pending body memory", ErrLimitExceeded)
	}
	if s.sequence >= 1<<53-1 {
		return Receipt{}, ErrSequenceExhausted
	}
	created := jsonTime(now)
	if _, err := created.MarshalJSON(); err != nil {
		return Receipt{}, ErrInvalidClock
	}
	frame := makeFrame(l.context, p, s.sequence+1, created, s.nonce)
	receipt := makeReceipt(frame)
	record := &storedRecord{
		Trusted: l.context.Trusted, Authority: l.context.Authority,
		SchemaVersion:  schemaVersion,
		ProducerPrefix: l.context.ProducerPrefix,
		Frame:          frame,
		DataTrust:      p.DataTrust,
		CoalesceKey:    p.CoalesceKey,
		Wake:           p.Wake,
		ExpiresAt:      p.ExpiresAt,
		State:          statePending,
		Receipt:        receipt,
	}
	if previous != nil {
		previous.State = stateSuperseded
		s.count--
		s.toHistoryLocked(previous)
	}
	s.sequence = frame.Sequence
	s.records = append(s.records, record)
	s.events[event] = record
	s.byID[frame.FrameID] = record
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
	if !endpoint.ValidSessionID(s.sessionID) {
		return ErrInvalidContext
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]*storedRecord, 0, len(snapshot))
	events := make(map[eventKey]*storedRecord, len(snapshot))
	pending := make(map[coalesceKey]*storedRecord)
	byID := make(map[string]*storedRecord, len(snapshot))
	var sequence int64
	var count, bodySize, historySize int
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
		if record.SchemaVersion != schemaVersion || record.Frame.SchemaVersion != schemaVersion || record.Frame.Sequence <= sequence || record.Frame.Sequence > 1<<53-1 {
			return fmt.Errorf("%w: record %d has an invalid version or sequence", ErrInvalidSnapshot, i)
		}
		ctx := Context{SessionID: s.sessionID, ProducerPrefix: record.ProducerPrefix, BranchLeaf: record.Frame.BranchLeaf, Epoch: record.Frame.Epoch, Trusted: record.Trusted, Authority: record.Authority}
		p := record.publish()
		normalized, err := ValidatePublish(p)
		if err != nil || !samePublish(normalized, p) || !validContext(ctx) || !ownsSource(ctx, p.Source) {
			return fmt.Errorf("%w: record %d has an invalid publication", ErrInvalidSnapshot, i)
		}
		expected := makeFrame(ctx, p, record.Frame.Sequence, record.Frame.CreatedAt, record.Frame.Nonce)
		if record.BodyDropped {
			if record.State == statePending || p.Body != "" || !validDigest(record.Frame.ContentDigest) {
				return fmt.Errorf("%w: record %d dropped its body inconsistently", ErrInvalidSnapshot, i)
			}
			expected.ContentDigest = record.Frame.ContentDigest
		}
		if !ValidNonce(record.Frame.Nonce) || record.Frame != expected || record.Receipt != makeReceipt(record.Frame) {
			return fmt.Errorf("%w: record %d has inconsistent provenance", ErrInvalidSnapshot, i)
		}
		event := eventKey{producer: ctx.ProducerPrefix, event: p.EventKey}
		if events[event] != nil {
			return fmt.Errorf("%w: record %d repeats an event key", ErrInvalidSnapshot, i)
		}
		if record.State == statePending && len(p.Body) > maxBodyMemory-bodySize {
			return fmt.Errorf("%w: pending body memory bound", ErrInvalidSnapshot)
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
		case stateDelivered, stateCancelled, stateFailed:
		case stateExpired:
			if record.ExpiresAt == nil {
				return fmt.Errorf("%w: record %d expired without an expiry", ErrInvalidSnapshot, i)
			}
		default:
			return fmt.Errorf("%w: record %d has an unsupported state", ErrInvalidSnapshot, i)
		}
		sequence = record.Frame.Sequence
		if record.State == statePending {
			bodySize += len(p.Body)
		} else {
			historySize += len(p.Body)
		}
		records = append(records, &record)
		events[event] = &record
		byID[record.Frame.FrameID] = &record
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
	s.byID = byID
	s.sequence = sequence
	s.count = count
	s.bodySize = bodySize
	s.historySize = historySize
	s.trimmed = 0
	s.trimHistoryLocked()
	return nil
}

// matches reports whether p repeats this record's publication; a dropped body is compared by digest.
func (r *storedRecord) matches(p Publish) bool {
	stored := r.publish()
	if r.BodyDropped {
		digest := sha256.Sum256([]byte(p.Body))
		if r.Frame.ContentDigest != "sha256:"+hex.EncodeToString(digest[:]) {
			return false
		}
		p.Body = ""
	}
	return samePublish(stored, p)
}

// toHistoryLocked moves the body of a record that just left pending from the pending bound to history.
func (s *Session) toHistoryLocked(record *storedRecord) {
	s.bodySize -= len(record.Frame.Body)
	s.historySize += len(record.Frame.Body)
	s.trimHistoryLocked()
}

// trimHistoryLocked drops the oldest non-pending bodies until history fits its bound.
// The session entries the host persisted still hold those bodies.
func (s *Session) trimHistoryLocked() {
	advance := true
	for i := s.trimmed; i < len(s.records) && s.historySize > maxHistoryMemory; i++ {
		record := s.records[i]
		if record.State == statePending {
			advance = false
			continue
		}
		if !record.BodyDropped {
			s.historySize -= len(record.Frame.Body)
			record.Frame.Body = ""
			record.BodyDropped = true
		}
		if advance {
			s.trimmed = i + 1
		}
	}
}

func validDigest(digest string) bool {
	hash, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(hash) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func (r *storedRecord) publish() Publish {
	f := r.Frame
	return Publish{
		EventKey: f.EventKey, Kind: f.Kind, Source: f.Source, Body: f.Body,
		Title: f.Title, Summary: f.Summary, SubjectKey: f.SubjectKey, Resource: f.Resource,
		Delivery: f.Delivery, Wake: r.Wake, DataTrust: r.DataTrust, CoalesceKey: r.CoalesceKey, ExpiresAt: r.ExpiresAt,
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

func makeFrame(ctx Context, p Publish, sequence int64, created time.Time, nonce string) Frame {
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
	trust, authority := "untrusted", "data"
	if ctx.Trusted {
		trust = p.DataTrust
		if ctx.Authority != "" {
			authority = ctx.Authority
		}
	}
	return Frame{
		SchemaVersion: schemaVersion, FrameID: "f-" + hex.EncodeToString(idDigest[:16]), Sequence: sequence,
		SessionID: ctx.SessionID, Source: p.Source, ProducerOwner: owner, Kind: p.Kind, Body: p.Body,
		Title: p.Title, Summary: p.Summary, SubjectKey: p.SubjectKey, Resource: p.Resource,
		Delivery: p.Delivery, Trust: trust, Authority: authority, EventKey: p.EventKey,
		ContentDigest: "sha256:" + hex.EncodeToString(bodyDigest[:]), CreatedAt: created,
		Nonce: nonce, BranchLeaf: ctx.BranchLeaf, Epoch: ctx.Epoch,
	}
}

func makeReceipt(frame Frame) Receipt {
	return Receipt{
		FrameID: frame.FrameID, Sequence: frame.Sequence, State: statePending,
		Delivery: frame.Delivery, EffectiveAuthority: frame.Authority,
	}
}
