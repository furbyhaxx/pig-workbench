package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func (a *app) registerTools(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "cd", Label: "cd", Description: "Inspect or change the Workbench working location without changing PiG's session cwd. Use {} for status, {path} for a local directory, or {reset:true} to return to the origin. Relative paths use the captured location. A change waits at most 2 seconds for active mutations and persists before activation.",
		PromptSnippet: "Inspect or change the working location for coding tools.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"path":  map[string]any{"type": "string", "minLength": 1, "description": "Absolute or current-location-relative directory."},
			"reset": map[string]any{"type": "boolean", "const": true, "description": "Return to the original session location."},
		}}, Execute: a.cd,
	})
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "read", Label: "read", Description: "Read text from local paths or registered scheme:// resources; relative paths use the Workbench location. Use path suffixes :N, :A-B, :A+C, :-N, comma ranges or :raw (for example src/main.go:20-40) when supported by the provider. Literal filenames win. Exactly one of path or paths (1–32) is required; explicit selector is for one path only. Output is bounded to 50 KiB/2000 lines across the batch; Details report actual source coverage and continuation. For local text, raw preserves source bytes; ordinary ranges include one preceding and three trailing lines.",
		PromptSnippet: "Read source text with line selectors, raw views and precise continuation.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"path":     map[string]any{"type": "string", "minLength": 1, "description": "Local path or registered scheme URI with an optional selector suffix, for example src/a.go:20-40 or src/a.go:raw. There is no separate raw parameter."},
			"paths":    map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Explicit ordered multi-read, sharing one output budget."},
			"selector": map[string]any{"type": "string", "minLength": 1, "description": "Single-path selector when punctuation is ambiguous; cannot accompany a suffix."},
		}}, Execute: a.read,
	})
}

type cdParams struct {
	Path  string `json:"path"`
	Reset bool   `json:"reset"`
}

func (a *app) cd(ctx sdk.Context, params map[string]any) (any, error) {
	var input cdParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	if len(params) > 1 || len(params) == 1 && input.Path == "" && !input.Reset {
		return nil, errors.New("cd accepts {}, {path}, or {reset:true}")
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if len(params) == 0 {
		return locationResult(s.location.Current()), nil
	}
	bounded, cancel := context.WithTimeout(work, 2*time.Second)
	defer cancel()
	change, err := s.location.Transition(bounded, input.Path, input.Reset, func(change location.Change) error {
		return ctx.AppendEntry("pig-workbench.location", change)
	})
	if err != nil {
		return nil, err
	}
	result := locationResult(change.Current)
	if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: fmt.Sprintf("%s:location:%d", s.generation, change.Current.Revision), Type: "location.changed", Data: s.apiLocationChange(work, change)}); err != nil {
		result.Content += "\nLocation committed, but change notification failed; query location.get to reconcile."
	}
	return result, nil
}

func locationResult(s location.Snapshot) sdk.ToolResult {
	return sdk.ToolResult{Content: fmt.Sprintf("Working location: %s\nAuthority: %s · revision %d\nRelative tool paths resolve here; PiG session cwd is unchanged.", s.Cwd, s.Authority, s.Revision), Details: s}
}

type readParams struct {
	Path     string   `json:"path"`
	Paths    []string `json:"paths"`
	Selector string   `json:"selector"`
}

