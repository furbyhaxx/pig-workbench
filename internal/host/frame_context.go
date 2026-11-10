package host

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/messaging"
	"github.com/furbyhaxx/pig-workbench/internal/modelknowledge"
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
	// retained is the most recent completed explicit instruction target set (SPEC-0007 §5); the next
	// prompt keeps those scopes so a nested rule is not withdrawn and re-delivered every turn.
	retained         []string
	retainedLocation uint64
	retainedEpoch    string
}

// retainedFor returns the retained explicit targets while the location and context epoch are unchanged.
func (c *contextFrameState) retainedFor(location uint64, epoch string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retainedLocation != location || c.retainedEpoch != epoch {
		return nil
	}
	return slices.Clone(c.retained)
}

// frameNonceEntry persists the frame nonce shared by a session, its forks and its branches.
const frameNonceEntry = "pig-workbench.nonce"

func restoreFrameLedger(ctx sdk.Context, id string) (*contextFrameState, error) {
	entries, err := ctx.GetEntries()
	if err != nil {
		return nil, err
	}
	ledger, lineageNonce, err := ledgerFromEntries(entries, id)
	if err != nil {
		return nil, err
	}
	// Forks and tree branches copy session entries, so one persisted nonce marks the whole lineage.
	if lineageNonce == "" {
		lineageNonce = ledger.Nonce()
		if err := ctx.AppendEntry(frameNonceEntry, map[string]any{"schemaVersion": 1, "nonce": lineageNonce}); err != nil {
			return nil, err
		}
	} else if err := ledger.SetNonce(lineageNonce); err != nil {
		return nil, err
	}
	return &contextFrameState{ledger: ledger, last: make(map[string]string), views: make(map[string]string), staged: make(map[string]stagedView), wakes: messaging.NewLimiter()}, nil
}

