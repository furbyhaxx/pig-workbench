package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
)

type contextFieldClaim struct {
	Source string `json:"source"`
	Field  string `json:"field"`
}

type frameProducerDescriptor struct {
	Label              string              `json:"label"`
	Description        string              `json:"description,omitempty"`
	Kinds              []string            `json:"kinds"`
	DefaultVisibility  string              `json:"defaultVisibility,omitempty"`
	RequestedAuthority string              `json:"requestedAuthority,omitempty"`
	AllowWake          bool                `json:"allowWake,omitempty"`
	ContextFields      []contextFieldClaim `json:"contextFields,omitempty"`
}

var (
	producerKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*$`)
	reservedPrefixes   = []string{"workbench", "session", "instructions", "pig", "host", "system"}
	cutoffDatePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func parseFrameProducer(data []byte) (frameProducerDescriptor, error) {
	var d frameProducerDescriptor
	if err := decodeStrict(data, &d); err != nil {
		return d, err
	}
	invalid := func(message string) (frameProducerDescriptor, error) {
		return frameProducerDescriptor{}, fmt.Errorf("%w: frameProducer %s", errInvalidContribution, message)
	}
	if d.Label == "" || !plainField(d.Label, 128) || !plainField(d.Description, 512) {
		return invalid("label and description must be plain text")
	}
	if len(d.Kinds) == 0 {
		return invalid("kinds must not be empty")
	}
	for _, kind := range d.Kinds {
		if !slices.Contains([]string{"instruction", "reminder", "warning", "error", "notice", "info"}, kind) {
			return invalid("kind " + kind + " is not available to external producers")
		}
	}
	if d.DefaultVisibility == "" {
		d.DefaultVisibility = "shown"
	}
	if !slices.Contains([]string{"shown", "collapsed", "hidden"}, d.DefaultVisibility) {
		return invalid("defaultVisibility must be shown, collapsed or hidden")
	}
	if d.RequestedAuthority == "" {
		d.RequestedAuthority = "data"
	}
	if d.RequestedAuthority != "data" && d.RequestedAuthority != "instruction" {
		return invalid("requestedAuthority must be data or instruction")
	}
	seen := map[contextFieldClaim]bool{}
	for _, claim := range d.ContextFields {
		if claim.Source != "session.model" || claim.Field != "knowledge_cutoff" {
			return invalid("only session.model knowledge_cutoff context fields are supported")
		}
		if seen[claim] {
			return invalid("context field is repeated")
		}
		seen[claim] = true
	}
	return d, nil
}

type frameProducer struct {
	registrationID string
	owner          api.Identity
	key            string
	descriptor     frameProducerDescriptor
}

type modelBinding struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

type contributedFields struct {
	owner          string
	registrationID string
	model          modelBinding
	fields         map[string]any
}

// producerRegistry tracks external frame producers and their model-bound context fields.
type producerRegistry struct {
	mu        sync.RWMutex
	producers map[string]frameProducer
	fields    map[string]contributedFields
}

func newProducerRegistry() *producerRegistry {
	return &producerRegistry{producers: make(map[string]frameProducer), fields: make(map[string]contributedFields)}
}

func (r *producerRegistry) get(registrationID string) (frameProducer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	producer, ok := r.producers[registrationID]
	return producer, ok
}

// modelFields returns the current model's contributed fields in stable field order.
func (r *producerRegistry) modelFields(model modelBinding) []contributedFields {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []contributedFields
	for _, contribution := range r.fields {
		if contribution.model == model {
			result = append(result, contribution)
		}
	}
	slices.SortFunc(result, func(a, b contributedFields) int { return strings.Compare(a.owner, b.owner) })
	return result
}

func installFrameProducer(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := parseFrameProducer(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	if !producerKeyPattern.MatchString(registration.Key) || len(registration.Key) > 64 {
		return nil, fmt.Errorf("%w: frameProducer key must be a dotted lowercase source prefix", errInvalidContribution)
	}
	first, _, _ := strings.Cut(registration.Key, ".")
	if slices.Contains(reservedPrefixes, first) {
		return nil, fmt.Errorf("%w: source prefix %q is reserved for Workbench", errConsumerConflict, first)
	}
	s.producers.mu.Lock()
	defer s.producers.mu.Unlock()
	for _, existing := range s.producers.producers {
		if existing.key == registration.Key || strings.HasPrefix(existing.key, registration.Key+".") || strings.HasPrefix(registration.Key, existing.key+".") {
			return nil, fmt.Errorf("%w: source prefix overlaps %q", errConsumerConflict, existing.key)
		}
	}
	s.producers.producers[registration.RegistrationID] = frameProducer{registrationID: registration.RegistrationID, owner: api.Identity{Owner: registration.Owner, Instance: registration.Instance}, key: registration.Key, descriptor: descriptor}
	return func() {
		s.producers.mu.Lock()
		delete(s.producers.producers, registration.RegistrationID)
		delete(s.producers.fields, registration.RegistrationID)
		s.producers.mu.Unlock()
	}, nil
}

type publishParams struct {
	EventKey    string     `json:"eventKey"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	Body        string     `json:"body"`
	Title       string     `json:"title,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	SubjectKey  string     `json:"subjectKey,omitempty"`
	Resource    string     `json:"resource,omitempty"`
	Scope       string     `json:"scope,omitempty"`
	Delivery    string     `json:"delivery,omitempty"`
	CoalesceKey string     `json:"coalesceKey,omitempty"`
	DataTrust   string     `json:"dataTrust,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

func (a *app) producerFor(s *session, request api.Request) (frameProducer, *api.Error) {
	if request.RegistrationID == "" {
		return frameProducer{}, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "frame methods require a frameProducer registrationId", Outcome: "notApplied"}
	}
	if _, err := s.contributions.Get(request.From, request.RegistrationID); err != nil {
		return frameProducer{}, contributionProtocolError(err)
	}
	producer, ok := s.producers.get(request.RegistrationID)
	if !ok {
		return frameProducer{}, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "registration is not a frameProducer", Outcome: "notApplied"}
	}
	return producer, nil
}

