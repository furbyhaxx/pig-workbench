package host

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/ask"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

const presentationDetailsKey = "workbenchPresentation"

// presented attaches the SPEC-0003 snapshot of an own tool's result to its Details; the model-facing content is untouched.
func presented(tool string, execute func(sdk.Context, map[string]any) (any, error)) func(sdk.Context, map[string]any) (any, error) {
	return func(ctx sdk.Context, params map[string]any) (any, error) {
		started := time.Now()
		out, err := execute(ctx, params)
		result, ok := out.(sdk.ToolResult)
		if err != nil || !ok {
			return out, err
		}
		details := detailsObject(result.Details)
		details[presentationDetailsKey] = builtinSnapshot(tool, params, result, details, time.Since(started))
		result.Details = details
		return result, nil
	}
}

// detailsObject returns Details as a JSON object so the snapshot can sit beside the tool's own facts.
func detailsObject(details any) map[string]any {
	switch value := details.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return value
	}
	var object map[string]any
	if encoded, err := json.Marshal(details); err == nil && json.Unmarshal(encoded, &object) == nil && object != nil {
		return object
	}
	return map[string]any{"result": details}
}

func builtinSnapshot(tool string, params map[string]any, result sdk.ToolResult, details map[string]any, elapsed time.Duration) presentation.Snapshot {
	outcome := presentation.OutcomeSucceeded
	if result.IsError {
		outcome = presentation.OutcomeFailed
	}
	snapshot := presentation.Snapshot{Version: presentation.Version, Revision: 1, Phase: presentation.PhaseSettled, Outcome: &outcome}
	if complete, ok := details["complete"].(bool); ok {
		snapshot.Complete = &complete
	}
	if blocks, ok := details[presentationBlocksKey].([]presentation.Block); ok {
		snapshot.Blocks = blocks
	}
	delete(details, presentationBlocksKey)
	if fill := builtinSnapshots[tool]; fill != nil {
		fill(&snapshot, params, result, details)
	}
	snapshot.Metrics = append(snapshot.Metrics, presentation.Metric{ID: "durationMs", Value: float64(elapsed.Milliseconds()), Unit: presentation.UnitMilliseconds})
	return snapshot
}

type snapshotFill func(snapshot *presentation.Snapshot, params map[string]any, result sdk.ToolResult, details map[string]any)

var builtinSnapshots = map[string]snapshotFill{
	"read": func(s *presentation.Snapshot, params map[string]any, result sdk.ToolResult, details map[string]any) {
		targets := readCallTargets(params)
		setTarget(s, targets)
		countMetric(s, "lines", contentLines(result.Content))
		if failed := detailInt(details, "failedTargets"); failed > 0 && !result.IsError {
			s.Summary = plural(failed, "target") + " of " + plural(len(targets), "target") + " failed"
		}
	},
	"ls": func(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
		setTarget(s, []string{stringArg(params, "path")})
		countMetric(s, "entries", detailLen(details, "entries"))
	},
	"find": func(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
		setTarget(s, []string{stringArg(params, "path")})
		countMetric(s, "entries", detailLen(details, "entries"))
	},
	"write": func(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
		setTarget(s, []string{stringArg(params, "path")})
		if bytes := detailInt(details, "bytes"); bytes > 0 {
			s.Metrics = append(s.Metrics, presentation.Metric{ID: "bytes", Value: float64(bytes), Unit: presentation.UnitBytes})
		}
	},
	"edit": func(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
		setTarget(s, editCallTargets(params))
		added, removed := 0, 0
		for _, block := range s.Blocks {
			if block.Diff == nil {
				continue
			}
			for _, hunk := range block.Diff.Hunks {
				for _, line := range hunk.Lines {
					switch line.Kind {
					case presentation.HunkAdd:
						added++
					case presentation.HunkRemove:
						removed++
					}
				}
			}
		}
		countMetric(s, "added", added)
		countMetric(s, "removed", removed)
		if partial, _ := details["partial"].(bool); partial {
			s.Summary = "some files did not change"
		}
	},
	"cd": func(s *presentation.Snapshot, _ map[string]any, _ sdk.ToolResult, details map[string]any) {
		cwd, _ := details["cwd"].(string)
		setTarget(s, []string{cwd})
	},
	"ask_user": askSnapshot,
	"bash":     jobSnapshot,
	"jobs":     jobSnapshot,
	"grep": func(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
		setTarget(s, []string{stringArg(params, "path")})
		countMetric(s, "matches", max(detailLen(details, "matches"), detailLen(details, "paths")))
	},
}

func setTarget(s *presentation.Snapshot, targets []string) {
	if len(targets) == 0 || targets[0] == "" {
		return
	}
	value := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, targets[0])
	if value == "" {
		return
	}
	kind := presentation.TargetPath
	if scheme, _, ok := strings.Cut(value, "://"); ok && scheme != "" && !strings.ContainsAny(value, " \t") {
		kind = presentation.TargetURI
	}
	s.Target = &presentation.Target{Kind: kind, Value: value}
}

