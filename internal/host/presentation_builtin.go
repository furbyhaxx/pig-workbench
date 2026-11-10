package host

import "github.com/furbyhaxx/pig-workbench/internal/presentation"

// builtinOwner owns the presentation of Workbench's own tools; a contribution cannot claim their names.
const builtinOwner = "pig-workbench"

type bodyMode int

const (
	bodyHead bodyMode = iota
	bodyTail
)

// builtinPresentation is what a wire descriptor cannot say about an own tool: call facts parsed from its arguments and how its text result is previewed.
type builtinPresentation struct {
	facts func(args map[string]any, root string, width int) []string
	body  bodyMode
}

var builtinMetrics = []presentation.MetricDefinition{
	{ID: "lines", Label: "lines", Unit: presentation.UnitCount, Priority: 10},
	{ID: "matches", Label: "matches", Unit: presentation.UnitCount, Priority: 10},
	{ID: "entries", Label: "entries", Unit: presentation.UnitCount, Priority: 10},
	{ID: "exitCode", Label: "exit", Unit: presentation.UnitCount, Priority: 15},
	{ID: "durationMs", Label: "time", Unit: presentation.UnitMilliseconds, Priority: 20},
}

type builtinTool struct {
	preset  presentation.Preset
	preview int
	builtinPresentation
}

var builtinTools = map[string]builtinTool{
	"read":     {presentation.PresetFileRead, 0, builtinPresentation{facts: readFacts}},
	"edit":     {presentation.PresetFileEdit, 6, builtinPresentation{facts: editFacts}},
	"write":    {presentation.PresetFileEdit, 2, builtinPresentation{facts: pathFacts}},
	"ls":       {presentation.PresetSearch, 8, builtinPresentation{facts: lsFacts}},
	"find":     {presentation.PresetSearch, 8, builtinPresentation{facts: findFacts}},
	"grep":     {presentation.PresetSearch, 8, builtinPresentation{facts: grepFacts}},
	"bash":     {presentation.PresetProcess, 10, builtinPresentation{facts: bashFacts, body: bodyTail}},
	"jobs":     {presentation.PresetProcess, 8, builtinPresentation{facts: jobsFacts}},
	"cd":       {presentation.PresetGeneric, 3, builtinPresentation{facts: cdFacts}},
	"ask_user": {presentation.PresetQuestionnaire, 0, builtinPresentation{facts: askFacts}},
}

func builtinRegistration(tool string) presentationRegistration {
	spec := builtinTools[tool]
	hooks := spec.builtinPresentation
	return presentationRegistration{
		registrationID: "builtin:" + tool, owner: builtinOwner, key: tool, enabled: true, builtin: &hooks,
		descriptor: presentation.Descriptor{
			ToolNames: []string{tool}, Label: tool, PresentationVersion: 1, Preset: spec.preset,
			DefaultDisplay:    presentation.DisplayDefaults{Density: "normal", PreviewLines: spec.preview},
			DetailsPath:       "/workbenchPresentation",
			MetricDefinitions: builtinMetrics,
		},
	}
}