func frameError(err error) *api.Error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, frames.ErrConflict):
		return &api.Error{Code: "conflict", Message: "eventKey was already used with a different payload", Outcome: "notApplied"}
	case errors.Is(err, frames.ErrLimitExceeded):
		return &api.Error{Code: "limitExceeded", Message: sanitizeProviderText(err.Error()), Outcome: "notApplied"}
	case errors.Is(err, frames.ErrOwnership):
		return &api.Error{Code: "permissionDenied", Message: "source is outside the producer's registered prefix", Outcome: "notApplied"}
	default:
		return &api.Error{Code: "invalidRequest", Message: sanitizeProviderText(err.Error()), Outcome: "notApplied"}
	}
}

// framesPublish admits one external frame and hands nextBoundary delivery to the host.
func (a *app) framesPublish(host sdk.Context, s *session, request api.Request) (any, *api.Error) {
	producer, failure := a.producerFor(s, request)
	if failure != nil {
		return nil, failure
	}
	var params publishParams
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "frames.publish params do not match SPEC-0002 §3", Outcome: "notApplied"}
	}
	if !slices.Contains(producer.descriptor.Kinds, params.Kind) {
		return nil, &api.Error{Code: "permissionDenied", Field: "params.kind", Message: "kind is not declared by this producer", Outcome: "notApplied"}
	}
	if params.Delivery == "" {
		params.Delivery = "nextBoundary"
	}
	if params.Delivery == "followUp" {
		return nil, &api.Error{Code: "permissionDenied", Field: "params.delivery", Message: "followUp wake is not granted to external producers; retry with nextPrompt", Outcome: "notApplied"}
	}
	if params.Scope != "" && params.Scope != "session" && params.Scope != "branch" {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.scope", Message: "scope must be session or branch", Outcome: "notApplied"}
	}
	if params.DataTrust == "" {
		params.DataTrust = "untrusted"
	}
	// External producers keep data authority: requesting instruction is never self-authorization.
	authority := "data"
	publish := frames.Publish{EventKey: params.EventKey, Kind: params.Kind, Source: params.Source, Body: params.Body, Title: params.Title, Summary: params.Summary, SubjectKey: params.SubjectKey, Resource: params.Resource, Delivery: params.Delivery, DataTrust: params.DataTrust, CoalesceKey: params.CoalesceKey, ExpiresAt: params.ExpiresAt}
	receipt, frame, admitted := a.admitFrame(host, s, producer.key, authority, false, publish)
	if admitted != nil {
		switch admitted.stage {
		case "scope":
			return nil, &api.Error{Code: "internal", Message: "cannot determine the current branch", Outcome: "notApplied"}
		case "admit":
			return nil, frameError(admitted.err)
		}
		return nil, &api.Error{Code: "internal", Message: "frame could not be persisted", Outcome: "unknown"}
	}
	receipt.Durable = true
	if params.Delivery == "nextBoundary" && frame.FrameID != "" {
		encoded, err := frames.EncodeEnvelope(frame)
		if err == nil {
			trigger := false
			err = host.SendMessage("pig-workbench.context", encoded, true, sdk.SendMessageOptions{DeliverAs: "steer", TriggerTurn: &trigger})
		}
		if err != nil {
			return map[string]any{"frameId": receipt.FrameID, "sequence": receipt.Sequence, "state": "pending", "delivery": receipt.Delivery, "effectiveAuthority": authority, "durable": true, "notice": "host delivery failed; the frame stays pending for the next prompt"}, nil
		}
	}
	return receipt, nil
}

