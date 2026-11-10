package host

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// contributedProvider routes one scheme to a registered resourceProvider owner.
type contributedProvider struct {
	scheme             string
	canonical          string
	registrationID     string
	descriptorRevision string
	generation         string
	cursors            *listingCursorStore
	owner              api.Identity
	descriptor         resource.ProviderDescriptor
	bus                *providerBus
	project            func(context.Context) *api.Project
}

func (p *contributedProvider) Scheme() string { return p.scheme }

func (p *contributedProvider) canonicalURI(uri string) string {
	if rest, ok := strings.CutPrefix(uri, p.scheme+":"); ok {
		return p.canonical + ":" + rest
	}
	return uri
}

func (p *contributedProvider) execute(ctx context.Context, request api.ResourceRequest) (api.ResourceResult, error) {
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		return api.ResourceResult{}, err
	}
	request.OperationID = "op-" + hex.EncodeToString(token[:])
	request.URI = p.canonicalURI(request.URI)
	request.Project = p.project(ctx)
	var result api.ResourceResult
	budget := 30 * time.Second
	if p.descriptor.Limits.QueryTimeMS > 0 {
		budget = time.Duration(p.descriptor.Limits.QueryTimeMS) * time.Millisecond
	}
	err := p.bus.call(ctx, p.owner, p.registrationID, "resource.execute", request, &result, budget)
	if err != nil {
		return result, providerFailure(p.canonical, err)
	}
	if result.CanonicalURI == "" || !strings.HasPrefix(result.CanonicalURI, p.canonical+"://") {
		return result, fmt.Errorf("%s:// provider returned a canonical URI outside its scheme", p.canonical)
	}
	return result, nil
}

func providerFailure(scheme string, err error) error {
	var protocol *providerError
	if errors.As(err, &protocol) {
		switch protocol.protocol.Code {
		case "notFound":
			return resource.ErrNotFound
		case "unsupported":
			return resource.ErrUnsupported
		}
		return fmt.Errorf("%s:// provider failed (%s): %s", scheme, protocol.protocol.Code, sanitizeProviderText(protocol.protocol.Message))
	}
	if errors.Is(err, errProviderUnavailable) {
		return fmt.Errorf("%s:// provider is unavailable: %w", scheme, err)
	}
	return err
}

func sanitizeProviderText(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, text)
	if len(cleaned) > 300 {
		cleaned = cleaned[:300] + "…"
	}
	return cleaned
}

func entryKind(kind string) (resource.EntryKind, error) {
	switch resource.EntryKind(kind) {
	case resource.EntryFile, resource.EntryDir, resource.EntryLink, resource.EntryControl:
		return resource.EntryKind(kind), nil
	}
	return "", fmt.Errorf("provider returned unknown entry kind %q", kind)
}

func (p *contributedProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	result, err := p.execute(ctx, api.ResourceRequest{Operation: "stat", RequestedURI: uri, URI: uri})
	if err != nil {
		return resource.StatResult{}, err
	}
	kind, err := entryKind(result.Kind)
	if err != nil {
		return resource.StatResult{}, err
	}
	return resource.StatResult{URI: result.CanonicalURI, Kind: kind, Size: result.Size, Lines: result.Lines, Revision: result.Revision}, nil
}