func countMetric(s *presentation.Snapshot, id string, value int) {
	if value > 0 {
		s.Metrics = append(s.Metrics, presentation.Metric{ID: id, Value: float64(value), Unit: presentation.UnitCount})
	}
}

func contentLines(content string) int {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func detailLen(details map[string]any, key string) int {
	if value := reflect.ValueOf(details[key]); value.IsValid() && value.Kind() == reflect.Slice {
		return value.Len()
	}
	return 0
}

func detailInt(details map[string]any, key string) int {
	switch value := details[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(count) + " " + noun + "s"
}

const (
	jobTailLines = 40
	jobTailBytes = 4 << 10
)

// jobSnapshot fills a bash or jobs result from the typed job facts: state, exit code and the output tail.
func jobSnapshot(s *presentation.Snapshot, params map[string]any, result sdk.ToolResult, details map[string]any) {
	countMetric(s, "jobs", detailLen(details, "jobs"))
	if waiting, _ := details["timedOutWait"].(bool); waiting {
		outcome := presentation.OutcomeTimedOut
		s.Outcome = &outcome
		return
	}
	state, _ := details["state"].(string)
	if state == "" {
		return
	}
	switch state {
	case "timedOut":
		outcome := presentation.OutcomeTimedOut
		s.Outcome = &outcome
	case "cancelled":
		outcome := presentation.OutcomeCancelled
		s.Outcome = &outcome
	case "unknown":
		outcome := presentation.OutcomeUnknown
		s.Outcome = &outcome
	}
	if state != "exited" {
		s.Summary = state
	}
	if _, ok := details["exitCode"]; ok {
		s.Metrics = append(s.Metrics, presentation.Metric{ID: "exitCode", Value: float64(detailInt(details, "exitCode")), Unit: presentation.UnitCount})
	}
	if truncated, _ := details["truncated"].(bool); truncated || detailInt(details, "gapFrom") > 0 {
		complete := false
		s.Complete = &complete
	}
	output := result.Content
	if first, rest, ok := strings.Cut(output, "\n"); strings.HasPrefix(first, "Job ") {
		output = rest
		if !ok {
			output = ""
		}
	}
	countMetric(s, "lines", contentLines(output))
	if tail := outputTail(output); tail != "" {
		s.Blocks = append(s.Blocks, presentation.Block{Kind: presentation.BlockCode, Code: &presentation.CodeBlock{Text: tail}})
	}
}

// outputTail keeps the last lines of command output, without control characters the decoder refuses.
func outputTail(output string) string {
	output = strings.TrimRight(strings.ToValidUTF8(output, "\uFFFD"), "\n")
	if len(output) > jobTailBytes {
		output = output[len(output)-jobTailBytes:]
		for len(output) > 0 && !utf8.RuneStart(output[0]) {
			output = output[1:]
		}
		if _, rest, ok := strings.Cut(output, "\n"); ok {
			output = rest
		}
	}
	lines := strings.Split(output, "\n")
	if len(lines) > jobTailLines {
		lines = lines[len(lines)-jobTailLines:]
	}
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.Join(lines, "\n"))
}

// askSnapshot maps the ask result to its outcome and lists each answer by question label, in the form's order.
func askSnapshot(s *presentation.Snapshot, params map[string]any, _ sdk.ToolResult, details map[string]any) {
	set := func(outcome presentation.Outcome, summary string) {
		s.Outcome = &outcome
		s.Summary = summary
	}
	reason, _ := details["reason"].(string)
	reason = oneLine(plainLine(reason), 200)
	switch {
	case details["cancelled"] == true:
		set(presentation.OutcomeCancelled, reason)
		return
	case details["timedOut"] == true:
		set(presentation.OutcomeTimedOut, reason)
		return
	case details["interactionUnavailable"] == true:
		set(presentation.OutcomeFailed, "interaction unavailable")
		return
	}
	var answers map[string]ask.Answered
	if encoded, err := json.Marshal(details["answers"]); err != nil || json.Unmarshal(encoded, &answers) != nil {
		return
	}
	table := &presentation.TableBlock{Columns: []presentation.Column{{ID: "question", Label: "Question", Cell: presentation.CellLabel}, {ID: "answer", Label: "Answer", Cell: presentation.CellText}}}
	questions, _ := params["questions"].([]any)
	for _, raw := range questions {
		question, _ := raw.(map[string]any)
		id := stringArg(question, "id")
		answer, ok := answers[id]
		if !ok || len(table.Rows) == presentation.MaxTableRows {
			continue
		}
		label := stringArg(question, "label")
		if label == "" {
			label = id
		}
		table.Rows = append(table.Rows, []presentation.Cell{{Kind: presentation.CellLabel, Text: plainLine(label)}, {Kind: presentation.CellText, Text: plainLine(answerLine(answer))}})
	}
	countMetric(s, "answers", len(table.Rows))
	if len(table.Rows) > 0 {
		s.Blocks = append(s.Blocks, presentation.Block{Kind: presentation.BlockTable, Table: table})
	}
}