type admitFailure struct {
	stage string
	err   error
}

// admitFrame admits and persists one frame for prefix on the current branch. A persistence failure fails the frame.
func (a *app) admitFrame(host sdk.Context, s *session, prefix, authority string, trusted bool, publish frames.Publish) (frames.Receipt, frames.Frame, *admitFailure) {
	state := s.contextFrames
	state.mu.Lock()
	scope, err := a.currentFrameScope(host, s, state)
	if err != nil {
		state.mu.Unlock()
		return frames.Receipt{}, frames.Frame{}, &admitFailure{"scope", err}
	}
	ledger := state.ledger.Ledger(frames.Context{SessionID: s.id, ProducerPrefix: prefix, BranchLeaf: scope.BranchLeaf, Ancestors: scope.Ancestors, Epoch: scope.Epoch, Trusted: trusted, Authority: authority})
	receipt, err := ledger.Admit(publish)
	if err != nil {
		state.mu.Unlock()
		return frames.Receipt{}, frames.Frame{}, &admitFailure{"admit", err}
	}
	var frame frames.Frame
	for _, raw := range state.ledger.Snapshot() {
		var record struct {
			Frame frames.Frame `json:"frame"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Frame.FrameID == receipt.FrameID {
			frame = record.Frame
		}
	}
	persistErr := persistFrames(host, state.ledger, []string{receipt.FrameID})
	state.mu.Unlock()
	if persistErr != nil {
		_ = state.ledger.Fail(receipt.FrameID)
		return frames.Receipt{}, frames.Frame{}, &admitFailure{"persist", persistErr}
	}
	return receipt, frame, nil
}

// currentFrameScope binds new frames to the current branch leaf and context epoch.
// The caller holds state.mu.
func (a *app) currentFrameScope(host sdk.Context, s *session, state *contextFrameState) (frames.DeliveryScope, error) {
	return branchScope(host, s.snapshots.Epoch())
}

type frameRecord struct {
	State string       `json:"state"`
	Frame frames.Frame `json:"frame"`
}

func ownedFrames(s *session, producer frameProducer) []frameRecord {
	var records []frameRecord
	for _, raw := range s.contextFrames.ledger.Snapshot() {
		var record frameRecord
		if json.Unmarshal(raw, &record) == nil && (record.Frame.Source == producer.key || strings.HasPrefix(record.Frame.Source, producer.key+".")) {
			records = append(records, record)
		}
	}
	return records
}

func (a *app) framesQuery(s *session, request api.Request) (any, *api.Error) {
	producer, failure := a.producerFor(s, request)
	if failure != nil {
		return nil, failure
	}
	records := ownedFrames(s, producer)
	switch request.Method {
	case "frames.list":
		var params struct {
			Cursor string `json:"cursor,omitempty"`
		}
		if err := api.Decode(request.Params, &params); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "frames.list accepts an optional cursor"}
		}
		start := 0
		if params.Cursor != "" {
			for i, record := range records {
				if record.Frame.FrameID == params.Cursor {
					start = i + 1
				}
			}
		}
		end := min(len(records), start+50)
		items := make([]map[string]any, 0, end-start)
		for _, record := range records[start:end] {
			items = append(items, map[string]any{"frameId": record.Frame.FrameID, "sequence": record.Frame.Sequence, "state": record.State, "kind": record.Frame.Kind, "source": record.Frame.Source, "title": record.Frame.Title, "eventKey": record.Frame.EventKey})
		}
		next := ""
		if end < len(records) && end > 0 {
			next = records[end-1].Frame.FrameID
		}
		return map[string]any{"frames": items, "nextCursor": next}, nil
	default:
		var params struct {
			FrameID string `json:"frameId"`
		}
		if err := api.Decode(request.Params, &params); err != nil || params.FrameID == "" {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.frameId", Message: "frameId is required"}
		}
		for _, record := range records {
			if record.Frame.FrameID != params.FrameID {
				continue
			}
			switch request.Method {
			case "frames.status":
				return map[string]any{"frameId": record.Frame.FrameID, "state": record.State, "delivery": record.Frame.Delivery}, nil
			case "frames.get":
				body := record.Frame.Body
				complete := true
				if len(body) > 32<<10 {
					body, complete = body[:32<<10], false
					for !utf8.ValidString(body) {
						body = body[:len(body)-1]
					}
				}
				return map[string]any{"frame": record.Frame, "state": record.State, "body": body, "bodyComplete": complete}, nil
			case "frames.cancel":
				if record.State != "pending" {
					return nil, &api.Error{Code: "conflict", Message: "only undelivered frames can be cancelled", Outcome: "notApplied"}
				}
				if err := s.contextFrames.ledger.Cancel(record.Frame.FrameID); err != nil {
					return nil, &api.Error{Code: "conflict", Message: "frame is no longer pending", Outcome: "notApplied"}
				}
				return map[string]any{"frameId": record.Frame.FrameID, "state": "cancelled"}, nil
			}
		}
		return nil, &api.Error{Code: "notFound", Message: "frame is not owned by this producer"}
	}
}

// contextFieldsSet replaces the caller's model-bound contributed fields (SPEC-0014 §5.1).
func (a *app) contextFieldsSet(host sdk.Context, s *session, request api.Request) (any, *api.Error) {
	producer, failure := a.producerFor(s, request)
	if failure != nil {
		return nil, failure
	}
	var params struct {
		Source string          `json:"source"`
		Model  modelBinding    `json:"model"`
		Fields json.RawMessage `json:"fields"`
	}
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "context.fields.set expects source, model and fields", Outcome: "notApplied"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params.Fields, &fields); err != nil || fields == nil && string(params.Fields) != "{}" {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.fields", Message: "fields must be an object", Outcome: "notApplied"}
	}
	values, failure := contributedFieldValues(producer, params.Source, fields)
	if failure != nil {
		return nil, failure
	}
	model, err := host.GetModelInfo()
	if err != nil || model.Provider != params.Model.Provider || model.ID != params.Model.ID {
		return nil, &api.Error{Code: "staleRevision", Field: "params.model", Message: "model does not match the current session model", Outcome: "notApplied"}
	}
	s.producers.mu.Lock()
	if len(values) == 0 {
		delete(s.producers.fields, producer.registrationID)
	} else {
		s.producers.fields[producer.registrationID] = contributedFields{owner: producer.owner.Owner, registrationID: producer.registrationID, model: params.Model, fields: values}
	}
	s.producers.mu.Unlock()
	return map[string]any{"accepted": true}, nil
}

// contributedFieldValues validates one owner's claimed fields and returns their model-facing values.
func contributedFieldValues(producer frameProducer, source string, fields map[string]json.RawMessage) (map[string]any, *api.Error) {
	for name := range fields {
		if !slices.Contains(producer.descriptor.ContextFields, contextFieldClaim{Source: source, Field: name}) {
			return nil, &api.Error{Code: "permissionDenied", Field: "params.fields." + name, Message: "field is not claimed by this producer", Outcome: "notApplied"}
		}
	}
	values := map[string]any{}
	if raw, ok := fields["knowledge_cutoff"]; ok {
		var cutoff struct {
			Date   string `json:"date"`
			Basis  string `json:"basis"`
			Source string `json:"source"`
		}
		if err := api.Decode(raw, &cutoff); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.fields.knowledge_cutoff", Message: "knowledge_cutoff expects date, basis and source", Outcome: "notApplied"}
		}
		if _, parseErr := time.Parse("2006-01-02", cutoff.Date); parseErr != nil || !cutoffDatePattern.MatchString(cutoff.Date) {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.fields.knowledge_cutoff.date", Message: "date must be a valid YYYY-MM-DD", Outcome: "notApplied"}
		}
		if cutoff.Basis != "explicit" && cutoff.Basis != "estimated" {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.fields.knowledge_cutoff.basis", Message: "basis must be explicit or estimated", Outcome: "notApplied"}
		}
		if cutoff.Source == "" || utf8.RuneCountInString(cutoff.Source) > 120 || !plainField(cutoff.Source, 480) {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.fields.knowledge_cutoff.source", Message: "source must be a plain attribution up to 120 characters", Outcome: "notApplied"}
		}
		values["knowledge_cutoff"] = map[string]any{"date": cutoff.Date, "basis": cutoff.Basis, "source": cutoff.Source, "contributedBy": producer.owner.Owner}
	}
	return values, nil
}
