package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type readProjection struct {
	Page   resource.Page `json:"page"`
	Bytes  int           `json:"bytes"`
	Digest string        `json:"digest"`
}

func readProof(result resource.ReadResult) *readProjection {
	if result.Facts.CoordinateSpace != "source-lines" || result.Facts.Revision == "" || result.Facts.CanonicalURI == "" || result.Facts.Handler != "" || len(result.Facts.Notices) > 0 || result.Facts.Truncation == "summary" || result.Content == "" {
		return nil
	}
	digest := sha256.Sum256([]byte(result.Content))
	return &readProjection{Page: result.Facts, Bytes: len(result.Content), Digest: hex.EncodeToString(digest[:])}
}

func (a *app) projectReadContext(_ sdk.Context, data map[string]any) (any, error) {
	messages, _ := data["messages"].([]any)
	if projected := projectReadMessages(messages); projected != nil {
		return map[string]any{"messages": projected}, nil
	}
	return nil, nil
}

func projectReadMessages(messages []any) []any {
	type coverage struct {
		call  string
		total int
	}
	complete := make(map[string]coverage)
	var result []any
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != "toolResult" || message["toolName"] != "read" || message["isError"] == true {
			continue
		}
		details, ok := message["details"].(map[string]any)
		if !ok {
			continue
		}
		encoded, err := json.Marshal(details["readProjection"])
		if err != nil || len(encoded) > 8192 {
			continue
		}
		var proof readProjection
		if json.Unmarshal(encoded, &proof) != nil {
			continue
		}
		p := proof.Page
		if p.CanonicalURI == "" || p.Revision == "" || p.CoordinateSpace != "source-lines" || p.Handler != "" || len(p.Notices) > 0 || p.Truncation == "summary" {
			continue
		}
		if _, ok := replaceReadSource(message["content"], proof, ""); !ok {
			continue
		}
		call, _ := message["toolCallId"].(string)
		if call == "" {
			continue
		}
		key := p.CanonicalURI + "\x00" + p.Revision
		if p.ResourceComplete && p.SelectionComplete && p.Truncation == "" && p.TotalLines > 0 && len(p.ShownRanges) == 1 && len(p.FullyShownRanges) == 1 && p.ShownRanges[0] == p.FullyShownRanges[0] && p.ShownRanges[0].Start == 1 && p.ShownRanges[0].End == p.TotalLines {
			if _, exists := complete[key]; !exists {
				complete[key] = coverage{call: call, total: p.TotalLines}
			}
			continue
		}
		newer, ok := complete[key]
		if !ok || p.ResourceComplete || p.TotalLines != newer.total {
			continue
		}
		replacement := fmt.Sprintf("[Read source superseded by read %s: complete same-revision coverage of %s, revision %s. Original tool provenance retained.]", callText(newer.call), callText(p.CanonicalURI), callText(p.Revision))
		content, ok := replaceReadSource(message["content"], proof, replacement)
		if !ok {
			continue
		}
		if result == nil {
			result = slices.Clone(messages)
		}
		copy := maps.Clone(message)
		copy["content"] = content
		result[i] = copy
	}
	return result
}

func callText(value string) string { return noticeText(value, 140) }

func replaceReadSource(content any, proof readProjection, replacement string) (any, bool) {
	if proof.Bytes <= 0 || proof.Bytes > 50<<10 || len(proof.Digest) != 64 {
		return nil, false
	}
	replace := func(text string) (string, bool) {
		if len(text) < proof.Bytes {
			return "", false
		}
		digest := sha256.Sum256([]byte(text[:proof.Bytes]))
		if hex.EncodeToString(digest[:]) != proof.Digest {
			return "", false
		}
		return replacement + text[proof.Bytes:], true
	}
	if text, ok := content.(string); ok {
		return replace(text)
	}
	blocks, ok := content.([]any)
	if !ok || len(blocks) == 0 {
		return nil, false
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["type"] != "text" {
		return nil, false
	}
	text, ok := block["text"].(string)
	if !ok {
		return nil, false
	}
	changed, ok := replace(text)
	if !ok {
		return nil, false
	}
	copy := maps.Clone(block)
	copy["text"] = changed
	cloned := slices.Clone(blocks)
	cloned[0] = copy
	return cloned, true
}
