package hashline

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"
)

const bom = "\xef\xbb\xbf"

// LineRecord locates a source line in the LF view; End excludes its terminator.
type LineRecord struct {
	Number         int
	Start, End     int
	OriginalEnding string
}

// Normalized holds a BOM-free LF view and the source metadata needed to restore it.
// Warnings reports ties in the source line-ending counts, which default to LF.
type Normalized struct {
	Bytes    []byte
	Tag      string
	Lines    []LineRecord
	Warnings []string

	original       []byte
	originalLines  []LineRecord
	hasBOM         bool
	dominantEnding string
	ready          bool
}

// AmbiguousEndingError reports an output line with multiple valid original endings.
type AmbiguousEndingError struct {
	Line       int
	Candidates []string
}

func (e *AmbiguousEndingError) Error() string {
	return fmt.Sprintf("line %d has ambiguous original endings %q; supply an unambiguous edit", e.Line, e.Candidates)
}

// Normalize validates UTF-8 and records byte offsets and source line endings.
func Normalize(source []byte) (Normalized, error) {
	if !utf8.Valid(source) {
		return Normalized{}, errors.New("source is not valid UTF-8")
	}
	result := Normalized{hasBOM: bytes.HasPrefix(source, []byte(bom)), ready: true}
	if result.hasBOM {
		source = source[len(bom):]
	}
	result.Bytes = make([]byte, 0, len(source))
	lf, crlf := 0, 0
	for len(source) > 0 {
		lineEnd := bytes.IndexByte(source, '\n')
		ending := ""
		consume := len(source)
		if lineEnd >= 0 {
			consume = lineEnd + 1
			ending = "\n"
			lf++
			if lineEnd > 0 && source[lineEnd-1] == '\r' {
				lineEnd--
				ending = "\r\n"
				lf--
				crlf++
			}
		} else {
			lineEnd = len(source)
		}
		start := len(result.Bytes)
		result.Bytes = append(result.Bytes, source[:lineEnd]...)
		result.Lines = append(result.Lines, LineRecord{
			Number: len(result.Lines) + 1, Start: start, End: len(result.Bytes), OriginalEnding: ending,
		})
		if ending != "" {
			result.Bytes = append(result.Bytes, '\n')
		}
		source = source[consume:]
	}
	result.dominantEnding = "\n"
	if crlf > lf {
		result.dominantEnding = "\r\n"
	} else if crlf == lf && lf > 0 {
		result.Warnings = []string{"line ending counts are tied; new and changed lines use LF"}
	}
	result.Tag = Tag(result.Bytes)
	result.original = bytes.Clone(result.Bytes)
	result.originalLines = append([]LineRecord(nil), result.Lines...)
	return result, nil
}

type sourceLine struct {
	text       string
	ending     string
	outputLine int
}

func snapshotLines(snapshot Normalized) []sourceLine {
	lines := make([]sourceLine, len(snapshot.originalLines))
	for i, line := range snapshot.originalLines {
		lines[i] = sourceLine{text: string(snapshot.original[line.Start:line.End]), ending: line.OriginalEnding}
	}
	return lines
}

func viewLines(view []byte) []sourceLine {
	var lines []sourceLine
	for len(view) > 0 {
		end := bytes.IndexByte(view, '\n')
		ending := ""
		consume := len(view)
		if end < 0 {
			end = len(view)
		} else {
			consume = end + 1
			ending = "\n"
		}
		lines = append(lines, sourceLine{text: string(view[:end]), ending: ending, outputLine: len(lines) + 1})
		view = view[consume:]
	}
	return lines
}

// Restore applies the retained BOM and uniquely attributable endings to an edited LF view.
// Changed and inserted lines use the dominant source ending, or LF when tied or absent.
func Restore(normalized Normalized, edited []byte) ([]byte, error) {
	if !normalized.ready {
		return nil, errors.New("restore requires a snapshot returned by Normalize")
	}
	if !utf8.Valid(edited) {
		return nil, errors.New("edited view is not valid UTF-8")
	}
	original, lines := snapshotLines(normalized), viewLines(edited)
	endings := make([]string, len(lines))
	for i := range endings {
		endings[i] = normalized.dominantEnding
	}
	if bytes.Equal(normalized.original, edited) {
		for i := range lines {
			endings[i] = original[i].ending
		}
	} else if err := alignEndings(original, lines, normalized.dominantEnding, endings); err != nil {
		return nil, err
	}
	return restoreView(normalized, lines, endings)
}

// restoreView assembles the restored bytes from LF view lines and their resolved
// endings, applying the retained BOM.
func restoreView(normalized Normalized, lines []sourceLine, endings []string) ([]byte, error) {
	var output bytes.Buffer
	output.Grow(len(normalized.original) + len(bom))
	if normalized.hasBOM {
		output.WriteString(bom)
	}
	for i, line := range lines {
		output.WriteString(line.text)
		if line.ending != "" {
			if endings[i] == "\n" && len(line.text) > 0 && line.text[len(line.text)-1] == '\r' {
				return nil, fmt.Errorf("line %d cannot preserve a trailing CR with an LF ending", line.outputLine)
			}
			output.WriteString(endings[i])
		}
	}
	return output.Bytes(), nil
}
