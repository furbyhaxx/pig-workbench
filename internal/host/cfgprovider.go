package host

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// cfgProvider exposes effective settings and status as read-only resources.
type cfgProvider struct {
	source func() (*config.Snapshot, config.Status)
	// integrations serves cfg://workbench/integrations/: owners with no argument, one owner's registrations otherwise.
	integrations func(owner string) (names []string, text string, ok bool)
	// activity returns the activity-line items so headless callers read the same facts.
	activity func() []activityItem
}

func (cfgProvider) Scheme() string { return "cfg" }

func (p cfgProvider) snapshot() (*config.Snapshot, config.Status, error) {
	snapshot, status := p.source()
	if snapshot == nil {
		return nil, status, errors.New("configuration is unavailable; fix the settings files and reload")
	}
	return snapshot, status, nil
}

func splitCfg(uri string) ([]string, error) {
	rest, ok := strings.CutPrefix(uri, "cfg:")
	if !ok {
		return nil, errors.New("cfg provider requires a cfg:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, nil
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("cfg:// segment must not be empty, '.' or '..'")
		}
	}
	return parts, nil
}

func (p cfgProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, err := splitCfg(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	if _, _, err := p.snapshot(); err != nil {
		return resource.StatResult{}, err
	}
	if len(parts) >= 2 && parts[0] == "workbench" && parts[1] == "integrations" && p.integrations != nil && len(parts) <= 3 {
		if len(parts) == 2 {
			return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
		}
		if _, _, ok := p.integrations(parts[2]); ok {
			return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
		}
		return resource.StatResult{}, resource.ErrNotFound
	}
	if len(parts) == 1 && parts[0] != "workbench" {
		for _, definition := range config.Catalog() {
			if strings.HasPrefix(definition.Key, parts[0]+".") {
				return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
			}
		}
		return resource.StatResult{}, resource.ErrNotFound
	}
	return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
}

func (p cfgProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, err := splitCfg(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	if len(parts) == 2 && parts[0] == "workbench" && parts[1] == "integrations" && p.integrations != nil {
		owners, _, _ := p.integrations("")
		var entries []resource.Entry
		for _, owner := range owners {
			entries = append(entries, resource.Entry{Name: owner, URI: "cfg://workbench/integrations/" + owner, Kind: resource.EntryFile})
		}
		return resource.PageEntries(entries, cursor, limit)
	}
	if len(parts) != 0 {
		return resource.Listing{}, resource.ErrUnsupported
	}
	groups := map[string]bool{}
	for _, definition := range config.Catalog() {
		group, _, _ := strings.Cut(definition.Key, ".")
		groups[group] = true
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]resource.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, resource.Entry{Name: name, URI: "cfg://" + name, Kind: resource.EntryDir})
	}
	return resource.PageEntries(entries, cursor, limit)
}

func (p cfgProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitCfg(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	snapshot, status, err := p.snapshot()
	if err != nil {
		return resource.ReadResult{}, err
	}
	var value any
	switch {
	case len(parts) == 0:
		value = map[string]any{"configRevision": snapshot.Revision(), "stale": status.Stale, "entries": snapshot.Entries(), "warnings": snapshot.Warnings()}
	case parts[0] == "workbench" && len(parts) == 2 && parts[1] == "status":
		value = map[string]any{
			"ready":          status.Ready,
			"stale":          status.Stale,
			"error":          status.Error,
			"configRevision": status.Revision,
			"read":           snapshot.Read(),
			"warnings":       snapshot.Warnings(),
		}
		if p.activity != nil {
			value.(map[string]any)["activity"] = p.activity()
		}
	case parts[0] == "workbench" && len(parts) == 3 && parts[1] == "integrations" && p.integrations != nil:
		_, text, ok := p.integrations(parts[2])
		if !ok {
			return resource.ReadResult{}, resource.ErrNotFound
		}
		page, err := resource.RenderText(ctx, request.URI, request.Requested, []byte(text), request.Selector, request.Budget)
		if err != nil {
			return resource.ReadResult{}, err
		}
		return resource.ReadResult{Content: page.Content, Facts: page}, nil
	case parts[0] == "workbench":
		return resource.ReadResult{}, resource.ErrNotFound
	default:
		entry, ok := snapshot.Get(strings.Join(parts, "."))
		if !ok {
			return resource.ReadResult{}, resource.ErrNotFound
		}
		value = entry
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, append(encoded, '\n'), request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}
