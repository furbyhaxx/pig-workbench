package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

const pinEntryType = "pig-workbench.instruction-pin"

// instructionPin keeps one file delivered regardless of the touched target (SPEC-0007 §6).
type instructionPin struct {
	Class  string `json:"class"`
	Path   string `json:"path"`
	Origin string `json:"origin"`
}

type pinRecord struct {
	Action string `json:"action"`
	instructionPin
}

// pinsFromBranch replays pin and unpin records that lie on the active branch, oldest first.
func pinsFromBranch(entries []json.RawMessage, branch []sdk.BranchEntry) []instructionPin {
	onBranch := make(map[string]bool, len(branch))
	for _, entry := range branch {
		onBranch[entry.ID] = true
	}
	var pins []instructionPin
	for _, raw := range entries {
		var entry struct {
			ID         string          `json:"id"`
			Type       string          `json:"type"`
			CustomType string          `json:"customType"`
			Data       json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &entry) != nil || !onBranch[entry.ID] || entry.Type != "custom" || entry.CustomType != pinEntryType {
			continue
		}
		var record pinRecord
		if json.Unmarshal(entry.Data, &record) != nil || record.Path == "" {
			continue
		}
		pins = withoutPin(pins, record.Path)
		if record.Action == "pin" {
			pins = append(pins, record.instructionPin)
		}
	}
	return pins
}

func withoutPin(pins []instructionPin, path string) []instructionPin {
	return slices.DeleteFunc(slices.Clone(pins), func(pin instructionPin) bool { return pin.Path == path })
}

// pinnableClass chooses the class a file belongs to. A class must allow pinning, and an
// ambiguous match needs an explicit class so an agent cannot pick a convenient one.
func pinnableClass(path, requested, privateRoot string) (instructions.Class, error) {
	if privateNotePath(path, privateRoot) {
		if requested != "" && requested != "project-private" {
			return instructions.Class{}, fmt.Errorf("private note does not belong to class %q", requested)
		}
		return instructions.ProjectPrivateClass(), nil
	}
	var matches []instructions.Class
	for _, class := range instructions.DefaultClasses() {
		if requested != "" && class.ID != requested || !slices.Contains(class.Filenames, filepath.Base(path)) {
			continue
		}
		matches = append(matches, class)
	}
	switch {
	case len(matches) == 0 && requested != "":
		return instructions.Class{}, fmt.Errorf("class %q has no file named %q", requested, filepath.Base(path))
	case len(matches) == 0:
		return instructions.Class{}, fmt.Errorf("%q is not an instruction file of any class", filepath.Base(path))
	case len(matches) > 1:
		ids := make([]string, len(matches))
		for i, class := range matches {
			ids[i] = class.ID
		}
		return instructions.Class{}, fmt.Errorf("%q matches several classes (%s); pass --class", filepath.Base(path), strings.Join(ids, ", "))
	case !matches[0].AllowPin:
		return instructions.Class{}, fmt.Errorf("class %q does not allow pins", matches[0].ID)
	}
	return matches[0], nil
}

func privateNotePath(path, root string) bool {
	if root == "" || !strings.HasSuffix(filepath.Base(path), ".md") {
		return false
	}
	canonical, err := filepath.EvalSymlinks(root)
	return err == nil && filepath.Dir(path) == canonical
}

func sessionPrivateRoot(ctx sdk.Context, s *session) (string, error) {
	agent, err := sdk.AgentDir()
	if err != nil {
		return "", err
	}
	registry, err := project.Open(agent)
	if err != nil {
		return "", err
	}
	current := s.location.Current()
	detected, err := registry.Detect(requestContext{ctx}, current.Cwd)
	if err != nil {
		return "", err
	}
	_, root, err := instructionFacts(ctx, current.Cwd, detected.Root, detected.ProjectID, false, func(dir string) bool { return trustAllows(s.settings, dir) })
	return root, err
}

