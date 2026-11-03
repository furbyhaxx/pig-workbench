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
		}, "oneOf": []any{
			map[string]any{"maxProperties": 0},
			map[string]any{"required": []string{"path"}, "maxProperties": 1},
			map[string]any{"required": []string{"reset"}, "maxProperties": 1},
		}}, Execute: a.cd,
	})
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "read", Label: "read", Description: "Read UTF-8 text from local paths or file:// URIs, relative to the Workbench location. Use path suffixes :N, :A-B, :A+C, :-N, comma ranges or :raw (for example src/main.go:20-40). Literal filenames win. Exactly one of path or paths (1–32) is required; explicit selector is for one path only. Output is bounded to 50 KiB/2000 lines across the batch; Details report actual source coverage and continuation. Raw preserves source bytes; ordinary ranges include one preceding and three trailing lines.",
		PromptSnippet: "Read source text with line selectors, raw views and precise continuation.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"path":     map[string]any{"type": "string", "minLength": 1, "description": "Local file or file:// URI, optionally with a selector suffix."},
			"paths":    map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Explicit ordered multi-read, sharing one output budget."},
			"selector": map[string]any{"type": "string", "minLength": 1, "description": "Single-path selector when punctuation is ambiguous; cannot accompany a suffix."},
		}, "oneOf": []any{
			map[string]any{"required": []string{"path"}, "not": map[string]any{"required": []string{"paths"}}},
			map[string]any{"required": []string{"paths"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"path"}}, map[string]any{"required": []string{"selector"}}}}},
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
	if err := ctx.Events().Emit(api.EventChannel, api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: fmt.Sprintf("%s:location:%d", s.generation, change.Current.Revision), Type: "location.changed", Data: change}); err != nil {
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
	targets := make([]resource.Target, len(input.Paths))
	for i, path := range input.Paths {
		targets[i], err = resource.ParsePath(captured.Cwd, path, input.Selector)
		if err != nil {
			return nil, fmt.Errorf("path %d: %w", i+1, err)
		}
	}
	policy := configuration.Read()
	budget := resource.Budget{Bytes: policy.MaxBytes, Lines: policy.MaxLines}
	var content strings.Builder
	facts := make([]any, 0, len(targets))
	complete := true
	failed := false
	for _, target := range targets {
		if err := work.Err(); err != nil {
			return nil, err
		}
		if budget.Bytes < 1024 || budget.Lines < 5 {
			facts = append(facts, map[string]any{"requestedUri": target.Requested, "error": "budgetExhausted", "complete": false})
			complete = false
			continue
		}
		page, err := s.router.Read(work, resource.Target{Requested: target.Requested, URI: target.URI, Scheme: target.Scheme, Path: target.Path, Selection: target.Selection})
		if err == nil && target.Scheme != "file" {
			// Non-local providers already applied their own budget.
			page.Facts.RequestedURI = target.Requested
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
			failed = true
		} else {
			text = page.Content
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
	return sdk.ToolResult{Content: content.String(), Details: map[string]any{"resources": facts, "complete": complete, "locationRevision": captured.Revision, "configRevision": configuration.Revision()}, IsError: failed}, nil
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
