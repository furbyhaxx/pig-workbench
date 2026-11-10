package host

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type lsParams struct {
	Path          string `json:"path"`
	Cursor        string `json:"cursor"`
	Limit         int    `json:"limit"`
	IncludeHidden *bool  `json:"includeHidden"`
}

type listingContinuation struct {
	Anchor    string           `json:"anchor,omitempty"`
	Digest    string           `json:"digest,omitempty"`
	PageLimit int              `json:"pageLimit,omitempty"`
	Revision  string           `json:"revision,omitempty"`
	Pending   []resource.Entry `json:"pending,omitempty"`
	Backend   string           `json:"backend,omitempty"`
	Complete  bool             `json:"complete"`
}

func (a *app) ls(ctx sdk.Context, params map[string]any) (any, error) {
	var input lsParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	_, explicitLimit := params["limit"]
	if input.Path == "" || input.Limit > 2000 || explicitLimit && input.Limit < 1 {
		return nil, errors.New("ls requires a path and a limit between 1 and 2000")
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
	target, err := resource.ParsePath(captured.Cwd, input.Path, "")
	if err != nil {
		return nil, err
	}
	if target.Selection.Explicit {
		return nil, errors.New("ls takes a directory, not a line selector")
	}
	hidden := true
	if input.IncludeHidden != nil {
		hidden = *input.IncludeHidden
	}
	result, err := listContainer(work, s, target, input.Cursor, input.Limit, hidden)
	if err != nil {
		return nil, err
	}
	result.Details.(map[string]any)["locationRevision"] = captured.Revision
	return result, nil
}

func listContainer(ctx context.Context, s *session, target resource.Target, cursor string, limit int, hidden bool) (sdk.ToolResult, error) {
	provider, err := s.router.Provider(target)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	if limit <= 0 {
		limit = 200
	}
	binding := cursorBinding{Registration: "ls:" + target.Scheme, Descriptor: fmt.Sprint(hidden), Generation: s.generation, Instance: "pig-workbench", Context: listingContext(ctx)}
	root := target.URI
	if p, ok := provider.(*contributedProvider); ok {
		binding = cursorBinding{Registration: "ls:" + p.registrationID, Descriptor: p.descriptorRevision + fmt.Sprint(hidden), Generation: p.generation, Instance: p.owner.Instance, Context: listingContext(ctx)}
		root = p.canonicalURI(root)
	}
	state := listingContinuation{}
	if cursor != "" {
		retained, err := s.listCursors.get(cursor, binding, root, time.Now())
		if err != nil {
			return sdk.ToolResult{}, err
		}
		if err := json.Unmarshal([]byte(retained.Raw), &state); err != nil {
			return sdk.ToolResult{}, errors.New("retained listing state is invalid; restart listing")
		}
	}
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Buffered rows must still belong to the same backend page when resumed.
	if len(state.Pending) > 0 {
		page, err := provider.List(work, root, state.Anchor, state.PageLimit)
		if err != nil {
			return sdk.ToolResult{}, err
		}
		if listingDigest(page) != state.Digest {
			return sdk.ToolResult{}, errors.New("staleRevision: retained listing page changed; restart listing")
		}
	}
	const bytesLimit = (50 << 10) - 512
	const metadataLimit = 48 << 10
	var body strings.Builder
	entries := make([]resource.Entry, 0, min(limit, 200))
	metadataBytes, pages, skipped := 0, 0, 0
	seen := make(map[string]bool)
	for len(entries) < limit && len(entries) < 1900 {
		if err := work.Err(); err != nil {
			return sdk.ToolResult{}, err
		}
		if len(state.Pending) == 0 {
			if state.Complete || pages >= 256 {
				break
			}
			if seen[state.Backend] {
				return sdk.ToolResult{}, errors.New("provider repeated a listing cursor without progress")
			}
			seen[state.Backend] = true
			anchor := state.Backend
			pageLimit := min(16, limit-len(entries))
			page, err := provider.List(work, root, anchor, pageLimit)
			if err != nil {
				return sdk.ToolResult{}, fmt.Errorf("ls %s: %w", target.Requested, err)
			}
			if len(page.Entries) > pageLimit {
				return sdk.ToolResult{}, errors.New("provider exceeded the requested listing entry limit")
			}
			if !page.Complete && page.NextCursor == "" {
				return sdk.ToolResult{}, errors.New("incomplete listing has no continuation")
			}
			state.Pending, state.Backend, state.Complete = page.Entries, page.NextCursor, page.Complete
			state.Anchor, state.Digest, state.Revision, state.PageLimit = anchor, listingDigest(page), page.Revision, pageLimit
			pages++
			if len(state.Pending) == 0 {
				continue
			}
		}
		entry := state.Pending[0]
		if !hidden && strings.HasPrefix(entry.Name, ".") {
			state.Pending = state.Pending[1:]
			skipped++
			continue
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return sdk.ToolResult{}, err
		}
		if len(encoded) > metadataLimit {
			return sdk.ToolResult{}, errors.New("listing entry exceeds the 48 KiB metadata limit")
		}
		row := listEntryRow(entry)
		if body.Len()+len(row) > bytesLimit || metadataBytes+len(encoded)+1 > metadataLimit {
			break
		}
		entries = append(entries, entry)
		metadataBytes += len(encoded) + 1
		body.WriteString(row)
		state.Pending = state.Pending[1:]
	}
	complete := state.Complete && len(state.Pending) == 0
	if complete && len(entries) == 0 && body.Len() == 0 {
		if skipped > 0 {
			fmt.Fprintf(&body, "(no visible entries; %d hidden)\n", skipped)
		} else {
			body.WriteString("(empty)\n")
		}
	}
	next := ""
	if !complete {
		raw, err := json.Marshal(state)
		if err != nil {
			return sdk.ToolResult{}, err
		}
		next, err = s.listCursors.put(listingCursor{Binding: binding, Requested: root, Canonical: root, Raw: string(raw)}, time.Now())
		if err != nil {
			return sdk.ToolResult{}, err
		}
		fmt.Fprintf(&body, "… more entries; continue with cursor %q\n", next)
	}
	details := map[string]any{"uri": root, "entries": entries, "complete": complete, "nextCursor": next, "includeHidden": hidden, "hiddenSkipped": skipped}
	if state.Revision != "" {
		details["revision"] = state.Revision
	}
	return sdk.ToolResult{Content: body.String(), Details: details}, nil
}

func listingDigest(page resource.Listing) string {
	encoded, _ := json.Marshal(page)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func listEntryRow(entry resource.Entry) string {
	name := plainLine(entry.Name)
	if name != entry.Name {
		name = fmt.Sprintf("%q", entry.Name)
	}
	row := fmt.Sprintf("%-4s %s", entry.Kind, name)
	if entry.Kind == resource.EntryLink {
		row += " -> " + noticeText(entry.Target, 256)
	} else if entry.Size > 0 {
		row += fmt.Sprintf(" (%d bytes)", entry.Size)
	}
	if entry.Detail != "" {
		row += "  " + noticeText(entry.Detail, 256)
	}
	return row + "\n"
}
