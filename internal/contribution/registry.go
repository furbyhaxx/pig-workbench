// Package contribution owns generation-scoped contribution registration for SPEC-0001 §4.
//
// A Registry validates, admits, replaces and withdraws contributions for one Workbench
// generation. It performs no I/O and holds no SDK types: the host supplies descriptor
// validation, routes generations and owns lifecycle notification.
package contribution

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/api"
)

const (
	// MaxRegistrationsPerOwner is the SPEC-0001 default and maximum live registration count per owner.
	MaxRegistrationsPerOwner = 128
	// MaxDescriptorBytes is the SPEC-0001 default and maximum canonical descriptor size.
	MaxDescriptorBytes = 64 * 1024
	maxIdentifierBytes = 128
	maxSchemeBytes     = 32
	retainedInstances  = 8
)

// Kind is one registered contribution category.
type Kind string

// The contribution kinds named by SPEC-0001 §4.
const (
	KindFrameProducer    Kind = "frameProducer"
	KindToolPresentation Kind = "toolPresentation"
	KindResourceProvider Kind = "resourceProvider"
	KindFetchBackend     Kind = "fetchBackend"
	KindStatusSegment    Kind = "statusSegment"
	KindDocs             Kind = "docs"
)

var registeredKinds = map[Kind]bool{
	KindFrameProducer: true, KindToolPresentation: true, KindResourceProvider: true,
	KindFetchBackend: true, KindStatusSegment: true, KindDocs: true,
}

var (
	// ErrInvalidRegistration rejects a malformed registration request.
	ErrInvalidRegistration = errors.New("invalid contribution registration")
	// ErrInvalidDescriptor reports a descriptor the validator refused or the registry cannot canonicalize.
	ErrInvalidDescriptor = errors.New("contribution descriptor rejected")
	// ErrConflict reports a registration that collides with a live one.
	ErrConflict = errors.New("contribution conflicts with a live registration")
	// ErrLimitExceeded reports a registration, descriptor or capacity cap.
	ErrLimitExceeded = errors.New("contribution registry limit exceeded")
	// ErrNotFound reports an unknown registration.
	ErrNotFound = errors.New("contribution registration not found")
	// ErrStaleInstance reports a call from an owner incarnation that was replaced or retired.
	ErrStaleInstance = errors.New("contribution owner instance is retired")
	// ErrStaleGeneration reports a handle from another Workbench generation.
	ErrStaleGeneration = errors.New("contribution registration belongs to another generation")
	// ErrInstanceConflict reports a second incarnation without an explicit replacement.
	ErrInstanceConflict = errors.New("contribution owner already has an active instance")
	// ErrStaleRevision reports a replacement against a superseded descriptor revision.
	ErrStaleRevision = errors.New("contribution descriptor revision is stale")
)

// ConflictError names the exclusive claim or stable key a registration could not take.
type ConflictError struct {
	// Namespace is "scheme", "source" or "key".
	Namespace string
	// Claim is the colliding scheme or "source/field"; empty for a stable key.
	Claim string
	// Owner is the owner of the live registration that holds the name.
	Owner string
}

// Error implements error without echoing descriptor content.
func (e *ConflictError) Error() string {
	if e.Claim == "" {
		return fmt.Sprintf("%s: owner %q already registered this kind and key", ErrConflict, e.Owner)
	}
	return fmt.Sprintf("%s: %s claim %q is held by owner %q", ErrConflict, e.Namespace, e.Claim, e.Owner)
}

// Unwrap permits matching ErrConflict.
func (e *ConflictError) Unwrap() error { return ErrConflict }

// LimitError names the cap that refused a registration.
type LimitError struct {
	// Scope is "owner" or "descriptor".
	Scope string
	// Limit is the configured ceiling.
	Limit int
}

// Error implements error and names the limit, as SPEC-0001 §4 requires.
func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s limit of %d exceeded", ErrLimitExceeded, e.Scope, e.Limit)
}

