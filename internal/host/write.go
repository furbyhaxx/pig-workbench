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

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func (a *app) registerWriteAndList(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "write", Label: "write",
		Description:   "Create a file or replace it completely. Use edit for partial changes. Relative paths resolve against the Workbench location; local paths and file:// only. Pass expectedRevision (from a prior read) to require the file to be unchanged; the write is refused otherwise. Content is written atomically and an existing file keeps its mode. This tool does not create directories.",
		PromptSnippet: "Create or fully replace one file, optionally guarded by the revision you read.",
		Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"path", "content"}, "properties": map[string]any{
			"path":             map[string]any{"type": "string", "minLength": 1, "description": "Local file or file:// URI."},
			"content":          map[string]any{"type": "string", "description": "Exact complete new content."},
			"expectedRevision": map[string]any{"type": "string", "minLength": 1, "description": "Revision reported by read; refused when the file changed."},
		}},
		Execute: a.write,
	})
	ext.RegisterTool(sdk.ToolDefinition{
		Name: "ls", Label: "ls",
		Description:   "List one directory through the Workbench resource router. Works for local paths, file://, local:// and out://; entries are returned directories-first then lexically, with typed kinds and a continuation cursor. Nothing is summarised or truncated without saying so.",
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
}

func (a *app) write(ctx sdk.Context, params map[string]any) (any, error) {
	var input writeParams
	if err := decode(params, &input); err != nil {
		return nil, err
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
	target, err := resource.ParsePath(captured.Cwd, input.Path, "")
	if err != nil {
		return nil, err
	}
	if target.Selection.Explicit {
		return nil, errors.New("write takes a whole path, not a line selector")
	}
	if target.Scheme != "file" {
		return nil, fmt.Errorf("write does not support %s:// yet; use a local path", target.Scheme)
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
		return nil, fmt.Errorf("staleRevision: %s changed since it was read (expected %s, found %s); read it again", target.Path, short(previous), short(input.ExpectedRevision))
	}
	mode := os.FileMode(0600)
	if existing != nil {
		mode = existing.Mode().Perm()
	}
	if err := atomicWriteFile(work, target.Path, []byte(input.Content), mode); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(input.Content))
	revision := hex.EncodeToString(digest[:])
	return sdk.ToolResult{
		Content: fmt.Sprintf("Wrote %d bytes to %s\nrevision %s", len(input.Content), target.Path, revision),
		Details: map[string]any{"uri": resource.FileURI(target.Path), "bytes": len(input.Content), "revision": revision, "previousRevision": previous, "created": existing == nil, "locationRevision": captured.Revision},
	}, nil
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
	if _, err := io.Copy(digest, io.LimitReader(file, 4<<20)); err != nil {
		return nil, "", fmt.Errorf("read existing file: %w", err)
	}
	return info, hex.EncodeToString(digest.Sum(nil)), nil
}

func atomicWriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	closed := false
	defer func() {
		if !closed {
			temp.Close()
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("flush file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
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
