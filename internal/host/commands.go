package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"

	"github.com/furbyhaxx/pig-workbench/internal/textclean"
)

func (a *app) registerCommands(ext *sdk.Extension) {
	ext.RegisterCommand("workbench", sdk.CommandOptions{
		Description:            "Inspect Workbench state, settings, tools or working location.",
		GetArgumentCompletions: a.workbenchCompletions,
		Handler:                a.workbench,
	})
	for _, name := range []string{"cd", "wb:cd"} {
		ext.RegisterCommand(name, sdk.CommandOptions{Description: "Inspect or change the Workbench local working location.", GetArgumentCompletions: a.cdCompletions, Handler: a.cdCommand})
	}
	key, note := activityShortcutKey()
	a.shortcutKey, a.shortcutNote = key, note
	ext.Shortcut(key, "Show what Workbench is doing and how to inspect it.", a.activityShortcut)
	ext.RegisterCommand("tools", sdk.CommandOptions{Description: "List actual active and available tool schemas.", GetArgumentCompletions: a.toolsCompletions, Handler: a.toolsCommand})
	ext.RegisterCommand("wb:jobs", sdk.CommandOptions{Description: "Inspect or control Workbench jobs.", GetArgumentCompletions: a.jobsCompletions, Handler: func(ctx sdk.Context, args string) error {
		return a.jobsCommand(ctx, canonicalSubverb(args))
	}})
}

func (a *app) show(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	info := s.location.Current()
	_, settings := s.settings.Inspect()
	text := fmt.Sprintf("Workbench\nActivity: %s\nSession: %s\nGeneration: %s\nLocation: %s\nLocation revision: %d\nConfig revision: %d", activitySummary(s.activityItems()), s.id, s.generation, info.Cwd, info.Revision, settings.Revision)
	if settings.Stale {
		text += "\nConfiguration error: " + settings.Error
	}
	text += "\nActivity shortcut: " + a.shortcutKey
	if a.shortcutNote != "" {
		text += " (" + a.shortcutNote + ")"
	}
	a.present(ctx, "Workbench › Status", text)
	return nil
}

func (a *app) cdCommand(ctx sdk.Context, args string) error {
	args = strings.TrimSpace(args)
	params := map[string]any{}
	if args == "--reset" {
		params["reset"] = true
	} else if args != "" {
		params["path"] = args
	}
	result, err := a.cd(ctx, params)
	if err != nil {
		return err
	}
	a.notify(ctx, result.(sdk.ToolResult).Content)
	return nil
}

const toolsUsage = "usage: /workbench tools [name] [--format text|json|markdown]"

