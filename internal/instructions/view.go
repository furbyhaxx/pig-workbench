package instructions

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var pointerPattern = regexp.MustCompile(`^- B-[0-9]{4,} [^[:space:]]+ [^[:space:]]+ — [^[:space:]].* → [^[:space:]].*$`)

type viewLine struct {
	text    string
	lines   int
	pointer bool
}

type viewDiagnostic struct {
	status string
	line   int
}

type cappedView struct {
	content     string
	truncated   bool
	omitted     int
	fits        bool
	diagnostics []viewDiagnostic
}

func makeView(ctx context.Context, data []byte, complete bool, class Class, continuation string, validatePointers bool) (cappedView, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	sourceLines := strings.SplitAfter(text, "\n")
	if len(sourceLines) > 0 && sourceLines[len(sourceLines)-1] == "" {
		sourceLines = sourceLines[:len(sourceLines)-1]
	}
	if !complete && len(sourceLines) > 0 && !strings.HasSuffix(sourceLines[len(sourceLines)-1], "\n") {
		sourceLines = sourceLines[:len(sourceLines)-1]
	}
	itemClass := class.ID == "backlog" || class.MaxItems > 0
	lines := make([]viewLine, 0, len(sourceLines))
	var diagnostics []viewDiagnostic
	totalItems := 0
	for i, line := range sourceLines {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return cappedView{}, err
			}
		}
		plain := strings.TrimSuffix(line, "\n")
		pointer := itemClass && pointerPattern.MatchString(plain)
		if pointer {
			totalItems++
			if validatePointers {
				diagnostics = append(diagnostics, viewDiagnostic{status: "pointer-validation-deferred", line: i + 1})
			}
		}
		entry := viewLine{text: line, lines: 1, pointer: pointer}
		if itemClass && strings.HasPrefix(strings.TrimSpace(plain), "- B-") && !pointer {
			diagnostics = append(diagnostics, viewDiagnostic{status: "malformed-pointer", line: i + 1})
			if !strings.HasSuffix(entry.text, "\n") {
				entry.text += "\n"
			}
			entry.text += fmt.Sprintf("[malformed backlog pointer at line %d]\n", i+1)
			entry.lines++
		}
		lines = append(lines, entry)
	}
	build := func(bytesLimit, linesLimit int) (string, int, bool, error) {
		var out strings.Builder
		usedLines, visibleItems := 0, 0
		omitted := !complete
		stopped := false
		for i, line := range lines {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return "", 0, false, err
				}
			}
			if line.pointer && class.MaxItems > 0 && visibleItems >= class.MaxItems {
				omitted = true
				continue
			}
			if stopped || usedLines+line.lines > linesLimit || out.Len()+len(line.text) > bytesLimit {
				omitted = true
				stopped = true
				continue
			}
			out.WriteString(line.text)
			usedLines += line.lines
			if line.pointer {
				visibleItems++
			}
		}
		return out.String(), visibleItems, omitted, nil
	}
	content, visible, truncated, err := build(class.MaxBytes, class.MaxLines)
	if err != nil {
		return cappedView{}, err
	}
	if !truncated {
		return cappedView{content: content, fits: true, diagnostics: diagnostics}, nil
	}
	bodyLimit := class.MaxBytes
	for {
		omitted := 0
		if itemClass {
			if complete {
				omitted = totalItems - visible
			} else {
				omitted = -1
			}
		}
		marker := continuationMarker(omitted, itemClass, continuation)
		// A final unterminated source line needs its own separator before the footer.
		reserve := len(marker) + 2
		if reserve > class.MaxBytes || class.MaxLines < 1 {
			return cappedView{truncated: true, omitted: omitted, fits: false, diagnostics: diagnostics}, nil
		}
		bodyLimit = min(bodyLimit, class.MaxBytes-reserve)
		next, nextVisible, _, err := build(bodyLimit, class.MaxLines-1)
		if err != nil {
			return cappedView{}, err
		}
		if next == content && nextVisible == visible {
			return cappedView{content: next, truncated: true, omitted: omitted, fits: true, diagnostics: diagnostics}, nil
		}
		content, visible = next, nextVisible
	}
}

func continuationMarker(omitted int, itemClass bool, path string) string {
	path = safePath(path)
	switch {
	case itemClass && omitted < 0:
		return "(more items; count unavailable → " + path + ")"
	case itemClass && omitted > 0:
		return fmt.Sprintf("(+%d more → %s)", omitted, path)
	default:
		return "(more content → " + path + ")"
	}
}

func safePath(path string) string {
	for _, r := range path {
		if unicode.IsControl(r) {
			return strconv.Quote(path)
		}
	}
	return path
}

// Render formats each delivered view using its authorization-relative display path.
func (s Set) Render() string {
	var out strings.Builder
	for _, file := range s.Files {
		fmt.Fprintf(&out, "## %s [%s, %s]\n", safePath(file.DisplayPath), file.Trust, file.Role)
		out.WriteString(file.Content)
		if file.Content != "" && !strings.HasSuffix(file.Content, "\n") {
			out.WriteByte('\n')
		}
		if file.Truncated {
			out.WriteString(continuationMarker(file.OmittedItems, file.Class == "backlog" || file.OmittedItems != 0, file.Continuation))
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	return out.String()
}
