package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/syntax"
)

const summarySourceCeiling = 4 << 20

// summaryView renders a declaration skeleton for a bare read of a large code
// file. It reports false when the request is not eligible, so the caller falls
// back to exact text without changing the requested selection.
func (a *app) summaryView(ctx context.Context, target resource.Target, configuration *config.Snapshot) (resource.ReadResult, bool, error) {
	if target.Scheme != "file" {
		return resource.ReadResult{}, false, nil
	}
	if target.Selection.Explicit {
		return resource.ReadResult{}, false, nil
	}
	if !enabled(configuration, "read.summarize.enabled", true) {
		return resource.ReadResult{}, false, nil
	}
	minimum := intValue(configuration, "read.summarize.min-total-lines", 200)
	file, info, err := fileio.OpenRegular(target.Path)
	if err != nil {
		return resource.ReadResult{}, false, err
	}
	defer file.Close()
	if info.Size() > summarySourceCeiling {
		return resource.ReadResult{}, false, nil
	}
	source, err := io.ReadAll(io.LimitReader(file, summarySourceCeiling+1))
	if err != nil {
		return resource.ReadResult{}, false, err
	}
	if int64(len(source)) > summarySourceCeiling {
		return resource.ReadResult{}, false, nil
	}
	lines := strings.Count(string(source), "\n")
	if !strings.HasSuffix(string(source), "\n") && len(source) > 0 {
		lines++
	}
	if lines < minimum {
		return resource.ReadResult{}, false, nil
	}
	summary, _, err := a.syntax.Analyze(ctx, info.ModTime().UTC().Format("20060102T150405.000000000"), target.Path, source)
	if err != nil {
		if errors.Is(err, syntax.ErrNoGrammar) || errors.Is(err, syntax.ErrTooLarge) {
			return resource.ReadResult{}, false, nil
		}
		if errors.Is(err, syntax.ErrUnparseable) {
			return resource.ReadResult{}, false, nil
		}
		return resource.ReadResult{}, false, err
	}
	if len(summary.Declarations) == 0 {
		return resource.ReadResult{}, false, nil
	}
	rendered, omitted := renderSummary(string(source), summary)
	if rendered == "" {
		return resource.ReadResult{}, false, nil
	}
	if len(rendered) > 51200 {
		return resource.ReadResult{}, false, nil
	}
	facts := resource.Page{
		RequestedURI:      target.Requested,
		CanonicalURI:      resource.FileURI(target.Path),
		CoordinateSpace:   "source-lines",
		TotalLines:        lines,
		ShownRanges:       declarationSpans(summary, lines),
		FullyShownRanges:  declarationSpans(summary, lines),
		SelectionComplete: false,
		ResourceComplete:  false,
		Continuation:      resource.FileURI(target.Path) + ":1-" + fmt.Sprint(lines),
		Truncation:        "summary",
	}
	facts.ShownRanges = mergeSpans(facts.ShownRanges)
	facts.FullyShownRanges = mergeSpans(facts.FullyShownRanges)
	content := "Summary of " + resource.FileURI(target.Path) + fmt.Sprintf(" (%s, %d declarations, %d lines).\n", summary.Language, len(summary.Declarations), lines)
	content += "Only declaration lines are shown; bodies are folded. This view never marks folded lines seen.\n"
	content += "Read the full text with selector :1-" + fmt.Sprint(lines) + " or use a line range.\n\n"
	content += rendered
	if len(omitted) > 0 {
		facts.OmittedRanges = omitted
	}
	return resource.ReadResult{Content: content, Facts: facts}, true, nil
}

func renderSummary(source string, summary syntax.Summary) (string, [][2]int) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	var out strings.Builder
	var omitted [][2]int
	previous := 0
	for _, declaration := range summary.Declarations {
		line := declaration.Line
		if line < 1 || line > len(lines) {
			continue
		}
		if line-1 > previous {
			omitted = append(omitted, [2]int{previous + 1, line - 1})
			fmt.Fprintf(&out, "⋯ lines %d-%d omitted (%d lines)\n", previous+1, line-1, line-previous-1)
		}
		fmt.Fprintf(&out, "%d:%s\n", line, strings.TrimRight(lines[line-1], "\r"))
		previous = line
	}
	if previous < len(lines) {
		omitted = append(omitted, [2]int{previous + 1, len(lines)})
		fmt.Fprintf(&out, "⋯ lines %d-%d omitted (%d lines)\n", previous+1, len(lines), len(lines)-previous)
	}
	return out.String(), omitted
}

func declarationSpans(summary syntax.Summary, total int) []resource.Range {
	spans := make([]resource.Range, 0, len(summary.Declarations))
	for _, declaration := range summary.Declarations {
		if declaration.Line >= 1 && declaration.Line <= total {
			spans = append(spans, resource.Range{Start: declaration.Line, End: declaration.Line})
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	return spans
}

func mergeSpans(spans []resource.Range) []resource.Range {
	if len(spans) == 0 {
		return []resource.Range{}
	}
	merged := []resource.Range{spans[0]}
	for _, span := range spans[1:] {
		last := &merged[len(merged)-1]
		if span.Start <= last.End+1 {
			if span.End > last.End {
				last.End = span.End
			}
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func enabled(snapshot *config.Snapshot, key string, fallback bool) bool {
	if snapshot == nil {
		return fallback
	}
	entry, ok := snapshot.Get(key)
	if !ok {
		return fallback
	}
	value, ok := entry.Value.(bool)
	return ok && value
}

func intValue(snapshot *config.Snapshot, key string, fallback int) int {
	if snapshot == nil {
		return fallback
	}
	entry, ok := snapshot.Get(key)
	if !ok {
		return fallback
	}
	value, ok := entry.Value.(int64)
	if !ok {
		return fallback
	}
	return int(value)
}