// toolRow is one tool as the tools view and its JSON format report it.
type toolRow struct {
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func (a *app) toolsCommand(ctx sdk.Context, args string) error {
	filter, format := "", "text"
	fields := strings.Fields(args)
	for i := 0; i < len(fields); i++ {
		switch {
		case fields[i] == "--format" && i+1 < len(fields):
			i++
			format = fields[i]
		case strings.HasPrefix(fields[i], "--") || filter != "":
			return errors.New(toolsUsage)
		default:
			filter = fields[i]
		}
	}
	if format != "text" && format != "json" && format != "markdown" {
		return errors.New(toolsUsage)
	}
	tools, err := ctx.GetAllTools()
	if err != nil {
		return err
	}
	a.rememberTools(tools)
	active, err := ctx.GetActiveTools()
	if err != nil {
		return err
	}
	var rows []toolRow
	for _, tool := range tools {
		if filter != "" && tool.Name != filter {
			continue
		}
		row := toolRow{Name: tool.Name, Active: slices.Contains(active, tool.Name), Description: tool.Description}
		// The list stays one line per tool; schemas appear for one tool or a machine format.
		if filter != "" || format == "json" {
			row.Parameters = tool.Parameters
		}
		rows = append(rows, row)
	}
	if filter != "" && len(rows) == 0 {
		names, _ := a.toolNames.Load().([]string)
		return fmt.Errorf("no tool named %q; tools: %s", noticeText(oneLine(filter, 40), 0), strings.Join(names, ", "))
	}
	text, err := formatTools(rows, format)
	if err != nil {
		return err
	}
	a.present(ctx, "Workbench › Tools", text)
	return nil
}

func formatTools(rows []toolRow, format string) (string, error) {
	if format == "json" {
		encoded, err := json.MarshalIndent(rows, "", "  ")
		return string(encoded), err
	}
	var b strings.Builder
	for i, row := range rows {
		state := "inactive"
		if row.Active {
			state = "active"
		}
		schema := ""
		if row.Parameters != nil {
			var indented bytes.Buffer
			if err := json.Indent(&indented, row.Parameters, "", "  "); err != nil {
				return "", err
			}
			schema = indented.String()
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		if format == "markdown" {
			fmt.Fprintf(&b, "## %s\n\n%s. %s\n", row.Name, state, row.Description)
			if schema != "" {
				fmt.Fprintf(&b, "\n```json\n%s\n```\n", schema)
			}
			continue
		}
		fmt.Fprintf(&b, "%s [%s] %s", row.Name, state, row.Description)
		if schema != "" {
			b.WriteString("\n" + schema)
		}
	}
	return b.String(), nil
}

// notify shows short text as a toast and anything longer in a scrollable panel when the TUI is available.
func (a *app) notify(ctx sdk.Context, text string) {
	title, _ := a.hubTitle.Load().(string)
	if title == "" {
		title = "Workbench"
	}
	a.present(ctx, title, text)
}

// present shows text under title: a scrollable panel for multi-line or wide text in the TUI, a notification elsewhere.
func (a *app) present(ctx sdk.Context, title, text string) {
	if panelsAvailable(ctx) && (a.forcePanels.Load() > 0 || strings.Contains(strings.TrimRight(text, "\n"), "\n") || tui.VisibleWidth(text) > max(20, ctx.Width()-4)) {
		if _, err := ctx.Custom(&textPanel{title: title, text: text, height: ctx.Height}, nil); err == nil {
			return
		}
	}
	toast(ctx, text)
}

func toast(ctx sdk.Context, text string) {
	width := 0
	if ctx.Mode() == "tui" {
		width = ctx.Width()
	}
	ctx.Notify(noticeText(text, width), "info")
}

func noticeText(text string, width int) string {
	text = tui.StripTerminalSequences(text)
	text = textclean.StripControl(strings.ReplaceAll(text, "\t", " "), '\n')
	if width > 0 {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			// The width helper adds ANSI resets even to plain text.
			lines[i] = tui.StripTerminalSequences(tui.TruncateToWidth(line, width, "…", false))
		}
		text = strings.Join(lines, "\n")
	}
	return text
}

func (a *app) jobsCommand(ctx sdk.Context, args string) error {
	fields := strings.Fields(args)
	params := map[string]any{"action": "list"}
	switch len(fields) {
	case 0:
	case 1:
		if fields[0] != "ls" {
			return fmt.Errorf("usage: /workbench jobs [ls | <id> status|output|kill|eof]")
		}
	case 2:
		params = map[string]any{"action": fields[1], "id": fields[0]}
	default:
		return fmt.Errorf("usage: /workbench jobs [ls | <id> status|output|kill|eof]")
	}
	if params["action"] == "list" {
		s, err := a.current()
		if err != nil {
			return err
		}
		if panelsAvailable(ctx) {
			title, _ := a.hubTitle.Load().(string)
			if title == "" {
				title = "Workbench › Jobs"
			}
			panel := &jobsPanel{title: title, rows: s.jobRows, height: ctx.Height}
			panel.act = func(id, action string) (string, error) {
				params := map[string]any{"action": action, "id": id}
				if action == "wait" {
					params["timeout_seconds"] = 3
				}
				result, err := a.jobsTool(ctx, params)
				if err != nil {
					return "", err
				}
				return result.(sdk.ToolResult).Content, nil
			}
			if _, err := ctx.Custom(panel, nil); err == nil {
				return nil
			}
		}
		a.notify(ctx, s.jobListText())
		return nil
	}
	result, err := a.jobsTool(ctx, params)
	if err != nil {
		return err
	}
	a.notify(ctx, result.(sdk.ToolResult).Content)
	return nil
}

// skillsCommand lists the whole catalog with each skill's state, or loads and unloads one for the user.
func (a *app) skillsCommand(ctx sdk.Context, args string) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	verb, name, _ := strings.Cut(strings.TrimSpace(args), " ")
	name = strings.TrimSpace(name)
	switch verb {
	case "", "ls", "list":
	case "load":
		skill, repeat, err := s.loadSkill(name)
		if err != nil {
			return fmt.Errorf("no skill %q; /workbench skills ls lists them", name)
		}
		if repeat {
			a.notify(ctx, "skill "+skill.Name+" is already loaded")
			return nil
		}
		a.skillFrame(ctx, s, "load/"+skill.Name, "skill "+skill.Name+" loaded by the user", "The user loaded skill "+skill.Name+"; apply it.\n"+skill.Body+"\nresolve relative paths against "+skill.Dir+"\n")
		a.notify(ctx, "skill "+skill.Name+" loaded")
		return nil
	case "unload":
		withdrawn, err := (skillProvider{}).unload(s, "skill://"+name+"/ctl", "unload")
		if err != nil {
			return fmt.Errorf("skill %q is not loaded", name)
		}
		a.skillUnloaded(ctx, s, withdrawn)
		a.notify(ctx, "unloaded "+strings.Join(withdrawn, ", "))
		return nil
	default:
		return fmt.Errorf("usage: /workbench skills [ls | load <name> | unload <name>]")
	}
	catalog, err := s.skills.catalog()
	if err != nil {
		return err
	}
	visible, loaded, err := s.listedSkills()
	if err != nil {
		return err
	}
	// A listed skill shows the description the model received after the budget ladder.
	listed := map[string]string{}
	for _, item := range visible {
		listed[item.Name] = item.Description
	}
	width := 0
	for _, skill := range catalog.Skills {
		width = max(width, len(skill.Name))
	}
	var b strings.Builder
	for _, skill := range catalog.Skills {
		state := "available"
		description, shown := listed[skill.Name]
		switch {
		case loaded[skill.Name]:
			state = "loaded"
		case shown:
		case skill.Parent != "":
			state, description = "hidden: load "+skill.Parent+" first", skill.Description
		default:
			state, description = "hidden: when "+strings.Join(append(slices.Clone(skill.WhenFiles), skill.WhenEnv...), ", ")+" not met", skill.Description
		}
		line := fmt.Sprintf("%-*s  %s", width, skill.Name, state)
		if description != "" {
			line += " · " + description
		}
		b.WriteString(line + "\n")
		for _, problem := range skill.Problems {
			fmt.Fprintf(&b, "%-*s  ! %s\n", width, "", problem)
		}
	}
	if b.Len() == 0 {
		b.WriteString(emptyState("skills", "") + "\n")
	}
	a.present(ctx, "Workbench › Skills", b.String())
	return nil
}

