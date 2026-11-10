package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func instructionFacts(ctx sdk.Context, target, root, projectID string, validate bool, allowNested func(dir string) bool) ([]contextFact, string, error) {
	trusted, err := ctx.IsProjectTrusted()
	if err != nil {
		return nil, "", err
	}
	agent, err := sdk.AgentDir()
	if err != nil {
		return nil, "", err
	}
	registry, err := project.Open(agent)
	if err != nil {
		return nil, "", err
	}
	origin, err := registry.Detect(requestContext{ctx}, ctx.Cwd())
	if err != nil {
		return nil, "", err
	}
	if root != origin.Root {
		rel, relErr := filepath.Rel(origin.Root, target)
		if projectID == "global" && origin.ProjectID == "global" && relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			root = origin.Root
		} else {
			trusted = allowNested != nil && allowNested(root)
		}
	}
	privateRoot := ""
	if trusted {
		privateRoot, err = registry.PrivateNotesRoot(projectID)
		if err != nil {
			return nil, "", err
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
	opts := instructions.Options{ProjectRoot: root, UserRoots: []string{agent}}
	if trusted {
		opts.PrivateRoot = privateRoot
	}
	selected, err := instructions.Discover(requestContext{ctx}, target, nil, opts)
	if err != nil {
		var nested *instructions.NestedRepositoryError
		if !errors.As(err, &nested) {
			return nil, "", err
		}
		if allowNested != nil && allowNested(nested.Dir) {
			opts.AllowNested = true
			selected, err = instructions.Discover(requestContext{ctx}, target, nil, opts)
		} else {
			boundary = nested.Dir
			selected, err = instructions.Discover(requestContext{ctx}, agent, nil, instructions.Options{ProjectRoot: agent})
		}
		if err != nil {
			return nil, "", err
		}
	}
	var facts []contextFact
	if boundary != "" {
		facts = append(facts, contextFact{source: "instructions.boundary", kind: "notice", authority: "data", resource: boundary, value: "Nested repository instructions were not loaded without a separate trust decision: " + boundary})
	}
	present := make(map[string]bool)
	for _, file := range selected.Files {
		facts = append(facts, selectedFact(agent, pointerRoot(validate, root), file))
		present[file.Class+"/"+file.Path] = true
	}
	for _, reason := range selected.Reasons {
		if reason.Status == "capped" && (reason.Class == "steering" || reason.Class == "project-private") && !present[reason.Class+"/"+reason.Path] {
			facts = append(facts, contextFact{source: "instructions." + reason.Class, resource: reason.Path, kind: "notice", authority: "data", value: "Instruction view unavailable due to its limit: " + reason.Path, incomplete: true})
		}
	}
	return facts, privateRoot, nil
}

// hostFacts delivers the host-level AGENTS.md and HOST.md (SPEC-0007 §3.1); they apply regardless of the touched path or project trust.
func hostFacts(ctx sdk.Context, agent, dir string) ([]contextFact, error) {
	if dir == "" {
		return nil, nil
	}
	set, err := instructions.ResolveHost(requestContext{ctx}, instructions.LocalHost(dir))
	if err != nil {
		return nil, err
	}
	var facts []contextFact
	for _, file := range set.Files {
		facts = append(facts, selectedFact(agent, "", file))
	}
	for _, reason := range set.Reasons {
		if reason.Status == "refused" {
			facts = append(facts, contextFact{source: "instructions.host", resource: reason.Path, kind: "notice", authority: "data", value: "Host file not loaded: " + reason.DisplayPath + " (" + reason.Detail + ")"})
		}
	}
	return facts, nil
}

// nearestExistingDir returns the closest existing ancestor, because a directory that is about to be created inherits its ancestors' instructions.
func nearestExistingDir(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

func (a *app) toolInstructions(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, err
	}
	input, _ := data["input"].(map[string]any)
	paths := readCallTargets(input)
	if patch, ok := input["hashline"].(string); ok && patch != "" {
		if document, err := hashline.Parse(patch); err == nil && len(document.Sections) <= 32 {
			for _, section := range document.Sections {
				paths = append(paths, section.Path)
				for _, op := range section.Ops {
					if op.Kind == hashline.MV {
						paths = append(paths, op.Dest)
					}
				}
			}
		}
	}
	facts := map[string]any{"workbenchToolContext": true}
	var targets []string
	seen := make(map[string]bool)
	cwd := s.location.Current().Cwd
	for _, name := range paths {
		target, err := resource.ParsePath(cwd, name, "")
		if err != nil || target.Scheme != "file" {
			continue
		}
		path := target.Path
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			path = filepath.Dir(path)
		}
		path = nearestExistingDir(path)
		if !seen[path] {
			seen[path] = true
			targets = append(targets, path)
		}
	}
	if len(targets) > 0 {
		facts["workbenchInstructionTargets"] = targets
	}
	result, err := a.collectContextFrames(ctx, facts)
	if err != nil {
		return nil, err
	}
	if output, ok := result.(map[string]any); ok {
		tool, _ := data["toolName"].(string)
		mutating := tool == "edit" || tool == "write" || tool == "bash" || tool == "jobs" && (input["action"] == "write" || input["action"] == "eof")
		changed := false
		if blocks := takeFrameBlocks(output); len(blocks) > 0 {
			if err := sendFrameBlocks(ctx, blocks); err != nil {
				return nil, err
			}
			changed = strings.Contains(strings.Join(blocks, "\n"), `src="instructions.`)
		}
		if mutating {
			if incomplete, ok := output["workbenchIncompleteInstructions"].([]string); ok && len(incomplete) > 0 {
				return map[string]any{"block": true, "reason": "incompleteInstruction: read the complete instruction file before mutating (files over 4 MiB must be reduced): " + strings.Join(incomplete, ", ")}, nil
			}
			if changed {
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
	return contextFact{source: "instructions." + file.Class, value: body, resource: file.Path, kind: kind, authority: authority, incomplete: file.Truncated && file.Role == "instruction", revision: file.Digest}
}

func (s *session) instructionFullySeen(fact contextFact) bool {
	if fact.revision == "" {
		return false
	}
	snapshot, ok := s.snapshots.Latest(resource.FileURI(fact.resource))
	if !ok || snapshot.Revision != fact.revision || snapshot.Epoch != s.snapshots.Epoch() {
		return false
	}
	source, err := hashline.Normalize(snapshot.Raw)
	if err != nil || len(source.Lines) == 0 {
		return false
	}
	return len(snapshot.Seen) == 1 && snapshot.Seen[0].Start == 1 && snapshot.Seen[0].End >= len(source.Lines)
}

// pointerRoot is the directory backlog pointer targets resolve against, or "" when validation is off.
func pointerRoot(validate bool, root string) string {
	if !validate {
		return ""
	}
	return root
}
