package host

import (
	"fmt"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/pathdisplay"
)

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