func (a *app) workbench(ctx sdk.Context, args string) error {
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	verb = canonicalVerb(verb)
	if subverbFamilies[verb] {
		rest = canonicalSubverb(rest)
	}
	switch verb {
	case "":
		if panelsAvailable(ctx) {
			return a.hub(ctx)
		}
		return a.show(ctx)
	case "show":
		return a.show(ctx)
	case "tools":
		return a.toolsCommand(ctx, rest)
	case "cd":
		return a.cdCommand(ctx, rest)
	case "settings":
		return a.settingsCommand(ctx, rest)
	case "jobs":
		return a.jobsCommand(ctx, rest)
	case "skills":
		return a.skillsCommand(ctx, rest)
	case "prompt":
		return a.promptCommand(ctx, rest)
	case "codemode":
		return a.codemodeCommand(ctx, rest)
	case "frames":
		return a.framesCommand(ctx, rest)
	case "instructions":
		return a.instructionsCommand(ctx, rest)
	case "ask":
		return a.askCommand(ctx, rest)
	case "projects":
		return a.projectsCommand(ctx, rest)
	case "sessions":
		return a.sessionsCommand(ctx, rest)
	case "integrations":
		return a.integrationsCommand(ctx, rest)
	case "resources":
		return a.resourcesCommand(ctx, rest)
	case "trust":
		return a.trustCommand(ctx, rest)
	default:
		return errors.New(unknownVerb(verb))
	}
}
