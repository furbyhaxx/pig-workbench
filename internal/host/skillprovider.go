package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	order    []string
	touched  []string
	roots    func() []string
	host     []skills.Source
	stamp    string
	cached   skills.Catalog
	persist  func(loaded, declined []string) error
}

// setHost retains PiG's discovered skills that the model may invoke; they lead the catalog.
func (s *skillState) setHost(discovered []sdk.SystemPromptSkill) {
	if s == nil {
		return
	}
	sources := make([]skills.Source, 0, len(discovered))
	for _, skill := range discovered {
		if skill.DisableModelInvocation || skill.FilePath == "" {
			continue
		}
		sources = append(sources, skills.Source{Name: skill.Name, Description: skill.Description, File: skill.FilePath})
	}
	s.mu.Lock()
	s.host = sources
	s.mu.Unlock()
}

func (s *skillState) catalog() (skills.Catalog, error) {
	if s == nil || s.roots == nil {
		return skills.Catalog{}, errors.New("skill catalog is unavailable")
	}
	s.mu.Lock()
	host := slices.Clone(s.host)
	s.mu.Unlock()
	// Every prompt and tool boundary asks for the listing; files are read again only after one changed.
	roots := s.roots()
	stamp := strings.Join(roots, "\n") + "\n" + skills.Stamp(host, roots)
	s.mu.Lock()
	if s.stamp == stamp && stamp != "" {
		cached := s.cached
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()
	catalog, err := skills.Build(host, roots)
	if err != nil {
		return catalog, err
	}
	s.mu.Lock()
	s.stamp, s.cached = stamp, catalog
	s.mu.Unlock()
	return catalog, nil
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
	slices.Sort(names)
	entries := make([]resource.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, resource.Entry{Name: name, URI: "skill://" + name, Kind: resource.EntryFile})
	}
	return resource.PageEntries(entries, cursor, limit)
}

func (p skillProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitSkill(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	name := strings.Join(parts, "/")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "loaded" || parts[len(parts)-1] == "ctl" {
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
	skill, err := skills.Find(catalog, name)
	if err != nil {
		return resource.ReadResult{}, resource.ErrNotFound
	}
	if request.Selector.Explicit {
		// A line range, including the continuation of a long body, shows the text itself and leaves the loaded set alone.
		page, err := resource.RenderText(ctx, request.URI, request.Requested, []byte(skill.Body+"\nresolve relative paths against "+skill.Dir+"\n"), request.Selector, request.Budget)
		if err != nil {
			return resource.ReadResult{}, err
		}
		return resource.ReadResult{Content: page.Content, Facts: page}, nil
	}
	s.skills.mu.Lock()
	if s.skills.loaded == nil {
		s.skills.loaded = map[string]bool{}
	}
	repeat := s.skills.loaded[skill.Name]
	replayed := s.skills.replayed[skill.Name]
	s.skills.markLoaded(skill.Name)
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

func (p skillProvider) unload(s *session, uri, content string) ([]string, error) {
	parts, err := splitSkill(uri)
	if err != nil {
		return nil, err
	}
	if len(parts) < 2 || len(parts) > 3 || parts[len(parts)-1] != "ctl" || strings.TrimSpace(content) != "unload" {
		return nil, fmt.Errorf("skill:// writes only <name>/ctl with unload")
	}
	name := strings.Join(parts[:len(parts)-1], "/")
	catalog, _ := s.skills.catalog()
	s.skills.mu.Lock()
	if s.skills.loaded == nil || !s.skills.loaded[name] {
		s.skills.mu.Unlock()
		return nil, resource.ErrNotFound
	}
	if s.skills.declined == nil {
		s.skills.declined = map[string]bool{}
	}
	withdrawn := []string{name}
	for i := 0; i < len(withdrawn); i++ {
		parent, err := skills.Find(catalog, withdrawn[i])
		if err != nil {
			continue
		}
		for _, skill := range catalog.Skills {
			if skill.Parent != "" && skill.Parent == parent.Name && s.skills.loaded[skill.Name] && !slices.Contains(withdrawn, skill.Name) {
				withdrawn = append(withdrawn, skill.Name)
			}
		}
	}
	for _, gone := range withdrawn {
		s.skills.markUnloaded(gone)
	}
	s.skills.declined[name] = true
	s.skills.mu.Unlock()
	s.skills.save()
	return withdrawn, nil
}

// markLoaded records a load; the caller holds mu.
func (st *skillState) markLoaded(name string) {
	st.loaded[name] = true
	if !slices.Contains(st.order, name) {
		st.order = append(st.order, name)
	}
}

// markUnloaded forgets a load; the caller holds mu.
func (st *skillState) markUnloaded(name string) {
	delete(st.loaded, name)
	delete(st.replayed, name)
	st.order = slices.DeleteFunc(st.order, func(item string) bool { return item == name })
}

func (st *skillState) save() {
	if st == nil || st.persist == nil {
		return
	}
	st.mu.Lock()
	loaded, declined := slices.Clone(st.order), keys(st.declined)
	for name, on := range st.loaded {
		if on && !slices.Contains(loaded, name) {
			loaded = append(loaded, name)
		}
	}
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
			state.markLoaded(name)
			state.replayed[name] = true
		}
		for _, name := range record.Declined {
			state.declined[name] = true
		}
		state.mu.Unlock()
		return
	}
}

// listedSkills returns the visible skills after the skills.listing-budget ladder, plus the loaded set.
func (s *session) listedSkills() ([]skills.Listed, map[string]bool, error) {
	catalog, err := s.skills.catalog()
	if err != nil {
		return nil, nil, err
	}
	s.skills.mu.Lock()
	loaded := maps.Clone(s.skills.loaded)
	s.skills.mu.Unlock()
	budget := 4096
	if configuration, err := s.settings.Current(); err == nil {
		if entry, ok := configuration.Get("skills.listing-budget"); ok {
			if value, ok := entry.Value.(int64); ok {
				budget = int(value)
			}
		}
	}
	return skills.List(skills.Visible(catalog, s.location.Current().Cwd, loaded), budget), loaded, nil
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
			s.skills.markLoaded(skill.Name)
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

const skillReplayBudget = 20

// replaySkills tells the model, after compaction, which skills it loaded earlier, newest first, as pointers.
func (s *session) replaySkills(ctx sdk.Context) {
	if s == nil || s.skills == nil {
		return
	}
	s.skills.mu.Lock()
	names := slices.Clone(s.skills.order)
	slices.Reverse(names)
	if len(names) > skillReplayBudget {
		names = names[:skillReplayBudget]
	}
	for _, name := range names {
		s.skills.replayed[name] = true
	}
	s.skills.mu.Unlock()
	if len(names) == 0 {
		return
	}
	var text strings.Builder
	text.WriteString("skills loaded earlier (read skill://<name> before relying on them):\n")
	for _, name := range names {
		text.WriteString("- skill://" + name + "\n")
	}
	_ = ctx.SendMessage("pig-workbench.skill", text.String(), false, sdk.SendMessageOptions{})
}

func (a *app) replaySkills(ctx sdk.Context, _ map[string]any) (any, error) {
	if s, err := a.current(); err == nil {
		s.replaySkills(ctx)
	}
	return nil, nil
}
