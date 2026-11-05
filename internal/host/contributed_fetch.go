package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// errInvalidContribution reports a host-validated descriptor outside its specification.
var errInvalidContribution = errors.New("invalid contribution descriptor")

type fetchMatch struct {
	Host       string `json:"host"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type fetchDescriptor struct {
	Label        string       `json:"label"`
	Description  string       `json:"description,omitempty"`
	Match        []fetchMatch `json:"match"`
	Capabilities []string     `json:"capabilities"`
	Egress       string       `json:"egress"`
	Retention    string       `json:"retention,omitempty"`
	Limits       struct {
		BodyBytes int `json:"bodyBytes,omitempty"`
		TimeMS    int `json:"timeMs,omitempty"`
	} `json:"limits"`
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", errInvalidContribution, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON", errInvalidContribution)
	}
	return nil
}

func plainField(value string, maxBytes int) bool {
	if len(value) > maxBytes {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 || r >= 0x80 && r <= 0x9f {
			return false
		}
	}
	return true
}

func parseFetchBackend(data []byte) (fetchDescriptor, error) {
	var d fetchDescriptor
	if err := decodeStrict(data, &d); err != nil {
		return d, err
	}
	invalid := func(message string) (fetchDescriptor, error) {
		return fetchDescriptor{}, fmt.Errorf("%w: fetchBackend %s", errInvalidContribution, message)
	}
	if d.Label == "" || !plainField(d.Label, 128) || !plainField(d.Description, 512) {
		return invalid("label and description must be plain text")
	}
	if len(d.Match) == 0 || len(d.Match) > 64 {
		return invalid("match must contain 1-64 rules")
	}
	for _, rule := range d.Match {
		host := strings.TrimPrefix(rule.Host, "*.")
		if rule.Host != "*" && (host == "" || strings.ContainsAny(host, "/:*@ ") || host != strings.ToLower(host)) {
			return invalid("match host must be exact, *.suffix or *")
		}
		if rule.PathPrefix != "" && !strings.HasPrefix(rule.PathPrefix, "/") {
			return invalid("pathPrefix must start with /")
		}
	}
	if !slices.Contains(d.Capabilities, "fetch") {
		return invalid("capabilities must include fetch")
	}
	for _, capability := range d.Capabilities {
		if !slices.Contains([]string{"fetch", "locate", "offline"}, capability) {
			return invalid("unknown capability")
		}
	}
	if d.Egress != "none" && d.Egress != "thirdParty" {
		return invalid("egress must be none or thirdParty")
	}
	if d.Retention != "" && d.Retention != "user" && d.Retention != "project" {
		return invalid("retention must be user or project")
	}
	if d.Limits.BodyBytes < 0 || d.Limits.TimeMS < 0 || d.Limits.TimeMS > 300000 {
		return invalid("limits are out of range")
	}
	return d, nil
}

type fetchBackend struct {
	registrationID string
	key            string
	owner          api.Identity
	order          uint64
	descriptor     fetchDescriptor
}

// fetchRouter selects contributed fetch backends for http(s) reads (SPEC-0004 §11).
type fetchRouter struct {
	mu       sync.RWMutex
	backends []fetchBackend
	next     uint64
	bus      *providerBus
	router   func() *resource.Router
	project  func(context.Context) *api.Project
}

func (f *fetchRouter) add(backend fetchBackend) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	backend.order = f.next
	f.backends = append(f.backends, backend)
}

func (f *fetchRouter) remove(registrationID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.backends = slices.DeleteFunc(f.backends, func(b fetchBackend) bool { return b.registrationID == registrationID })
}

func (f *fetchRouter) list() []fetchBackend {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return slices.Clone(f.backends)
}

// specificity ranks a rule: exact host beats a longer suffix beats a shorter one beats "*",
// then a longer path prefix wins. Negative means no match.
func ruleSpecificity(rule fetchMatch, host, path string) int {
	score := -1
	switch {
	case rule.Host == "*":
		score = 0
	case strings.HasPrefix(rule.Host, "*."):
		suffix := rule.Host[1:]
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			score = 1000 + len(suffix)
		}
	case rule.Host == host:
		score = 100000
	}
	if score < 0 || !strings.HasPrefix(path, rule.PathPrefix) {
		return -1
	}
	return score*1000 + len(rule.PathPrefix)
}

func (f *fetchRouter) selectBackend(raw string) (fetchBackend, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fetchBackend{}, false
	}
	host := strings.ToLower(parsed.Hostname())
	path := parsed.EscapedPath()
	f.mu.RLock()
	defer f.mu.RUnlock()
	best, bestScore, found := fetchBackend{}, -1, false
	for _, backend := range f.backends {
		for _, rule := range backend.descriptor.Match {
			score := ruleSpecificity(rule, host, path)
			if score > bestScore || score == bestScore && found && backend.order < best.order {
				best, bestScore, found = backend, score, score >= 0
			}
		}
	}
	return best, found
}

// Fetch implements resource.FetchRouter.
func (f *fetchRouter) Fetch(ctx context.Context, uri string, limit int) (resource.BackendFetch, bool, error) {
	backend, ok := f.selectBackend(uri)
	if !ok || f.bus == nil {
		return resource.BackendFetch{}, false, nil
	}
	if backend.descriptor.Limits.BodyBytes > 0 {
		limit = min(limit, backend.descriptor.Limits.BodyBytes)
	}
	budget := 30 * time.Second
	if backend.descriptor.Limits.TimeMS > 0 {
		budget = time.Duration(backend.descriptor.Limits.TimeMS) * time.Millisecond
	}
	request := api.FetchRequest{URL: uri, TimeoutMS: int(budget / time.Millisecond), MaxBytes: limit, Accept: "text/html, text/plain, text/markdown, application/json"}
	if f.project != nil {
		request.Project = f.project(ctx)
	}
	var answer api.FetchResult
	if err := f.bus.call(ctx, backend.owner, backend.registrationID, "fetch", request, &answer, budget); err != nil {
		return resource.BackendFetch{}, true, fmt.Errorf("%s: %w", backend.key, err)
	}
	if answer.UsedThirdParty && backend.descriptor.Egress == "none" {
		return resource.BackendFetch{}, true, fmt.Errorf("%s reported third-party egress despite declaring none; answer rejected", backend.key)
	}
	result := resource.BackendFetch{ContentType: answer.ContentType, FinalURI: answer.FinalURL, Redirects: answer.Redirects, Revision: answer.Revision, Status: answer.Status, UsedThirdParty: answer.UsedThirdParty, Handler: "backend:" + backend.owner.Owner + "/" + backend.key}
	if result.FinalURI == "" {
		result.FinalURI = uri
	}
	if answer.Handler != "" {
		result.Handler += "/" + sanitizeProviderText(answer.Handler)
	}
	for _, notice := range answer.Notices {
		result.Notices = append(result.Notices, sanitizeProviderText(notice))
	}
	if backend.descriptor.Retention != "" {
		result.Notices = append(result.Notices, "stored by "+backend.owner.Owner+" with "+backend.descriptor.Retention+" retention")
	}
	if answer.Charset != "" && result.ContentType != "" && !strings.Contains(strings.ToLower(result.ContentType), "charset=") {
		result.ContentType += "; charset=" + answer.Charset
	}
	switch {
	case answer.Body != nil && answer.Locate != "":
		return resource.BackendFetch{}, true, errors.New("backend answered with both body and locate")
	case answer.Body != nil:
		result.Body = []byte(*answer.Body)
	case answer.Locate != "":
		body, err := f.locate(ctx, backend, answer.Locate, limit)
		if err != nil {
			return resource.BackendFetch{}, true, err
		}
		result.Body = body
	default:
		if answer.Status >= 400 && answer.Status < 500 {
			return result, true, nil
		}
		return resource.BackendFetch{}, true, errors.New("backend answered with neither body nor locate")
	}
	return result, true, nil
}

// locate reads a stored snapshot through the router once, only from a provider of the same owner.
func (f *fetchRouter) locate(ctx context.Context, backend fetchBackend, uri string, limit int) ([]byte, error) {
	if !slices.Contains(backend.descriptor.Capabilities, "locate") {
		return nil, errors.New("backend answered locate without declaring the locate capability")
	}
	target, err := resource.ParsePath("/", uri, ":raw")
	if err != nil {
		return nil, fmt.Errorf("backend locate target is invalid: %w", err)
	}
	router := f.router()
	provider, err := router.Provider(target)
	if err != nil {
		return nil, fmt.Errorf("backend locate target is unavailable: %w", err)
	}
	contributed, ok := provider.(*contributedProvider)
	if !ok || contributed.owner.Owner != backend.owner.Owner {
		return nil, errors.New("backend locate must name a resource provider registered by the same owner")
	}
	result, err := contributed.Read(ctx, resource.ReadRequest{URI: target.URI, Requested: uri, Selector: target.Selection, Budget: resource.Budget{Bytes: limit + 1024, Lines: 1 << 30}})
	if err != nil {
		return nil, fmt.Errorf("read located snapshot: %w", err)
	}
	if !result.Facts.SelectionComplete {
		return nil, fmt.Errorf("located snapshot exceeds %d bytes", limit)
	}
	return []byte(result.Content), nil
}

func installFetchBackend(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := parseFetchBackend(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	s.fetch.add(fetchBackend{registrationID: registration.RegistrationID, key: registration.Key, owner: api.Identity{Owner: registration.Owner, Instance: registration.Instance}, descriptor: descriptor})
	return func() { s.fetch.remove(registration.RegistrationID) }, nil
}
