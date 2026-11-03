package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// RenderText pages already-loaded text with the shared selector and budget rules.
// It is for providers whose content is produced rather than read from a file.
func RenderText(ctx context.Context, canonical, requested string, data []byte, selection Selection, budget Budget) (Page, error) {
	if budget.Bytes <= 0 || budget.Lines <= 0 {
		return Page{}, errors.New("read budget must be positive")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	page := Page{RequestedURI: requested, CanonicalURI: canonical, CoordinateSpace: "source-lines", TotalLines: len(lines)}
	ranges, err := selection.Resolve(len(lines), true)
	if err != nil {
		return Page{}, err
	}
	header := ""
	if !selection.Raw {
		header = "[" + displayText(canonical) + "]\n"
	}
	const reserve = 256
	available := budget.Bytes - len(header) - reserve
	lineBudget := budget.Lines - 3
	if available < 1 || lineBudget < 1 {
		return Page{}, errors.New("read budget too small for resource identity and continuation")
	}
	var body strings.Builder
	next := 0
	for _, span := range ranges {
		for line := span.Start; line <= span.End && line <= len(lines); line++ {
			if err := ctx.Err(); err != nil {
				return Page{}, err
			}
			prefix := ""
			if !selection.Raw {
				prefix = fmt.Sprintf("%d:", line)
			}
			content := lines[line-1]
			room := available - body.Len() - len(prefix) - 1
			if lineBudget <= 0 || room <= 0 {
				next = line
				page.Truncation = "budget"
				break
			}
			whole := true
			if len(content) > room {
				cut := room
				for cut > 0 && !utf8ValidPrefix(content[:cut]) {
					cut--
				}
				content = content[:cut]
				whole = false
			}
			body.WriteString(prefix)
			body.WriteString(content)
			body.WriteByte('\n')
			lineBudget--
			appendSpan(&page.ShownRanges, line)
			if whole {
				appendSpan(&page.FullyShownRanges, line)
			} else if next == 0 {
				next = line
				page.Truncation = "partialLine"
			}
		}
		if next != 0 {
			break
		}
	}
	selected := 0
	for _, span := range ranges {
		selected += span.End - span.Start + 1
	}
	shown := 0
	for _, span := range page.FullyShownRanges {
		shown += span.End - span.Start + 1
	}
	page.SelectionComplete = shown == selected
	page.ResourceComplete = page.SelectionComplete && len(ranges) == 1 && ranges[0].Start == 1 && ranges[0].End == len(lines)
	if next != 0 {
		page.Continuation = canonical + continuationSelection(ranges, next, selection.Raw)
	}
	content := header + body.String()
	if next != 0 {
		content += "\n… incomplete; continue with selector " + continuationSelection(ranges, next, selection.Raw)
	}
	if len(content) > budget.Bytes {
		return Page{}, errors.New("internal read framing exceeded the byte budget")
	}
	page.Content = content
	return page, nil
}

func utf8Valid(s string) bool { return utf8.ValidString(s) }

func utf8ValidPrefix(candidate string) bool {
	for len(candidate) > 0 {
		if r := candidate[len(candidate)-1]; r < 0x80 {
			return true
		}
		for cut := len(candidate) - 1; cut >= 0 && cut >= len(candidate)-4; cut-- {
			if candidate[cut]&0xC0 != 0x80 {
				return utf8Valid(candidate[cut:])
			}
		}
		return false
	}
	return true
}
