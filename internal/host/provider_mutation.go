package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func mutationURI(p *contributedProvider, uri string) (string, error) {
	uri = p.canonicalURI(uri)
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != p.canonical || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(u.Path, "\\\x00") {
		return "", errors.New("mutation requires a canonical provider URI without query or fragment")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return "", errors.New("mutation URI must not contain traversal segments")
		}
	}
	return uri, nil
}

func (a *app) mutateProvider(work context.Context, s *session, target resource.Target, operation, destination string, content *string, expected string) (sdk.ToolResult, error) {
	p, ok := s.contributedDescriptor(target.Scheme)
	if !ok || !p.descriptor.Has(operation) || p.descriptor.Mutations == nil {
		return sdk.ToolResult{}, fmt.Errorf("%s:// does not advertise %s; nothing was dispatched", target.Scheme, operation)
	}
	if target.Selection.Explicit {
		return sdk.ToolResult{}, errors.New("mutation requires a whole resource, not a selector")
	}
	uri, err := mutationURI(p, target.URI)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	mutation := &api.ResourceMutation{Content: content, LocationRevision: s.routingLocation(work).Revision}
	switch operation {
	case "write", "mkdir", "remove", "move", "copy":
	default:
		return sdk.ToolResult{}, errors.New("unsupported provider mutation")
	}
	if content != nil && len(*content) > 32<<10 {
		return sdk.ToolResult{}, errors.New("provider write exceeds the 32 KiB content limit")
	}
	if destination != "" {
		dest, err := resource.ParsePath("/", destination, "")
		if err != nil {
			return sdk.ToolResult{}, err
		}
		other, ok := s.contributedDescriptor(dest.Scheme)
		if !ok || other.registrationID != p.registrationID || other.canonical != p.canonical || dest.Selection.Explicit {
			return sdk.ToolResult{}, errors.New("cross-provider transfers and destination selectors are unsupported")
		}
		mutation.DestinationURI, err = mutationURI(other, dest.URI)
		if err != nil {
			return sdk.ToolResult{}, err
		}
		if mutation.DestinationURI == uri {
			return sdk.ToolResult{}, errors.New("source and destination must differ")
		}
		_, err = p.Stat(work, mutation.DestinationURI)
		if !errors.Is(err, resource.ErrNotFound) {
			if err != nil {
				return sdk.ToolResult{}, err
			}
			return sdk.ToolResult{}, errors.New("destination already exists; nothing was dispatched")
		}
		mutation.DestinationMustNotExist = true
	}
	stat, err := p.Stat(work, uri)
	if errors.Is(err, resource.ErrNotFound) && (operation == "write" || operation == "mkdir") {
		if expected != "" {
			return sdk.ToolResult{}, errors.New("staleRevision: expected resource no longer exists")
		}
		mutation.MustNotExist = true
	} else {
		if err != nil {
			return sdk.ToolResult{}, err
		}
		if stat.URI != uri {
			return sdk.ToolResult{}, errors.New("provider redirected the mutation target; use its canonical URI explicitly")
		}
		if operation == "mkdir" {
			return sdk.ToolResult{}, errors.New("destination already exists; nothing was dispatched")
		}
		if stat.Kind == resource.EntryLink || stat.Kind == resource.EntryControl || operation == "copy" && stat.Kind != resource.EntryFile || operation == "write" && stat.Kind != resource.EntryFile {
			return sdk.ToolResult{}, errors.New("mutation target kind is not supported")
		}
		if stat.Revision == "" {
			return sdk.ToolResult{}, errors.New("provider did not supply the required source revision")
		}
		if expected != "" && expected != stat.Revision {
			return sdk.ToolResult{}, errors.New("staleRevision: resource changed; nothing was dispatched")
		}
		mutation.ExpectedRevision = stat.Revision
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return sdk.ToolResult{}, err
	}
	id := "op-" + hex.EncodeToString(token[:])
	mutation.IdempotencyKey, mutation.AuthorizationRef = id, "router:"+id
	request := api.ResourceRequest{OperationID: id, Operation: operation, URI: uri, RequestedURI: target.Requested, Mutation: mutation, Project: p.project(work)}
	if err := work.Err(); err != nil {
		return sdk.ToolResult{}, err
	}
	var result api.ResourceResult
	budget := 30 * time.Second
	if p.descriptor.Limits.QueryTimeMS > 0 {
		budget = min(budget, time.Duration(p.descriptor.Limits.QueryTimeMS)*time.Millisecond)
	}
	err = p.bus.call(work, p.owner, p.registrationID, "resource.execute", request, &result, budget)
	receipt := result.Receipt
	if err != nil {
		status := "unknown"
		var protocol *providerError
		if errors.As(err, &protocol) && protocol.protocol.Outcome == "notApplied" {
			status = "notApplied"
		}
		receipt = &api.MutationReceipt{OperationID: id, IdempotencyKey: id, CanonicalURI: uri, Status: status, Warnings: []string{sanitizeProviderText(err.Error()), "Do not retry automatically; reconcile the resource state."}}
	} else if !validMutationReceipt(receipt, id, uri, mutation.DestinationURI, operation) {
		receipt = &api.MutationReceipt{OperationID: id, IdempotencyKey: id, CanonicalURI: uri, Status: "unknown", Warnings: []string{"Provider returned an invalid mutation receipt; reconcile before retrying."}}
	}
	text := fmt.Sprintf("%s %s: %s\n", operation, uri, receipt.Status)
	for _, warning := range receipt.Warnings {
		text += sanitizeProviderText(warning) + "\n"
	}
	return sdk.ToolResult{Content: text, Details: receipt, IsError: receipt.Status != "committed"}, nil
}

func validMutationReceipt(receipt *api.MutationReceipt, id, uri, destination, operation string) bool {
	if receipt == nil || receipt.OperationID != id || receipt.IdempotencyKey != id || receipt.CanonicalURI != uri || len(receipt.Effects) > 2 || len(receipt.ChangedResources) > 2 {
		return false
	}
	switch receipt.Status {
	case "committed", "notApplied", "partial", "unknown":
	default:
		return false
	}
	seen := make(map[string]bool)
	for _, effect := range receipt.Effects {
		if _, exists := seen[effect.URI]; exists {
			return false
		}
		seen[effect.URI] = effect.Status == "committed"
		if effect.URI != uri && (destination == "" || effect.URI != destination) {
			return false
		}
		switch effect.Status {
		case "committed", "notApplied", "unknown":
		default:
			return false
		}
		if receipt.Status == "committed" && effect.Status != "committed" {
			return false
		}
		if receipt.Status == "notApplied" && effect.Status != "notApplied" {
			return false
		}
	}
	for _, changed := range receipt.ChangedResources {
		if receipt.Status == "notApplied" || !seen[changed] || changed != uri && (destination == "" || changed != destination) {
			return false
		}
	}
	if receipt.Status != "committed" {
		return true
	}
	if destination == "" {
		return seen[uri]
	}
	return seen[destination] && (operation == "copy" || seen[uri])
}