func (p *contributedProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	if !p.descriptor.Has("list") {
		return resource.Listing{}, resource.ErrUnsupported
	}
	binding := cursorBinding{Registration: p.registrationID, Descriptor: p.descriptorRevision, Generation: p.generation, Instance: p.owner.Instance, Context: listingContext(ctx)}
	request := api.ResourceRequest{Operation: "list", RequestedURI: uri, URI: uri, Limits: api.PageLimits{Lines: max(1, limit)}}
	if cursor != "" {
		retained, err := p.cursors.get(cursor, binding, p.canonicalURI(uri), time.Now())
		if err != nil {
			return resource.Listing{}, err
		}
		request.Cursor, request.ExpectedRevision = retained.Raw, retained.Revision
	}
	result, err := p.execute(ctx, request)
	if err != nil {
		return resource.Listing{}, err
	}
	if len(result.Entries) > max(1, limit) {
		return resource.Listing{}, fmt.Errorf("%s:// provider returned more entries than requested", p.canonical)
	}
	if request.ExpectedRevision != "" && result.Revision != request.ExpectedRevision {
		return resource.Listing{}, errors.New("staleRevision: provider listing changed; restart listing")
	}
	if result.Complete != nil && *result.Complete && result.NextCursor != "" {
		return resource.Listing{}, errors.New("provider returned a completed listing with a continuation cursor")
	}
	listing := resource.Listing{Revision: result.Revision, Complete: result.NextCursor == ""}
	if result.NextCursor != "" {
		listing.NextCursor, err = p.cursors.put(listingCursor{Binding: binding, Requested: p.canonicalURI(uri), Canonical: result.CanonicalURI, Raw: result.NextCursor, Revision: result.Revision}, time.Now())
		if err != nil {
			return resource.Listing{}, err
		}
	}
	if result.Complete != nil && !*result.Complete {
		listing.Complete = false
	}
	if !listing.Complete && listing.NextCursor == "" {
		return resource.Listing{}, errors.New("provider returned an incomplete listing without a continuation cursor")
	}
	for _, entry := range result.Entries {
		kind, err := entryKind(entry.Kind)
		if err != nil {
			return resource.Listing{}, err
		}
		if !strings.HasPrefix(entry.URI, p.canonical+"://") || entry.Name == "" || strings.ContainsAny(entry.Name, "/\x00") {
			return resource.Listing{}, fmt.Errorf("%s:// provider returned an invalid entry", p.canonical)
		}
		listing.Entries = append(listing.Entries, resource.Entry{Name: sanitizeProviderText(entry.Name), URI: entry.URI, Kind: kind, Size: entry.Size, Revision: entry.Revision, Target: entry.Target, NoWalk: entry.NoWalk})
	}
	return listing, nil
}

// CanSearch reports whether the registration declares the search capability.
func (p *contributedProvider) CanSearch() bool { return p.descriptor.Has("search") }

// Search forwards one grep or find query to the provider and validates its hits.
func (p *contributedProvider) Search(ctx context.Context, request resource.SearchRequest) (resource.SearchResult, error) {
	if !p.CanSearch() {
		return resource.SearchResult{}, resource.ErrUnsupported
	}
	budget := request.Budget
	if limit := p.descriptor.Limits.PageBytes; limit > 0 {
		budget.Bytes = min(budget.Bytes, limit)
	}
	if limit := p.descriptor.Limits.PageLines; limit > 0 {
		budget.Lines = min(budget.Lines, limit)
	}
	query := &api.SearchQuery{Mode: request.Mode, Patterns: request.Patterns, Literal: request.Literal, IgnoreCase: request.IgnoreCase, Glob: request.Glob, Exclude: request.Exclude, Context: request.Context, MaxMatches: request.MaxMatches}
	result, err := p.execute(ctx, api.ResourceRequest{Operation: "search", RequestedURI: request.Root, URI: request.Root, Limits: api.PageLimits{Bytes: budget.Bytes, Lines: budget.Lines}, Search: query})
	if err != nil {
		return resource.SearchResult{}, err
	}
	return convertSearchResult(p.canonical, p.canonicalURI(request.Root), request.Mode, request.MaxMatches, result)
}

// convertSearchResult validates a provider's search answer against the searched root and mode.
func convertSearchResult(scheme, root, mode string, maxMatches int, result api.ResourceResult) (resource.SearchResult, error) {
	if len(result.Matches) > maxMatches {
		return resource.SearchResult{}, fmt.Errorf("%s:// provider returned more matches than requested", scheme)
	}
	hits := make([]resource.SearchHit, 0, len(result.Matches))
	for _, match := range result.Matches {
		if !withinSearchRoot(root, match.URI) {
			return resource.SearchResult{}, fmt.Errorf("%s:// provider returned a match outside the searched root", scheme)
		}
		hit := resource.SearchHit{URI: match.URI, Line: match.Line, Column: match.Column, Text: sanitizeProviderText(match.Text), Revision: match.Revision, Size: match.Size}
		if mode == "content" {
			if match.Line < 1 || match.Column < 0 {
				return resource.SearchResult{}, fmt.Errorf("%s:// provider returned a match without a valid line", scheme)
			}
			for _, line := range match.Before {
				hit.Before = append(hit.Before, resource.HitLine{Line: line.Line, Text: sanitizeProviderText(line.Text)})
			}
			for _, line := range match.After {
				hit.After = append(hit.After, resource.HitLine{Line: line.Line, Text: sanitizeProviderText(line.Text)})
			}
		} else {
			kind, err := entryKind(cmp.Or(match.Kind, string(resource.EntryFile)))
			if err != nil {
				return resource.SearchResult{}, err
			}
			hit.Dir = kind == resource.EntryDir
		}
		hits = append(hits, hit)
	}
	complete := result.Complete == nil || *result.Complete
	return resource.SearchResult{Hits: hits, Complete: complete}, nil
}

