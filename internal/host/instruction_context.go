package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func instructionFacts(ctx sdk.Context, target, root, projectID string, validate bool, allowNested func(dir string) bool) ([]contextFact, error) {
	trusted, err := ctx.IsProjectTrusted()
	if err != nil {
		return nil, err
	}
	agent, err := sdk.AgentDir()
	if err != nil {
		return nil, err
	}
	registry, err := project.Open(agent)
	if err != nil {
		return nil, err
	}
	origin, err := registry.Detect(requestContext{ctx}, ctx.Cwd())
	if err != nil {
		return nil, err
	}
	if root != origin.Root {
		rel, relErr := filepath.Rel(origin.Root, target)
		if projectID == "global" && origin.ProjectID == "global" && relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			root = origin.Root
		} else {
			trusted = false
		}
	}
	relative, relErr := filepath.Rel(root, target)
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		trusted = false
	}
	if !trusted {
		target = agent
		root = agent
	}
	var boundary string
	selected, err := instructions.Discover(requestContext{ctx}, target, nil, instructions.Options{ProjectRoot: root, UserRoots: []string{agent}})
	if err != nil {
		var nested *instructions.NestedRepositoryError
		if !errors.As(err, &nested) {
			return nil, err
		}
		if allowNested != nil && allowNested(nested.Dir) {
			selected, err = instructions.Discover(requestContext{ctx}, target, nil, instructions.Options{ProjectRoot: root, UserRoots: []string{agent}, AllowNested: true})
		} else {
			boundary = nested.Dir
			selected, err = instructions.Discover(requestContext{ctx}, agent, nil, instructions.Options{ProjectRoot: agent})
		}
		if err != nil {
			return nil, err
		}
	}
	var facts []contextFact
	if boundary != "" {
		facts = append(facts, contextFact{source: "instructions.boundary", kind: "notice", authority: "data", resource: boundary, value: "Nested repository instructions were not loaded without a separate trust decision: " + boundary})
	}
	for _, file := range selected.Files {
		facts = append(facts, selectedFact(agent, pointerRoot(validate, root), file))
	}
	return facts, nil
}

func (a *app) toolInstructions(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, err
	}
	input, _ := data["input"].(map[string]any)
	targetPath, _ := input["path"].(string)
	facts := map[string]any{"workbenchToolContext": true}
	if targetPath != "" {
		target, err := resource.ParsePath(s.location.Current().Cwd, targetPath, "")
		if err != nil {
			return nil, nil
		}
		if target.Scheme != "file" {
			return nil, nil
		}
		path := target.Path
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			path = filepath.Dir(path)
		}
		facts["workbenchInstructionTarget"] = path
	}
	result, err := a.contextFrames(ctx, facts)
	if err != nil {
		return nil, err
	}
	if output, ok := result.(map[string]any); ok {
		if message, ok := output["message"].(map[string]any); ok {
			text, _ := message["content"].(string)
			if err := ctx.SendMessage("pig-workbench.context", text, true, sdk.SendMessageOptions{DeliverAs: "steer", TriggerTurn: sdk.Bool(false)}); err != nil {
				return nil, err
			}
			if tool, _ := data["toolName"].(string); (tool == "edit" || tool == "write") && strings.Contains(text, `src="instructions.`) {
				return map[string]any{"block": true, "reason": "instructionContextChanged: instructions applying to this target were just delivered to you and nothing was changed. Read them, then repeat the call if it still fits."}, nil
			}
		}
	}
	return nil, nil
}

// selectedFact turns a discovered file into the frame fact that delivers it.
func selectedFact(agent, pointerRoot string, file instructions.Selected) contextFact {
	if pointerRoot != "" && file.Class == "backlog" {
		file.Content = instructions.AnnotatePointers(pointerRoot, file.Content)
	}
	if rel, err := filepath.Rel(agent, file.Path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		file.Trust = "user"
	}
	kind, authority := "info", "data"
	if file.Role == "instruction" {
		kind = "instruction"
		authority = "instruction"
	}
	body := "Instructions from: " + file.Path + "\n" + (instructions.Set{Files: []instructions.Selected{file}}).Render()
	return contextFact{source: "instructions." + file.Class, value: body, resource: file.Path, kind: kind, authority: authority}
}

// pointerRoot is the directory backlog pointer targets resolve against, or "" when validation is off.
func pointerRoot(validate bool, root string) string {
	if !validate {
		return ""
	}
	return root
}