// Unwrap permits matching ErrLimitExceeded.
func (e *LimitError) Unwrap() error { return ErrLimitExceeded }

// SourceClaim is one exclusive (source, field) context claim (SPEC-0014 §5.1).
type SourceClaim struct {
	Source string `json:"source"`
	Field  string `json:"field"`
}

// Claims are the exclusive names a descriptor takes; the registry admits all of them or none.
type Claims struct {
	// Schemes is a resourceProvider's canonical scheme and its aliases (SPEC-0004 §2).
	Schemes []string
	// Sources are context field claims on Workbench context sources.
	Sources []SourceClaim
}

// Validator validates one descriptor and reports the exclusive names it takes.
//
// The host owns per-kind descriptor schema. The registry calls the validator before it
// takes its lock, so the validator may read registry state but must not register.
type Validator func(kind Kind, key string, descriptor json.RawMessage) (Claims, error)

// Limits bound one registry; a zero value selects the SPEC-0001 default.
type Limits struct {
	// PerOwner caps live registrations per owner; applied after validation, atomically.
	PerOwner int
	// DescriptorBytes caps one canonical descriptor.
	DescriptorBytes int
}

// Options configure descriptor validation and capacity for one generation.
type Options struct {
	Validator Validator
	Limits    Limits
}

// Registration is one immutable accepted contribution snapshot.
type Registration struct {
	RegistrationID     string          `json:"registrationId"`
	DescriptorRevision string          `json:"descriptorRevision"`
	Generation         string          `json:"generation"`
	Owner              string          `json:"owner"`
	Instance           string          `json:"instance"`
	Kind               Kind            `json:"kind"`
	Key                string          `json:"key"`
	Descriptor         json.RawMessage `json:"descriptor"`
}

// Receipt acknowledges an accepted registration or replacement.
type Receipt struct {
	RegistrationID     string `json:"registrationId"`
	DescriptorRevision string `json:"descriptorRevision"`
	Generation         string `json:"generation"`
	Kind               Kind   `json:"kind"`
	Key                string `json:"key"`
	// Idempotent reports an identical descriptor that changed nothing.
	Idempotent bool `json:"idempotent"`
}

// Withdrawal lists the registrations removed with one owner incarnation.
type Withdrawal struct {
	Owner         string         `json:"owner"`
	Instance      string         `json:"instance"`
	Registrations []Registration `json:"registrations"`
}

// Registry holds one generation's live contributions.
//
// Identity is routing information, not authentication: SPEC-0001 §4 does not authenticate
// bus senders, so this registry prevents accidents and cross-owner collisions only. It keeps
// no global state; a generation change builds a new Registry and old handles report
// ErrStaleGeneration.
type Registry struct {
	generation       string
	generationPrefix string
	validator        Validator
	limits           Limits
	mu               sync.RWMutex
	entries          map[string]*entry
	keys             map[registrationKey]string
	schemeHolders    map[string]string
	sourceHolders    map[SourceClaim]string
	owners           map[string]*ownerState
}

type registrationKey struct {
	owner string
	kind  Kind
	key   string
}

type entry struct {
	registrationID     string
	descriptorRevision string
	owner              string
	instance           string
	kind               Kind
	key                string
	descriptor         []byte
	schemes            []string
	sources            []SourceClaim
}

type ownerState struct {
	active  string
	retired map[string]struct{}
	order   []string
}

