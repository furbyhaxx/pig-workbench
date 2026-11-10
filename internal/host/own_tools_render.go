package host

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/pathdisplay"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

// Workbench's own tools share one appearance with contributed presentations:
//
//	call:   label · target · fact…
//	result: label · status · metric…                      then a bounded body (the call row above carries the target)
//
// Statuses are running, succeeded and failed; metrics use formatMetric; every line
// goes through fitPresentationLines. The row is derived at render time from the call
// arguments and the result text, so tool Details stay the model-facing contract.

const bodyNone bodyMode = 2

type ownToolStyle struct {
	label string
	facts func(args map[string]any, root string, width int) []string
	body  bodyMode
	// preview is the collapsed body size; expanded views show ownExpandedBody lines.
	preview int
	// outcome refines the status word from the result text; empty keeps the generic one.
	outcome func(text string) string
	// lineMetric reports the number of result lines when the body does not show them all.
	lineMetric bool
}

const ownExpandedBody = 80

var ownToolStyles = map[string]ownToolStyle{
	"read":     {label: "read", facts: readFacts, body: bodyNone, lineMetric: true},
	"edit":     {label: "edit", facts: editFacts, body: bodyHead, preview: 6},
	"write":    {label: "write", facts: pathFacts, body: bodyHead, preview: 2},
	"ls":       {label: "ls", facts: lsFacts, body: bodyHead, preview: 8, lineMetric: true},
	"find":     {label: "find", facts: findFacts, body: bodyHead, preview: 8, lineMetric: true},
	"grep":     {label: "grep", facts: grepFacts, body: bodyHead, preview: 8, lineMetric: true},
	"bash":     {label: "bash", facts: bashFacts, body: bodyTail, preview: 10, lineMetric: true},
	"jobs":     {label: "jobs", facts: jobsFacts, body: bodyHead, preview: 8},
	"cd":       {label: "cd", facts: cdFacts, body: bodyHead, preview: 3},
	"ask_user": {label: "ask_user", facts: askFacts, body: bodyNone, outcome: askOutcome},
}

func ownToolRenderers(toolName string) *sdk.ToolRenderers {
	style, ok := ownToolStyles[toolName]
	if !ok {
		return nil
	}
	return &sdk.ToolRenderers{
		Call: func(_ sdk.Context, args map[string]any, render sdk.ToolRenderContext, width int) ([]string, error) {
			parts := append([]string{style.label}, style.facts(args, render.Cwd, width)...)
			if len(args) > 0 && !render.ArgsComplete {
				parts = append(parts, "partial arguments")
			}
			return fitPresentationLines([]string{strings.Join(parts, " · ")}, width, 1), nil
		},
		Result: func(_ sdk.Context, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) ([]string, error) {
			return renderOwnResult(style, result, options, render, width), nil
		},
	}
}

func renderOwnResult(style ownToolStyle, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) []string {
	text := resultText(result.Content)
	var content []string
	if strings.TrimSpace(text) != "" {
		content = splitPresentationText(strings.TrimRight(text, "\n"))
	}
	status := presentation.OutcomeSucceeded
	statusText := string(status)
	switch {
	case render.IsError:
		statusText = string(presentation.OutcomeFailed)
	case options.IsPartial || render.IsPartial:
		statusText = string(presentation.PhaseRunning)
	}
	if style.outcome != nil && !render.IsError && statusText == string(presentation.OutcomeSucceeded) {
		if refined := style.outcome(text); refined != "" {
			statusText = refined
		}
	}
	parts := []string{style.label, statusText}
	if style.lineMetric && len(content) > 0 {
		parts = append(parts, "lines "+formatMetric(presentation.Metric{Value: float64(len(content)), Unit: presentation.UnitCount}))
	}
	if render.DurationMs != nil {
		parts = append(parts, "time "+formatMetric(presentation.Metric{Value: float64(*render.DurationMs), Unit: presentation.UnitMilliseconds}))
	}
	lines := []string{strings.Join(parts, " · ")}
	mode, limit := style.body, style.preview
	if render.IsError && mode == bodyNone {
		mode, limit = bodyHead, 6
	}
	if options.Expanded {
		limit = ownExpandedBody
		if mode == bodyNone {
			mode = bodyHead
		}
	}
	if mode != bodyNone && len(content) > 0 {
		lines = append(lines, ownBody(content, mode, limit)...)
	}
	return fitPresentationLines(lines, width, ownExpandedBody+4)
}

func ownBody(content []string, mode bodyMode, limit int) []string {
	if len(content) <= limit {
		return content
	}
	hidden := len(content) - limit
	if mode == bodyTail {
		return append([]string{fmt.Sprintf("… %d earlier lines", hidden)}, content[hidden:]...)
	}
	return append(append([]string(nil), content[:limit]...), fmt.Sprintf("… %d more lines", hidden))
}

