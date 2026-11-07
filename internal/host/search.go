package host

import (
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/search"
)

func (a *app) registerSearchTools(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "grep", Label: "grep",
		Description:   "Search file contents at a local path or registered provider file/directory. Line selectors are accepted only for explicit files. Pass exactly one regex pattern, or literal patterns for an OR search (literal:true). Relative paths resolve against the Workbench location. Binary, oversized and incomplete provider files are skipped and listed in Details; hitting a bound reports complete:false instead of pretending the results are exhaustive.",
		PromptSnippet: "Search file contents with exact matches and honest limits.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"pattern":    map[string]any{"type": "string", "minLength": 1, "description": "Regular expression, or one literal when literal is true."},
			"patterns":   map[string]any{"type": "array", "minItems": 1, "maxItems": 16, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Literal OR patterns; requires literal:true and cannot accompany pattern."},
			"literal":    map[string]any{"type": "boolean", "description": "Treat patterns as literal text instead of a regex."},
			"ignoreCase": map[string]any{"type": "boolean", "description": "Case-insensitive matching."},
			"path":       map[string]any{"type": "string", "description": "Directory to search; default is the Workbench location."},
			"glob":       map[string]any{"type": "string", "description": "Only search file names matching this glob."},
			"exclude":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "File or directory name globs to skip."},
			"filesOnly":  map[string]any{"type": "boolean", "description": "Return only the first match per file."},
			"context":    map[string]any{"type": "integer", "minimum": 0, "maximum": 10, "description": "Number of surrounding source lines before and after each match."},
			"limit":      map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Maximum matches (default 200)."},
		}, "oneOf": []any{
			map[string]any{"required": []string{"pattern"}, "not": map[string]any{"required": []string{"patterns"}}},
			map[string]any{"required": []string{"patterns", "literal"}, "not": map[string]any{"required": []string{"pattern"}}, "properties": map[string]any{"literal": map[string]any{"const": true}}},
		}},
		Execute: a.grep,
	})
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "find", Label: "find",
		Description:   "Walk a local directory or registered provider root for file names matching a glob pattern. Links and noWalk subtrees are not followed, control files and .git are skipped, exclusions apply, and bounds report complete:false.",
		PromptSnippet: "Find files by name glob under a directory.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"pattern"}, "properties": map[string]any{
			"pattern": map[string]any{"type": "string", "minLength": 1, "description": "File-name glob, e.g. *.go."},
			"path":    map[string]any{"type": "string", "description": "Directory to walk; default is the Workbench location."},
			"exclude": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Name globs to skip."},
			"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Maximum entries (default 200)."},
		}},
		Execute: a.find,
	})
}

type grepParams struct {
	Pattern    string   `json:"pattern"`
	Patterns   []string `json:"patterns"`
	Literal    bool     `json:"literal"`
	IgnoreCase bool     `json:"ignoreCase"`
	Path       string   `json:"path"`
	Glob       string   `json:"glob"`
	Exclude    []string `json:"exclude"`
	FilesOnly  bool     `json:"filesOnly"`
	Context    int      `json:"context"`
	Limit      int      `json:"limit"`
}

