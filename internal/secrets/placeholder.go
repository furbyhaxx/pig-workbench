package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"regexp"
	"sort"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`red-sec-[a-z2-7]{16}`)

// Placeholder returns a deterministic placeholder for value using a derived HMAC key.
func Placeholder(hmacKey, value []byte) string {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(value)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil))
	return "red-sec-" + strings.ToLower(encoded[:16])
}

// DetectPlaceholder returns distinct placeholders in their first-occurrence order.
func DetectPlaceholder(text string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, match := range placeholderPattern.FindAllString(text, -1) {
		if _, ok := seen[match]; !ok {
			seen[match] = struct{}{}
			result = append(result, match)
		}
	}
	return result
}

type redactionValue struct {
	value       string
	placeholder string
}

type protectedSpan struct {
	start, end int
}

// Redact replaces known values, protecting existing and newly generated placeholders.
// A larger known value may contain a placeholder; replacing the whole value is
// mandatory, while a value that would only cut into a placeholder is refused.
// It returns no output on invalid keys or mappings.
func Redact(text string, hmacKey []byte, known map[string]string) (string, error) {
	if len(hmacKey) < keySize {
		return "", errors.New("secret HMAC key must contain at least 32 bytes")
	}
	values := make([]redactionValue, 0, len(known))
	for value, placeholder := range known {
		if value == "" || placeholder != Placeholder(hmacKey, []byte(value)) {
			return "", errors.New("invalid known secret mapping")
		}
		values = append(values, redactionValue{value: value, placeholder: placeholder})
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i].value) != len(values[j].value) {
			return len(values[i].value) > len(values[j].value)
		}
		return values[i].value < values[j].value
	})

	var protected []protectedSpan
	for _, match := range placeholderPattern.FindAllStringIndex(text, -1) {
		protected = append(protected, protectedSpan{start: match[0], end: match[1]})
	}
	for _, value := range values {
		text, protected = replaceKnown(text, protected, value)
	}
	return text, nil
}

// replaceKnown replaces one value longest-first across the current text. An
// occurrence wholly inside a protected placeholder is skipped; one that
// contains a complete placeholder replaces the whole larger value.
func replaceKnown(text string, protected []protectedSpan, value redactionValue) (string, []protectedSpan) {
	type replacement struct{ start, end int }
	var replacements []replacement
	for offset := 0; offset+len(value.value) <= len(text); {
		index := strings.Index(text[offset:], value.value)
		if index < 0 {
			break
		}
		start := offset + index
		end := start + len(value.value)
		if allowedReplacement(protected, start, end) {
			start, end = extendOverPlaceholders(protected, start, end)
			replacements = append(replacements, replacement{start: start, end: end})
		}
		offset = end
	}
	if len(replacements) == 0 {
		return text, protected
	}
	var result strings.Builder
	result.Grow(len(text))
	spans := make([]protectedSpan, 0, len(protected)+len(replacements))
	cursor, adjusted, delta := 0, 0, 0
	for _, item := range replacements {
		result.WriteString(text[cursor:item.start])
		start := result.Len()
		result.WriteString(value.placeholder)
		spans = append(spans, protectedSpan{start: start, end: result.Len()})
		for adjusted < len(protected) && protected[adjusted].end <= item.start {
			span := protected[adjusted]
			spans = append(spans, protectedSpan{start: span.start + delta, end: span.end + delta})
			adjusted++
		}
		for adjusted < len(protected) && protected[adjusted].start < item.end {
			adjusted++
		}
		delta += len(value.placeholder) - len(value.value)
		cursor = item.end
	}
	result.WriteString(text[cursor:])
	for ; adjusted < len(protected); adjusted++ {
		span := protected[adjusted]
		spans = append(spans, protectedSpan{start: span.start + delta, end: span.end + delta})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return result.String(), spans
}

// extendOverPlaceholders consumes a whole intersecting placeholder so a partial
// overlap cannot leave a marker fragment beside the replacement.
func extendOverPlaceholders(protected []protectedSpan, start, end int) (int, int) {
	for _, span := range protected {
		if end <= span.start || start >= span.end {
			continue
		}
		start = min(start, span.start)
		end = max(end, span.end)
	}
	return start, end
}

func allowedReplacement(protected []protectedSpan, start, end int) bool {
	for _, span := range protected {
		if end <= span.start || start >= span.end {
			continue
		}
		// Only a shorter value wholly inside an existing placeholder is shielded.
		// Any overlap extending beyond it is part of the larger credential and
		// must be replaced rather than leaked.
		if span.start <= start && end <= span.end {
			return false
		}
	}
	return true
}
