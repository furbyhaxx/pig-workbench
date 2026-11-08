package host

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/messaging"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

type stagedView struct{ key, full string }

type contextFrameState struct {
	mu     sync.Mutex
	ledger *frames.Session
	last   map[string]string
	// views holds the full instruction text the model has actually received, keyed like last; it is
	// the only valid base for a diff. staged holds admitted full views until their frame is delivered.
	views     map[string]string
	pins      []instructionPin
	staged    map[string]stagedView
	scope     frames.DeliveryScope
	nextEvent uint64
	// steered holds frames already sent as a boundary steer but not yet confirmed in context,
	// so concurrent tool calls cannot send the same pending frame twice.
	steered map[string]bool
	// wakes bounds how often each external producer may start a turn.
	wakes *messaging.Limiter
}

func restoreFrameLedger(ctx sdk.Context, id string) (*contextFrameState, error) {
	entries, err := ctx.GetEntries()
	if err != nil {
		return nil, err
	}
	latest := make(map[string]json.RawMessage)
	for _, raw := range entries {
		var entry struct {
			Type       string          `json:"type"`
			CustomType string          `json:"customType"`
			Data       json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if entry.Type != "custom" || entry.CustomType != "pig-workbench.frame" {
			continue
		}
		var record struct {
			Frame frames.Frame `json:"frame"`
		}
		if err := json.Unmarshal(entry.Data, &record); err != nil {
			return nil, err
		}
		if record.Frame.SessionID == id && record.Frame.FrameID != "" {
			latest[record.Frame.FrameID] = entry.Data
		}
	}
	records := make([]json.RawMessage, 0, len(latest))
	for _, record := range latest {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		var left, right struct {
			Frame frames.Frame `json:"frame"`
		}
		_ = json.Unmarshal(records[i], &left)
		_ = json.Unmarshal(records[j], &right)
		return left.Frame.Sequence < right.Frame.Sequence
	})
	ledger := frames.NewSession(id)
	if err := ledger.Restore(records); err != nil {
		return nil, err
	}
	return &contextFrameState{ledger: ledger, last: make(map[string]string), views: make(map[string]string), staged: make(map[string]stagedView), wakes: messaging.NewLimiter()}, nil
}

type contextFact struct {
	source                    string
	value                     any
	resource, kind, authority string
	incomplete                bool
	revision                  string
}

