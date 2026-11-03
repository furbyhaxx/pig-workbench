package instructions

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var pointerPattern = regexp.MustCompile(`^- B-[0-9]{4,} [^[:space:]]+ [^[:space:]]+ — .+ → [^[:space:]].*$`)

type viewLine struct {
	text    string
	lines   int
	pointer bool
}

type cappedView struct {
	content   string
	truncated bool
	omitted   int
	fits      bool
}

func makeView(data []byte, complete bool, class Class, continuation string) cappedView {
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
	totalItems := 0
	for i, line := range sourceLines {
		plain := strings.TrimSuffix(line, "\n")
		pointer := itemClass && pointerPattern.MatchString(plain)
		if pointer {
			totalItems++
		}
		entry := viewLine{text: line, lines: 1, pointer: pointer}
		if itemClass && strings.HasPrefix(strings.TrimSpace(plain), "- B-") && !pointer {
			if !strings.HasSuffix(entry.text, "\n") {
				entry.text += "\n"
			}
			entry.text += fmt.Sprintf("[malformed backlog pointer at line %d]\n", i+1)
			entry.lines++
		}
		lines = append(lines, entry)
	}
	build := func(bytesLimit, linesLimit int) (string, int, bool) {
		var out strings.Builder
		usedLines, visibleItems := 0, 0
		omitted := !complete
		stopped := false
		for _, line := range lines {
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
		return out.String(), visibleItems, omitted
	}
	content, visible, truncated := build(class.MaxBytes, class.MaxLines)
	if !truncated {
		return cappedView{content: content, fits: true}
	}
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
			return cappedView{truncated: true, omitted: omitted, fits: false}
		}
		next, nextVisible, _ := build(class.MaxBytes-reserve, class.MaxLines-1)
		if next == content && nextVisible == visible {
			return cappedView{content: next, truncated: true, omitted: omitted, fits: true}
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