// ledgerFromEntries rebuilds the ledger from the newest stored record of each frame and returns the lineage nonce, if one was stored.
func ledgerFromEntries(entries []json.RawMessage, id string) (*frames.Session, string, error) {
	latest := make(map[string]json.RawMessage)
	lineageNonce := ""
	for _, raw := range entries {
		var entry struct {
			Type       string          `json:"type"`
			CustomType string          `json:"customType"`
			Data       json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, "", err
		}
		if entry.Type == "custom" && entry.CustomType == frameNonceEntry && lineageNonce == "" {
			var stored struct {
				Nonce string `json:"nonce"`
			}
			if json.Unmarshal(entry.Data, &stored) == nil && frames.ValidNonce(stored.Nonce) {
				lineageNonce = stored.Nonce
			}
			continue
		}
		if entry.Type != "custom" || entry.CustomType != "pig-workbench.frame" {
			continue
		}
		var record struct {
			Frame frames.Frame `json:"frame"`
		}
		if err := json.Unmarshal(entry.Data, &record); err != nil {
			return nil, "", err
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
		return nil, "", err
	}
	return ledger, lineageNonce, nil
}

type contextFact struct {
	source                    string
	value                     any
	resource, kind, authority string
	incomplete                bool
	revision                  string
}

// contextFrames is the before_agent_start handler: each pending frame is its own session entry.
func (a *app) contextFrames(ctx sdk.Context, data map[string]any) (any, error) {
	result, err := a.collectContextFrames(ctx, data)
	if err != nil {
		return nil, err
	}
	output, _ := result.(map[string]any)
	return output, sendFrameBlocks(ctx, takeFrameBlocks(output))
}

// takeFrameBlocks removes and returns the encoded frames a collection selected.
func takeFrameBlocks(result map[string]any) []string {
	blocks, _ := result["workbenchFrameBlocks"].([]string)
	delete(result, "workbenchFrameBlocks")
	return blocks
}

func sendFrameBlocks(ctx sdk.Context, blocks []string) error {
	for _, block := range blocks {
		if err := ctx.SendMessage("pig-workbench.context", block, true, sdk.SendMessageOptions{DeliverAs: "steer", TriggerTurn: sdk.Bool(false)}); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) collectContextFrames(ctx sdk.Context, data map[string]any) (any, error) {
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
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	var contributedFields []contributedFields
	if s.producers != nil {
		contributedFields = s.producers.modelFields(modelBinding{Provider: model.Provider, ID: model.ID})
	}
	thinking, _ := ctx.GetThinkingLevel()
	modelFacts := modelContextFacts(model, thinking, configuration, contributedFields)
	facts := []contextFact{
		{source: "session.date", value: map[string]any{"date": time.Now().Format("2006-01-02"), "timezone": time.Now().Format("MST -07:00")}},
		{source: "session.environment", value: map[string]any{"os": runtime.GOOS, "architecture": runtime.GOARCH, "hostCwd": ctx.Cwd()}},
		{source: "session.project", value: projectFacts},
		{source: "session.location", value: locationSentence(current)},
		{source: "session.model", value: modelFacts},
		{source: "session.capabilities", value: map[string]any{"activeTools": tools, "schemes": s.router.Schemes()}},
	}
	if missing := missingContextSources(facts); len(missing) > 0 {
		return nil, fmt.Errorf("before-prompt delivery is missing %s", strings.Join(missing, ", "))
	}
	s.editState.mu.Lock()
	if data["workbenchToolContext"] != true || s.editState.mode == "" {
		entry, _ := configuration.Get("edit.mode")
		s.editState.mode = entry.Value.(string)
	}
	editMode := s.editState.mode
	s.editState.mu.Unlock()
	a.syncEditText(ctx, editMode)
	for _, fact := range facts {
		if fact.source == "session.capabilities" {
			capabilities := fact.value.(map[string]any)
			capabilities["editMode"] = editMode
			if s.settingString("skills.listing", "prompt") == "frame" {
				if listed, _, err := s.listedSkills(); err == nil && len(listed) > 0 {
					capabilities["skills"] = listed
				}
			}
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
			if data["workbenchToolContext"] != true {
				targets = append(targets, s.contextFrames.retainedFor(current.Revision, epoch)...)
			}
		}
		// The cwd-derived scope stays active beside any tool target (SPEC-0007 §4): touching a path in
		// another repository must not withdraw this project's root instructions.
		if data["workbenchToolContext"] == true && !slices.Contains(targets, current.Cwd) {
			targets = append(targets, current.Cwd)
		}
		seen := make(map[string]bool)
		if dir, ok := configuration.Get("instructions.system-dir"); ok {
			agentDir, err := sdk.AgentDir()
			if err != nil {
				return nil, err
			}
			system, err := hostFacts(ctx, agentDir, dir.Value.(string))
			if err != nil {
				return nil, err
			}
			for _, fact := range system {
				seen[fact.source+"/"+fact.resource] = true
				facts = append(facts, fact)
			}
		}
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
	explicitTargets, _ := data["workbenchInstructionTargets"].([]string)
	if len(explicitTargets) > 0 {
		state.retained, state.retainedLocation, state.retainedEpoch = explicitTargets, current.Revision, epoch
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
		// A source the model already holds is never sent whole again: only what changed goes out.
		instruction := strings.HasPrefix(fact.source, "instructions.") && fact.kind != "notice"
		if previous, ok := state.views[key]; ok {
			if instruction {
				if diff := instructionDiff(fact, previous, fullBody, updateContext(configuration)); diff != "" {
					body = []byte(diff)
				}
			} else if fact.source == "session.location" {
				was, _, _ := strings.Cut(strings.TrimPrefix(previous, locationIntro), "; ")
				now, rest, _ := strings.Cut(strings.TrimPrefix(fullBody, locationIntro), "; ")
				body, kind = []byte(locationChangedIntro+was+" to "+now+"; "+strings.Replace(rest, "cwd resolve", "cwd now resolve", 1)), "notice"
			} else if change := factChange(fact.source, previous, fullBody); change != "" {
				body, kind = []byte(change), "notice"
			}
		}
		ledger := state.ledger.Ledger(frames.Context{ProducerPrefix: prefix, BranchLeaf: state.scope.BranchLeaf, Epoch: epoch, Trusted: true, Authority: authority})
		state.nextEvent++
		receipt, err := ledger.Admit(frames.Publish{EventKey: fmt.Sprintf("%s/%s/%s/%d", epoch, fact.source, digest, state.nextEvent), Kind: kind, Source: fact.source, Resource: fact.resource, SubjectKey: fact.resource, Body: string(body), Delivery: delivery, DataTrust: "trusted", CoalesceKey: key})
		if err != nil {
			return nil, err
		}
		admitted = append(admitted, receipt.FrameID)
		if instruction || fact.kind == "" {
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
	result["workbenchFrameBlocks"] = blocks
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

const (
	locationIntro        = "Working location: "
	locationChangedIntro = "Working location changed from "
)

// locationSentence states where plain paths resolve; a non-local authority prefixes the directory.
func locationSentence(current location.Snapshot) string {
	where := current.DisplayCwd
	if where == "" {
		where = current.Cwd
	}
	if current.Authority != "" && current.Authority != "local" {
		where = current.Authority + ":" + where
	}
	if !current.Available {
		return locationIntro + where + "; it is currently unavailable; use file:///... for local files."
	}
	return locationIntro + where + "; plain paths and cwd resolve there; use file:///... for local files."
}

const factChangeIntro = "Changed in "

// factChange lists the fields of a JSON fact that differ from the delivered view as "field: old -> new"
// lines, lists as added and removed members. It returns "" when either view is not a JSON object.
func factChange(source, previous, current string) string {
	var before, after map[string]any
	if json.Unmarshal([]byte(previous), &before) != nil || json.Unmarshal([]byte(current), &after) != nil {
		return ""
	}
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	for name := range before {
		if _, kept := after[name]; !kept {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	compact := func(value any, present bool) string {
		if !present {
			return "(unset)"
		}
		if text, ok := value.(string); ok {
			return text
		}
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	var lines []string
	for _, name := range names {
		old, had := before[name]
		now, has := after[name]
		if had && has && reflect.DeepEqual(old, now) {
			continue
		}
		oldList, wasList := old.([]any)
		nowList, isList := now.([]any)
		if wasList && isList {
			var parts []string
			for _, item := range nowList {
				if !slices.ContainsFunc(oldList, func(other any) bool { return reflect.DeepEqual(other, item) }) {
					parts = append(parts, "+"+compact(item, true))
				}
			}
			for _, item := range oldList {
				if !slices.ContainsFunc(nowList, func(other any) bool { return reflect.DeepEqual(other, item) }) {
					parts = append(parts, "-"+compact(item, true))
				}
			}
			if len(parts) == 0 {
				parts = []string{"reordered"}
			}
			lines = append(lines, name+": "+strings.Join(parts, ", "))
			continue
		}
		lines = append(lines, name+": "+compact(old, had)+" -> "+compact(now, has))
	}
	if len(lines) == 0 {
		return ""
	}
	return factChangeIntro + source + "; every other field is unchanged.\n" + strings.Join(lines, "\n")
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

// modelContextFacts builds the session.model data: core facts, the knowledge cutoff and its guidance.
// The cutoff comes from the user's setting, else a contributed explicit value, else the bundled
// models.dev snapshot, else an estimate labelled "estimated" (release date minus one year); when no
// release date is known either it is omitted. Nothing here depends on today's
// date, so the frame stays unchanged between days.
// modelContextFacts states the thinking level only for a model that reasons; "off" is a level.
func modelContextFacts(model *sdk.ModelInfo, thinking string, configuration *config.Snapshot, contributed []contributedFields) map[string]any {
	facts := map[string]any{"provider": model.Provider, "id": model.ID, "name": model.Name}
	if model.Reasoning && thinking != "" {
		facts["thinking"] = thinking
	}
	if model.ContextWindow > 0 {
		facts["contextWindow"] = model.ContextWindow
	}
	if model.MaxOutputTokens > 0 {
		facts["maxOutputTokens"] = model.MaxOutputTokens
	}
	if cutoff, ok := modelknowledge.Lookup(model.Provider, model.ID); ok {
		facts["knowledge_cutoff"] = map[string]any{"date": cutoff.Date, "basis": cutoff.Basis, "source": cutoff.Source}
	}
	for _, fields := range contributed {
		for name, value := range fields.fields {
			facts[name] = value
		}
	}
	if entry, ok := configuration.Get("models.knowledge-cutoffs"); ok {
		if entries, ok := entry.Value.([]string); ok {
			cutoff, found, err := modelknowledge.ParseSetting(entries, model.Provider, model.ID)
			if err != nil {
				// A typo in a setting must not block the user's prompt; the frame names the ignored entry.
				facts["knowledge_cutoff_setting_error"] = "models.knowledge-cutoffs ignored: " + err.Error()
			} else if found {
				facts["knowledge_cutoff"] = map[string]any{"date": cutoff.Date, "basis": cutoff.Basis, "source": cutoff.Source}
			}
		}
	}
	if _, ok := facts["knowledge_cutoff"]; !ok {
		if cutoff, ok := modelknowledge.Estimate(model.Provider, model.ID); ok {
			facts["knowledge_cutoff"] = map[string]any{"date": cutoff.Date, "basis": cutoff.Basis, "source": cutoff.Source}
		}
	}
	facts["training_data_notice"] = modelknowledge.Guidance
	return facts
}
