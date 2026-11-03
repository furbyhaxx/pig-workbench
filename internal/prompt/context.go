package prompt

import (
	"maps"
	"slices"

	"github.com/nikolalohinski/gonja/v2/exec"
)

type templateTools struct {
	Selected   []string   `jinja:"selected"`
	Declared   []string   `jinja:"declared"`
	Hidden     []string   `jinja:"hidden"`
	Snippets   *exec.Dict `jinja:"snippets"`
	Guidelines *exec.Dict `jinja:"guidelines"`
}

type templateHost struct {
	CustomPrompt     string        `jinja:"custom_prompt"`
	Append           string        `jinja:"append"`
	PromptGuidelines []string      `jinja:"prompt_guidelines"`
	Docs             bool          `jinja:"docs"`
	ContextFiles     []ContextFile `jinja:"context_files"`
	Skills           []HostSkill   `jinja:"skills"`
	Sections         *exec.Dict    `jinja:"sections"`
	Mode             string        `jinja:"mode"`
	OS               string        `jinja:"os"`
	Shell            string        `jinja:"shell"`
}

func templateContext(data Data) *exec.Context {
	guidelines := make(map[string][]string, len(data.Tools.Guidelines))
	for name, lines := range data.Tools.Guidelines {
		guidelines[name] = slices.Clone(lines)
	}
	tools := templateTools{
		Selected:   slices.Clone(data.Tools.Selected),
		Declared:   slices.Clone(data.Tools.Declared),
		Hidden:     slices.Clone(data.Tools.Hidden),
		Snippets:   orderedDict(data.Tools.Snippets),
		Guidelines: orderedDict(guidelines),
	}
	host := templateHost{
		CustomPrompt:     data.Host.CustomPrompt,
		Append:           data.Host.Append,
		PromptGuidelines: slices.Clone(data.Host.PromptGuidelines),
		Docs:             data.Host.Docs,
		ContextFiles:     slices.Clone(data.Host.ContextFiles),
		Skills:           slices.Clone(data.Host.Skills),
		Sections:         orderedDict(data.Host.Sections),
		Mode:             data.Host.Mode,
		OS:               data.Host.OS,
		Shell:            data.Host.Shell,
	}
	skills := data.Skills
	skills.Items = slices.Clone(skills.Items)
	workbench := data.Workbench
	workbench.Schemes = slices.Clone(workbench.Schemes)
	return exec.NewContext(map[string]any{
		"model":        data.Model,
		"tools":        tools,
		"host":         host,
		"instructions": slices.Clone(data.Instructions),
		"skills":       skills,
		"workbench":    workbench,
		"project_root": data.ProjectRoot,
	})
}

func orderedDict[T any](values map[string]T) *exec.Dict {
	result := exec.NewDict()
	for _, key := range slices.Sorted(maps.Keys(values)) {
		result.Pairs = append(result.Pairs, &exec.Pair{Key: exec.AsValue(key), Value: exec.AsValue(values[key])})
	}
	return result
}
