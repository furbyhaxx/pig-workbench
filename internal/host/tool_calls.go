package host

import (
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
)

// maxCallTargets bounds how many targets one call row names before it counts the rest.
const maxCallTargets = 3

// callRow renders "label target, target (+N)" for a tool call, or the bare label when no target is known yet.
func callRow(label string, targets []string, width int) []string {
	line := label
	if len(targets) > 0 {
		shown := targets
		if len(shown) > maxCallTargets {
			shown = shown[:maxCallTargets]
		}
		line += " " + strings.Join(shown, ", ")
		if extra := len(targets) - len(shown); extra > 0 {
			line += fmt.Sprintf(" (+%d)", extra)
		}
	}
	return fitPresentationLines([]string{line}, width, 1)
}

func readCallTargets(args map[string]any) []string {
	var targets []string
	if path, ok := args["path"].(string); ok && path != "" {
		targets = append(targets, path)
	}
	if paths, ok := args["paths"].([]any); ok {
		for _, item := range paths {
			if path, ok := item.(string); ok && path != "" {
				targets = append(targets, path)
			}
		}
	}
	return targets
}

// editCallTargets names the files an edit call touches, from either input form, tolerating a partial hashline.
func editCallTargets(args map[string]any) []string {
	if path, ok := args["path"].(string); ok && path != "" {
		return []string{path}
	}
	text, _ := args["hashline"].(string)
	if document, err := hashline.Parse(text); err == nil {
		return document.Paths()
	}
	var targets []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "[") {
			continue
		}
		if end := strings.LastIndex(line, "#"); end > 1 {
			targets = append(targets, strings.Trim(line[1:end], `"`))
		}
	}
	return targets
}

func renderReadCall(_ sdk.Context, args map[string]any, _ sdk.ToolRenderContext, width int) ([]string, error) {
	return callRow("read", readCallTargets(args), width), nil
}

func renderEditCall(_ sdk.Context, args map[string]any, _ sdk.ToolRenderContext, width int) ([]string, error) {
	return callRow("edit", editCallTargets(args), width), nil
}

func renderCdCall(_ sdk.Context, args map[string]any, _ sdk.ToolRenderContext, width int) ([]string, error) {
	if reset, _ := args["reset"].(bool); reset {
		return callRow("cd", []string{"(reset)"}, width), nil
	}
	if path, ok := args["path"].(string); ok && path != "" {
		return callRow("cd", []string{path}, width), nil
	}
	return callRow("cd", nil, width), nil
}
