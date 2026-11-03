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

type redactionSegment struct {
	text      string
	protected bool
}

// Redact replaces known values, protecting existing and newly generated placeholders.
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

	var segments []redactionSegment
	start := 0
	for _, match := range placeholderPattern.FindAllStringIndex(text, -1) {
		segments = append(segments, redactionSegment{text: text[start:match[0]]})
		segments = append(segments, redactionSegment{text: text[match[0]:match[1]], protected: true})
		start = match[1]
	}
	segments = append(segments, redactionSegment{text: text[start:]})
	for _, value := range values {
		next := make([]redactionSegment, 0, len(segments))
		for _, segment := range segments {
			if segment.protected {
				next = append(next, segment)
				continue
			}
			remaining := segment.text
			for {
				before, after, found := strings.Cut(remaining, value.value)
				if !found {
					next = append(next, redactionSegment{text: remaining})
					break
				}
				if before != "" {
					next = append(next, redactionSegment{text: before})
				}
				next = append(next, redactionSegment{text: value.placeholder, protected: true})
				remaining = after
			}
		}
		segments = next
	}
	var result strings.Builder
	for _, segment := range segments {
		result.WriteString(segment.text)
	}
	return result.String(), nil
}
