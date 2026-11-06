package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/skills"
)

type skillState struct {
	mu     sync.Mutex
	loaded map[string]bool
	roots  func() []string
}

func (s *skillState) catalog() (skills.Catalog, error) {
	if s == nil || s.roots == nil {
		return skills.Catalog{}, errors.New("skill catalog is unavailable")
	}
	return skills.Scan(s.roots())
}

type skillProvider struct{ source func() (*session, bool) }

func (skillProvider) Scheme() string { return "skill" }

func splitSkill(uri string) ([]string, error) {
	rest, ok := strings.CutPrefix(uri, "skill://")
	if !ok {
		return nil, errors.New("skill provider requires a skill:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, nil
	}
	return strings.Split(rest, "/"), nil
}

func (p skillProvider) session() (*session, error) {
	s, ok := p.source()
	if !ok || s == nil || s.skills == nil {
		return nil, errors.New("skill resources are unavailable outside an active session")
	}
	return s, nil
}

func (p skillProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, err := splitSkill(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	if _, err := p.session(); err != nil {
		return resource.StatResult{}, err
	}
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "loaded") {
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	}
	return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
}

func (p skillProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, err := splitSkill(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	s, err := p.session()
	if err != nil {
		return resource.Listing{}, err
	}
	catalog, err := s.skills.catalog()
	if err != nil {
		return resource.Listing{}, err
	}
	s.skills.mu.Lock()
	loaded := s.skills.loaded
	s.skills.mu.Unlock()
	var names []string
	if len(parts) == 1 && parts[0] == "loaded" {
		for name, on := range loaded {
			if on {
				names = append(names, name)
			}
		}
	} else if len(parts) == 0 {
		for _, skill := range skills.Visible(catalog, s.location.Current().Cwd, loaded) {
			names = append(names, skill.Name)
		}
	} else {
		return resource.Listing{}, errors.New("skill:// lists the catalog or skill://loaded")
	}
	entries := make([]resource.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, resource.Entry{Name: name, URI: "skill://" + name, Kind: resource.EntryFile})
	}
	return resource.Listing{Entries: entries, Complete: true}, nil
}

func (p skillProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitSkill(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	if len(parts) != 1 || parts[0] == "loaded" {
		return resource.ReadResult{}, errors.New("read skill://<name> to load one skill")
	}
	s, err := p.session()
	if err != nil {
		return resource.ReadResult{}, err
	}
	catalog, err := s.skills.catalog()
	if err != nil {
		return resource.ReadResult{}, err
	}
	skill, err := skills.Find(catalog, parts[0])
	if err != nil {
		return resource.ReadResult{}, resource.ErrNotFound
	}
	s.skills.mu.Lock()
	if s.skills.loaded == nil {
		s.skills.loaded = map[string]bool{}
	}
	repeat := s.skills.loaded[skill.Name]
	s.skills.loaded[skill.Name] = true
	s.skills.mu.Unlock()
	body := skill.Body
	if repeat {
		body = "already loaded; read the directory " + skill.Dir + " before relying on it\n"
	} else {
		body += "\nresolve relative paths against " + skill.Dir + "\n"
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, []byte(body), request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func (p skillProvider) unload(s *session, uri, content string) error {
	parts, err := splitSkill(uri)
	if err != nil {
		return err
	}
	if len(parts) != 2 || parts[1] != "ctl" || strings.TrimSpace(content) != "unload" {
		return fmt.Errorf("skill:// writes only <name>/ctl with unload")
	}
	s.skills.mu.Lock()
	defer s.skills.mu.Unlock()
	if s.skills.loaded == nil || !s.skills.loaded[parts[0]] {
		return resource.ErrNotFound
	}
	delete(s.skills.loaded, parts[0])
	return nil
}
