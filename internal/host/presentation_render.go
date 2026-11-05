package host

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/pathdisplay"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

func renderPresentationCall(descriptor presentation.Descriptor, args map[string]any, width int) []string {
	parts := []string{descriptor.Label}
	if target, ok := firstArgumentField(args, descriptor.CallFields); ok {
		parts = append(parts, pathdisplay.Display(pathdisplay.Target{Value: target}, width/2))
	}
	return fitPresentationLines([]string{strings.Join(parts, " · ")}, width, 1)
}

func renderPresentationResult(descriptor presentation.Descriptor, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) []string {
	snapshot, err := decodeToolSnapshot(descriptor, result.Details)
	if err != nil {
		lines := []string{descriptor.Label + " · result"}
		if result.Details != nil {
			lines = append(lines, "presentation unavailable; showing raw result")
		}
		lines = append(lines, contentPreview(result.Content, 6)...)
		return fitPresentationLines(lines, width, 40)
	}
	lines := []string{presentationHeader(descriptor, snapshot, render.IsError, width)}
	if snapshot.Summary != "" {
		lines = append(lines, snapshot.Summary)
	}
	preview := descriptor.DefaultDisplay.PreviewLines
	if preview <= 0 {
		preview = 8
	}
	switch descriptor.DefaultDisplay.Density {
	case "compact":
		preview = min(preview, 3)
	case "full":
		preview = max(preview, 40)
	}
	if options.Expanded {
		preview = max(preview, 80)
	}
	for _, block := range snapshot.Blocks {
		if len(lines) >= preview+2 {
			lines = append(lines, "… more presentation blocks")
			break
		}
		lines = append(lines, presentationBlockLines(block)...)
	}
	if snapshot.Continuation != nil {
		continuation := "continue: " + snapshot.Continuation.Explanation
		if snapshot.Continuation.URI != "" {
			continuation += " (" + snapshot.Continuation.URI + ")"
		}
		lines = append(lines, continuation)
	}
	if snapshot.Error != nil {
		lines = append(lines, snapshot.Error.Code+": "+snapshot.Error.Message)
	}
	if options.IsPartial || render.IsPartial {
		lines = append(lines, "partial output")
	}
	return fitPresentationLines(lines, width, preview+8)
}

func presentationHeader(descriptor presentation.Descriptor, snapshot presentation.Snapshot, hostIsError bool, width int) string {
	title := snapshot.Title
	if title == "" {
		title = descriptor.Label
	}
	parts := []string{title}
	if snapshot.Target != nil && snapshot.Target.Value != "" {
		parts = append(parts, pathdisplay.Display(pathdisplay.Target{Value: snapshot.Target.Value}, width/2))
	}
	status := string(snapshot.Phase)
	if snapshot.Outcome != nil {
		status = string(*snapshot.Outcome)
	}
	if hostIsError {
		switch status {
		case "failed", "cancelled", "timedOut", "denied", "unknown":
		default:
			status = "failed (host)"
		}
	}
	parts = append(parts, status)
	if snapshot.Complete != nil && !*snapshot.Complete {
		parts = append(parts, "incomplete")
	}
	if snapshot.Effects != nil && *snapshot.Effects == presentation.EffectsPartial {
		parts = append(parts, "partial effects")
	}
	definitions := make(map[string]presentation.MetricDefinition, len(descriptor.MetricDefinitions))
	for _, definition := range descriptor.MetricDefinitions {
		definitions[definition.ID] = definition
	}
	for _, metric := range snapshot.Metrics {
		label := metric.ID
		if definition, ok := definitions[metric.ID]; ok {
			label = definition.Label
		}
		parts = append(parts, label+" "+formatMetric(metric))
	}
	return strings.Join(parts, " · ")
}

func formatMetric(metric presentation.Metric) string {
	value := strconv.FormatFloat(metric.Value, 'f', -1, 64)
	switch metric.Unit {
	case presentation.UnitPercent:
		return value + "%"
	case presentation.UnitMilliseconds:
		if metric.Value >= 1000 {
			return strconv.FormatFloat(metric.Value/1000, 'f', 2, 64) + "s"
		}
		return value + "ms"
	case presentation.UnitBytes:
		return value + "B"
	case presentation.UnitLines:
		return value + "L"
	case presentation.UnitCount:
		return value
	default:
		return value + " " + string(metric.Unit)
	}
}

