package host

import (
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/prompt"
)

const promptUsage = "usage: /workbench prompt [show | render [--model <provider>/<id>] [--tools <name,…>] | diff]"

// promptCommand inspects the system prompt; no verb sends anything to the model.
func (a *app) promptCommand(ctx sdk.Context, args string) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	fields := strings.Fields(args)
	verb := "show"
	if len(fields) > 0 {
		verb, fields = fields[0], fields[1:]
	}
	options, err := ctx.GetSystemPromptOptions()
	if err != nil {
		return err
	}
	template := s.settingString("prompt.template", defaultPromptTemplate)
	switch verb {
	case "show":
		if len(fields) > 0 {
			return errors.New(promptUsage)
		}
		rendered, err := renderPrompt(ctx, s, template, options, nil)
		if err != nil {
			return fmt.Errorf("render %s: %w", template, err)
		}
		a.notify(ctx, s.promptStatus(template, rendered))
	case "render":
		model, tools, err := promptRenderFlags(ctx, fields)
		if err != nil {
			return err
		}
		if tools != nil {
			options.SelectedTools = tools
		}
		rendered, err := renderPrompt(ctx, s, template, options, model)
		if err != nil {
			return fmt.Errorf("render %s: %w", template, err)
		}
		a.present(ctx, "Workbench › Prompt › Render", fmt.Sprintf("rendered %s, %d bytes, not sent\n\n%s", template, len(rendered), strings.TrimRight(rendered, "\n")))
	case "diff":
		if len(fields) > 0 {
			return errors.New(promptUsage)
		}
		served, err := ctx.GetSystemPrompt()
		if err != nil {
			return err
		}
		rendered, err := renderPrompt(ctx, s, template, options, nil)
		if err != nil {
			return fmt.Errorf("render %s: %w", template, err)
		}
		diff := instructions.UnifiedDiff(served, rendered, 2)
		if diff == "" {
			diff = "The prompt in use and a fresh render of " + template + " are identical."
		} else {
			diff = "--- prompt in use\n+++ " + template + " rendered now\n" + diff
		}
		a.present(ctx, "Workbench › Prompt › Diff", diff)
	default:
		return errors.New(promptUsage)
	}
	return nil
}

// promptStatus is the show view: settings, delivery state and section sizes, without the prompt text.
func (s *session) promptStatus(template, rendered string) string {
	var b strings.Builder
	if configuration, err := s.settings.Current(); err == nil {
		for _, key := range []string{"prompt.enabled", "prompt.template", "prompt.delivery"} {
			if entry, ok := configuration.Get(key); ok {
				fmt.Fprintf(&b, "%s: %v\n", key, entry.Value)
			}
		}
	}
	_, epoch, lastErr, forced := s.prompt.get()
	if forced {
		fmt.Fprintf(&b, "in use: %s, epoch %s\n", s.prompt.source(), epoch)
	} else {
		b.WriteString("in use: PiG's prompt\n")
	}
	if lastErr != "" {
		fmt.Fprintf(&b, "last error: %s\n", lastErr)
	}
	fmt.Fprintf(&b, "rendered now: epoch %s, %d bytes\n", promptEpoch(rendered), len(rendered))
	b.WriteString(promptSectionSizes(rendered))
	b.WriteString("render shows the text, diff compares it with the prompt in use; read session://self/system-prompt for the served prompt")
	return b.String()
}

// promptRenderFlags reads --model and --tools; an absent flag leaves the session's value in place.
func promptRenderFlags(ctx sdk.Context, fields []string) (*prompt.Model, []string, error) {
	var model *prompt.Model
	var tools []string
	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			return nil, nil, errors.New(promptUsage)
		}
		switch fields[i] {
		case "--model":
			provider, id, ok := strings.Cut(fields[i+1], "/")
			if !ok || provider == "" || id == "" {
				return nil, nil, errors.New("--model takes <provider>/<id>")
			}
			found, err := ctx.ModelRegistry().Find(provider, id)
			if err != nil {
				return nil, nil, err
			}
			if found == nil {
				return nil, nil, fmt.Errorf("no model %s in PiG's registry", noticeText(oneLine(fields[i+1], 80), 0))
			}
			model = &prompt.Model{Provider: provider, ID: id}
			model.Name, _ = found["name"].(string)
			model.Reasoning, _ = found["reasoning"].(bool)
			if window, ok := found["contextWindow"].(float64); ok {
				model.ContextWindow = int(window)
			}
			if output, ok := found["maxTokens"].(float64); ok {
				model.MaxOutput = int(output)
			}
		case "--tools":
			tools = []string{}
			for _, name := range strings.Split(fields[i+1], ",") {
				if name != "" {
					tools = append(tools, name)
				}
			}
		default:
			return nil, nil, errors.New(promptUsage)
		}
	}
	return model, tools, nil
}
