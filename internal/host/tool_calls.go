package host

import (
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/hashline"
)

// maxCallTargets bounds how many targets one call row names before it counts the rest.
const maxCallTargets = 3

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
