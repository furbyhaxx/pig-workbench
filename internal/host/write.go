package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func (a *app) registerWriteAndList(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "write", Label: "write",
		Description:   "Create or fully replace a local file; use edit for partial changes. Relative paths resolve against the Workbench location. For local files, expectedRevision requires an unchanged file, content is written atomically, existing modes are preserved, and directories are not created. Control writes also support job:// stdin and ctl, skill:// unload, agent://<id>/inbox, session://<id>/title and session://self/instructions/ctl; their own policy and confirmation rules apply.",
		PromptSnippet: "Create or fully replace one file, optionally guarded by the revision you read.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"path", "content"}, "properties": map[string]any{
			"path":             map[string]any{"type": "string", "minLength": 1, "description": "Local file, file:// URI or supported control URI."},
			"content":          map[string]any{"type": "string", "description": "Exact complete new content."},
			"raw":              map[string]any{"type": "boolean", "description": "Write literal content without interpreting copied tagged-read formatting."},
			"expectedRevision": map[string]any{"type": "string", "minLength": 1, "description": "For local files, require the revision reported by read; refused when the file changed."},
		}},
		Execute: a.write,
	})
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "ls", Label: "ls",
		Description:   "List one container through the Workbench resource router, including registered scheme URIs whose providers support listing. Entries have typed kinds and a continuation cursor; incomplete listings are reported.",
		PromptSnippet: "List a directory with typed entries and a continuation cursor.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"path"}, "properties": map[string]any{
			"path":   map[string]any{"type": "string", "minLength": 1, "description": "Directory path or scheme URI."},
			"cursor": map[string]any{"type": "string", "description": "Opaque cursor from a previous listing."},
			"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Maximum entries (default 200)."},
		}},
		Execute: a.ls,
	})
}

type writeParams struct {
	Path             string `json:"path"`
	Content          string `json:"content"`
	ExpectedRevision string `json:"expectedRevision"`
	Raw              bool   `json:"raw"`
}

func (a *app) write(ctx sdk.Context, params map[string]any) (any, error) {
	var input writeParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	_, hasExpectedRevision := params["expectedRevision"]
	if hasExpectedRevision && input.ExpectedRevision == "" {
		return nil, errors.New("expectedRevision must not be empty")
	}
	var copied hashline.ReadCopy
	recognized := false
	if !input.Raw {
		var err error
		copied, recognized, err = hashline.UnwrapRead(input.Content)
		if err != nil {
			return nil, err
		}
		input.Content = copied.Content
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
	if recognized {
		sourceTarget, err := resource.ParsePath(captured.Cwd, copied.Path, "")
		if err != nil {
			return nil, err
		}
		if sourceTarget.Scheme == "file" {
			if canonical, err := filepath.EvalSymlinks(sourceTarget.Path); err == nil {
				sourceTarget.URI = resource.FileURI(canonical)
			}
		}
		source, err := s.snapshots.Resolve(sourceTarget.URI, copied.Tag)
		if err != nil {
			return nil, errors.New("copied read snapshot unavailable; reread the complete source in raw mode before copying")
		}
		normalized, err := hashline.Normalize(source.Raw)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(normalized.Bytes, []byte(copied.Content)) {
			return nil, errors.New("copied view differs from its retained source; provide complete unformatted content")
		}
		input.Content = string(source.Raw)
	}
	target, err := resource.ParsePath(captured.Cwd, input.Path, "")
	if err != nil {
		return nil, err
	}
	if target.Selection.Explicit {
		return nil, errors.New("write takes a whole path, not a line selector")
	}
	if hasExpectedRevision && target.Scheme != "file" {
		return nil, errors.New("expectedRevision is supported only for local files; the control write was not performed")
	}
	if target.Scheme != "file" {
		return a.writeControl(ctx, work, s, target, input.Content)
	}
	if err := work.Err(); err != nil {
		return nil, err
	}
	parent := filepath.Dir(target.Path)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("parent directory does not exist or is not a directory: %s", parent)
	}
	existing, previous, err := readExisting(target.Path)
	if err != nil {
		return nil, err
	}
	if input.ExpectedRevision != "" && input.ExpectedRevision != previous {
		return nil, fmt.Errorf("staleRevision: %s changed since it was read (expected %s, found %s); read it again", target.Path, short(input.ExpectedRevision), short(previous))
	}
	epoch := s.snapshots.Epoch()
	mode := os.FileMode(0600)
	if existing != nil {
		mode = existing.Mode().Perm()
	}
	if existing != nil {
		agent, err := sdk.AgentDir()
		if err != nil {
			return nil, err
		}
		outcomes, err := fileedit.Commit(work, filepath.Join(agent, "workbench", "locks", "edits"), []fileedit.Effect{{Path: target.Path, Revision: previous, Bytes: []byte(input.Content)}})
		if err != nil {
			if len(outcomes) > 0 && outcomes[0].Committed {
				return sdk.ToolResult{Content: "Replacement landed but its durability check failed. Read the file before retrying.", Details: outcomes, IsError: true}, nil
			}
			return nil, err
		}
	} else {
		created, err := atomicWriteFile(work, target.Path, []byte(input.Content), mode)
		if err != nil && !created {
			return nil, err
		}
		if err != nil {
			return sdk.ToolResult{Content: "Creation landed but its durability check failed. Read the file before retrying.", Details: map[string]any{"path": target.Path, "created": true}, IsError: true}, nil
		}
	}
	digest := sha256.Sum256([]byte(input.Content))
	revision := hex.EncodeToString(digest[:])
	canonical, err := filepath.EvalSymlinks(target.Path)
	if err != nil {
		return nil, err
	}
	target.URI = resource.FileURI(canonical)
	receipt := fmt.Sprintf("Wrote %d bytes to %s\nRevision: %s", len(input.Content), target.URI, revision)
	s.rememberWritten(ctx.ToolCallID(), epoch, target, []byte(input.Content), receipt)
	return sdk.ToolResult{Content: receipt, Details: map[string]any{"uri": target.URI, "revision": revision, "bytes": len(input.Content), "created": existing == nil, "previousRevision": previous, "locationRevision": captured.Revision}}, nil
}

