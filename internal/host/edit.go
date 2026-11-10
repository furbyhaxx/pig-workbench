package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/syntax"
)

type editState struct {
	mode      string
	mu        sync.Mutex
	registers map[string]hashline.Register
	noops     *recent[int]
}
type editParams struct {
	Hashline string              `json:"hashline"`
	Path     string              `json:"path"`
	Edits    []hashline.TextEdit `json:"edits"`
}

func (a *app) registerEdit(ext *sdk.Extension) {
	ext.RegisterTool(a.editDefinition(editModeDefault))
}

// syncEditText re-registers edit when the session's effective mode differs from the text the model currently sees.
func (a *app) syncEditText(ctx sdk.Context, mode string) {
	a.editTextMu.Lock()
	defer a.editTextMu.Unlock()
	if mode == a.editTextMode || mode == "" {
		return
	}
	ctx.RegisterTool(a.editDefinition(mode))
	a.editTextMode = mode
}

// editModeDefault is the catalog default of edit.mode; the tool text starts here and is refreshed once the session's effective mode is known.
const editModeDefault = "hashline"

// editDefinition describes edit for the session's effective mode, so the model is told only the form that will be accepted.
func (a *app) editDefinition(mode string) sdk.ToolDefinition {
	return sdk.ToolDefinition{Name: "edit", Label: "edit", Description: editDescription(mode), PromptSnippet: "Apply tagged original-coordinate edits or unique exact-text replacements.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "properties": map[string]any{"hashline": map[string]any{"type": "string", "minLength": 1}, "path": map[string]any{"type": "string", "minLength": 1}, "edits": map[string]any{"type": "array", "minItems": 1, "maxItems": 128, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"oldText", "newText"}, "properties": map[string]any{"oldText": map[string]any{"type": "string", "minLength": 1}, "newText": map[string]any{"type": "string"}}}}}}, Execute: presented("edit", a.edit)}
}

const editHashlineText = `Edit a local UTF-8 file with a hashline batch: read the file first, then send {hashline} that starts with the header line copied from that read, [path#TAG], followed by operations. Line numbers refer to the file as you read it, and every payload row starts with +.
- PUT 4.=4: replace line 4; PUT 4.=6: replace lines 4-6 (a blank payload line is a lone +)
- PUT >5: insert after line 5; PUT <1: insert before line 1; PUT >$: append at the end
- CUT 7.=8 deletes lines 7-8 and keeps them in a register for PUT @name; PUT 6*: replaces the syntax block starting at line 6
- REM deletes the file; MV new/path moves it
Example:
[src/a.go#7F3A]
PUT 4.=4:
+	return a * b
`

const editReplaceText = `Edit a local UTF-8 file with exact replacements: {path, edits:[{oldText,newText}]}. Each oldText must match exactly once in the file and edits must not overlap.
`

const editSharedText = `Writes are atomic per file and rejected when the file changed since your read; a changed source is relocated only when its anchors are unambiguous, otherwise read it again. A partial I/O failure reports what landed (no rollback).`

// editDescription returns the model-facing edit text for one edit.mode value.
func editDescription(mode string) string {
	switch mode {
	case "replace":
		return editReplaceText + "Tagged {hashline} batches are not accepted in this session. " + editSharedText
	case "auto":
		return editHashlineText + "Alternatively send {path, edits:[{oldText,newText}]} with exact replacements: each oldText must match exactly once and edits must not overlap. There is no automatic fallback after a rejected patch. " + editSharedText
	}
	return editHashlineText + "Exact-text {path, edits} is not accepted in this session. " + editSharedText
}

func (a *app) edit(ctx sdk.Context, params map[string]any) (any, error) {
	var input editParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	_, tagged := params["hashline"]
	if tagged && (len(params) != 1 || input.Hashline == "") || !tagged && (len(params) != 2 || input.Path == "" || len(input.Edits) == 0 || len(input.Edits) > 128) {
		return nil, errors.New("edit accepts either {hashline} or {path, edits:[{oldText,newText}]}")
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	captured, done, err := s.location.Acquire(ctx.ToolCallID(), true)
	if err != nil {
		return nil, err
	}
	defer done()
	work = withCapturedLocation(work, captured)
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	epoch := s.snapshots.Epoch()
	s.editState.mu.Lock()
	defer s.editState.mu.Unlock()
	mode := s.editState.mode
	if mode == "" {
		entry, _ := configuration.Get("edit.mode")
		mode = entry.Value.(string)
	}
	if mode == "hashline" && !tagged {
		return nil, errors.New("this session's edit.mode is hashline: read the file, then send {hashline} with [path#TAG] copied from the read output; exact-text {path, edits} is not accepted here")
	}
	if mode == "replace" && tagged {
		return nil, errors.New("this session's edit.mode is replace: send {path, edits:[{oldText,newText}]}; tagged {hashline} batches are not accepted here")
	}
	var effects []fileedit.Effect
	var effectURIs []string
	var plan hashline.BatchPlan
	if tagged {
		document, err := hashline.Parse(input.Hashline)
		if err != nil {
			return nil, err
		}
		if len(document.Sections) > 32 {
			return nil, errors.New("an edit batch accepts at most 32 sections")
		}
		sources := make(map[string]hashline.Snapshot)
		currentSources := make(map[string]hashline.Snapshot)
		destinations := make(map[string]string)
		paths := make(map[string]string)
		for _, section := range document.Sections {
			target, err := localEditTarget(captured.Cwd, section.Path)
			if err != nil {
				return nil, err
			}
			snapshot, err := s.snapshots.Resolve(target.URI, section.Tag)
			if err != nil {
				return nil, err
			}
			currentRaw, err := readEditSource(work, target.Path)
			if err != nil {
				return nil, fmt.Errorf("read current source %s: %w", target.Path, err)
			}
			if limit := s.snapshotFileBytes(); int64(len(currentRaw)) > limit {
				return nil, fmt.Errorf("current source %s exceeds the %d-byte hashline snapshot limit (edit.snapshot.max-file-bytes); read a range and use exact-text edit", target.Path, limit)
			}
			sources[section.Path] = snapshot
			currentSources[section.Path] = hashline.Snapshot{URI: snapshot.URI, Revision: hashline.SourceRevision(currentRaw), Epoch: snapshot.Epoch, Raw: currentRaw}
			paths[snapshot.URI] = target.Path
			for _, op := range section.Ops {
				if op.Kind == hashline.MV {
					destination, err := localEditTarget(captured.Cwd, op.Dest)
					if err != nil {
						return nil, err
					}
					destinations[op.Dest] = destination.Path
				}
			}
		}
		enforce, _ := configuration.Get("edit.enforce-seen-lines")
		plan, err = hashline.PlanBatch(hashline.BatchInput{Document: document, Sources: sources, CurrentSources: currentSources, Epoch: epoch, EnforceSeen: enforce.Value == true, Registers: s.editState.registers, ResolveBlock: func(path string, source hashline.Normalized, line int) (hashline.LineSpan, error) {
			snapshot := sources[path]
			_, spans, err := a.syntax.Analyze(work, snapshot.Revision, paths[snapshot.URI], source.Bytes, syntax.DefaultFolding)
			if err != nil {
				return hashline.LineSpan{}, err
			}
			var found *hashline.LineSpan
			for _, span := range spans {
				if span.StartLine == line && span.EndLine > line {
					if found != nil || span.Ambiguous {
						return hashline.LineSpan{}, errors.New("block anchor is ambiguous; use an explicit range")
					}
					value := hashline.LineSpan{Start: span.StartLine, End: span.EndLine}
					found = &value
				}
			}
			if found == nil {
				return hashline.LineSpan{}, errors.New("no structural multiline block starts at that anchor; use an explicit range")
			}
			return *found, nil
		}})
		if err != nil {
			var relocation *hashline.RelocationError
			if errors.As(err, &relocation) {
				if context := currentSourceContext(work, s, ctx.ToolCallID(), captured.Cwd, epoch, relocation, currentSources[relocation.Path].Raw); context != "" {
					return nil, fmt.Errorf("%w\nCurrent tagged source around the affected anchor:\n%s\nRead the current source and retry with its coordinates", err, context)
				}
			}
			return nil, err
		}
		for _, effect := range plan.Effects {
			sourceURI := sources[effect.Path].URI
			staged := fileedit.Effect{Path: paths[sourceURI], Revision: effect.BeforeRevision, Bytes: effect.Bytes, Delete: effect.Delete}
			effectURIs = append(effectURIs, sourceURI)
			if effect.Delete {
				staged.Bytes = nil
			}
			if effect.MoveTo != "" {
				staged.MoveTo = destinations[effect.MoveTo]
			}
			effects = append(effects, staged)
		}
	} else {
		target, err := localEditTarget(captured.Cwd, input.Path)
		if err != nil {
			return nil, err
		}
		source, err := readEditSource(work, target.Path)
		if err != nil {
			return nil, err
		}
		after, _, err := hashline.ReplaceExact(source, input.Edits)
		if err != nil {
			return nil, err
		}
		if after == nil {
			after = []byte{}
		}
		effects = []fileedit.Effect{{Path: target.Path, Revision: hashline.SourceRevision(source), Bytes: after}}
	}
	registerBytes := 0
	proposed := make(map[string]hashline.Register, len(s.editState.registers)+len(plan.Registers))
	for name, value := range s.editState.registers {
		proposed[name] = value
	}
	for name, value := range plan.Registers {
		proposed[name] = value
	}
	for _, value := range proposed {
		for _, row := range value.Lines {
			registerBytes += len(row) + 1
		}
	}
	if len(proposed) > 256 || registerBytes > 64<<20 {
		return nil, errors.New("named register retention limit exceeded; reduce captures")
	}
	fingerprintBytes, _ := json.Marshal(struct {
		Input   editParams
		Effects []fileedit.Effect
	}{input, effects})
	fingerprint := hashline.SourceRevision(fingerprintBytes)
	if repeats, _ := s.editState.noops.get(fingerprint); repeats > 0 {
		return nil, errors.New("repeated no-op edit refused; change the intended operation or read the current source")
	}
	if s.snapshots.Epoch() != epoch {
		return nil, errors.New("context changed while staging; read the active branch and retry")
	}
	agent, err := sdk.AgentDir()
	if err != nil {
		return nil, err
	}
	outcomes, commitErr := fileedit.Commit(work, editLockDir(agent), effects)
	changed := false
	for index, outcome := range outcomes {
		changed = changed || outcome.Committed
		if outcome.Committed && index < len(effects) && !effects[index].Delete && effects[index].MoveTo == "" {
			s.contextFrames.recordAuthored(outcome.Path, effects[index].Bytes)
		}
	}
	if !changed && commitErr == nil {
		repeats, _ := s.editState.noops.get(fingerprint)
		s.editState.noops.put(fingerprint, repeats+1)
	} else {
		s.editState.noops.remove(fingerprint)
	}
	if s.snapshots.Epoch() == epoch {
		for name, value := range plan.Registers {
			for index, sourceURI := range effectURIs {
				if sourceURI == value.SourceURI && index < len(outcomes) && (outcomes[index].Committed || outcomes[index].Error == "") {
					s.editState.registers[name] = value
				}
			}
		}
	}
	var content strings.Builder
	if commitErr != nil {
		content.WriteString("Edit batch incomplete. Landed changes were not rolled back.\n")
		message := strings.ReplaceAll(noticeText(commitErr.Error(), 0), "\n", " ")
		if len(message) > 4096 {
			message = message[:4096]
			for !utf8.ValidString(message) {
				message = message[:len(message)-1]
			}
		}
		content.WriteString(message + "\n")
	} else if !changed {
		content.WriteString("No content changed. Repeating the same edit without a source change is refused.\n")
	} else {
		content.WriteString("Edit batch committed.\n")
	}
	for _, outcome := range outcomes {
		state := "unchanged"
		if outcome.Committed {
			state = "committed"
		}
		if outcome.Error != "" {
			state = "failed or unattempted"
			if outcome.Committed {
				state = "committed with a follow-up error"
			}
		}
		fmt.Fprintf(&content, "%q: %s\n", filepath.Base(outcome.Path), state)
	}
	if tagged && s.snapshots.Epoch() == epoch {
		for index, outcome := range outcomes {
			s.rememberCommitted(plan.Effects[index], outcome, epoch)
		}
	}
	remainingBytes, remainingLines := 50000-content.Len(), 1900
	for index, outcome := range outcomes {
		if !outcome.Committed || effects[index].Delete || remainingBytes < 1024 || remainingLines < 6 {
			continue
		}
		path := outcome.Path
		if outcome.Destination != "" {
			path = outcome.Destination
		}
		target, err := localEditTarget(captured.Cwd, path)
		if err != nil {
			continue
		}
		line := 1
		if tagged && len(plan.Effects[index].Touched) > 0 {
			line = plan.Effects[index].Touched[0].Start
		}
		target.Selection = resource.Selection{Explicit: true, Ranges: []resource.Range{{Start: line, End: line + 8}}}
		page, err := (resource.FileProvider{}).Read(work, resource.ReadRequest{URI: target.URI, Requested: path, Selector: target.Selection, Budget: resource.Budget{Bytes: remainingBytes, Lines: remainingLines}})
		if err != nil {
			continue
		}
		content.WriteByte('\n')
		content.WriteString(page.Content)
		remainingBytes -= len(page.Content) + 1
		remainingLines -= strings.Count(page.Content, "\n") + 2
		s.rememberRead(ctx.ToolCallID(), epoch, target, page)
	}
	return sdk.ToolResult{Content: content.String(), Details: map[string]any{"changed": changed, "outcomes": outcomes, "partial": commitErr != nil, "locationRevision": captured.Revision}, IsError: commitErr != nil}, nil
}

func currentSourceContext(work context.Context, s *session, callID, cwd, epoch string, relocation *hashline.RelocationError, raw []byte) string {
	target, err := localEditTarget(cwd, relocation.Path)
	if err != nil {
		return ""
	}
	source, err := hashline.Normalize(raw)
	if err != nil || len(source.Lines) == 0 {
		return ""
	}
	line := relocation.Target.Start
	if len(relocation.Candidates) > 0 {
		line = relocation.Candidates[0].Start
	}
	line = max(1, min(line, len(source.Lines)))
	start := max(1, line-2)
	end := min(len(source.Lines), start+7)
	target.Selection = resource.Selection{Explicit: true, Ranges: []resource.Range{{Start: start, End: end}}}
	page, err := (resource.FileProvider{}).Read(work, resource.ReadRequest{URI: target.URI, Requested: target.Path, Selector: target.Selection, Budget: resource.Budget{Bytes: 8192, Lines: 12}})
	if err != nil {
		return ""
	}
	s.rememberRead(callID, epoch, target, page)
	return page.Content
}

func localEditTarget(cwd, path string) (resource.Target, error) {
	target, err := resource.ParsePath(cwd, path, "")
	if err != nil {
		return target, err
	}
	if target.Scheme != "file" {
		return target, fmt.Errorf("edit supports local files only; %s:// does not accept edits here and nothing was sent to its provider", target.Scheme)
	}
	if target.Selection.Explicit {
		return target, errors.New("edit targets must be local files without selector suffixes")
	}
	canonical, err := filepath.EvalSymlinks(target.Path)
	if err == nil {
		target.URI = resource.FileURI(canonical)
	}
	return target, nil
}

func readEditSource(ctx context.Context, path string) ([]byte, error) {
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > 64<<20 {
		return nil, errors.New("exact editing is limited to 64 MiB per file")
	}
	var output strings.Builder
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			if output.Len()+n > 64<<20 {
				return nil, errors.New("source grew beyond the exact edit limit")
			}
			output.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return []byte(output.String()), nil
}