func (a *app) grep(ctx sdk.Context, params map[string]any) (any, error) {
	var input grepParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	_, hasPath := params["path"]
	_, hasPatterns := params["patterns"]
	patterns := input.Patterns
	if input.Pattern != "" {
		if hasPatterns {
			return nil, errors.New("grep takes pattern or patterns, not both")
		}
		patterns = []string{input.Pattern}
	}
	if len(patterns) == 0 {
		return nil, errors.New("grep needs a pattern")
	}
	if hasPatterns && !input.Literal {
		return nil, errors.New("patterns[] requires literal:true; use pattern for a regex")
	}
	if input.Context < 0 || input.Context > 10 {
		return nil, errors.New("grep context must be between 0 and 10 lines")
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	captured, done, err := s.location.Acquire(ctx.ToolCallID(), false)
	if err != nil {
		return nil, err
	}
	defer done()
	work = withCapturedLocation(work, captured)
	root := captured.Cwd
	var provider resource.Provider
	var selection resource.Selection
	if hasPath && input.Path != "" {
		target, parseErr := resource.ParsePath(captured.Cwd, input.Path, "")
		if parseErr != nil {
			return nil, parseErr
		}
		selection = target.Selection
		if target.Scheme == "file" {
			root = target.Path
		} else {
			provider, err = s.router.Provider(target)
			if err != nil {
				return nil, err
			}
			root = target.URI
		}
	}
	options := search.Options{
		Root: root, Patterns: patterns, Literal: input.Literal, IgnoreCase: input.IgnoreCase,
		Glob: input.Glob, Exclude: input.Exclude, FilesOnly: input.FilesOnly, Selection: selection,
		Limits: search.Limits{MaxMatches: input.Limit, Context: input.Context},
	}
	var result search.Result
	if provider != nil {
		result, err = search.GrepProvider(work, provider, options)
	} else {
		result, err = search.Grep(work, options)
	}
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	for _, match := range result.Matches {
		path := plainLine(match.Path)
		if match.ContextTruncated {
			body.WriteString("… surrounding context truncated\n")
		}
		for _, line := range match.Before {
			_, _ = fmt.Fprintf(&body, "%s-%d- %s\n", path, line.Line, plainLine(line.Text))
		}
		if match.Windowed {
			_, _ = fmt.Fprintf(&body, "%s:%d:%d: [line window from byte %d] %s\n", path, match.Line, match.Column, match.WindowStart, plainLine(match.Text))
		} else {
			_, _ = fmt.Fprintf(&body, "%s:%d:%d: %s\n", path, match.Line, match.Column, plainLine(match.Text))
		}
		for _, line := range match.After {
			_, _ = fmt.Fprintf(&body, "%s-%d- %s\n", path, line.Line, plainLine(line.Text))
		}
	}
	if !result.Complete {
		body.WriteString("… results incomplete due to a line window, skipped provider file or search/output limit\n")
	}
	if len(result.Skipped) > 0 {
		_, _ = fmt.Fprintf(&body, "skipped %d unreadable, binary or oversized file(s)\n", len(result.Skipped))
	}
	return sdk.ToolResult{Content: body.String(), Details: map[string]any{"matches": result.Matches, "filesScanned": result.FilesScanned, "complete": result.Complete, "skipped": result.Skipped, "locationRevision": captured.Revision}}, nil
}

type findParams struct {
	Pattern string   `json:"pattern"`
	Path    string   `json:"path"`
	Exclude []string `json:"exclude"`
	Limit   int      `json:"limit"`
}

func (a *app) find(ctx sdk.Context, params map[string]any) (any, error) {
	var input findParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	captured, done, err := s.location.Acquire(ctx.ToolCallID(), false)
	if err != nil {
		return nil, err
	}
	defer done()
	work = withCapturedLocation(work, captured)
	root := captured.Cwd
	var provider resource.Provider
	if input.Path != "" {
		target, parseErr := resource.ParsePath(captured.Cwd, input.Path, "")
		if parseErr != nil {
			return nil, parseErr
		}
		if target.Selection.Explicit {
			return nil, errors.New("find does not accept a line selector")
		}
		if target.Scheme == "file" {
			root = target.Path
		} else {
			provider, err = s.router.Provider(target)
			if err != nil {
				return nil, err
			}
			root = target.URI
		}
	}
	var result search.FindResult
	if provider != nil {
		result, err = search.FindProvider(work, provider, root, input.Pattern, input.Exclude, input.Limit)
	} else {
		result, err = search.Find(work, root, input.Pattern, input.Exclude, input.Limit)
	}
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	for _, entry := range result.Entries {
		body.WriteString(plainLine(entry.Path))
		body.WriteByte('\n')
	}
	if !result.Complete {
		body.WriteString("… truncated at the configured limit\n")
	}
	return sdk.ToolResult{Content: body.String(), Details: map[string]any{"entries": result.Entries, "scanned": result.Scanned, "complete": result.Complete, "locationRevision": captured.Revision}}, nil
}

func plainLine(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 32 || r >= 127 && r <= 159 {
			return -1
		}
		return r
	}, value)
}
