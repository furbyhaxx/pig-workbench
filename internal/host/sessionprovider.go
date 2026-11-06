package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// sessionProvider projects the running session as read-only resources.
type sessionProvider struct{ ctx func() (sdk.Context, bool) }

func (sessionProvider) Scheme() string { return "session" }

func (p sessionProvider) current() (sdk.Context, error) {
	ctx, ok := p.ctx()
	if !ok {
		return sdk.Context{}, errors.New("session resources are unavailable outside an active session")
	}
	return ctx, nil
}

func splitSession(uri string) ([]string, error) {
	rest, ok := strings.CutPrefix(uri, "session://")
	if !ok {
		return nil, errors.New("session provider requires a session:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, nil
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("session:// segment must not be empty, '.' or '..'")
		}
	}
	return parts, nil
}

var sessionFiles = []string{"meta", "transcript", "entries", "title", "system-prompt"}

func (p sessionProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, err := splitSession(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	if _, err := p.current(); err != nil {
		return resource.StatResult{}, err
	}
	switch {
	case len(parts) == 0, len(parts) == 1 && parts[0] == "self":
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	case len(parts) == 2 && (parts[0] == "self"):
		for _, name := range sessionFiles {
			if name == parts[1] {
				return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
			}
		}
		return resource.StatResult{}, resource.ErrNotFound
	}
	return resource.StatResult{}, resource.ErrNotFound
}

func (p sessionProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, err := splitSession(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	if _, err := p.current(); err != nil {
		return resource.Listing{}, err
	}
	entries := make([]resource.Entry, 0, len(sessionFiles)+1)
	if len(parts) == 0 {
		return resource.Listing{Complete: true, Entries: []resource.Entry{{Name: "self", URI: "session://self/", Kind: resource.EntryDir}}}, nil
	}
	for _, name := range sessionFiles {
		entries = append(entries, resource.Entry{Name: name, URI: "session://self/" + name, Kind: resource.EntryFile})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return resource.Listing{Complete: true, Entries: entries}, nil
}

func (p sessionProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitSession(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	session, err := p.current()
	if err != nil {
		return resource.ReadResult{}, err
	}
	if len(parts) != 2 || parts[0] != "self" {
		return resource.ReadResult{}, resource.ErrUnsupported
	}
	body, err := p.render(ctx, session, parts[1])
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, body, request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func (p sessionProvider) render(ctx context.Context, session sdk.Context, name string) ([]byte, error) {
	switch name {
	case "meta":
		id, err := session.GetSessionID()
		if err != nil {
			return nil, err
		}
		entries, err := session.GetEntries()
		if err != nil {
			return nil, err
		}
		branch, err := session.GetBranch()
		if err != nil {
			return nil, err
		}
		leaf, err := session.GetLeafID()
		if err != nil {
			return nil, err
		}
		name, err := session.GetSessionName()
		if err != nil {
			return nil, err
		}
		meta := map[string]any{
			"sessionId":     id,
			"name":          name,
			"cwd":           session.Cwd(),
			"mode":          session.Mode(),
			"entries":       len(entries),
			"branchEntries": len(branch),
			"leafId":        leaf,
		}
		file, err := session.GetSessionFile()
		if err != nil {
			return nil, err
		}
		if file != nil {
			meta["file"] = *file
		} else {
			meta["durable"] = false
		}
		return json.MarshalIndent(meta, "", "  ")
	case "title":
		title, err := session.GetSessionName()
		if err != nil {
			return nil, err
		}
		if title == nil {
			return []byte("\n"), nil
		}
		return []byte(*title + "\n"), nil
	case "system-prompt":
		return []byte("delivery auto still serves PiG; /workbench prompt renders builtin/workbench without sending it\n"), nil
	case "entries":
		entries, err := session.GetEntries()
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, raw := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			out.WriteString(string(raw))
			out.WriteByte('\n')
		}
		return []byte(out.String()), nil
	case "transcript":
		branch, err := session.GetBranch()
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, entry := range branch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			switch entry.Role {
			case "user":
				fmt.Fprintf(&out, "## user\n\n%s\n\n", strings.TrimRight(entry.Content, "\n"))
			case "assistant":
				fmt.Fprintf(&out, "## assistant\n\n%s\n\n", strings.TrimRight(entry.Content, "\n"))
			case "toolResult":
				fmt.Fprintf(&out, "## tool %s\n\n%s\n\n", entry.ToolName, strings.TrimRight(entry.Content, "\n"))
			default:
				if entry.Content != "" {
					fmt.Fprintf(&out, "## %s\n\n%s\n\n", entry.Role, strings.TrimRight(entry.Content, "\n"))
				}
			}
		}
		return []byte(out.String()), nil
	}
	return nil, resource.ErrNotFound
}
