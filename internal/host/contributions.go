package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type registrationParams struct {
	Kind             string          `json:"kind"`
	Key              string          `json:"key"`
	Descriptor       json.RawMessage `json:"descriptor"`
	PreviousRevision string          `json:"previousRevision,omitempty"`
}

type describeParams struct {
	Kind   string `json:"kind,omitempty"`
	Key    string `json:"key,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

const describePageSize = 50

// errUnsupportedContribution reports a valid descriptor whose capability this version does not route.
var errUnsupportedContribution = errors.New("contribution capability is unsupported")

// errConsumerConflict reports a registration its subsystem cannot take beside a live one.
var errConsumerConflict = errors.New("contribution conflicts with a live consumer")

// contributionConsumer applies one admitted registration to its owning subsystem and
// returns the cleanup that withdraws it. A failed install leaves nothing behind.
type contributionConsumer func(s *session, registration contribution.Registration) (func(), error)

var contributionConsumers = map[contribution.Kind]contributionConsumer{
	contribution.KindToolPresentation: installPresentation,
	contribution.KindResourceProvider: installResourceProvider,
	contribution.KindFetchBackend:     installFetchBackend,
	contribution.KindStatusSegment:    installStatusSegment,
	contribution.KindDocs:             installDocs,
	contribution.KindFrameProducer:    installFrameProducer,
}

func validateContributionDescriptor(kind contribution.Kind, _ string, descriptor json.RawMessage) (contribution.Claims, error) {
	switch kind {
	case contribution.KindToolPresentation:
		_, err := presentation.DecodeDescriptor(descriptor)
		return contribution.Claims{}, err
	case contribution.KindResourceProvider:
		parsed, err := resource.ParseProviderDescriptor(descriptor)
		return contribution.Claims{Schemes: parsed.Schemes}, err
	case contribution.KindFetchBackend:
		_, err := parseFetchBackend(descriptor)
		return contribution.Claims{}, err
	case contribution.KindStatusSegment:
		_, err := parseStatusSegment(descriptor)
		return contribution.Claims{}, err
	case contribution.KindDocs:
		_, err := parseDocsDescriptor(descriptor)
		return contribution.Claims{}, err
	case contribution.KindFrameProducer:
		parsed, err := parseFrameProducer(descriptor)
		claims := contribution.Claims{}
		for _, field := range parsed.ContextFields {
			claims.Sources = append(claims.Sources, contribution.SourceClaim{Source: field.Source, Field: field.Field})
		}
		return claims, err
	default:
		return contribution.Claims{}, fmt.Errorf("%w: unknown contribution kind %q", contribution.ErrInvalidDescriptor, kind)
	}
}

func installPresentation(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := presentation.DecodeDescriptor(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	if err := s.presentations.add(registration.RegistrationID, registration.Owner, descriptor); err != nil {
		return nil, fmt.Errorf("%w: %v", errConsumerConflict, err)
	}
	return func() { s.presentations.remove(registration.RegistrationID) }, nil
}

func (s *session) setInstalled(registrationID string, cleanup func()) {
	s.installedMu.Lock()
	defer s.installedMu.Unlock()
	if s.installed == nil {
		s.installed = make(map[string]func())
	}
	s.installed[registrationID] = cleanup
}

func (s *session) withdrawInstalled(registrationID string) {
	s.installedMu.Lock()
	cleanup := s.installed[registrationID]
	delete(s.installed, registrationID)
	s.installedMu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func receiptResult(receipt contribution.Receipt) map[string]any {
	return map[string]any{
		"registrationId":     receipt.RegistrationID,
		"descriptorRevision": receipt.DescriptorRevision,
		"kind":               receipt.Kind,
		"key":                receipt.Key,
		"idempotent":         receipt.Idempotent,
	}
}

func (a *app) registerContribution(ctx context.Context, s *session, request api.Request) (any, *api.Error) {
	var params registrationParams
	if err := api.Decode(request.Params, &params); err != nil || len(params.Descriptor) == 0 || !bytes.HasPrefix(bytes.TrimSpace(params.Descriptor), []byte("{")) {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "register requires kind, key and an object descriptor", Outcome: "notApplied"}
	}
	kind := contribution.Kind(params.Kind)
	consumer, ok := contributionConsumers[kind]
	if !ok {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.kind", Message: "unknown contribution kind", Outcome: "notApplied"}
	}
	if kind == contribution.KindToolPresentation {
		descriptor, err := presentation.DecodeDescriptor(params.Descriptor)
		if err != nil {
			return nil, contributionProtocolError(err)
		}
		tools, _ := ctx.Value(presentationToolsKey{}).([]sdk.ToolInfo)
		if failure := verifyPresentationOwner(tools, request.From.Owner, descriptor.ToolNames); failure != nil {
			return nil, failure
		}
	}
	if params.PreviousRevision != "" || request.RegistrationID != "" {
		return a.replaceContribution(s, request, params, consumer)
	}
	receipt, err := s.contributions.Register(request.From, kind, params.Key, params.Descriptor)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if receipt.Idempotent {
		return receiptResult(receipt), nil
	}
	registration, err := s.contributions.Get(request.From, receipt.RegistrationID)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	cleanup, err := consumer(s, registration)
	if err != nil {
		_ = s.contributions.Unregister(request.From, receipt.RegistrationID)
		return nil, contributionProtocolError(err)
	}
	s.setInstalled(receipt.RegistrationID, cleanup)
	a.emitRegistrationChanged(s, receipt.RegistrationID, "registered")
	if kind == contribution.KindResourceProvider {
		a.emitResourcesChanged(s, receipt.RegistrationID, "registered")
	}
	return receiptResult(receipt), nil
}

func (a *app) replaceContribution(s *session, request api.Request, params registrationParams, consumer contributionConsumer) (any, *api.Error) {
	if request.RegistrationID == "" || params.PreviousRevision == "" {
		return nil, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "replace requires registrationId and params.previousRevision", Outcome: "notApplied"}
	}
	current, err := s.contributions.Get(request.From, request.RegistrationID)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if string(current.Kind) != params.Kind || current.Key != params.Key {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.kind", Message: "replace cannot change kind or key", Outcome: "notApplied"}
	}
	receipt, err := s.contributions.Replace(request.From, request.RegistrationID, params.PreviousRevision, params.Descriptor)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if receipt.Idempotent {
		return receiptResult(receipt), nil
	}
	s.withdrawInstalled(request.RegistrationID)
	replaced, err := s.contributions.Get(request.From, request.RegistrationID)
	if err == nil {
		var cleanup func()
		if cleanup, err = consumer(s, replaced); err == nil {
			s.setInstalled(request.RegistrationID, cleanup)
			a.emitRegistrationChanged(s, request.RegistrationID, "replaced")
			if contribution.Kind(params.Kind) == contribution.KindResourceProvider {
				a.emitResourcesChanged(s, request.RegistrationID, "replaced")
			}
			return receiptResult(receipt), nil
		}
	}
	// Restore the previous descriptor so a failed install never leaves the owner unregistered.
	if _, restoreErr := s.contributions.Replace(request.From, request.RegistrationID, receipt.DescriptorRevision, current.Descriptor); restoreErr == nil {
		if cleanup, installErr := consumer(s, current); installErr == nil {
			s.setInstalled(request.RegistrationID, cleanup)
		}
	}
	return nil, contributionProtocolError(err)
}

func (a *app) unregisterContribution(_ context.Context, s *session, request api.Request) (any, *api.Error) {
	if request.RegistrationID == "" {
		return nil, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "unregister requires a registrationId", Outcome: "notApplied"}
	}
	var params struct{}
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "unregister takes an empty object", Outcome: "notApplied"}
	}
	current, lookupErr := s.contributions.Get(request.From, request.RegistrationID)
	if err := s.contributions.Unregister(request.From, request.RegistrationID); err != nil {
		return nil, contributionProtocolError(err)
	}
	s.withdrawInstalled(request.RegistrationID)
	a.emitRegistrationChanged(s, request.RegistrationID, "unregistered")
	if lookupErr == nil && current.Kind == contribution.KindResourceProvider {
		a.emitResourcesChanged(s, request.RegistrationID, "unregistered")
	}
	return map[string]any{"registrationId": request.RegistrationID, "removed": true}, nil
}

func (a *app) describeContributions(_ context.Context, s *session, request api.Request) (any, *api.Error) {
	var params describeParams
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "describe expects optional kind, key and cursor"}
	}
	offset := 0
	if params.Cursor != "" {
		parsed, err := strconv.Atoi(params.Cursor)
		if err != nil || parsed < 0 {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.cursor", Message: "cursor is invalid"}
		}
		offset = parsed
	}
	all := s.contributions.List()
	filtered := make([]map[string]any, 0, len(all))
	for _, registration := range all {
		if params.Kind != "" && string(registration.Kind) != params.Kind {
			continue
		}
		if params.Key != "" && registration.Key != params.Key {
			continue
		}
		filtered = append(filtered, map[string]any{
			"registrationId":     registration.RegistrationID,
			"descriptorRevision": registration.DescriptorRevision,
			"owner":              registration.Owner,
			"instance":           registration.Instance,
			"kind":               registration.Kind,
			"key":                registration.Key,
			"descriptor":         registration.Descriptor,
		})
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset
	total := 0
	for end < len(filtered) && end < offset+describePageSize {
		descriptor, _ := json.Marshal(filtered[end]["descriptor"])
		if end > offset && total+len(descriptor)+512 > 48*1024 {
			break
		}
		total += len(descriptor) + 512
		end++
	}
	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}
	return map[string]any{"registrations": filtered[offset:end], "nextCursor": next}, nil
}

func contributionProtocolError(err error) *api.Error {
	switch {
	case errors.Is(err, errUnsupportedContribution):
		return &api.Error{Code: "unsupported", Message: sanitizeProviderText(err.Error()), Outcome: "notApplied"}
	case errors.Is(err, errConsumerConflict):
		return &api.Error{Code: "conflict", Message: err.Error(), Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrInvalidDescriptor), errors.Is(err, presentation.ErrInvalid), errors.Is(err, resource.ErrInvalidDescriptor), errors.Is(err, errInvalidContribution):
		return &api.Error{Code: "invalidRequest", Message: sanitizeProviderText(err.Error()), Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrInvalidRegistration):
		return &api.Error{Code: "invalidRequest", Message: "contribution registration is invalid", Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrConflict):
		return &api.Error{Code: "conflict", Message: "contribution conflicts with a live registration", Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrLimitExceeded):
		return &api.Error{Code: "limitExceeded", Message: "contribution registry limit exceeded", Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrStaleGeneration):
		return &api.Error{Code: "staleGeneration", Message: "registration belongs to another generation", Retryable: true, Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrStaleRevision):
		return &api.Error{Code: "staleRevision", Message: "descriptor revision was superseded", Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrStaleInstance):
		return &api.Error{Code: "staleGeneration", Message: "owner instance is retired", Retryable: true, Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrInstanceConflict):
		return &api.Error{Code: "conflict", Message: "owner has another active instance", Outcome: "notApplied"}
	case errors.Is(err, contribution.ErrNotFound):
		return &api.Error{Code: "notFound", Message: "registration was not found", Outcome: "notApplied"}
	default:
		return &api.Error{Code: "internal", Message: "contribution registration failed", Outcome: "notApplied"}
	}
}

func (a *app) emitResourcesChanged(s *session, registrationID, change string) {
	if s.host == (sdk.Context{}) {
		return
	}
	_ = s.host.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: s.generation + ":resources:" + registrationID + ":" + change, Type: "resources.changed", RegistrationID: registrationID, Data: map[string]any{"change": change}})
}

func (a *app) emitRegistrationChanged(s *session, registrationID, change string) {
	if s.host == (sdk.Context{}) {
		return
	}
	_ = s.host.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: s.generation + ":" + registrationID + ":" + change, Type: "registration.changed", RegistrationID: registrationID, Data: map[string]any{"change": change}})
}
