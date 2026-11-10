package hashline

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

// TextEdit replaces one uniquely occurring original-source byte sequence.
type TextEdit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// TextChange records an original half-open byte interval and replacement length.
type TextChange struct{ Start, End, ReplacementBytes int }

// ReplaceExact stages disjoint exact matches against the original UTF-8 bytes.
func ReplaceExact(source []byte, edits []TextEdit) ([]byte, []TextChange, error) {
	if !utf8.Valid(source) {
		return nil, nil, errors.New("exact edits require UTF-8 source")
	}
	if len(edits) == 0 {
		return nil, nil, errors.New("edits must contain at least one replacement")
	}
	type replacement struct {
		TextChange
		bytes []byte
	}
	replacements := make([]replacement, 0, len(edits))
	for index, edit := range edits {
		if edit.OldText == "" {
			return nil, nil, fmt.Errorf("edit %d: oldText cannot be empty; use write to create content", index+1)
		}
		if !utf8.ValidString(edit.OldText) || !utf8.ValidString(edit.NewText) {
			return nil, nil, fmt.Errorf("edit %d: replacement must be UTF-8", index+1)
		}
		old := []byte(edit.OldText)
		start := bytes.Index(source, old)
		if start < 0 {
			return nil, nil, fmt.Errorf("edit %d: oldText not found; read the current file and retry", index+1)
		}
		if bytes.Contains(source[start+1:], old) {
			return nil, nil, fmt.Errorf("edit %d: oldText is ambiguous; include more unchanged context", index+1)
		}
		replacements = append(replacements, replacement{TextChange{Start: start, End: start + len(old), ReplacementBytes: len(edit.NewText)}, []byte(edit.NewText)})
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].Start < replacements[j].Start })
	for i := 1; i < len(replacements); i++ {
		if replacements[i].Start < replacements[i-1].End {
			return nil, nil, errors.New("exact replacements overlap; combine changes into one original-source replacement")
		}
	}
	var output bytes.Buffer
	changes := make([]TextChange, 0, len(replacements))
	previous := 0
	for _, replacement := range replacements {
		output.Write(source[previous:replacement.Start])
		output.Write(replacement.bytes)
		previous = replacement.End
		changes = append(changes, replacement.TextChange)
	}
	output.Write(source[previous:])
	return output.Bytes(), changes, nil
}