func insideRegistryPrivate(agent, path string) bool {
	canonical, err := filepath.EvalSymlinks(agent)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Join(canonical, "projects"), path)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) >= 3 && parts[1] == "private"
}

// pinBoundary is the directory a pinned file must lie in: the trusted project, or the user's agent directory.
func (a *app) pinBoundary(ctx sdk.Context, s *session, canonical string) (string, error) {
	agent, err := sdk.AgentDir()
	if err != nil {
		return "", err
	}
	inside := func(root string) bool {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(resolved, canonical)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}
	if inside(agent) {
		return agent, nil
	}
	trusted, err := ctx.IsProjectTrusted()
	if err != nil {
		return "", err
	}
	root := s.location.Current().ProjectRoot
	if trusted && root != "" && inside(root) {
		return root, nil
	}
	return "", errors.New("pins reach only files inside the trusted project or the user's agent directory")
}

// pinInstruction records a pin after the class policy, boundary and file checks.
func (a *app) pinInstruction(ctx sdk.Context, class, rawPath, origin string) (string, error) {
	s, err := a.current()
	if err != nil {
		return "", err
	}
	target, err := resource.ParsePath(s.location.Current().Cwd, rawPath, "")
	if err != nil || target.Scheme != "file" {
		return "", errors.New("pin takes a local file path")
	}
	canonical, err := filepath.EvalSymlinks(target.Path)
	if err != nil {
		return "", fmt.Errorf("pin target: %w", err)
	}
	if info, err := os.Stat(canonical); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("pin target is not a regular file")
	}
	privateRoot, err := sessionPrivateRoot(ctx, s)
	if err != nil {
		return "", err
	}
	agent, err := sdk.AgentDir()
	if err != nil {
		return "", err
	}
	if insideRegistryPrivate(agent, canonical) && !privateNotePath(canonical, privateRoot) {
		return "", errors.New("private note pins must belong to the active trusted project")
	}
	selected, err := pinnableClass(canonical, class, privateRoot)
	if err != nil {
		return "", err
	}
	if _, err := a.pinBoundary(ctx, s, canonical); err != nil {
		return "", err
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, existing := range state.pins {
		if existing.Path == canonical && existing.Class == selected.ID {
			return "already pinned: " + canonical, nil
		}
	}
	pin := instructionPin{Class: selected.ID, Path: canonical, Origin: origin}
	if err := ctx.AppendEntry(pinEntryType, pinRecord{Action: "pin", instructionPin: pin}); err != nil {
		return "", err
	}
	state.pins = append(withoutPin(state.pins, canonical), pin)
	return "pinned " + canonical + " (" + selected.ID + ")", nil
}

