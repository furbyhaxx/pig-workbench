package contribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"unicode/utf8"
)

// revisionPrefix labels every descriptor revision in receipts and wire replies.
const revisionPrefix = "rev-"

// claimSet is the validated, deduplicated and sorted form of Claims.
type claimSet struct {
	schemes []string
	sources []SourceClaim
}

// prepare canonicalizes a descriptor and derives its content revision, so key order and
// whitespace cannot change the revision while any content change does.
func (r *Registry) prepare(descriptor json.RawMessage) ([]byte, string, error) {
	if len(descriptor) > r.limits.DescriptorBytes {
		return nil, "", &LimitError{Scope: "descriptor", Limit: r.limits.DescriptorBytes}
	}
	trimmed := bytes.TrimSpace(descriptor)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, "", fmt.Errorf("%w: descriptor must be a JSON object", ErrInvalidDescriptor)
	}
	if !utf8.Valid(trimmed) {
		return nil, "", fmt.Errorf("%w: descriptor is not valid UTF-8", ErrInvalidDescriptor)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidDescriptor, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, "", fmt.Errorf("%w: descriptor has a trailing JSON value", ErrInvalidDescriptor)
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidDescriptor, err)
	}
	canonical := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
	if len(canonical) > r.limits.DescriptorBytes {
		return nil, "", &LimitError{Scope: "descriptor", Limit: r.limits.DescriptorBytes}
	}
	digest := sha256.Sum256(canonical)
	return canonical, revisionPrefix + hex.EncodeToString(digest[:16]), nil
}

func (r *Registry) claims(kind Kind, key string, descriptor []byte) (claimSet, error) {
	reported, err := r.validator(kind, key, bytes.Clone(descriptor))
	if err != nil {
		return claimSet{}, fmt.Errorf("%w: %w", ErrInvalidDescriptor, err)
	}
	return normalizeClaims(reported)
}

func normalizeClaims(reported Claims) (claimSet, error) {
	normalized := claimSet{}
	seenSchemes := make(map[string]struct{}, len(reported.Schemes))
	for _, scheme := range reported.Schemes {
		if !validScheme(scheme) {
			return claimSet{}, fmt.Errorf("%w: scheme claim %q must match [a-z][a-z0-9+.-]* in 1–%d bytes", ErrInvalidDescriptor, scheme, maxSchemeBytes)
		}
		if _, duplicate := seenSchemes[scheme]; duplicate {
			continue
		}
		seenSchemes[scheme] = struct{}{}
		normalized.schemes = append(normalized.schemes, scheme)
	}
	sort.Strings(normalized.schemes)
	seenSources := make(map[SourceClaim]struct{}, len(reported.Sources))
	for _, source := range reported.Sources {
		if !validIdentifier(source.Source) || !validIdentifier(source.Field) {
			return claimSet{}, fmt.Errorf("%w: source claim must name a source and field of 1–%d UTF-8 bytes without controls", ErrInvalidDescriptor, maxIdentifierBytes)
		}
		if _, duplicate := seenSources[source]; duplicate {
			continue
		}
		seenSources[source] = struct{}{}
		normalized.sources = append(normalized.sources, source)
	}
	sort.Slice(normalized.sources, func(i, j int) bool {
		if normalized.sources[i].Source != normalized.sources[j].Source {
			return normalized.sources[i].Source < normalized.sources[j].Source
		}
		return normalized.sources[i].Field < normalized.sources[j].Field
	})
	return normalized, nil
}

// validScheme implements the SPEC-0004 §2 scheme grammar.
func validScheme(scheme string) bool {
	if scheme == "" || len(scheme) > maxSchemeBytes {
		return false
	}
	for i, r := range scheme {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '.' || r == '-'):
		default:
			return false
		}
	}
	return true
}
