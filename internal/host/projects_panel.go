package host

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

type projectRow struct{ slug, id, name string }

// projectsView is what the panel shows; it is read again after every change.
type projectsView struct {
	status           string
	rows             []projectRow
	defaultSlug      string
	defaultComponent string
	hereID           string
}

// projectsPanel creates a project at the current location or joins it to a registered one.
type projectsPanel struct {
	title    string
	height   func() int
	view     func() projectsView
	create   func(slug, name, description string) (string, error)
	join     func(ref string, mode project.JoinMode, component string) (string, error)
	mode     string // list, join or form
	selected int
	target   projectRow
	prompts  []string
	values   []string
	step     int
	input    string
	finish   func(values []string) (string, error)
	message  string
	defaults []string
	loaded   bool
	cached   projectsView
}

// current returns the cached view, reading it again only after a change.
func (p *projectsPanel) current() projectsView {
	if !p.loaded {
		p.cached, p.loaded = p.view(), true
	}
	return p.cached
}

func (p *projectsPanel) View(width int) kit.View {
	view := p.current()
	switch p.mode {
	case "form":
		input := kit.NewInput("field")
		input.Value = p.input
		prompt := fmt.Sprintf("%s (%d/%d)", p.prompts[p.step], p.step+1, len(p.prompts))
		body := kit.NewContainer(kit.NewText(fitLine(prompt, width), 1, 0), input)
		hint := "enter next · " + sdk.KeyText("tui.select.cancel") + " back"
		return kit.View{Root: panelFrame(p.title, body, p.message, hint, width), Focus: "field"}
	case "join":
		items := []kit.SelectItem{
			{Value: "file", Label: "Join by file", Description: fitLine("write .project/Project.toml; commit it to share", max(20, width-24))},
			{Value: "marker", Label: "Join by local marker", Description: fitLine("git config only; no files written", max(20, width-24))},
		}
		list := kit.NewSelectList("mode", items, 2)
		list.SetSelectedIndex(max(0, min(p.selected, 1)))
		hint := sdk.KeyHint("tui.select.confirm", "choose") + " · " + sdk.KeyHint("tui.select.cancel", "back")
		return kit.View{Root: panelFrame(p.title+" › join "+p.target.slug, list, p.message, hint, width), Focus: "mode"}
	}
	items := []kit.SelectItem{{Value: "", Label: "+ New project here", Description: fitLine("create .project/Project.toml and register it", max(20, width-24))}}
	for _, row := range view.rows {
		label := fitLine(row.slug+"  "+row.id, max(12, width-4))
		items = append(items, kit.SelectItem{Value: row.slug, Label: label, Description: fitLine(row.name, max(20, width-4))})
	}
	p.selected = max(0, min(p.selected, len(items)-1))
	list := kit.NewSelectList("projects", items, listRows(p.height(), 8, len(items)))
	list.SetSelectedIndex(p.selected)
	status := p.message
	if status == "" {
		status = view.status
	}
	hint := sdk.KeyHint("tui.select.confirm", "select") + " · " + sdk.KeyHint("tui.select.cancel", "close")
	return kit.View{Root: panelFrame(p.title, list, status, hint, width), Focus: "projects"}
}

func (p *projectsPanel) startForm(prompts, defaults []string, finish func([]string) (string, error)) {
	p.mode, p.prompts, p.defaults, p.values, p.step, p.finish, p.message = "form", prompts, defaults, nil, 0, finish, ""
	p.input = defaults[0]
}

func (p *projectsPanel) back() {
	p.message, p.selected = "", 0
	switch p.mode {
	case "form":
		p.mode = "list"
		if p.target.slug != "" {
			p.mode = "join"
		}
	default:
		p.mode, p.target = "list", projectRow{}
	}
}

func (p *projectsPanel) submit(value string) {
	value = strings.TrimSpace(value)
	if p.step == 0 && value == "" {
		p.message = p.prompts[0] + " must not be empty"
		return
	}
	p.values = append(p.values, value)
	if p.step+1 < len(p.prompts) {
		p.step++
		p.input, p.message = p.defaults[p.step], ""
		return
	}
	message, err := p.finish(p.values)
	p.loaded = false
	if err != nil {
		// Keep what the user typed so a typo is corrected, not retyped.
		p.message = noticeText(err.Error(), 0)
		p.defaults = p.values
		p.values, p.step, p.input = nil, 0, p.defaults[0]
		return
	}
	p.mode, p.target, p.selected, p.message = "list", projectRow{}, 0, noticeText(message, 0)
}

