package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

type registrationParams struct {
	Kind       string          `json:"kind"`
	Key        string          `json:"key"`
	Descriptor json.RawMessage `json:"descriptor"`
}

type describeParams struct {
	Kind   string `json:"kind,omitempty"`
	Key    string `json:"key,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

const describePageSize = 50

func validateContributionDescriptor(kind contribution.Kind, _ string, descriptor json.RawMessage) (contribution.Claims, error) {
	switch kind {
	case contribution.KindToolPresentation:
		if _, err := presentation.DecodeDescriptor(descriptor); err != nil {
			return contribution.Claims{}, err
		}
		return contribution.Claims{}, nil
	default:
		return contribution.Claims{}, fmt.Errorf("%w: contribution kind %q is not wired yet", contribution.ErrInvalidDescriptor, kind)
	}
}

func (a *app) registerContribution(_ context.Context, s *session, request api.Request) (any, *api.Error) {
	var params registrationParams
	if err := api.Decode(request.Params, &params); err != nil || len(params.Descriptor) == 0 || !bytes.HasPrefix(bytes.TrimSpace(params.Descriptor), []byte("{")) {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "register requires kind, key and an object descriptor", Outcome: "notApplied"}
	}
	kind := contribution.Kind(params.Kind)
	if kind != contribution.KindToolPresentation {
		return nil, &api.Error{Code: "unsupported", Field: "params.kind", Message: "only toolPresentation registration is wired", Outcome: "notApplied"}
	}
	receipt, err := s.contributions.Register(request.From, kind, params.Key, params.Descriptor)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if kind == contribution.KindToolPresentation && !receipt.Idempotent {
		descriptor, decodeErr := presentation.DecodeDescriptor(params.Descriptor)
		if decodeErr != nil {
			_ = s.contributions.Unregister(request.From, receipt.RegistrationID)
			return nil, contributionProtocolError(decodeErr)
		}
		if addErr := s.presentations.add(receipt.RegistrationID, request.From.Owner, descriptor); addErr != nil {
			_ = s.contributions.Unregister(request.From, receipt.RegistrationID)
			return nil, &api.Error{Code: "conflict", Message: addErr.Error(), Outcome: "notApplied"}
		}
	}
	return map[string]any{
		"registrationId":     receipt.RegistrationID,
		"descriptorRevision": receipt.DescriptorRevision,
		"kind":               receipt.Kind,
		"key":                receipt.Key,
		"idempotent":         receipt.Idempotent,
	}, nil
}

func (a *app) unregisterContribution(_ context.Context, s *session, request api.Request) (any, *api.Error) {
	if request.RegistrationID == "" {
		return nil, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "unregister requires a registrationId", Outcome: "notApplied"}
	}
	var params struct{}
	if err := api.Decode(request.Params, &params); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "unregister takes an empty object", Outcome: "notApplied"}
	}
	if err := s.contributions.Unregister(request.From, request.RegistrationID); err != nil {
		return nil, contributionProtocolError(err)
	}
	s.presentations.remove(request.RegistrationID)
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
	case errors.Is(err, contribution.ErrInvalidDescriptor), errors.Is(err, presentation.ErrInvalid):
		return &api.Error{Code: "invalidRequest", Message: "contribution descriptor is invalid", Outcome: "notApplied"}
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
