package host

import (
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// promptOverride replaces PiG's prompt only for an explicit force delivery.
// auto stays on PiG until the approved A/B run.
func promptOverride(enabled bool, delivery, rendered string) (string, bool) {
	if !enabled || delivery != "force" || rendered == "" {
		return "", false
	}
	return rendered, true
}

func walkReplace(value any, from, to string) (any, int) {
	switch typed := value.(type) {
	case string:
		if typed == from {
			return to, 1
		}
		return typed, 0
	case map[string]any:
		n := 0
		for key, child := range typed {
			var count int
			typed[key], count = walkReplace(child, from, to)
			n += count
		}
		return typed, n
	case []any:
		n := 0
		for i, child := range typed {
			var count int
			typed[i], count = walkReplace(child, from, to)
			n += count
		}
		return typed, n
	default:
		return value, 0
	}
}

func (a *app) spliceLatePrompt(ctx sdk.Context, data map[string]any) (any, error) {
	session, err := a.current()
	if err != nil || session.promptText == "" {
		return nil, nil
	}
	payload, ok := data["payload"]
	if !ok {
		return nil, nil
	}
	extra := lateSections(ctx, session.promptText)
	if extra == "" {
		return nil, nil
	}
	replaced, n := walkReplace(payload, session.promptText, session.promptText+"\n\n"+extra)
	if n == 0 {
		return nil, nil
	}
	return map[string]any{"payload": replaced}, nil
}

func lateSections(ctx sdk.Context, served string) string {
	options, err := ctx.GetSystemPromptOptions()
	if err != nil || options.Sections == nil {
		return ""
	}
	var b strings.Builder
	for _, section := range *options.Sections {
		if section.Name == "" || strings.Contains(served, "<"+section.Name+">") {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "<%s>\n%s\n</%s>", section.Name, section.Value, section.Name)
	}
	return b.String()
}