// withinSearchRoot reports whether uri is the searched root or below it.
func withinSearchRoot(root, uri string) bool {
	if uri == root {
		return true
	}
	if strings.HasSuffix(root, "/") {
		return strings.HasPrefix(uri, root)
	}
	return strings.HasPrefix(uri, root+"/")
}

func (p *contributedProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	if !p.descriptor.Has("read") {
		return resource.ReadResult{}, resource.ErrUnsupported
	}
	budget := request.Budget
	if limit := p.descriptor.Limits.PageBytes; limit > 0 {
		budget.Bytes = min(budget.Bytes, limit)
	}
	if limit := p.descriptor.Limits.PageLines; limit > 0 {
		budget.Lines = min(budget.Lines, limit)
	}
	operation := api.ResourceRequest{Operation: "read", RequestedURI: request.Requested, URI: request.URI, Limits: api.PageLimits{Bytes: budget.Bytes, Lines: budget.Lines}}
	selection := request.Selector
	explicit := len(selection.Ranges) > 0 || len(selection.Tails) > 0
	switch p.descriptor.SelectorMode {
	case "lines":
		selector := &api.Selector{Kind: "lines", Raw: selection.Raw, Tails: selection.Tails}
		for _, span := range selection.Ranges {
			selector.Ranges = append(selector.Ranges, api.LineRange{Start: span.Start, End: span.End})
		}
		operation.Selector = selector
	default:
		if explicit {
			return resource.ReadResult{}, fmt.Errorf("%s:// does not support line selectors", p.canonical)
		}
	}
	result, err := p.execute(ctx, operation)
	if err != nil {
		return resource.ReadResult{}, err
	}
	if result.Page == nil {
		return resource.ReadResult{}, fmt.Errorf("%s:// provider returned no text page", p.canonical)
	}
	text := strings.ReplaceAll(result.Page.Text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	spans := make([]resource.Range, 0, len(result.Page.ShownRanges))
	for _, span := range result.Page.ShownRanges {
		spans = append(spans, resource.Range{Start: span.Start, End: span.End})
	}
	total := 0
	if result.Page.TotalLines != nil {
		total = *result.Page.TotalLines
	}
	if result.Page.TotalLines != nil && total == 0 && len(lines) != 0 {
		return resource.ReadResult{}, errors.New("provider reported nonempty source with a known zero total")
	}
	if result.Page.ResourceComplete {
		if !result.Page.SelectionComplete {
			return resource.ReadResult{}, errors.New("provider reported complete source with an incomplete selection")
		}
		end := 0
		for _, span := range spans {
			if span.Start != end+1 {
				return resource.ReadResult{}, errors.New("provider complete-source coverage contains gaps")
			}
			end = span.End
		}
		if result.Page.TotalLines != nil && end != total {
			return resource.ReadResult{}, errors.New("provider complete-source coverage differs from its known total")
		}
	}
	page, err := resource.RenderSpans(ctx, result.CanonicalURI, request.Requested, spans, lines, total, selection.Raw, budget)
	if err != nil {
		return resource.ReadResult{}, fmt.Errorf("%s:// provider page is inconsistent: %w", p.canonical, err)
	}
	page.Revision = result.Revision
	page.UnknownTotal = result.Page.TotalLines == nil
	page.Handler = "provider:" + p.owner.Owner
	page.SelectionComplete = page.SelectionComplete && result.Page.SelectionComplete
	page.ResourceComplete = page.SelectionComplete && result.Page.ResourceComplete
	if !result.Page.SelectionComplete && page.Continuation == "" && len(spans) > 0 {
		next := spans[len(spans)-1].End + 1
		suffix := ":" + strconv.Itoa(next) + "-"
		if selection.Raw {
			suffix = ":" + strconv.Itoa(next) + "-:raw"
		}
		page.Continuation = result.CanonicalURI + suffix
		page.Content += "\n… incomplete; continue with selector " + suffix
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func installResourceProvider(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := resource.ParseProviderDescriptor(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	if descriptor.Has("edit") {
		return nil, fmt.Errorf("%w: provider snapshot editing is not routed; omit edit", errUnsupportedContribution)
	}
	owner := api.Identity{Owner: registration.Owner, Instance: registration.Instance}
	providers := make([]resource.Provider, 0, len(descriptor.Schemes))
	for _, scheme := range descriptor.Schemes {
		providers = append(providers, &contributedProvider{scheme: scheme, canonical: descriptor.Schemes[0], registrationID: registration.RegistrationID, descriptorRevision: registration.DescriptorRevision, generation: registration.Generation, cursors: &s.listCursors, owner: owner, descriptor: descriptor, bus: s.bus, project: s.projectIdentity})
	}
	if err := s.router.RegisterContributed(providers); err != nil {
		return nil, fmt.Errorf("%w: %v", errConsumerConflict, err)
	}
	return func() { s.router.UnregisterContributed(descriptor.Schemes) }, nil
}

// contributedDescriptor returns the descriptor of a contributed scheme, if any.
func (s *session) contributedDescriptor(scheme string) (*contributedProvider, bool) {
	provider, err := s.router.Provider(resource.Target{Scheme: scheme})
	if err != nil {
		return nil, false
	}
	contributed, ok := provider.(*contributedProvider)
	return contributed, ok
}

// firstTouchGuide returns the guide for a contributed scheme once per session and
// descriptor revision, so a later read does not repeat it (SPEC-0012 §5.5).
func (s *session) firstTouchGuide(scheme string) (string, bool) {
	provider, ok := s.contributedDescriptor(scheme)
	if !ok {
		return "", false
	}
	limit := 2048
	if s.settings != nil {
		if snapshot, err := s.settings.Current(); err == nil {
			if !enabled(snapshot, "guides.enabled", true) {
				return "", false
			}
			if entry, ok := snapshot.Get("guides.disabled"); ok {
				disabled, _ := entry.Value.([]string)
				if slices.Contains(disabled, provider.canonical) {
					return "", false
				}
			}
			limit = intValue(snapshot, "guides.max-bytes", limit)
		}
	}
	revision := provider.registrationID + ":" + provider.descriptor.GuideRevision + ":" + provider.descriptor.Summary
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if s.guideInjected == nil {
		s.guideInjected = make(map[string]string)
	}
	if s.guideInjected[provider.canonical] == revision {
		return "", false
	}
	guide, ok := s.schemeGuide(scheme)
	if !ok {
		return "", false
	}
	s.guideInjected[provider.canonical] = revision
	if limit == 0 || len(provider.descriptor.Guide) > limit {
		return guidePointer(provider.canonical), true
	}
	return guide, true
}

func guidePointer(scheme string) string {
	return "Provider guide: read docs://workbench/schemes/" + scheme
}

// schemeGuide renders the docs://workbench/schemes/<scheme> page for a contributed scheme.
func (s *session) schemeGuide(scheme string) (string, bool) {
	provider, ok := s.contributedDescriptor(scheme)
	if !ok {
		return "", false
	}
	d := provider.descriptor
	var b strings.Builder
	fmt.Fprintf(&b, "# %s://\n\n%s\n\n", provider.canonical, d.Summary)
	if d.Description != "" {
		b.WriteString(d.Description + "\n\n")
	}
	fmt.Fprintf(&b, "- Owner: %s\n- Label: %s\n- Capabilities: %s\n- Selector mode: %s\n- Backing: %s, scope: %s\n", provider.owner.Owner, d.Label, strings.Join(d.Capabilities, ", "), d.SelectorMode, d.Backing, d.Scope)
	if len(d.Schemes) > 1 {
		fmt.Fprintf(&b, "- Aliases: %s\n", strings.Join(d.Schemes[1:], ", "))
	}
	if d.Guide != "" {
		b.WriteString("\n" + d.Guide + "\n")
	}
	for _, guide := range d.Guides {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", guide.Path, guide.Markdown)
	}
	return b.String(), true
}

type projectIdentityCache struct {
	revision uint64
	valid    bool
	value    *api.Project
}

// projectIdentity returns the calling session's project; nil in the builtin global project.
func (s *session) projectIdentity(ctx context.Context) *api.Project {
	return s.projectOf(ctx, s.routingLocation(ctx))
}

// projectOf returns the project of one location snapshot, cached per location revision.
func (s *session) projectOf(ctx context.Context, current location.Snapshot) *api.Project {
	s.projectMu.Lock()
	defer s.projectMu.Unlock()
	if s.projectCache.valid && s.projectCache.revision == current.Revision {
		return s.projectCache.value
	}
	if s.projects == nil {
		return nil
	}
	detected, err := s.projects.Detect(ctx, current.Cwd)
	if err != nil {
		return nil
	}
	var value *api.Project
	if detected.ProjectID != "" && detected.ProjectID != "global" {
		value = &api.Project{ID: detected.ProjectID, Slug: detected.Slug}
	}
	s.projectCache = projectIdentityCache{revision: current.Revision, valid: true, value: value}
	return value
}
