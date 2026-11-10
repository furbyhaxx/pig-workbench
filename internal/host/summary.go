package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
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
	folding := syntax.Folding{BodyLines: intValue(configuration, "read.summarize.min-body-lines", syntax.DefaultFolding.BodyLines), CommentLines: intValue(configuration, "read.summarize.min-comment-lines", syntax.DefaultFolding.CommentLines)}
	summary, _, err := a.syntax.Analyze(ctx, info.ModTime().UTC().Format("20060102T150405.000000000"), target.Path, source, folding)
	if err != nil {
		if errors.Is(err, syntax.ErrNoGrammar) || errors.Is(err, syntax.ErrTooLarge) {
			return resource.ReadResult{}, false, nil
		}
		if errors.Is(err, syntax.ErrUnparseable) {
			return resource.ReadResult{}, false, nil
		}
		return resource.ReadResult{}, false, err
	}
	if len(summary.Declarations) == 0 || len(summary.Omitted) == 0 {
		return resource.ReadResult{}, false, nil
	}
	if summary.Language == "markdown" && !enabled(configuration, "read.summarize.prose", false) {
		return resource.ReadResult{}, false, nil
	}
	rendered, shown := renderSummary(string(source), summary.Omitted)
	if rendered == "" {
		return resource.ReadResult{}, false, nil
	}
	if len(rendered) > 51200 {
		return resource.ReadResult{}, false, nil
	}
	normalized, err := hashline.Normalize(source)
	if err != nil {
		return resource.ReadResult{}, false, err
	}
	facts := resource.Page{
		Revision:          hashline.SourceRevision(source),
		Tag:               normalized.Tag,
		RequestedURI:      target.Requested,
		CanonicalURI:      resource.FileURI(target.Path),
		CoordinateSpace:   "source-lines",
		TotalLines:        lines,
		ShownRanges:       shown,
		FullyShownRanges:  shown,
		SelectionComplete: false,
		ResourceComplete:  false,
		Continuation:      resource.FileURI(target.Path) + ":1-" + fmt.Sprint(lines),
		Truncation:        "summary",
	}
	content := hashline.Header(resource.FileURI(target.Path), normalized.Tag) + "\nSummary of " + resource.FileURI(target.Path) + fmt.Sprintf(" (%s, %d declarations, %d lines).\n", summary.Language, len(summary.Declarations), lines)
	content += fmt.Sprintf("Declaration bodies of %d or more lines and comment blocks of %d or more are folded. This view never marks folded lines seen.\n", folding.BodyLines, folding.CommentLines)
	content += "Read the full text with selector :1-" + fmt.Sprint(lines) + " or use a line range.\n\n"
	content += rendered
	facts.OmittedRanges = summary.Omitted
	return resource.ReadResult{Content: content, Facts: facts}, true, nil
}

// renderSummary prints every line outside the folded ranges at its source coordinate and returns the shown spans.
func renderSummary(source string, omitted [][2]int) (string, []resource.Range) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var out strings.Builder
	shown := []resource.Range{}
	next := 1
	show := func(until int) {
		if until >= next {
			shown = append(shown, resource.Range{Start: next, End: until})
		}
		for ; next <= until; next++ {
			fmt.Fprintf(&out, "%d:%s\n", next, lines[next-1])
		}
	}
	for _, fold := range omitted {
		if fold[0] < next || fold[1] > len(lines) || fold[1] < fold[0] {
			continue
		}
		show(fold[0] - 1)
		fmt.Fprintf(&out, "⋯ lines %d-%d omitted (%d lines)\n", fold[0], fold[1], fold[1]-fold[0]+1)
		next = fold[1] + 1
	}
	show(len(lines))
	return out.String(), shown
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
