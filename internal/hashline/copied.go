package hashline

import (
	"errors"
	"strconv"
	"strings"
)

// ReadCopy contains the source binding and normalized text of a copied read view.
type ReadCopy struct{ Path, Tag, Content string }

// UnwrapRead recognizes complete canonical tagged reads, never partial views.
// Callers restore source BOM/endings from the bound snapshot; raw writes bypass this.
func UnwrapRead(text string) (ReadCopy, bool, error) {
	literal := ReadCopy{Content: text}
	header, body, ok := strings.Cut(text, "\n")
	if !ok {
		return literal, false, nil
	}
	section, err := parseSection(header, 1)
	if err != nil {
		return literal, false, nil
	}
	result := ReadCopy{Path: section.Path, Tag: section.Tag}
	if body == "" {
		if Tag(nil) == section.Tag {
			return result, true, nil
		}
		return ReadCopy{}, true, errors.New("tagged write payload has no source rows; use complete raw content")
	}
	rows := strings.Split(body, "\n")
	if rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	content := make([]string, len(rows))
	for index, row := range rows {
		label, value, ok := strings.Cut(row, ":")
		if !ok || label != strconv.Itoa(index+1) {
			return ReadCopy{}, true, errors.New("tagged write payload is partial, summarized or malformed; use complete raw content (raw=true writes the supplied text literally)")
		}
		content[index] = value
	}
	without := strings.Join(content, "\n")
	with := without + "\n"
	first, second := Tag([]byte(without)) == section.Tag, Tag([]byte(with)) == section.Tag
	if first == second {
		return ReadCopy{}, true, errors.New("tagged write payload does not identify a complete unambiguous source; remove presentation prefixes explicitly")
	}
	result.Content = with
	if first {
		result.Content = without
	}
	return result, true, nil
}