// New creates an empty registry for one Workbench generation.
func New(generation string, options Options) (*Registry, error) {
	if !validIdentifier(generation) {
		return nil, fmt.Errorf("%w: generation must be 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	if options.Validator == nil {
		return nil, fmt.Errorf("%w: registry requires a descriptor validator", ErrInvalidRegistration)
	}
	limits := options.Limits
	if limits.PerOwner < 0 || limits.DescriptorBytes < 0 {
		return nil, fmt.Errorf("%w: limits must not be negative", ErrInvalidRegistration)
	}
	if limits.PerOwner == 0 {
		limits.PerOwner = MaxRegistrationsPerOwner
	}
	if limits.DescriptorBytes == 0 {
		limits.DescriptorBytes = MaxDescriptorBytes
	}
	if limits.PerOwner > MaxRegistrationsPerOwner || limits.DescriptorBytes > MaxDescriptorBytes {
		return nil, fmt.Errorf("%w: limits can be reduced, not raised above %d registrations and %d bytes", ErrInvalidRegistration, MaxRegistrationsPerOwner, MaxDescriptorBytes)
	}
	digest := sha256.Sum256([]byte(generation))
	return &Registry{
		generation:       generation,
		generationPrefix: hex.EncodeToString(digest[:4]),
		validator:        options.Validator,
		limits:           limits,
		entries:          make(map[string]*entry),
		keys:             make(map[registrationKey]string),
		schemeHolders:    make(map[string]string),
		sourceHolders:    make(map[SourceClaim]string),
		owners:           make(map[string]*ownerState),
	}, nil
}

// Register admits one contribution and returns its receipt.
//
// An identical descriptor for a live (owner, kind, key) is idempotent and returns the
// existing registration ID. A different descriptor conflicts; use Replace.
func (r *Registry) Register(identity api.Identity, kind Kind, key string, descriptor json.RawMessage) (Receipt, error) {
	if err := validateIdentity(identity); err != nil {
		return Receipt{}, err
	}
	if !registeredKinds[kind] {
		return Receipt{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidRegistration, kind)
	}
	if !validIdentifier(key) {
		return Receipt{}, fmt.Errorf("%w: key must be 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	canonical, revision, err := r.prepare(descriptor)
	if err != nil {
		return Receipt{}, err
	}
	claims, err := r.claims(kind, key, canonical)
	if err != nil {
		return Receipt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.admitLocked(identity); err != nil {
		return Receipt{}, err
	}
	index := registrationKey{owner: identity.Owner, kind: kind, key: key}
	if id, ok := r.keys[index]; ok {
		live := r.entries[id]
		if live.descriptorRevision == revision {
			return r.receipt(live, true), nil
		}
		return Receipt{}, &ConflictError{Namespace: "key", Owner: live.owner}
	}
	if held := r.liveLocked(identity.Owner); held >= r.limits.PerOwner {
		return Receipt{}, &LimitError{Scope: "owner", Limit: r.limits.PerOwner}
	}
	if err := r.checkClaimsLocked("", claims); err != nil {
		return Receipt{}, err
	}
	id, err := r.newIDLocked()
	if err != nil {
		return Receipt{}, err
	}
	live := &entry{
		registrationID: id, descriptorRevision: revision, owner: identity.Owner, instance: identity.Instance,
		kind: kind, key: key, descriptor: canonical, schemes: claims.schemes, sources: claims.sources,
	}
	r.entries[id] = live
	r.keys[index] = id
	r.holdLocked(id, live.schemes, live.sources)
	return r.receipt(live, false), nil
}

// Replace swaps one live registration's descriptor.
//
// descriptorRevision must equal the revision the caller read; a mismatch leaves the live
// descriptor untouched and reports ErrStaleRevision. Failed claim checks are equally atomic.
func (r *Registry) Replace(identity api.Identity, registrationID, descriptorRevision string, descriptor json.RawMessage) (Receipt, error) {
	current, err := r.Get(identity, registrationID)
	if err != nil {
		return Receipt{}, err
	}
	if !validIdentifier(descriptorRevision) {
		return Receipt{}, fmt.Errorf("%w: previous descriptor revision is required", ErrInvalidRegistration)
	}
	canonical, revision, err := r.prepare(descriptor)
	if err != nil {
		return Receipt{}, err
	}
	claims, err := r.claims(current.Kind, current.Key, canonical)
	if err != nil {
		return Receipt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkInstanceLocked(identity); err != nil {
		return Receipt{}, err
	}
	live, ok := r.entries[registrationID]
	if !ok {
		return Receipt{}, r.missLocked(registrationID)
	}
	if live.owner != identity.Owner {
		return Receipt{}, fmt.Errorf("%w: %s", ErrNotFound, registrationID)
	}
	if live.descriptorRevision != descriptorRevision {
		return Receipt{}, fmt.Errorf("%w: revision %q was superseded", ErrStaleRevision, descriptorRevision)
	}
	if live.descriptorRevision == revision {
		return r.receipt(live, true), nil
	}
	if err := r.checkClaimsLocked(registrationID, claims); err != nil {
		return Receipt{}, err
	}
	r.releaseLocked(registrationID, live.schemes, live.sources)
	live.descriptor, live.descriptorRevision = canonical, revision
	live.schemes, live.sources = claims.schemes, claims.sources
	r.holdLocked(registrationID, live.schemes, live.sources)
	return r.receipt(live, false), nil
}

// Get returns one live registration snapshot for its owning identity.
func (r *Registry) Get(identity api.Identity, registrationID string) (Registration, error) {
	if err := validateIdentity(identity); err != nil {
		return Registration{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkInstanceLocked(identity); err != nil {
		return Registration{}, err
	}
	live, ok := r.entries[registrationID]
	if !ok {
		return Registration{}, r.missLocked(registrationID)
	}
	if live.owner != identity.Owner {
		return Registration{}, fmt.Errorf("%w: %s", ErrNotFound, registrationID)
	}
	return live.registration(r.generation), nil
}

// Unregister removes one registration; repeating it for the same owner is a no-op.
func (r *Registry) Unregister(identity api.Identity, registrationID string) error {
	if err := validateIdentity(identity); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkInstanceLocked(identity); err != nil {
		return err
	}
	live, ok := r.entries[registrationID]
	if !ok {
		if r.foreignGenerationLocked(registrationID) {
			return fmt.Errorf("%w: %s", ErrStaleGeneration, registrationID)
		}
		return nil
	}
	if live.owner != identity.Owner {
		return fmt.Errorf("%w: %s", ErrNotFound, registrationID)
	}
	r.releaseLocked(registrationID, live.schemes, live.sources)
	delete(r.entries, registrationID)
	delete(r.keys, registrationKey{owner: live.owner, kind: live.kind, key: live.key})
	return nil
}

// List returns every live registration in (owner, kind, key) order with deep-copied descriptors.
func (r *Registry) List() []Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	registrations := make([]Registration, 0, len(r.entries))
	for _, live := range r.entries {
		registrations = append(registrations, live.registration(r.generation))
	}
	sortRegistrations(registrations)
	return registrations
}

// RetireOwner withdraws one owner incarnation's registrations and refuses it further.
//
// The owner's next incarnation registers normally. Repeating the call for a retired
// instance is a no-op. Other owners are never touched.
func (r *Registry) RetireOwner(identity api.Identity) (Withdrawal, error) {
	if err := validateIdentity(identity); err != nil {
		return Withdrawal{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	withdrawal := Withdrawal{Owner: identity.Owner, Instance: identity.Instance}
	state := r.owners[identity.Owner]
	if state == nil || state.active != identity.Instance {
		return withdrawal, nil
	}
	withdrawal = r.withdrawLocked(identity.Owner, identity.Instance)
	r.markRetiredLocked(state, identity.Instance)
	state.active = ""
	return withdrawal, nil
}

// ReplaceOwner retires one explicit owner incarnation and admits its named successor.
//
// It never infers the newest incarnation from arrival order: until the host calls it, a
// second instance is refused with ErrInstanceConflict. An empty previous activates the
// successor of an owner that has no active incarnation, which is the only way such an
// owner becomes registrable again. Repeating the transition is a no-op.
func (r *Registry) ReplaceOwner(owner, previous, next string) (Withdrawal, error) {
	if !validIdentifier(owner) || !validIdentifier(next) {
		return Withdrawal{}, fmt.Errorf("%w: owner and successor must be 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	if previous != "" && !validIdentifier(previous) {
		return Withdrawal{}, fmt.Errorf("%w: predecessor must be empty or 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	if previous == next {
		return Withdrawal{}, fmt.Errorf("%w: replacement needs two distinct instances", ErrInvalidRegistration)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.owners[owner]
	if state == nil {
		return Withdrawal{}, fmt.Errorf("%w: owner %q has no incarnation to replace", ErrNotFound, owner)
	}
	if state.active == next {
		return Withdrawal{Owner: owner, Instance: previous}, nil
	}
	if previous == "" {
		if state.active != "" {
			return Withdrawal{}, fmt.Errorf("%w: owner %q is active on %q", ErrInstanceConflict, owner, state.active)
		}
		if _, retired := state.retired[next]; retired {
			return Withdrawal{}, fmt.Errorf("%w: owner %q instance %q was replaced", ErrStaleInstance, owner, next)
		}
		state.active = next
		return Withdrawal{Owner: owner, Instance: ""}, nil
	}
	if state.active != previous {
		return Withdrawal{}, fmt.Errorf("%w: owner %q is not active on %q", ErrStaleInstance, owner, previous)
	}
	withdrawal := r.withdrawLocked(owner, previous)
	r.markRetiredLocked(state, previous)
	state.active = next
	return withdrawal, nil
}

// admitLocked accepts a registration only from the owner's active incarnation. A retired
// owner stays inactive: an unknown or forgotten instance never reactivates it, because the
// host must name the successor through ReplaceOwner first.
func (r *Registry) admitLocked(identity api.Identity) error {
	state := r.owners[identity.Owner]
	if state == nil {
		r.owners[identity.Owner] = &ownerState{active: identity.Instance, retired: make(map[string]struct{})}
		return nil
	}
	if state.active == identity.Instance {
		return nil
	}
	if _, retired := state.retired[identity.Instance]; retired {
		return fmt.Errorf("%w: owner %q instance %q was replaced", ErrStaleInstance, identity.Owner, identity.Instance)
	}
	if state.active == "" {
		return fmt.Errorf("%w: owner %q has no active incarnation; the host must activate one with ReplaceOwner", ErrInstanceConflict, identity.Owner)
	}
	return fmt.Errorf("%w: owner %q is active on another instance", ErrInstanceConflict, identity.Owner)
}

func (r *Registry) checkInstanceLocked(identity api.Identity) error {
	state := r.owners[identity.Owner]
	if state == nil {
		return nil
	}
	if _, retired := state.retired[identity.Instance]; retired {
		return fmt.Errorf("%w: owner %q instance %q was replaced", ErrStaleInstance, identity.Owner, identity.Instance)
	}
	if state.active == "" {
		return fmt.Errorf("%w: owner %q has no active incarnation; the host must activate one with ReplaceOwner", ErrInstanceConflict, identity.Owner)
	}
	if state.active != identity.Instance {
		return fmt.Errorf("%w: owner %q is active on another instance", ErrInstanceConflict, identity.Owner)
	}
	return nil
}

func (r *Registry) markRetiredLocked(state *ownerState, instance string) {
	if _, retired := state.retired[instance]; retired {
		return
	}
	state.retired[instance] = struct{}{}
	state.order = append(state.order, instance)
	for len(state.order) > retainedInstances {
		delete(state.retired, state.order[0])
		state.order = state.order[1:]
	}
}

func (r *Registry) withdrawLocked(owner, instance string) Withdrawal {
	withdrawal := Withdrawal{Owner: owner, Instance: instance}
	for id, live := range r.entries {
		if live.owner != owner || live.instance != instance {
			continue
		}
		withdrawal.Registrations = append(withdrawal.Registrations, live.registration(r.generation))
		delete(r.entries, id)
		delete(r.keys, registrationKey{owner: live.owner, kind: live.kind, key: live.key})
		r.releaseLocked(id, live.schemes, live.sources)
	}
	sortRegistrations(withdrawal.Registrations)
	return withdrawal
}

func (r *Registry) liveLocked(owner string) int {
	held := 0
	for _, live := range r.entries {
		if live.owner == owner {
			held++
		}
	}
	return held
}

func (r *Registry) checkClaimsLocked(exceptRegistrationID string, claims claimSet) error {
	for _, scheme := range claims.schemes {
		holder, taken := r.schemeHolders[scheme]
		if !taken || holder == exceptRegistrationID {
			continue
		}
		return &ConflictError{Namespace: "scheme", Claim: scheme, Owner: r.entries[holder].owner}
	}
	for _, source := range claims.sources {
		holder, taken := r.sourceHolders[source]
		if !taken || holder == exceptRegistrationID {
			continue
		}
		return &ConflictError{Namespace: "source", Claim: source.Source + "/" + source.Field, Owner: r.entries[holder].owner}
	}
	return nil
}

func (r *Registry) holdLocked(registrationID string, schemes []string, sources []SourceClaim) {
	for _, scheme := range schemes {
		r.schemeHolders[scheme] = registrationID
	}
	for _, source := range sources {
		r.sourceHolders[source] = registrationID
	}
}

func (r *Registry) releaseLocked(registrationID string, schemes []string, sources []SourceClaim) {
	for _, scheme := range schemes {
		if r.schemeHolders[scheme] == registrationID {
			delete(r.schemeHolders, scheme)
		}
	}
	for _, source := range sources {
		if r.sourceHolders[source] == registrationID {
			delete(r.sourceHolders, source)
		}
	}
}

func (r *Registry) missLocked(registrationID string) error {
	if r.foreignGenerationLocked(registrationID) {
		return fmt.Errorf("%w: %s", ErrStaleGeneration, registrationID)
	}
	return fmt.Errorf("%w: %s", ErrNotFound, registrationID)
}

func (r *Registry) foreignGenerationLocked(registrationID string) bool {
	prefix, ok := idGeneration(registrationID)
	return ok && prefix != r.generationPrefix
}

func (r *Registry) newIDLocked() (string, error) {
	var raw [8]byte
	for range 4 {
		if _, err := rand.Read(raw[:]); err != nil {
			return "", fmt.Errorf("generate registration ID: %w", err)
		}
		id := "r-" + r.generationPrefix + "-" + hex.EncodeToString(raw[:])
		if _, exists := r.entries[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("generate registration ID: exhausted random candidates")
}

func (r *Registry) receipt(live *entry, idempotent bool) Receipt {
	return Receipt{
		RegistrationID: live.registrationID, DescriptorRevision: live.descriptorRevision, Generation: r.generation,
		Kind: live.kind, Key: live.key, Idempotent: idempotent,
	}
}

func (e *entry) registration(generation string) Registration {
	return Registration{
		RegistrationID: e.registrationID, DescriptorRevision: e.descriptorRevision, Generation: generation,
		Owner: e.owner, Instance: e.instance, Kind: e.kind, Key: e.key, Descriptor: bytes.Clone(e.descriptor),
	}
}

func sortRegistrations(registrations []Registration) {
	sort.Slice(registrations, func(i, j int) bool {
		left, right := registrations[i], registrations[j]
		if left.Owner != right.Owner {
			return left.Owner < right.Owner
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Key < right.Key
	})
}

// idGeneration returns the generation prefix embedded in a well-formed registration ID.
func idGeneration(registrationID string) (string, bool) {
	rest, ok := strings.CutPrefix(registrationID, "r-")
	if !ok || len(rest) != 8+1+16 || rest[8] != '-' {
		return "", false
	}
	if !isHex(rest[:8]) || !isHex(rest[9:]) {
		return "", false
	}
	return rest[:8], true
}

func isHex(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func validateIdentity(identity api.Identity) error {
	if !validIdentifier(identity.Owner) {
		return fmt.Errorf("%w: owner must be 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	if !validIdentifier(identity.Instance) {
		return fmt.Errorf("%w: instance must be 1–%d UTF-8 bytes without controls", ErrInvalidRegistration, maxIdentifierBytes)
	}
	return nil
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= maxIdentifierBytes && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