// unpinInstruction removes a pin; automatic selection applies again at the next boundary.
func (a *app) unpinInstruction(ctx sdk.Context, rawPath string) (string, error) {
	s, err := a.current()
	if err != nil {
		return "", err
	}
	target, err := resource.ParsePath(s.location.Current().Cwd, rawPath, "")
	if err != nil || target.Scheme != "file" {
		return "", errors.New("unpin takes a local file path")
	}
	canonical := target.Path
	if resolved, err := filepath.EvalSymlinks(target.Path); err == nil {
		canonical = resolved
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	if !slices.ContainsFunc(state.pins, func(pin instructionPin) bool { return pin.Path == canonical }) {
		return "", fmt.Errorf("not pinned: %s", canonical)
	}
	if err := ctx.AppendEntry(pinEntryType, pinRecord{Action: "unpin", instructionPin: instructionPin{Path: canonical}}); err != nil {
		return "", err
	}
	state.pins = withoutPin(state.pins, canonical)
	return "unpinned " + canonical, nil
}

// pinFacts delivers every pin. A vanished or no-longer-selected file becomes a notice, never stale content.
func pinFacts(ctx sdk.Context, agent, privateRoot, pointers string, pins []instructionPin) []contextFact {
	var facts []contextFact
	for _, pin := range pins {
		class, ok := classByID(pin.Class)
		unavailable := func(why string) {
			facts = append(facts, contextFact{source: "instructions." + pin.Class, resource: pin.Path, kind: "notice", authority: "data", value: "Pinned instruction file is unavailable (" + why + "): " + pin.Path})
		}
		if !ok {
			unavailable("its class no longer exists")
			continue
		}
		if class.ID == "project-private" {
			if !privateNotePath(pin.Path, privateRoot) {
				unavailable("it does not belong to the active trusted project")
				continue
			}
		} else if insideRegistryPrivate(agent, pin.Path) {
			unavailable("private notes require their own class")
			continue
		}
		root := filepath.Dir(pin.Path)
		if rel, err := filepath.Rel(agent, pin.Path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			root = agent
		}
		options := instructions.Options{ProjectRoot: root, AllowNested: true}
		if class.ID == "project-private" {
			options.PrivateRoot = privateRoot
		}
		set, err := instructions.Discover(requestContext{ctx}, filepath.Dir(pin.Path), []instructions.Class{class}, options)
		if err != nil {
			unavailable("it cannot be read")
			continue
		}
		found := false
		for _, file := range set.Files {
			if file.Path == pin.Path {
				facts = append(facts, selectedFact(agent, pointers, file))
				found = true
			}
		}
		if !found {
			unavailable("the file is missing")
		}
	}
	return facts
}

func classByID(id string) (instructions.Class, bool) {
	if id == "project-private" {
		return instructions.ProjectPrivateClass(), true
	}
	for _, class := range instructions.DefaultClasses() {
		if class.ID == id {
			return class, true
		}
	}
	return instructions.Class{}, false
}

// instructionsCommand serves /workbench instructions pin|unpin|ls.
func (a *app) instructionsCommand(ctx sdk.Context, args string) error {
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	rest = strings.TrimSpace(rest)
	var result string
	var err error
	switch verb {
	case "pin":
		class := ""
		if before, after, found := strings.Cut(rest, "--class"); found {
			rest, class = strings.TrimSpace(before), strings.TrimSpace(after)
		}
		result, err = a.pinInstruction(ctx, class, rest, "user")
	case "unpin":
		result, err = a.unpinInstruction(ctx, rest)
	case "":
		if panelsAvailable(ctx) {
			return a.instructionsPanel(ctx)
		}
		result, err = a.listInstructionPins()
	case "ls":
		result, err = a.listInstructionPins()
	case "rules":
		result, err = a.instructionRules(ctx)
	case "reload":
		result, err = a.reloadInstructions()
	default:
		return errors.New("usage: /workbench instructions pin <path> [--class <id>] | unpin <path> | ls | rules | reload")
	}
	if err != nil {
		return err
	}
	a.notify(ctx, result)
	return nil
}

func (a *app) listInstructionPins() (string, error) {
	s, err := a.current()
	if err != nil {
		return "", err
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.pins) == 0 {
		return "No pinned instruction files.", nil
	}
	lines := make([]string, len(state.pins))
	for i, pin := range state.pins {
		lines[i] = pin.Class + "  " + pin.Path + "  (" + pin.Origin + ")"
	}
	return "Pinned instruction files:\n" + strings.Join(lines, "\n"), nil
}

// instructionsControl applies a write to session://self/instructions/ctl: `pin <path> [--class <id>]` or `unpin <path>`.
func (a *app) instructionsControl(ctx sdk.Context, uri, content string) (string, error) {
	if strings.TrimRight(uri, "/") != "session://self/instructions/ctl" {
		return "", errors.New("session:// accepts writes only at session://self/instructions/ctl")
	}
	verb, rest, _ := strings.Cut(strings.TrimSpace(content), " ")
	rest = strings.TrimSpace(rest)
	switch verb {
	case "pin":
		class := ""
		if before, after, found := strings.Cut(rest, "--class"); found {
			rest, class = strings.TrimSpace(before), strings.TrimSpace(after)
		}
		return a.pinInstruction(ctx, class, rest, "agent")
	case "unpin":
		return a.unpinInstruction(ctx, rest)
	}
	return "", errors.New("instructions ctl accepts `pin <path> [--class <id>]` or `unpin <path>`")
}

// instructionRules reports the class policies and what discovery selects for the current location.
func (a *app) instructionRules(ctx sdk.Context) (string, error) {
	s, err := a.current()
	if err != nil {
		return "", err
	}
	var lines []string
	classes := append(instructions.DefaultClasses(), instructions.ProjectPrivateClass())
	for _, class := range classes {
		lines = append(lines, fmt.Sprintf("class %s: %s, placement %s, select %s, role %s, pin %t", class.ID, strings.Join(class.Filenames, ", "), class.Placement, class.Select, class.Role, class.AllowPin))
	}
	current := s.location.Current()
	agent, err := sdk.AgentDir()
	if err != nil {
		return "", err
	}
	privateRoot, err := sessionPrivateRoot(ctx, s)
	if err != nil {
		return "", err
	}
	if dir := s.settingString("instructions.system-dir", "/etc/agents"); dir != "" {
		host, hostErr := instructions.ResolveHost(requestContext{ctx}, instructions.LocalHost(dir))
		lines = append(lines, "Host files (always on, "+dir+"):")
		switch {
		case hostErr != nil:
			lines = append(lines, "  unavailable: "+hostErr.Error())
		case len(host.Files) == 0 && len(host.Reasons) == 0:
			lines = append(lines, "  none present")
		}
		for _, file := range host.Files {
			lines = append(lines, "  "+file.Class+"  "+file.Path)
		}
		for _, reason := range host.Reasons {
			if reason.Status == "refused" {
				lines = append(lines, "  refused  "+reason.DisplayPath+": "+reason.Detail)
			}
		}
	}
	set, err := instructions.Discover(requestContext{ctx}, current.Cwd, nil, instructions.Options{ProjectRoot: current.ProjectRoot, UserRoots: []string{agent}, PrivateRoot: privateRoot, AllowNested: false})
	if err != nil {
		return strings.Join(lines, "\n") + "\nDiscovery for " + current.Cwd + " failed: " + err.Error(), nil
	}
	lines = append(lines, "Selected for "+current.Cwd+":")
	for _, file := range set.Files {
		note := ""
		if file.Truncated {
			note = " (capped)"
		}
		lines = append(lines, "  "+file.Class+"  "+file.Path+note)
	}
	if len(set.Files) == 0 {
		lines = append(lines, "  none")
	}
	return strings.Join(lines, "\n"), nil
}

// reloadInstructions forgets what was delivered so the next boundary sends every instruction view in full.
func (a *app) reloadInstructions() (string, error) {
	s, err := a.current()
	if err != nil {
		return "", err
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	for key := range state.last {
		if strings.HasPrefix(key, "instructions.") {
			delete(state.last, key)
		}
	}
	clear(state.views)
	return "Instruction views will be delivered again in full at the next boundary.", nil
}

func (a *app) instructionsPanel(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	title, _ := a.hubTitle.Load().(string)
	if title == "" {
		title = "Workbench › Instructions"
	}
	panel := &instructionsPanel{
		title:  title,
		height: ctx.Height,
		pins: func() []instructionPin {
			s.contextFrames.mu.Lock()
			defer s.contextFrames.mu.Unlock()
			return append([]instructionPin(nil), s.contextFrames.pins...)
		},
		rules: func() (string, error) { return a.instructionRules(ctx) },
		pin:   func(path string) (string, error) { return a.pinInstruction(ctx, "", path, "user") },
		unpin: func(path string) (string, error) { return a.unpinInstruction(ctx, path) },
	}
	_, err = ctx.Custom(panel, nil)
	return err
}