func (p *projectsPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	if tui.GetKeybindings().Matches(data, "tui.select.cancel") {
		return p.cancel(), nil
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *projectsPanel) cancel() sdk.RemoteComponentResult {
	if p.mode == "list" {
		return sdk.RemoteComponentResult{Done: true}
	}
	p.back()
	return sdk.RemoteComponentResult{}
}

func (p *projectsPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if event.Type == kit.EventCancel || event.Type == kit.EventEscape {
		return p.cancel(), nil
	}
	switch p.mode {
	case "form":
		switch event.Type {
		case kit.EventChange:
			p.input = event.Value
		case kit.EventSubmit:
			value := p.input
			if event.Value != "" {
				value = event.Value
			}
			p.submit(value)
		}
	case "join":
		switch event.Type {
		case kit.EventSelectionChange:
			p.selected = event.Index
		case kit.EventSelect:
			if event.Item == nil {
				break
			}
			if event.Item.Value == "marker" {
				message, err := p.join(p.target.slug, project.JoinMarker, "")
				p.loaded = false
				p.mode, p.target, p.selected = "list", projectRow{}, 0
				p.message = noticeText(firstText(message, err), 0)
				break
			}
			target := p.target
			view := p.current()
			if target.id == view.hereID {
				message, err := p.join(target.slug, project.JoinFile, "")
				p.loaded = false
				p.mode, p.target, p.selected = "list", projectRow{}, 0
				p.message = noticeText(firstText(message, err), 0)
				break
			}
			p.startForm([]string{"Component slug of this repository"}, []string{view.defaultComponent}, func(values []string) (string, error) {
				return p.join(target.slug, project.JoinFile, values[0])
			})
		}
	default:
		switch event.Type {
		case kit.EventSelectionChange:
			p.selected = event.Index
		case kit.EventSelect:
			if event.Item == nil {
				break
			}
			if event.Item.Value == "" {
				view := p.current()
				p.target = projectRow{}
				p.startForm([]string{"Slug (lowercase, kebab)", "Name (optional)", "Description (optional)"}, []string{view.defaultSlug, "", ""}, func(values []string) (string, error) {
					return p.create(values[0], values[1], values[2])
				})
				break
			}
			for _, row := range p.current().rows {
				if row.slug == event.Item.Value {
					p.mode, p.target, p.selected, p.message = "join", row, 0, ""
				}
			}
		}
	}
	return sdk.RemoteComponentResult{}, nil
}

func firstText(message string, err error) string {
	if err != nil {
		return err.Error()
	}
	return message
}

// projectsPanel opens the interactive creation and selection panel for the session's current location.
func (a *app) projectsPanel(ctx sdk.Context) error {
	title, _ := a.hubTitle.Load().(string)
	if title == "" {
		title = "Workbench › Projects"
	}
	with := func(run func(s *session, work context.Context) (string, error)) (string, error) {
		s, work, release, err := a.admit(ctx)
		if err != nil {
			return "", err
		}
		defer release()
		return run(s, work)
	}
	view := func() projectsView {
		var v projectsView
		_, err := with(func(s *session, work context.Context) (string, error) {
			cwd := s.location.Current().Cwd
			v.defaultSlug = project.SlugFromName(filepath.Base(cwd))
			v.defaultComponent = v.defaultSlug
			detected, err := s.projects.Detect(work, cwd)
			if err != nil {
				v.status = "Recognition failed: " + noticeText(err.Error(), 0)
			} else {
				v.status = fmt.Sprintf("Here: %s (%s), recognised by %s", detected.Slug, detected.ProjectID, detected.How)
				v.hereID = detected.ProjectID
			}
			entries, _ := s.projects.Peek(work)
			for _, entry := range entries {
				v.rows = append(v.rows, projectRow{slug: entry.Project.Slug, id: entry.Project.ID, name: entry.Project.Name})
			}
			return "", nil
		})
		if err != nil {
			v.status = err.Error()
		}
		return v
	}
	create := func(slug, name, description string) (string, error) {
		return with(func(s *session, work context.Context) (string, error) {
			return s.createProject(work, slug, name, description)
		})
	}
	join := func(ref string, mode project.JoinMode, component string) (string, error) {
		return with(func(s *session, work context.Context) (string, error) {
			return s.joinProject(work, ref, mode, component)
		})
	}
	_, err := ctx.Custom(&projectsPanel{title: title, height: ctx.Height, view: view, create: create, join: join, mode: "list"}, nil)
	return err
}