func resultText(blocks []map[string]any) string {
	var out strings.Builder
	for _, block := range blocks {
		if block["type"] != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			out.WriteString(text)
		}
	}
	return out.String()
}

func displayTarget(value, root string, width int) string {
	return pathdisplay.Display(pathdisplay.Target{Value: value, Root: root, NoFish: true}, max(8, width/2))
}

func listFacts(targets []string, root string, width int) []string {
	if len(targets) == 0 {
		return nil
	}
	shown := targets
	if len(shown) > maxCallTargets {
		shown = shown[:maxCallTargets]
	}
	display := make([]string, len(shown))
	for i, target := range shown {
		display[i] = displayTarget(target, root, width)
	}
	fact := strings.Join(display, ", ")
	if extra := len(targets) - len(shown); extra > 0 {
		fact += fmt.Sprintf(" (+%d)", extra)
	}
	return []string{fact}
}

func stringArg(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return value
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

func readFacts(args map[string]any, root string, width int) []string {
	facts := listFacts(readCallTargets(args), root, width)
	if selector := stringArg(args, "selector"); selector != "" {
		facts = append(facts, selector)
	}
	return facts
}

func editFacts(args map[string]any, root string, width int) []string {
	return listFacts(editCallTargets(args), root, width)
}

func pathFacts(args map[string]any, root string, width int) []string {
	return listFacts(readCallTargets(args), root, width)
}

func lsFacts(args map[string]any, root string, width int) []string {
	path := stringArg(args, "path")
	if path == "" {
		return nil
	}
	return listFacts([]string{path}, root, width)
}

func findFacts(args map[string]any, root string, width int) []string {
	facts := []string{}
	if pattern := stringArg(args, "pattern"); pattern != "" {
		facts = append(facts, oneLine(pattern, 60))
	}
	return append(facts, lsFacts(args, root, width)...)
}

func grepFacts(args map[string]any, root string, width int) []string {
	var facts []string
	if pattern := stringArg(args, "pattern"); pattern != "" {
		facts = append(facts, oneLine(pattern, 60))
	} else if patterns, ok := args["patterns"].([]any); ok && len(patterns) > 0 {
		names := make([]string, 0, len(patterns))
		for _, item := range patterns {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		facts = append(facts, oneLine(strings.Join(names, " | "), 60))
	}
	facts = append(facts, lsFacts(args, root, width)...)
	if glob := stringArg(args, "glob"); glob != "" {
		facts = append(facts, "glob "+glob)
	}
	return facts
}

func bashFacts(args map[string]any, _ string, _ int) []string {
	facts := []string{}
	if title := stringArg(args, "title"); title != "" {
		facts = append(facts, oneLine(title, 80))
	}
	if command := stringArg(args, "command"); command != "" {
		facts = append(facts, oneLine(command, 100))
	}
	if cwd := stringArg(args, "cwd"); cwd != "" {
		facts = append(facts, "in "+oneLine(cwd, 60))
	}
	if background, _ := args["background"].(bool); background {
		facts = append(facts, "background")
	}
	if interactive, _ := args["interactive"].(bool); interactive {
		facts = append(facts, "interactive")
	}
	return facts
}

func jobsFacts(args map[string]any, _ string, _ int) []string {
	var facts []string
	if action := stringArg(args, "action"); action != "" {
		facts = append(facts, action)
	}
	if id := stringArg(args, "id"); id != "" {
		facts = append(facts, id)
	}
	return facts
}

func cdFacts(args map[string]any, root string, width int) []string {
	if reset, _ := args["reset"].(bool); reset {
		return []string{"(reset)"}
	}
	if path := stringArg(args, "path"); path != "" {
		return listFacts([]string{path}, root, width)
	}
	return nil
}

func askOutcome(text string) string {
	var result struct {
		Answers     map[string]json.RawMessage `json:"answers"`
		Cancelled   bool                       `json:"cancelled"`
		TimedOut    bool                       `json:"timedOut"`
		Unavailable string                     `json:"interactionUnavailable"`
	}
	if json.Unmarshal([]byte(text), &result) != nil {
		return ""
	}
	switch {
	case result.Cancelled:
		return string(presentation.OutcomeCancelled)
	case result.TimedOut:
		return string(presentation.OutcomeTimedOut)
	case result.Unavailable != "" || strings.Contains(text, "interactionUnavailable"):
		return "unavailable"
	case len(result.Answers) > 0:
		return fmt.Sprintf("answered %d", len(result.Answers))
	}
	return ""
}

func askFacts(args map[string]any, _ string, _ int) []string {
	var facts []string
	if title := stringArg(args, "title"); title != "" {
		facts = append(facts, oneLine(title, 60))
	}
	if questions, ok := args["questions"].([]any); ok && len(questions) > 0 {
		noun := "questions"
		if len(questions) == 1 {
			noun = "question"
		}
		facts = append(facts, fmt.Sprintf("%d %s", len(questions), noun))
	}
	return facts
}
