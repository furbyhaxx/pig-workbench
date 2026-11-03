package secrets

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// minSecretLength is the shortest value ever treated as a secret (SPEC-0013 §2).
const minSecretLength = 8

const urlPasswordKind = "url_password"

var (
	// urlPasswordPattern captures the password of scheme://user:password@host.
	urlPasswordPattern = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://[^\s/:@]*:([^\s/@]{8,})@`)
	patternNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)
)

// tokenPattern couples a recorded kind with its compiled expression.
type tokenPattern struct {
	kind string
	expr *regexp.Regexp
}

// builtinTokenPatterns are the high-confidence token shapes of SPEC-0013 §2, most
// specific first. Expressions adapted from @furbyhaxx/pi-secret-guard detect.ts at
// commit 441cdf8 (MIT License, Copyright (c) 2026 Furby Haxx).
var builtinTokenPatterns = []tokenPattern{
	{"github_pat", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,255}`)},
	{"github_token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,255}`)},
	{"gitlab_token", regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20,255}`)},
	{"anthropic_key", regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,255}`)},
	{"openai_key", regexp.MustCompile(`sk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,255}`)},
	{"slack_token", regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,255}`)},
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{"private_key", regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)},
}

// Pattern is one user token shape: a name plus exactly one of Regex, Prefix or Suffix.
type Pattern struct {
	// Name becomes the recorded kind. Lowercase letters, digits and underscores.
	Name string
	// Regex matches the whole value and must not match an empty string.
	Regex string
	// Prefix is a literal token prefix; at least the remaining characters of the
	// minimum length follow it.
	Prefix string
	// Suffix is a literal token suffix; at least the remaining characters of the
	// minimum length precede it.
	Suffix string
	// MinLength is the total token length of a Prefix pattern. At least 8, and
	// invalid on Regex and Suffix patterns.
	MinLength int
}

// Exclusions lists what is never learned or replaced, so false positives stay
// readable. Hashes are placeholder hashes, with or without the red-sec- prefix.
type Exclusions struct {
	Values []string
	Hashes []string
}

// DetectionPolicy supplies user token shapes and exclusions. Loading policy
// files is the caller's concern; this package only compiles the declarations.
type DetectionPolicy struct {
	Patterns   []Pattern
	Exclusions Exclusions
}

// Detector learns, redacts and substitutes values against one vault. Its
// methods are safe for concurrent use and never return output on error.
type Detector struct {
	hmacKey              []byte
	vault                *Vault
	patterns             []tokenPattern
	excludedPlaceholders map[string]struct{}
	excludedValues       map[string]struct{}
}

type detectedValue struct {
	kind  string
	value string
}

// NewDetector validates the master key and policy and returns a detector for vault.
func NewDetector(master []byte, vault *Vault, policy DetectionPolicy) (*Detector, error) {
	hmacKey, err := DeriveHMACKey(master)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, errors.New("secret detection requires a vault")
	}
	patterns, excludedPlaceholders, err := compilePolicy(policy, hmacKey)
	if err != nil {
		return nil, err
	}
	excludedValues := make(map[string]struct{}, len(policy.Exclusions.Values))
	for _, value := range policy.Exclusions.Values {
		if value != "" {
			excludedValues[value] = struct{}{}
		}
	}
	return &Detector{
		hmacKey:              hmacKey,
		vault:                vault,
		patterns:             patterns,
		excludedPlaceholders: excludedPlaceholders,
		excludedValues:       excludedValues,
	}, nil
}

func compilePolicy(policy DetectionPolicy, hmacKey []byte) ([]tokenPattern, map[string]struct{}, error) {
	patterns := make([]tokenPattern, 0, len(builtinTokenPatterns)+len(policy.Patterns))
	patterns = append(patterns, builtinTokenPatterns...)
	names := make(map[string]struct{}, len(builtinTokenPatterns)+len(policy.Patterns))
	for _, pattern := range builtinTokenPatterns {
		names[pattern.kind] = struct{}{}
	}
	for _, spec := range policy.Patterns {
		expr, err := patternExpression(spec, names)
		if err != nil {
			return nil, nil, err
		}
		names[spec.Name] = struct{}{}
		patterns = append(patterns, tokenPattern{kind: spec.Name, expr: expr})
	}
	excludedPlaceholders := make(map[string]struct{}, len(policy.Exclusions.Hashes))
	for _, hash := range policy.Exclusions.Hashes {
		placeholder := hash
		if !strings.HasPrefix(placeholder, "red-sec-") {
			placeholder = "red-sec-" + placeholder
		}
		if placeholderPattern.FindString(placeholder) != placeholder {
			return nil, nil, errors.New("secret exclusion hash is not a red-sec placeholder hash")
		}
		excludedPlaceholders[placeholder] = struct{}{}
	}
	return patterns, excludedPlaceholders, nil
}

func patternExpression(spec Pattern, names map[string]struct{}) (*regexp.Regexp, error) {
	if !patternNamePattern.MatchString(spec.Name) {
		return nil, errors.New("secret pattern names must be non-empty lowercase letters, digits or underscores")
	}
	if _, exists := names[spec.Name]; exists {
		return nil, fmt.Errorf("secret pattern %q is declared more than once", spec.Name)
	}
	declared := 0
	for _, source := range []string{spec.Regex, spec.Prefix, spec.Suffix} {
		if source != "" {
			declared++
		}
	}
	if declared != 1 {
		return nil, fmt.Errorf("secret pattern %q needs exactly one of regex, prefix or suffix", spec.Name)
	}
	if spec.MinLength < 0 || (spec.MinLength > 0 && spec.MinLength < minSecretLength) {
		return nil, fmt.Errorf("secret pattern %q min length must be at least %d", spec.Name, minSecretLength)
	}
	if spec.MinLength > 0 && spec.Prefix == "" {
		return nil, fmt.Errorf("secret pattern %q sets min length without a prefix", spec.Name)
	}
	var source string
	switch {
	case spec.Regex != "":
		source = spec.Regex
	case spec.Prefix != "":
		total := spec.MinLength
		if total == 0 {
			total = minSecretLength
		}
		source = regexp.QuoteMeta(spec.Prefix) + fmt.Sprintf(`[A-Za-z0-9_-]{%d,}`, max(0, total-len(spec.Prefix)))
	default:
		source = fmt.Sprintf(`[A-Za-z0-9_-]{%d,}`, max(0, minSecretLength-len(spec.Suffix))) + regexp.QuoteMeta(spec.Suffix)
	}
	expr, err := regexp.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("secret pattern %q is not a valid regex, prefix or suffix", spec.Name)
	}
	if expr.MatchString("") {
		return nil, fmt.Errorf("secret pattern %q must not match an empty string", spec.Name)
	}
	return expr, nil
}

// Learn stores value in the vault under its placeholder with the given kind and
// source. Values shorter than 8 characters, whole placeholders and excluded
// values are not secrets and are ignored. Vault failures are returned and
// nothing is learned.
func (d *Detector) Learn(value []byte, kind, source string) error {
	if len(value) < minSecretLength || isPlaceholder(string(value)) || d.excludedValue(string(value)) {
		return nil
	}
	return d.vault.Put(Record{
		Placeholder: Placeholder(d.hmacKey, value),
		Kind:        kind,
		Source:      source,
		Scope:       "global",
	}, value)
}

// Redact replaces learned and newly detected values with their placeholders. It
// snapshots the vault per call, so values learned by other sessions are covered.
// New values are persisted before they are used; no error case returns text.
func (d *Detector) Redact(text string) (string, error) {
	known, err := d.knownValues()
	if err != nil {
		return "", err
	}
	for _, found := range d.detect(text) {
		if _, ok := known[found.value]; ok || d.excludedValue(found.value) {
			continue
		}
		if err := d.Learn([]byte(found.value), found.kind, "detected"); err != nil {
			return "", err
		}
		known[found.value] = Placeholder(d.hmacKey, []byte(found.value))
	}
	return Redact(text, d.hmacKey, known)
}

// Substitute replaces canonical placeholders with their exact values. Unknown,
// deleted or tampered placeholders fail the call instead of staying unresolved.
func (d *Detector) Substitute(text string) (string, error) {
	matches := placeholderPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	values := make(map[string][]byte, len(matches))
	var result strings.Builder
	start := 0
	for _, match := range matches {
		placeholder := text[match[0]:match[1]]
		value, ok := values[placeholder]
		if !ok {
			stored, err := d.vault.Value(placeholder)
			if err != nil {
				return "", err
			}
			if Placeholder(d.hmacKey, stored) != placeholder {
				return "", &ErrRecordInvalid{}
			}
			values[placeholder] = stored
			value = stored
		}
		result.WriteString(text[start:match[0]])
		result.Write(value)
		start = match[1]
	}
	result.WriteString(text[start:])
	return result.String(), nil
}

// knownValues decrypts the current vault contents and drops empty and excluded
// values. The 8-character minimum filters detection, never an existing record:
// explicitly registered short values must still be redacted.
func (d *Detector) knownValues() (map[string]string, error) {
	values, err := d.vault.Values()
	if err != nil {
		return nil, err
	}
	known := make(map[string]string, len(values))
	for placeholder, value := range values {
		if len(value) == 0 || d.excludedValue(string(value)) {
			continue
		}
		if Placeholder(d.hmacKey, value) != placeholder {
			return nil, &ErrRecordInvalid{}
		}
		known[string(value)] = placeholder
	}
	return known, nil
}

// isPlaceholder reports whether value is exactly one canonical placeholder.
func isPlaceholder(value string) bool {
	return value != "" && placeholderPattern.FindString(value) == value
}

// excludedValue reports whether value is excluded by literal or by placeholder hash.
func (d *Detector) excludedValue(value string) bool {
	if _, ok := d.excludedValues[value]; ok {
		return true
	}
	_, ok := d.excludedPlaceholders[Placeholder(d.hmacKey, []byte(value))]
	return ok
}

// detect returns distinct built-in, user and URL-password matches outside placeholders.
func (d *Detector) detect(text string) []detectedValue {
	if text == "" {
		return nil
	}
	var placeholders [][2]int
	for _, match := range placeholderPattern.FindAllStringIndex(text, -1) {
		placeholders = append(placeholders, [2]int{match[0], match[1]})
	}
	seen := make(map[string]struct{})
	var found []detectedValue
	add := func(kind string, start, end int) {
		value := text[start:end]
		if len(value) < minSecretLength {
			return
		}
		for _, span := range placeholders {
			if start >= span[0] && end <= span[1] {
				return
			}
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		found = append(found, detectedValue{kind: kind, value: value})
	}
	for _, pattern := range d.patterns {
		for _, match := range pattern.expr.FindAllStringIndex(text, -1) {
			add(pattern.kind, match[0], match[1])
		}
	}
	for _, match := range urlPasswordPattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) >= 4 && match[2] >= 0 {
			add(urlPasswordKind, match[2], match[3])
		}
	}
	return found
}
