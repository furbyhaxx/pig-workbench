package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/skills"
)

type skillState struct {
	mu       sync.Mutex
	loaded   map[string]bool
	declined map[string]bool
	replayed map[string]bool
	touched  []string
	roots    func() []string
	persist  func(loaded, declined []string) error
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
	rest, ok := cutScheme(uri, "skill")
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
	replayed := s.skills.replayed[skill.Name]
	s.skills.loaded[skill.Name] = true
	delete(s.skills.declined, skill.Name)
	if replayed {
		delete(s.skills.replayed, skill.Name)
	}
	s.skills.mu.Unlock()
	s.skills.save()
	body := skill.Body
	if replayed {
		body = "loaded earlier; read before relying on it\n"
	} else if repeat {
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
	if s.skills.loaded == nil || !s.skills.loaded[parts[0]] {
		s.skills.mu.Unlock()
		return resource.ErrNotFound
	}
	delete(s.skills.loaded, parts[0])
	if s.skills.declined == nil {
		s.skills.declined = map[string]bool{}
	}
	s.skills.declined[parts[0]] = true
	s.skills.mu.Unlock()
	s.skills.save()
	return nil
}

func (st *skillState) save() {
	if st == nil || st.persist == nil {
		return
	}
	st.mu.Lock()
	loaded, declined := keys(st.loaded), keys(st.declined)
	st.mu.Unlock()
	_ = st.persist(loaded, declined)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name, on := range set {
		if on {
			out = append(out, name)
		}
	}
	return out
}

func (st *skillState) noteTouch(path string) {
	if st == nil || path == "" {
		return
	}
	st.mu.Lock()
	st.touched = append(st.touched, path)
	if len(st.touched) > 200 {
		st.touched = st.touched[len(st.touched)-200:]
	}
	st.mu.Unlock()
}

func restoreSkills(ctx sdk.Context, state *skillState) {
	entries, err := ctx.GetEntries()
	if err != nil || state == nil {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		var entry storedEntry
		if json.Unmarshal(entries[i], &entry) != nil || entry.CustomType != "pig-workbench.skills" {
			continue
		}
		var record struct {
			Loaded   []string `json:"loaded"`
			Declined []string `json:"declined"`
		}
		if json.Unmarshal(entry.Data, &record) != nil {
			return
		}
		state.mu.Lock()
		for _, name := range record.Loaded {
			state.loaded[name] = true
			state.replayed[name] = true
		}
		for _, name := range record.Declined {
			state.declined[name] = true
		}
		state.mu.Unlock()
		return
	}
}

func (s *session) deliverAutoSkills(ctx sdk.Context) {
	if s == nil || s.skills == nil {
		return
	}
	catalog, err := s.skills.catalog()
	if err != nil {
		return
	}
	s.skills.mu.Lock()
	declined := s.skills.declined
	loaded := s.skills.loaded
	touched := append([]string(nil), s.skills.touched...)
	s.skills.mu.Unlock()
	for _, skill := range skills.AutoLoad(skills.Visible(catalog, s.location.Current().Cwd, loaded), s.location.Current().Cwd, touched, declined) {
		s.skills.mu.Lock()
		already := s.skills.loaded[skill.Name]
		if !already {
			if s.skills.loaded == nil {
				s.skills.loaded = map[string]bool{}
			}
			s.skills.loaded[skill.Name] = true
		}
		s.skills.mu.Unlock()
		if already {
			continue
		}
		body := skill.Body
		if len(body) > 50<<10 {
			body = body[:50<<10] + "\n… truncated; read skill://" + skill.Name + "\n"
		}
		_ = ctx.SendMessage("pig-workbench.skill", "skill "+skill.Name+" auto-loaded\n"+body, false, sdk.SendMessageOptions{})
		s.skills.save()
	}
}
