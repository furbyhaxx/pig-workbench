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

func renderPresentationCall(entry presentationRegistration, args map[string]any, render sdk.ToolRenderContext, root string, width int, shorten bool) []string {
	parts := []string{entry.descriptor.Label}
	if entry.builtin != nil {
		parts = append(parts, entry.builtin.facts(args, root, width)...)
	} else if target, ok := firstArgumentField(args, entry.descriptor.CallFields, root, width, shorten); ok {
		parts = append(parts, target)
	} else if len(args) == 0 {
		parts = append(parts, "arguments not retained")
	}
	if len(args) > 0 && !render.ArgsComplete {
		parts = append(parts, "partial arguments")
	}
	return fitPresentationLines([]string{strings.Join(parts, " · ")}, width, 1)
}

func rawPresentationResult(descriptor presentation.Descriptor, result sdk.ToolRenderResult, width int, hostIsError bool, diagnostic string) []string {
	status := "result"
	if hostIsError {
		status = "failed (host)"
	}
	lines := []string{descriptor.Label + " · " + status}
	if diagnostic != "" {
		lines = append(lines, diagnostic)
	}
	lines = append(lines, contentPreview(result.Content, 6)...)
	return fitPresentationLines(lines, width, 40)
}

const expandedBodyLines = 80

// builtinPendingResult renders an own tool's result that has no snapshot yet: output still streaming, or a call the host failed before the tool returned one.
func builtinPendingResult(descriptor presentation.Descriptor, builtin *builtinPresentation, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int, preview int) []string {
	status := string(presentation.OutcomeSucceeded)
	switch {
	case render.IsError:
		status = string(presentation.OutcomeFailed)
		preview = max(preview, 6)
	case options.IsPartial || render.IsPartial:
		status = string(presentation.PhaseRunning)
	}
	if options.Expanded {
		preview = expandedBodyLines
	}
	parts := []string{descriptor.Label, status}
	if render.DurationMs != nil {
		parts = append(parts, "time "+formatMetric(presentation.Metric{Value: float64(*render.DurationMs), Unit: presentation.UnitMilliseconds}))
	}
	lines := append([]string{strings.Join(parts, " · ")}, previewBody(resultLines(result.Content), builtin.body, preview)...)
	return fitPresentationLines(lines, width, preview+4)
}

func renderPresentationResult(descriptor presentation.Descriptor, builtin *builtinPresentation, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int, display effectiveDisplay, record presentationRecord, root string) []string {
	if !record.valid {
		if builtin != nil {
			return builtinPendingResult(descriptor, builtin, result, options, render, width, descriptor.DefaultDisplay.PreviewLines)
		}
		diagnostic := ""
		if result.Details != nil {
			diagnostic = "presentation unavailable; showing raw result"
		}
		return rawPresentationResult(descriptor, result, width, render.IsError, diagnostic)
	}
	snapshot := record.snapshot
	metrics := make([]presentation.Metric, 0, len(snapshot.Metrics))
	for _, metric := range snapshot.Metrics {
		if display.metrics[metric.ID] {
			metrics = append(metrics, metric)
		}
	}
	snapshot.Metrics = metrics
	mode := bodyHead
	if builtin != nil {
		// The call row above an own tool's result already names the target.
		snapshot.Target = nil
		mode = builtin.body
	}
	lines := []string{presentationHeader(descriptor, snapshot, render.IsError, width, root, render.DurationMs, display.shorten)}
	preview := descriptor.DefaultDisplay.PreviewLines
	switch descriptor.DefaultDisplay.Density {
	case "compact":
		preview = 0
	case "full":
		if preview > 0 {
			preview = max(preview, 40)
		}
	}
	if builtin != nil && render.IsError {
		preview = max(preview, 6)
	}
	if options.Expanded {
		preview = max(preview, expandedBodyLines)
	}
	if snapshot.Summary != "" && preview > 0 {
		lines = append(lines, snapshot.Summary)
	}
	if preview > 0 {
		var body []string
		if builtin != nil && builtinShowsContent(snapshot.Blocks, options.Expanded) {
			body = resultLines(result.Content)
		} else {
			// An own tool's code block is an output tail, so its preview keeps the end.
			mode = bodyHead
			if builtin != nil && onlyCodeBlocks(snapshot.Blocks) {
				mode = bodyTail
			}
			for _, block := range snapshot.Blocks {
				body = append(body, presentationBlockLines(block)...)
			}
		}
		lines = append(lines, previewBody(body, mode, preview)...)
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
	if record.diagnostic != "" {
		lines = append(lines, record.diagnostic)
	}
	if options.IsPartial || render.IsPartial {
		lines = append(lines, "partial output")
	}
	return fitPresentationLines(lines, width, preview+8)
}

// builtinShowsContent reports whether an own tool's body is its text result: when the snapshot has no blocks, and when an expanded view would otherwise show only the bounded output tail.
func builtinShowsContent(blocks []presentation.Block, expanded bool) bool {
	return len(blocks) == 0 || expanded && onlyCodeBlocks(blocks)
}

func onlyCodeBlocks(blocks []presentation.Block) bool {
	for _, block := range blocks {
		if block.Kind != presentation.BlockCode {
			return false
		}
	}
	return len(blocks) > 0
}

// previewBody keeps limit lines from the head or the tail and says how many it left out.
func previewBody(body []string, mode bodyMode, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(body) <= limit {
		return body
	}
	hidden := len(body) - limit
	if mode == bodyTail {
		return append([]string{fmt.Sprintf("… %d earlier lines", hidden)}, body[hidden:]...)
	}
	return append(append([]string(nil), body[:limit]...), fmt.Sprintf("… %d more presentation lines", hidden))
}

func resultLines(blocks []map[string]any) []string {
	var out strings.Builder
	for _, block := range blocks {
		if block["type"] != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			out.WriteString(text)
		}
	}
	text := strings.TrimRight(out.String(), "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return splitPresentationText(text)
}

func presentationHeader(descriptor presentation.Descriptor, snapshot presentation.Snapshot, hostIsError bool, width int, root string, hostDuration *int64, shorten bool) string {
	title := snapshot.Title
	if title == "" {
		title = descriptor.Label
	}
	parts := []string{title}
	if snapshot.Target != nil && snapshot.Target.Value != "" {
		parts = append(parts, pathdisplay.Display(pathdisplay.Target{Value: snapshot.Target.Value, Root: root, NoFish: !shorten}, width/2))
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
		if hostDuration != nil && metric.ID == "durationMs" && metric.Unit == presentation.UnitMilliseconds {
			metric.Value = float64(*hostDuration)
		}
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
			lines := splitPresentationText(block.Code.Text)
			if block.Code.Language != "" {
				lines = append([]string{"[" + block.Code.Language + "]"}, lines...)
			}
			return lines
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

func firstArgumentField(args map[string]any, fields []presentation.CallField, root string, width int, shorten bool) (string, bool) {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		value, ok := argumentField(args, field.Pointer)
		if !ok || value == "" {
			continue
		}
		if field.Format == "path" {
			value = pathdisplay.Display(pathdisplay.Target{Value: value, Root: root, NoFish: !shorten}, max(8, width/2))
		}
		values = append(values, strings.TrimSpace(field.Label+" "+value))
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
