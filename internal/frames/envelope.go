package frames

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var envelopeOpening = regexp.MustCompile(`^<system-(instruction|reminder|warning|error|notice|info|message) src="([^"]*)" seq="([0-9]+)" n="([^"]*)" ts="([^"]*)" trust="(trusted|untrusted)" authority="(data|instruction)">$`)

// Envelope is the retained model-facing projection, not an authority credential.
type Envelope struct {
	Kind, Source, Nonce, Trust, Authority, Body string
	Sequence                                    int64
	CreatedAt                                   time.Time
}

// EncodeEnvelope deterministically projects a retained frame without minting metadata.
func EncodeEnvelope(frame Frame) (string, error) {
	if _, err := ValidatePublish(Publish{EventKey: frame.EventKey, Kind: frame.Kind, Source: frame.Source, Body: frame.Body, DataTrust: frame.Trust}); err != nil {
		return "", err
	}
	if frame.Sequence < 1 || frame.Nonce == "" || frame.CreatedAt.IsZero() || (frame.Authority != "data" && frame.Authority != "instruction") {
		return "", errors.New("frame projection lacks retained provenance")
	}
	closing := "</system-" + frame.Kind + ">"
	escaped := "&lt;/system-" + frame.Kind + "&gt;"
	// Only text that could be mistaken for an escape is escaped, so a body reads as written
	// ("a && b" stays "a && b") and the projection still decodes to exactly one body.
	body := strings.NewReplacer("&amp;", "&amp;amp;", "&lt;/system-", "&amp;lt;/system-").Replace(frame.Body)
	body = strings.ReplaceAll(body, closing, escaped)
	return fmt.Sprintf("<system-%s src=\"%s\" seq=\"%d\" n=\"%s\" ts=\"%s\" trust=\"%s\" authority=\"%s\">\n%s\n%s", frame.Kind, html.EscapeString(frame.Source), frame.Sequence, html.EscapeString(frame.Nonce), frame.CreatedAt.Format(time.RFC3339Nano), frame.Trust, frame.Authority, body, closing), nil
}

// DecodeEnvelope reads only Workbench's canonical projection; it grants no trust.
func DecodeEnvelope(text string) (Envelope, error) {
	var result Envelope
	opening, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return result, errors.New("frame envelope is missing its body")
	}
	fields := envelopeOpening.FindStringSubmatch(opening)
	if fields == nil {
		return result, errors.New("invalid frame envelope opening")
	}
	closing := "\n</system-" + fields[1] + ">"
	if !strings.HasSuffix(rest, closing) {
		return result, errors.New("frame envelope closing does not match")
	}
	body := strings.TrimSuffix(rest, closing)
	if strings.Contains(body, "</system-"+fields[1]+">") {
		return result, errors.New("frame body contains an unescaped closing delimiter")
	}
	sequence, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || sequence < 1 {
		return result, errors.New("invalid frame sequence")
	}
	created, err := time.Parse(time.RFC3339Nano, fields[5])
	if err != nil {
		return result, errors.New("invalid frame timestamp")
	}
	result = Envelope{Kind: fields[1], Source: html.UnescapeString(fields[2]), Sequence: sequence, Nonce: html.UnescapeString(fields[4]), CreatedAt: created, Trust: fields[6], Authority: fields[7], Body: strings.NewReplacer("&amp;", "&", "&lt;/system-"+fields[1]+"&gt;", "</system-"+fields[1]+">").Replace(body)}
	if !validSource(result.Source) || result.Nonce == "" {
		return Envelope{}, errors.New("invalid frame attribution")
	}
	return result, nil
}