func (a *app) contextFrames(ctx sdk.Context, data map[string]any) (any, error) {
	s, _, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	epoch := s.snapshots.Epoch()
	delivery := "beforePrompt"
	if data["workbenchToolContext"] == true {
		delivery = "nextBoundary"
	}
	model, err := ctx.GetModelInfo()
	if err != nil {
		return nil, err
	}
	tools, err := ctx.GetActiveTools()
	if err != nil {
		return nil, err
	}
	sort.Strings(tools)
	current := s.location.Current()
	root := current.ProjectRoot
	projectID := "global"
	projectFacts := any(map[string]any{"id": "global", "root": current.ProjectRoot})
	if agent, err := sdk.AgentDir(); err == nil {
		registry, err := project.Open(agent)
		if err != nil {
			return nil, err
		}
		detected, err := registry.Detect(requestContext{ctx}, current.Cwd)
		if err != nil {
			return nil, err
		}
		projectFacts = map[string]any{"id": detected.ProjectID, "slug": detected.Slug, "name": detected.Name, "root": detected.Root, "recognizedBy": detected.How}
		root = detected.Root
		projectID = detected.ProjectID
	}
	modelFacts := map[string]any{"provider": model.Provider, "id": model.ID, "name": model.Name, "reasoning": model.Reasoning}
	if model.ContextWindow > 0 {
		modelFacts["contextWindow"] = model.ContextWindow
	}
	if model.MaxOutputTokens > 0 {
		modelFacts["maxOutputTokens"] = model.MaxOutputTokens
	}
	if s.producers != nil {
		for _, contributed := range s.producers.modelFields(modelBinding{Provider: model.Provider, ID: model.ID}) {
			for name, value := range contributed.fields {
				modelFacts[name] = value
			}
		}
	}
	facts := []contextFact{
		{source: "session.date", value: map[string]any{"date": time.Now().Format("2006-01-02"), "timezone": time.Now().Format("MST -07:00")}},
		{source: "session.environment", value: map[string]any{"os": runtime.GOOS, "architecture": runtime.GOARCH, "hostCwd": ctx.Cwd()}},
		{source: "session.project", value: projectFacts},
		{source: "session.location", value: current},
		{source: "session.model", value: modelFacts},
		{source: "session.capabilities", value: map[string]any{"activeTools": tools, "schemes": s.router.Schemes()}},
	}
	if missing := missingContextSources(facts); len(missing) > 0 {
		return nil, fmt.Errorf("before-prompt delivery is missing %s", strings.Join(missing, ", "))
	}
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	s.editState.mu.Lock()
	if data["workbenchToolContext"] != true || s.editState.mode == "" {
		entry, _ := configuration.Get("edit.mode")
		s.editState.mode = entry.Value.(string)
	}
	editMode := s.editState.mode
	s.editState.mu.Unlock()
	for _, fact := range facts {
		if fact.source == "session.capabilities" {
			fact.value.(map[string]any)["editMode"] = editMode
		}
	}
	enabled, _ := configuration.Get("instructions.enabled")
	validate, _ := configuration.Get("instructions.backlog.validate-pointers")
	validating, _ := validate.Value.(bool)
	privateRoot := ""
	if enabled.Value == true {
		targets, _ := data["workbenchInstructionTargets"].([]string)
		if len(targets) == 0 {
			targets = []string{current.Cwd}
		}
		seen := make(map[string]bool)
		for _, target := range targets {
			selected, activePrivate, err := instructionFacts(ctx, target, root, projectID, validating, func(dir string) bool { return trustAllows(s.settings, dir) })
			if err != nil {
				return nil, err
			}
			if activePrivate != "" {
				privateRoot = activePrivate
			}
			for _, fact := range selected {
				key := fact.source + "/" + fact.resource
				if !seen[key] {
					seen[key] = true
					facts = append(facts, fact)
				}
			}
		}
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	if enabled.Value == true {
		agent, err := sdk.AgentDir()
		if err != nil {
			return nil, err
		}
		have := make(map[string]bool)
		for _, fact := range facts {
			have[fact.source+"/"+fact.resource] = true
		}
		for _, fact := range pinFacts(ctx, agent, privateRoot, pointerRoot(validating, root), state.pins) {
			if !have[fact.source+"/"+fact.resource] {
				facts = append(facts, fact)
			}
		}
	}
	scope, err := branchScope(ctx, epoch)
	if err != nil {
		return nil, err
	}
	if state.scope.Epoch != epoch {
		// Frames already bound to the current branch (published before this boundary) stay pending.
		for _, raw := range state.ledger.Snapshot() {
			var record struct {
				State string       `json:"state"`
				Frame frames.Frame `json:"frame"`
			}
			if json.Unmarshal(raw, &record) == nil && record.State == "pending" && !scope.Matches(record.Frame.BranchLeaf, record.Frame.Epoch) {
				_ = state.ledger.Cancel(record.Frame.FrameID)
			}
		}
		entries, err := ctx.GetEntries()
		if err != nil {
			return nil, err
		}
		branch, err := ctx.GetBranch()
		if err != nil {
			return nil, err
		}
		baseline, err := frameBaseline(entries, branch)
		if err != nil {
			return nil, err
		}
		state.last = baseline
		state.pins = pinsFromBranch(entries, branch)
		state.views = make(map[string]string)
		state.staged = make(map[string]stagedView)
	}
	state.scope = scope
	currentInstructionKeys := make(map[string]bool)
	for _, fact := range facts {
		if strings.HasPrefix(fact.source, "instructions.") {
			currentInstructionKeys[fact.source+"/"+fact.resource] = true
		}
	}
	var withdrawals []string
	explicitTargets, _ := data["workbenchInstructionTargets"].([]string)
	// A tool call without its own file targets (codemode, bash) only adds cwd-derived scope; it must not
	// withdraw what an explicit target selected, or a nested call would lose its own instructions.
	keepTouched := data["workbenchToolContext"] == true && len(explicitTargets) == 0
	for key := range state.last {
		if strings.HasPrefix(key, "instructions.") && !currentInstructionKeys[key] && !keepTouched {
			withdrawals = append(withdrawals, key)
		}
	}
	sort.Strings(withdrawals)
	for _, key := range withdrawals {
		source, path, _ := strings.Cut(key, "/")
		facts = append(facts, contextFact{source: source, resource: path, kind: "notice", authority: "data", value: "This instruction source no longer applies to the active tool target: " + path})
	}
	var admitted []string
	for _, fact := range facts {
		body, err := json.MarshalIndent(fact.value, "", "  ")
		if text, ok := fact.value.(string); ok {
			body = []byte(text)
		}
		if err != nil {
			return nil, err
		}
		digest := hashline.SourceRevision(body)
		key := fact.source + "/" + fact.resource
		if state.last[key] == digest {
			continue
		}
		prefix, _, _ := strings.Cut(fact.source, ".")
		kind, authority := fact.kind, fact.authority
		if kind == "" {
			kind = "info"
		}
		if authority == "" {
			authority = "data"
		}
		fullBody := string(body)
		if strings.HasPrefix(fact.source, "instructions.") && fact.kind != "notice" {
			if previous, ok := state.views[key]; ok {
				if diff := instructionDiff(fact, previous, fullBody, updateContext(configuration)); diff != "" {
					body = []byte(diff)
				}
			}
		}
		ledger := state.ledger.Ledger(frames.Context{ProducerPrefix: prefix, BranchLeaf: state.scope.BranchLeaf, Epoch: epoch, Trusted: true, Authority: authority})
		state.nextEvent++
		receipt, err := ledger.Admit(frames.Publish{EventKey: fmt.Sprintf("%s/%s/%s/%d", epoch, fact.source, digest, state.nextEvent), Kind: kind, Source: fact.source, Resource: fact.resource, SubjectKey: fact.resource, Body: string(body), Delivery: delivery, DataTrust: "trusted", CoalesceKey: key})
		if err != nil {
			return nil, err
		}
		admitted = append(admitted, receipt.FrameID)
		if strings.HasPrefix(fact.source, "instructions.") && fact.kind != "notice" {
			state.staged[receipt.FrameID] = stagedView{key: key, full: fullBody}
		}
		state.last[key] = digest
	}
	if err := persistFrames(ctx, state.ledger, admitted); err != nil {
		return nil, err
	}
	pending, err := state.ledger.SelectPending(delivery, state.scope)
	if err != nil {
		return nil, err
	}
	if delivery == "beforePrompt" {
		prompt, err := state.ledger.SelectPending("nextPrompt", state.scope)
		if err != nil {
			return nil, err
		}
		pending = append(pending, prompt...)
	}
	if delivery == "beforePrompt" {
		state.steered = nil
	} else {
		if state.steered == nil {
			state.steered = make(map[string]bool)
		}
		fresh := pending[:0:0]
		for _, frame := range pending {
			if !state.steered[frame.FrameID] {
				state.steered[frame.FrameID] = true
				fresh = append(fresh, frame)
			}
		}
		pending = fresh
	}
	result := map[string]any{}
	if data["workbenchToolContext"] == true {
		var incomplete []string
		for _, fact := range facts {
			if fact.incomplete && !s.instructionFullySeen(fact) {
				incomplete = append(incomplete, fact.resource)
			}
		}
		if len(incomplete) > 0 {
			result["workbenchIncompleteInstructions"] = incomplete
		}
	}
	if enabled.Value == true {
		if options, ok := data["systemPromptOptions"].(map[string]any); ok {
			options["contextFiles"] = []any{}
			deliveryMode := "auto"
			if configuration, err := s.settings.Current(); err == nil {
				if entry, ok := configuration.Get("prompt.delivery"); ok {
					if value, ok := entry.Value.(string); ok {
						deliveryMode = value
					}
				}
			}
			promptEnabled := false
			if configuration, err := s.settings.Current(); err == nil {
				if entry, ok := configuration.Get("prompt.enabled"); ok {
					promptEnabled, _ = entry.Value.(bool)
				}
			}
			rendered, renderErr := renderBuiltinPrompt(s, decodePromptOptions(options))
			text, force, recorded := applyPromptRender(promptEnabled, rendered, renderErr, deliveryMode)
			s.promptErr = recorded
			if renderErr == nil {
				s.promptText = rendered
				s.promptEpoch = promptEpoch(rendered)
			}
			if force {
				options["forceSystemPrompt"] = text
			}
			result["systemPromptOptions"] = options
		}
	}
	if len(pending) == 0 {
		return result, nil
	}
	var blocks []string
	for _, frame := range pending {
		encoded, err := frames.EncodeEnvelope(frame)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, encoded)
	}
	result["message"] = map[string]any{"customType": "pig-workbench.context", "content": strings.Join(blocks, "\n\n"), "display": true}
	return result, nil
}

