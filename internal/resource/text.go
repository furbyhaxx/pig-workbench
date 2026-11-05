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
	ranges, err := selection.Resolve(len(lines), true)
	if err != nil {
		return Page{}, err
	}
	page, err := renderLines(ctx, canonical, requested, ranges, func(line int) (string, bool) {
		if line < 1 || line > len(lines) {
			return "", false
		}
		return lines[line-1], true
	}, selection.Raw, budget)
	if err != nil {
		return Page{}, err
	}
	page.TotalLines = len(lines)
	page.ResourceComplete = page.SelectionComplete && len(ranges) == 1 && ranges[0].Start == 1 && ranges[0].End == len(lines)
	return page, nil
}

// RenderSpans pages provider-supplied lines that cover exactly the given spans.
// totalLines is zero when the provider did not report a total.
func RenderSpans(ctx context.Context, canonical, requested string, spans []Range, lines []string, totalLines int, raw bool, budget Budget) (Page, error) {
	if budget.Bytes <= 0 || budget.Lines <= 0 {
		return Page{}, errors.New("read budget must be positive")
	}
	count := 0
	for i, span := range spans {
		if span.Start < 1 || span.End < span.Start || i > 0 && span.Start <= spans[i-1].End {
			return Page{}, errors.New("provider shown ranges must be ordered, disjoint and positive")
		}
		count += span.End - span.Start + 1
	}
	if count != len(lines) {
		return Page{}, fmt.Errorf("provider returned %d lines for ranges covering %d", len(lines), count)
	}
	index := make(map[int]string, len(lines))
	next := 0
	for _, span := range spans {
		for line := span.Start; line <= span.End; line++ {
			index[line] = lines[next]
			next++
		}
	}
	page, err := renderLines(ctx, canonical, requested, spans, func(line int) (string, bool) {
		content, ok := index[line]
		return content, ok
	}, raw, budget)
	if err != nil {
		return Page{}, err
	}
	page.TotalLines = totalLines
	return page, nil
}

func renderLines(ctx context.Context, canonical, requested string, ranges []Range, lineAt func(int) (string, bool), raw bool, budget Budget) (Page, error) {
	page := Page{RequestedURI: requested, CanonicalURI: canonical, CoordinateSpace: "source-lines"}
	header := ""
	if !raw {
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
		for line := span.Start; line <= span.End; line++ {
			if err := ctx.Err(); err != nil {
				return Page{}, err
			}
			prefix := ""
			if !raw {
				prefix = fmt.Sprintf("%d:", line)
			}
			content, ok := lineAt(line)
			if !ok {
				break
			}
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
	if next != 0 {
		page.Continuation = canonical + continuationSelection(ranges, next, raw)
	}
	content := header + body.String()
	if next != 0 {
		content += "\n… incomplete; continue with selector " + continuationSelection(ranges, next, raw)
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
