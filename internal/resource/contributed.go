package resource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/textclean"
)

// ErrInvalidDescriptor reports a resourceProvider descriptor outside SPEC-0004 §2.
var ErrInvalidDescriptor = errors.New("invalid resource provider descriptor")

var schemePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]{0,31}$`)

// Provider capabilities SPEC-0004 §2 names.
var providerCapabilities = []string{"stat", "read", "list", "search", "locate", "write", "edit", "remove", "move", "copy", "mkdir", "watch"}

// ProviderLimits are a contributed provider's declared ceilings.
type ProviderLimits struct {
	PageBytes   int `json:"pageBytes,omitempty"`
	PageLines   int `json:"pageLines,omitempty"`
	QueryTimeMS int `json:"queryTimeMs,omitempty"`
	Concurrency int `json:"concurrency,omitempty"`
}

// ProviderSnapshots declares revision and consistency support.
type ProviderSnapshots struct {
	StableRevision bool `json:"stableRevision"`
	RangedRead     bool `json:"rangedRead"`
	CompareAndSwap bool `json:"compareAndSwap"`
	Expiry         bool `json:"expiry,omitempty"`
}

// ProviderGuide is one per-sub-namespace first-touch guide.
type ProviderGuide struct {
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

// MutationPolicy declares provider-enforced preconditions and receipt retention.
type MutationPolicy struct {
	RevisionChecked bool `json:"revisionChecked"`
	RetentionMS     int  `json:"retentionMs"`
	Durable         bool `json:"durable"`
}

// ProviderDescriptor is one validated resourceProvider registration.
type ProviderDescriptor struct {
	Label         string            `json:"label"`
	Description   string            `json:"description,omitempty"`
	Summary       string            `json:"summary"`
	Schemes       []string          `json:"schemes"`
	Capabilities  []string          `json:"capabilities"`
	SelectorMode  string            `json:"selectorMode"`
	Backing       string            `json:"backing"`
	Scope         string            `json:"scope"`
	ContentKinds  []string          `json:"contentKinds"`
	Mutations     *MutationPolicy   `json:"mutations,omitempty"`
	Snapshots     ProviderSnapshots `json:"snapshots"`
	Guide         string            `json:"guide,omitempty"`
	Guides        []ProviderGuide   `json:"guides,omitempty"`
	GuideRevision string            `json:"guideRevision,omitempty"`
	Display       json.RawMessage   `json:"display,omitempty"`
	Limits        ProviderLimits    `json:"limits"`
	QuerySchema   json.RawMessage   `json:"querySchema,omitempty"`
}

// Has reports whether the provider advertises one capability.
func (d ProviderDescriptor) Has(capability string) bool {
	return slices.Contains(d.Capabilities, capability)
}

// ParseProviderDescriptor strictly validates a resourceProvider descriptor.
func ParseProviderDescriptor(data []byte) (ProviderDescriptor, error) {
	var d ProviderDescriptor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return ProviderDescriptor{}, fmt.Errorf("%w: %v", ErrInvalidDescriptor, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ProviderDescriptor{}, fmt.Errorf("%w: trailing JSON", ErrInvalidDescriptor)
	}
	invalid := func(format string, args ...any) (ProviderDescriptor, error) {
		return ProviderDescriptor{}, fmt.Errorf("%w: %s", ErrInvalidDescriptor, fmt.Sprintf(format, args...))
	}
	if !plainLine(d.Label, 128) || d.Label == "" {
		return invalid("label must be 1-128 bytes of plain text")
	}
	if d.Description != "" && !plainLine(d.Description, 512) {
		return invalid("description must be plain text up to 512 bytes")
	}
	if d.Summary == "" || utf8.RuneCountInString(d.Summary) > 120 || !plainLine(d.Summary, 480) {
		return invalid("summary must be one plain line of 1-120 characters")
	}
	if len(d.Schemes) == 0 || len(d.Schemes) > 8 {
		return invalid("schemes must name 1-8 schemes")
	}
	seen := map[string]bool{}
	for _, scheme := range d.Schemes {
		if !schemePattern.MatchString(scheme) {
			return invalid("scheme %q does not match [a-z][a-z0-9+.-]* up to 32 bytes", scheme)
		}
		if reserved[scheme] {
			return invalid("scheme %q is a reserved Workbench builtin", scheme)
		}
		if replacement, retired := retiredSchemes[scheme]; retired {
			return invalid("scheme %q is retired; Workbench answers it with %s://", scheme, replacement)
		}
		if seen[scheme] {
			return invalid("scheme %q is repeated", scheme)
		}
		seen[scheme] = true
	}
	if len(d.Capabilities) == 0 {
		return invalid("capabilities must not be empty")
	}
	capabilities := map[string]bool{}
	for _, capability := range d.Capabilities {
		if !slices.Contains(providerCapabilities, capability) {
			return invalid("unknown capability %q", capability)
		}
		if capabilities[capability] {
			return invalid("capability %q is repeated", capability)
		}
		capabilities[capability] = true
	}
	if !capabilities["stat"] {
		return invalid("every provider must support stat")
	}
	if capabilities["edit"] && !capabilities["write"] {
		return invalid("edit requires write")
	}
	if capabilities["edit"] && (!d.Snapshots.StableRevision || !capabilities["read"]) {
		return invalid("edit requires read and stable revisions")
	}
	mutating := false
	for _, mutation := range []string{"write", "edit", "remove", "move", "copy", "mkdir"} {
		mutating = mutating || capabilities[mutation]
	}
	if mutating {
		if d.Mutations == nil || !d.Mutations.RevisionChecked || d.Mutations.RetentionMS < 30000 || d.Mutations.RetentionMS > 86400000 || !d.Snapshots.StableRevision || !d.Snapshots.CompareAndSwap {
			return invalid("mutations require revisionChecked, stableRevision, compareAndSwap and retentionMs between 30000 and 86400000")
		}
	} else if d.Mutations != nil {
		return invalid("mutations are declared for an immutable provider")
	}
	if !slices.Contains([]string{"lines", "provider", "none"}, d.SelectorMode) {
		return invalid("selectorMode must be lines, provider or none")
	}
	if d.SelectorMode == "lines" && !capabilities["read"] {
		return invalid("selectorMode lines requires read")
	}
	if !slices.Contains([]string{"file", "virtual", "remote", "device"}, d.Backing) {
		return invalid("backing must be file, virtual, remote or device")
	}
	if !slices.Contains([]string{"session", "project", "user", "external"}, d.Scope) {
		return invalid("scope must be session, project, user or external")
	}
	if len(d.ContentKinds) == 0 {
		return invalid("contentKinds must not be empty")
	}
	for _, kind := range d.ContentKinds {
		if !slices.Contains([]string{"text", "binary", "image", "container"}, kind) {
			return invalid("unknown content kind %q", kind)
		}
	}
	if slices.Contains(d.ContentKinds, "container") && !capabilities["list"] {
		return invalid("container content requires list")
	}
	if len(d.Guide) > 2048 {
		return invalid("guide exceeds 2 KiB")
	}
	if len(d.Guides) > 16 {
		return invalid("at most 16 sub-namespace guides are allowed")
	}
	for i, guide := range d.Guides {
		if guide.Path == "" || len(guide.Markdown) == 0 || len(guide.Markdown) > 2048 || !plainLine(guide.Path, 256) {
			return invalid("guides[%d] is invalid", i)
		}
	}
	if (d.Guide != "" || len(d.Guides) > 0) && d.GuideRevision == "" {
		return invalid("guideRevision is required with a guide")
	}
	if d.Limits.PageBytes < 0 || d.Limits.PageLines < 0 || d.Limits.QueryTimeMS < 0 || d.Limits.Concurrency < 0 || d.Limits.Concurrency > 8 {
		return invalid("limits must be nonnegative and concurrency at most 8")
	}
	return d, nil
}

func plainLine(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !textclean.HasControl(value)
}
