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
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

type contextFrameState struct {
	mu        sync.Mutex
	ledger    *frames.Session
	last      map[string]string
	scope     frames.DeliveryScope
	nextEvent uint64
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
	return &contextFrameState{ledger: ledger, last: make(map[string]string)}, nil
}

type contextFact struct {
	source                    string
	value                     any
	resource, kind, authority string
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
	facts := []contextFact{
		{source: "session.date", value: map[string]any{"date": time.Now().Format("2006-01-02"), "timezone": time.Now().Format("MST -07:00")}},
		{source: "session.environment", value: map[string]any{"os": runtime.GOOS, "architecture": runtime.GOARCH, "hostCwd": ctx.Cwd()}},
		{source: "session.project", value: projectFacts},
		{source: "session.location", value: current},
		{source: "session.model", value: modelFacts},
		{source: "session.capabilities", value: map[string]any{"activeTools": tools, "schemes": s.router.Schemes()}},
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
	if enabled.Value == true {
		target := current.Cwd
		if path, ok := data["workbenchInstructionTarget"].(string); ok {
			target = path
		}
		selected, err := instructionFacts(ctx, target, root, projectID)
		if err != nil {
			return nil, err
		}
		facts = append(facts, selected...)
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.scope.Epoch != epoch {
		for _, raw := range state.ledger.Snapshot() {
			var record struct {
				State string       `json:"state"`
				Frame frames.Frame `json:"frame"`
			}
			if json.Unmarshal(raw, &record) == nil && record.State == "pending" {
				_ = state.ledger.Cancel(record.Frame.FrameID)
			}
		}
		clear(state.last)
		leaf, err := ctx.GetLeafID()
		if err != nil {
			return nil, err
		}
		state.scope = frames.DeliveryScope{Epoch: epoch}
		if leaf != nil {
			state.scope.BranchLeaf = *leaf
		}
	}
	currentInstructionKeys := make(map[string]bool)
	for _, fact := range facts {
		if strings.HasPrefix(fact.source, "instructions.") {
			currentInstructionKeys[fact.source+"/"+fact.resource] = true
		}
	}
	var withdrawals []string
	for key := range state.last {
		if strings.HasPrefix(key, "instructions.") && !currentInstructionKeys[key] {
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
		ledger := state.ledger.Ledger(frames.Context{ProducerPrefix: prefix, BranchLeaf: state.scope.BranchLeaf, Epoch: epoch, Trusted: true, Authority: authority})
		state.nextEvent++
		receipt, err := ledger.Admit(frames.Publish{EventKey: fmt.Sprintf("%s/%s/%s/%d", epoch, fact.source, digest, state.nextEvent), Kind: kind, Source: fact.source, Resource: fact.resource, SubjectKey: fact.resource, Body: string(body), Delivery: delivery, DataTrust: "trusted", CoalesceKey: key})
		if err != nil {
			return nil, err
		}
		admitted = append(admitted, receipt.FrameID)
		state.last[key] = digest
	}
	if err := persistFrames(ctx, state.ledger, admitted); err != nil {
		return nil, err
	}
	pending, err := state.ledger.SelectPending(delivery, state.scope)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if enabled.Value == true {
		if options, ok := data["systemPromptOptions"].(map[string]any); ok {
			options["contextFiles"] = []any{}
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
	pending, err := state.ledger.SelectPending("beforePrompt", state.scope)
	if err != nil {
		return nil, err
	}
	steer, err := state.ledger.SelectPending("nextBoundary", state.scope)
	if err != nil {
		return nil, err
	}
	pending = append(pending, steer...)
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
	return nil, persistFrames(ctx, state.ledger, delivered)
}