func presentationBlockLines(block presentation.Block) []string {
	switch block.Kind {
	case presentation.BlockText:
		if block.Text != nil {
			return prefixedRoleLines(block.Text.Text, block.Text.Role)
		}
	case presentation.BlockCode:
		if block.Code != nil {
			prefix := "code"
			if block.Code.Language != "" {
				prefix = block.Code.Language
			}
			lines := []string{"[" + prefix + "]"}
			return append(lines, splitPresentationText(block.Code.Text)...)
		}
	case presentation.BlockNotice:
		if block.Notice != nil {
			return splitPresentationText("[" + string(block.Notice.Severity) + "] " + block.Notice.Text)
		}
	case presentation.BlockDiff:
		if block.Diff != nil {
			return diffPresentationLines(block.Diff)
		}
	case presentation.BlockTable:
		if block.Table != nil {
			return tablePresentationLines(block.Table)
		}
	case presentation.BlockImageRef:
		if block.Image != nil {
			target := block.Image.Resource
			if target == "" {
				target = block.Image.BlockID
			}
			label := "image: " + target
			if block.Image.Alt != "" {
				label += " (" + block.Image.Alt + ")"
			}
			return []string{label}
		}
	case presentation.BlockChildren:
		if block.Children != nil {
			return []string{fmt.Sprintf("children: %d (%d retained)", len(block.Children.ChildCallIDs), len(block.Children.Retained))}
		}
	}
	return []string{"[invalid presentation block]"}
}

func prefixedRoleLines(text string, role presentation.TextRole) []string {
	lines := splitPresentationText(text)
	if role == "" || role == presentation.TextRolePlain {
		return lines
	}
	for i := range lines {
		lines[i] = "[" + string(role) + "] " + lines[i]
	}
	return lines
}

func diffPresentationLines(diff *presentation.DiffBlock) []string {
	lines := []string{"diff " + string(diff.Operation)}
	before, after := "", ""
	if diff.Before != nil {
		before = diff.Before.Path
	}
	if diff.After != nil {
		after = diff.After.Path
	}
	switch {
	case before != "" && after != "" && before != after:
		lines = append(lines, before+" → "+after)
	case before != "":
		lines = append(lines, before)
	case after != "":
		lines = append(lines, after)
	}
	shown := 0
	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			prefix := " "
			switch line.Kind {
			case presentation.HunkAdd:
				prefix = "+"
			case presentation.HunkRemove:
				prefix = "-"
			}
			lines = append(lines, prefix+line.Text)
			shown++
			if shown >= 20 {
				lines = append(lines, "… diff truncated")
				return lines
			}
		}
	}
	return lines
}

func tablePresentationLines(table *presentation.TableBlock) []string {
	headers := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		headers[i] = column.Label
	}
	lines := []string{strings.Join(headers, " | ")}
	for _, row := range table.Rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			switch cell.Kind {
			case presentation.CellCount, presentation.CellBytes, presentation.CellDuration:
				cells[i] = strconv.FormatFloat(cell.Number, 'f', -1, 64)
			default:
				cells[i] = cell.Text
			}
		}
		lines = append(lines, strings.Join(cells, " | "))
		if len(lines) > 21 {
			lines = append(lines, "… table truncated")
			break
		}
	}
	return lines
}

func firstArgumentField(args map[string]any, fields []presentation.CallField) (string, bool) {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		value, ok := argumentField(args, field.Pointer)
		if !ok || value == "" {
			continue
		}
		values = append(values, field.Label+" "+value)
		if len(values) == 3 {
			break
		}
	}
	return strings.Join(values, " · "), len(values) > 0
}

func contentPreview(blocks []map[string]any, limit int) []string {
	var lines []string
	for _, block := range blocks {
		if block["type"] != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			lines = append(lines, splitPresentationText(text)...)
		}
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) > limit {
		lines = append(lines[:limit], "…")
	}
	return lines
}

func splitPresentationText(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func fitPresentationLines(lines []string, width, limit int) []string {
	if width <= 0 {
		width = 80
	}
	if limit > 0 && len(lines) > limit {
		lines = append(lines[:limit], "…")
	}
	fitted := make([]string, len(lines))
	for i, line := range lines {
		cleaned := tui.StripTerminalSequences(line)
		cleaned = strings.Map(func(r rune) rune {
			if r == '\t' {
				return ' '
			}
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, cleaned)
		fitted[i] = tui.StripTerminalSequences(tui.TruncateToWidth(cleaned, width, "…", false))
	}
	return fitted
}