func persistFrames(ctx sdk.Context, ledger *frames.Session, ids []string) error {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	for _, record := range ledger.Snapshot() {
		var header struct {
			Frame frames.Frame `json:"frame"`
		}
		if err := json.Unmarshal(record, &header); err != nil {
			return err
		}
		if selected[header.Frame.FrameID] {
			if err := ctx.AppendEntry("pig-workbench.frame", record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *app) confirmContextFrames(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.scope.Epoch != s.snapshots.Epoch() {
		return nil, nil
	}
	scope, err := branchScope(ctx, state.scope.Epoch)
	if err != nil {
		return nil, err
	}
	state.scope = scope
	pending, err := state.ledger.SelectPending("beforePrompt", state.scope)
	if err != nil {
		return nil, err
	}
	steer, err := state.ledger.SelectPending("nextBoundary", state.scope)
	if err != nil {
		return nil, err
	}
	pending = append(pending, steer...)
	prompt, err := state.ledger.SelectPending("nextPrompt", state.scope)
	if err != nil {
		return nil, err
	}
	pending = append(pending, prompt...)
	messages, _ := data["messages"].([]any)
	var delivered []string
	for _, frame := range pending {
		encoded, err := frames.EncodeEnvelope(frame)
		if err != nil {
			return nil, err
		}
		for _, raw := range messages {
			if message, ok := raw.(map[string]any); ok && containsTextBlock(message["content"], encoded) {
				delivered = append(delivered, frame.FrameID)
				break
			}
		}
	}
	if err := state.ledger.MarkDelivered(state.scope, delivered); err != nil {
		return nil, err
	}
	for _, id := range delivered {
		if view, ok := state.staged[id]; ok {
			state.views[view.key] = view.full
			delete(state.staged, id)
		}
	}
	return nil, persistFrames(ctx, state.ledger, delivered)
}

// branchScope binds delivery to the current context epoch and every entry on the active branch.
func branchScope(ctx sdk.Context, epoch string) (frames.DeliveryScope, error) {
	scope := frames.DeliveryScope{Epoch: epoch, Ancestors: map[string]bool{"": true}}
	leaf, err := ctx.GetLeafID()
	if err != nil {
		return scope, err
	}
	if leaf != nil {
		scope.BranchLeaf = *leaf
		scope.Ancestors[*leaf] = true
	}
	branch, err := ctx.GetBranch()
	if err != nil {
		return scope, err
	}
	for _, entry := range branch {
		if entry.ID != "" {
			scope.Ancestors[entry.ID] = true
		}
	}
	return scope, nil
}

var requiredContextSources = []string{
	"session.date",
	"session.environment",
	"session.project",
	"session.location",
	"session.model",
	"session.capabilities",
}

func missingContextSources(facts []contextFact) []string {
	have := map[string]bool{}
	for _, fact := range facts {
		have[fact.source] = true
	}
	var missing []string
	for _, source := range requiredContextSources {
		if !have[source] {
			missing = append(missing, source)
		}
	}
	return missing
}

func updateContext(configuration *config.Snapshot) int {
	entry, ok := configuration.Get("instructions.updates.context")
	if value, isInt := entry.Value.(int64); ok && isInt {
		return int(value)
	}
	return 3
}

// instructionDiff returns the update body for a changed instruction view, or "" to send the full view.
// A diff is used only when it is smaller and applies to the view the model received.
func instructionDiff(fact contextFact, previous, current string, context int) string {
	diff := instructions.UnifiedDiff(previous, current, context)
	if diff == "" {
		return ""
	}
	if rebuilt, err := instructions.ApplyDiff(previous, diff); err != nil || rebuilt != current {
		return ""
	}
	body := "Instruction update for " + fact.resource + ". Apply this diff to the view delivered earlier in this context; it replaces that view.\n" + diff
	if len(body) >= len(current) {
		return ""
	}
	return body
}