func (a *app) read(ctx sdk.Context, params map[string]any) (any, error) {
	var input readParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	_, one := params["path"]
	_, many := params["paths"]
	if one == many {
		return nil, errors.New("read requires exactly one of path or paths")
	}
	if many {
		if _, ok := params["selector"]; ok {
			return nil, errors.New("selector is only allowed with one path")
		}
	}
	if one {
		if input.Path == "" {
			return nil, errors.New("path cannot be empty")
		}
		input.Paths = []string{input.Path}
	}
	if len(input.Paths) < 1 || len(input.Paths) > 32 {
		return nil, errors.New("read paths must have 1–32 items")
	}
	if _, ok := params["selector"]; ok && input.Selector == "" {
		return nil, errors.New("selector cannot be empty")
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	captured, done, err := s.location.Acquire(ctx.ToolCallID(), false)
	if err != nil {
		return nil, err
	}
	defer done()
	work = withCapturedLocation(work, captured)
	targets := make([]resource.Target, len(input.Paths))
	for i, path := range input.Paths {
		targets[i], err = resource.ParsePath(captured.Cwd, path, input.Selector)
		if err != nil {
			return nil, fmt.Errorf("path %d: %w", i+1, err)
		}
	}
	epoch := ""
	if s.snapshots != nil {
		epoch = s.snapshots.Epoch()
	}
	threshold := intValue(configuration, "read.loop-hint-after", 3)
	policy := configuration.Read()
	budget := resource.Budget{Bytes: policy.MaxBytes, Lines: policy.MaxLines, Columns: intValue(configuration, "read.output-max-columns", 512)}
	var content strings.Builder
	facts := make([]any, 0, len(targets))
	complete := true
	failedCount := 0
	var projection *readProjection
	reserves := make([]int, len(targets))
	pointers := make([]string, len(targets))
	for i, target := range targets {
		if provider, ok := s.contributedDescriptor(target.Scheme); ok {
			pointers[i] = guidePointer(provider.canonical)
			reserves[i] = len(pointers[i]) + 2
		}
	}
	readTarget := func(ctx context.Context, i int, available resource.Budget) (resource.ReadResult, error) {
		target := targets[i]
		return s.router.Read(ctx, resource.Target{Requested: target.Requested, URI: target.URI, Scheme: target.Scheme, Path: target.Path, Selection: target.Selection}, resource.Budget{Bytes: available.Bytes - readHintReserve - reserves[i], Lines: available.Lines, Columns: available.Columns})
	}
	whole := budget
	ahead := startReadAhead(work, policy.Concurrency, len(targets), func(ctx context.Context, i int) (resource.ReadResult, error) {
		return readTarget(ctx, i, whole)
	})
	defer ahead.stop()
	for i, target := range targets {
		if err := work.Err(); err != nil {
			return nil, err
		}
		if budget.Bytes < 1024 || budget.Lines < 5 {
			ahead.stop()
			facts = append(facts, map[string]any{"requestedUri": target.Requested, "error": "budgetExhausted", "complete": false})
			complete = false
			continue
		}
		guidePointer := pointers[i]
		pageBudget := resource.Budget{Bytes: budget.Bytes - readHintReserve - reserves[i], Lines: budget.Lines}
		page, err, early := ahead.take(work, i, pageBudget)
		if !early {
			page, err = readTarget(work, i, budget)
		}
		if err == nil && target.Scheme != "file" {
			// Non-local providers already applied their own budget.
			page.Facts.RequestedURI = target.Requested
		}
		if err == nil {
			if summary, ok, summaryErr := a.summaryView(work, target, configuration); summaryErr != nil {
				return nil, summaryErr
			} else if ok && len(summary.Content) <= pageBudget.Bytes && strings.Count(summary.Content, "\n")+1 <= pageBudget.Lines {
				facts = append(facts, summary.Facts)
				s.skills.noteTouch(target.Path)
				s.rememberRead(ctx.ToolCallID(), epoch, target, summary)
				text := appendReadNote(summary.Content, s.readHistory.hint(target, summary, threshold))
				content.WriteString(text)
				budget.Bytes -= len(text)
				budget.Lines -= strings.Count(text, "\n") + 1
				complete = false
				continue
			}
		}
		text := ""
		if err != nil {
			if len(targets) == 1 {
				return nil, err
			}
			text = "Read failed: " + err.Error() + "\n"
			if len(text) > budget.Bytes-256 {
				text = "Read failed; inspect Details.\n"
			}
			facts = append(facts, map[string]any{"requestedUri": target.Requested, "error": err.Error()})
			complete = false
			failedCount++
		} else {
			text = appendReadNote(page.Content, s.readHistory.hint(target, page, threshold))
			if len(targets) == 1 {
				projection = readProof(page)
			}
			s.skills.noteTouch(target.Path)
			s.rememberRead(ctx.ToolCallID(), epoch, target, page)
			if guide, ok := s.firstTouchGuide(target.Scheme); ok {
				if len(text)+len(guide)+2 <= budget.Bytes && strings.Count(text, "\n")+strings.Count(guide, "\n")+3 <= budget.Lines {
					text += "\n\n" + guide
				} else {
					text = appendReadNote(text, guidePointer)
				}
			}
			facts = append(facts, page.Facts)
			complete = complete && page.Facts.SelectionComplete
		}
		content.WriteString(text)
		budget.Bytes -= len(text)
		budget.Lines -= strings.Count(text, "\n") + 1
		if len(targets) > 1 && budget.Bytes > 0 && budget.Lines > 0 {
			content.WriteByte('\n')
			budget.Bytes--
			budget.Lines--
		}
	}
	if !complete && budget.Bytes >= 64 && budget.Lines > 0 {
		content.WriteString("\nBatch incomplete; consult per-resource continuation in Details.")
	}
	// One failed target in a multi-read must not mark the whole result as an
	// error: the successful content is still valid and useful to the model.
	allFailed := failedCount == len(targets)
	a.deliverAutoSkills(ctx, s)
	return sdk.ToolResult{
		Content: content.String(),
		Details: map[string]any{
			"resources":        facts,
			"complete":         complete,
			"failedTargets":    failedCount,
			"locationRevision": captured.Revision,
			"configRevision":   configuration.Revision(),
			"readProjection":   projection,
		},
		IsError: allFailed,
	}, nil
}

func decode(params map[string]any, dst any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return errors.New("invalid tool arguments")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing tool arguments")
	}
	return nil
}
