package host

import (
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// hubItems lists every Workbench surface; counts appear only where they are known.
func (a *app) hubItems(ctx sdk.Context, s *session) []hubItem {
	tools := "Inspect tool schemas, owners and exposure"
	if all, err := ctx.GetAllTools(); err == nil {
		tools = fmt.Sprintf("%d registered · inspect schemas, owners and exposure", len(all))
	}
	jobs := "Inspect, wait for and stop shell jobs"
	var jobItems []activityItem
	for _, item := range s.activityItems() {
		if strings.HasPrefix(item.Open, "/workbench jobs") || item.Key == "jobs" {
			jobItems = append(jobItems, item)
		}
	}
	if len(jobItems) > 0 {
		jobs = activitySummary(jobItems) + " · inspect, wait for and stop shell jobs"
	}
	items := a.hubItemsStatic()
	items[0].description, items[1].description = tools, jobs
	display := "Nothing to configure: no extension registered one"
	if snapshot, _ := s.settings.Inspect(); snapshot != nil {
		if n := len(s.displayGroups(snapshot, "", "")); n > 0 {
			display = fmt.Sprintf("%d tool(s) with display preferences", n)
		}
	}
	for i := range items {
		if items[i].value == "settings tools" {
			items[i].description = display
		}
	}
	return items
}

// hubItemsStatic is the fixed entry list; hubItems fills in live counts.
func (a *app) hubItemsStatic() []hubItem {
	return []hubItem{
		{"tools", "Tools", "Inspect tool schemas, owners and exposure"},
		{"jobs", "Jobs", "Inspect, wait for and stop shell jobs"},
		{"settings", "Settings", "Edit scoped settings with provenance"},
		{"settings tools", "Tool display", "Per-tool display preferences"},
		{"instructions", "Instructions", "Active AGENTS.md and notes, pins and caps"},
		{"frames", "Frames", "Context frames delivered this session"},
		{"skills", "Skills", "Available and loaded skills"},
		{"prompt", "System prompt", "Rendered prompt, delivery mode and epoch"},
		{"sessions", "Sessions", "Live sessions and messaging"},
		{"integrations", "Integrations", "Extensions registered with Workbench"},
		{"resources", "Resources", "Registered URI schemes and providers"},
		{"trust", "Trust", "Trust rules and the effective decision"},
		{"projects", "Projects", "Project identity and session storage"},
		{"ask", "Questions", "Unanswered ask_user forms"},
		{"codemode", "Codemode calls", "Nested calls observed this runtime"},
		{"cd", "Location", "Working location without changing PiG's cwd"},
		{"show", "Status", "Session, location and configuration revision"},
	}
}

// hub opens the entry menu and runs the chosen view, returning to the menu afterwards.
func (a *app) hub(ctx sdk.Context) error {
	selected := 0
	for {
		s, err := a.current()
		if err != nil {
			return err
		}
		location := s.location.Current().Cwd
		panel := &hubPanel{title: "Workbench · " + location, items: a.hubItems(ctx, s), selected: selected, height: ctx.Height}
		result, err := ctx.Custom(panel, nil)
		if err != nil {
			return err
		}
		choice, _ := result.(string)
		if strings.TrimSpace(choice) == "" {
			return nil
		}
		label := choice
		for i, item := range panel.items {
			if item.value == choice {
				selected, label = i, item.label
			}
		}
		a.runFromHub(ctx, choice, label)
		if err := ctx.Err(); err != nil {
			return nil
		}
	}
}

// runFromHub runs one entry with panel output forced, showing a failure in a panel too.
func (a *app) runFromHub(ctx sdk.Context, choice, label string) {
	a.forcePanels.Add(1)
	a.hubTitle.Store("Workbench › " + label)
	defer func() { a.hubTitle.Store(""); a.forcePanels.Add(-1) }()
	if err := a.workbench(ctx, choice); err != nil {
		a.present(ctx, "Workbench › "+label, "Error: "+err.Error())
	}
}