func (a *app) writeControl(ctx sdk.Context, work context.Context, s *session, target resource.Target, content string) (sdk.ToolResult, error) {
	if err := work.Err(); err != nil {
		return sdk.ToolResult{}, err
	}
	switch target.Scheme {
	case "skill":
		if err := (skillProvider{}).unload(s, target.URI, content); err != nil {
			return sdk.ToolResult{}, err
		}
		return sdk.ToolResult{Content: "skill unloaded\n", Details: map[string]any{"uri": target.URI}}, nil
	case "agent":
		message, receipt, err := a.agentWrite(work, s, target.URI, content)
		if err != nil {
			return sdk.ToolResult{}, err
		}
		return sdk.ToolResult{Content: message + "\n", Details: map[string]any{"uri": target.URI, "receipt": receipt}}, nil
	case "session":
		var message string
		var err error
		if strings.HasSuffix(strings.TrimRight(target.URI, "/"), "/title") {
			message, err = a.writeSessionTitle(ctx, s, target.URI, content)
		} else {
			message, err = a.instructionsControl(ctx, target.URI, content)
		}
		if err != nil {
			return sdk.ToolResult{}, err
		}
		return sdk.ToolResult{Content: message + "\n", Details: map[string]any{"uri": target.URI, "applied": true}}, nil
	case "job":
		if err := (jobProvider{}).jobAction(s, target.URI, content); err != nil {
			return sdk.ToolResult{}, err
		}
		return sdk.ToolResult{Content: "job action applied to " + target.URI + "\n", Details: map[string]any{"uri": target.URI, "applied": true}}, nil
	default:
		return sdk.ToolResult{}, fmt.Errorf("write supports local files and job://, skill://, agent://<id>/inbox, session://<id>/title and session://self/instructions/ctl actions only; %s:// does not accept writes here and nothing was sent to its provider", target.Scheme)
	}
}

func readExisting(path string) (os.FileInfo, string, error) {
	file, info, err := fileio.OpenRegular(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return nil, "", fmt.Errorf("read existing file: %w", err)
	}
	return info, hex.EncodeToString(digest.Sum(nil)), nil
}

func atomicWriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) (bool, error) {
	return fileedit.Create(ctx, path, data, mode)
}

type lsParams struct {
	Path   string `json:"path"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

func (a *app) ls(ctx sdk.Context, params map[string]any) (any, error) {
	var input lsParams
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
	target, err := resource.ParsePath(captured.Cwd, input.Path, "")
	if err != nil {
		return nil, err
	}
	if target.Selection.Explicit {
		return nil, errors.New("ls takes a directory, not a line selector")
	}
	provider, err := s.router.Provider(target)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 200
	}
	listing, err := provider.List(work, target.URI, input.Cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("ls %s: %w", target.Requested, err)
	}
	var body bytes.Buffer
	for _, entry := range listing.Entries {
		_, _ = fmt.Fprintf(&body, "%-4s %s", entry.Kind, entry.Name)
		switch {
		case entry.Kind == resource.EntryLink:
			_, _ = fmt.Fprintf(&body, " -> %s", entry.Target)
		case entry.Size > 0:
			_, _ = fmt.Fprintf(&body, " (%d bytes)", entry.Size)
		}
		if entry.Detail != "" {
			_, _ = fmt.Fprintf(&body, "  %s", sanitizeProviderText(entry.Detail))
		}
		body.WriteByte('\n')
	}
	if !listing.Complete {
		_, _ = fmt.Fprintf(&body, "… more entries; continue with cursor %q\n", listing.NextCursor)
	}
	return sdk.ToolResult{Content: body.String(), Details: map[string]any{"uri": target.URI, "entries": listing.Entries, "complete": listing.Complete, "nextCursor": listing.NextCursor, "locationRevision": captured.Revision}}, nil
}

func short(revision string) string {
	if revision == "" {
		return "absent"
	}
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
