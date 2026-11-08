package host

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/prompt"
	"github.com/furbyhaxx/pig-workbench/internal/skills"
)

func (a *app) registerCommands(ext *sdk.Extension) {
	ext.RegisterCommand("workbench", sdk.CommandOptions{
		Description:            "Inspect Workbench state, settings, tools or working location.",
		GetArgumentCompletions: a.workbenchCompletions,
		Handler:                a.workbench,
	})
	for _, name := range []string{"cd", "wb:cd"} {
		ext.RegisterCommand(name, sdk.CommandOptions{Description: "Inspect or change the Workbench local working location.", Handler: a.cdCommand})
	}
	ext.Shortcut("alt+w", "Show what Workbench is doing and how to inspect it.", a.activityShortcut)
	ext.RegisterCommand("tools", sdk.CommandOptions{Description: "List actual active and available tool schemas.", Handler: a.toolsCommand})
	ext.RegisterCommand("wb:jobs", sdk.CommandOptions{Description: "Inspect or control Workbench jobs.", Handler: a.jobsCommand})
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

func (a *app) toolsCommand(ctx sdk.Context, args string) error {
	tools, err := ctx.GetAllTools()
	if err != nil {
		return err
	}
	active, err := ctx.GetActiveTools()
	if err != nil {
		return err
	}
	enabled := make(map[string]bool, len(active))
	for _, name := range active {
		enabled[name] = true
	}
	var lines []string
	filter := strings.TrimSpace(args)
	for _, tool := range tools {
		if filter != "" && tool.Name != filter {
			continue
		}
		state := "inactive"
		if enabled[tool.Name] {
			state = "active"
		}
		line := tool.Name + " [" + state + "] " + tool.Description
		if filter != "" {
			schema, err := json.MarshalIndent(tool.Parameters, "", "  ")
			if err != nil {
				return err
			}
			line += "\n" + string(schema)
		}
		lines = append(lines, line)
	}
	if filter != "" && len(lines) == 0 {
		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.Name
		}
		ctx.Notify(noticeText(fmt.Sprintf("No tool named %q. Tools: %s. Settings and other views start from /workbench.", filter, strings.Join(names, ", ")), 0), "warning")
		return nil
	}
	a.present(ctx, "Workbench › Tools", strings.Join(lines, "\n"))
	return nil
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
	text = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if r < 32 || r >= 127 && r <= 159 {
			return -1
		}
		return r
	}, text)
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
		if fields[0] != "list" {
			return fmt.Errorf("usage: /workbench jobs [list | <id> status|output|kill|eof]")
		}
	case 2:
		params = map[string]any{"action": fields[1], "id": fields[0]}
	default:
		return fmt.Errorf("usage: /workbench jobs [list | <id> status|output|kill|eof]")
	}
	if params["action"] == "list" {
		s, err := a.current()
		if err != nil {
			return err
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

func (a *app) skillsCommand(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	catalog, err := s.skills.catalog()
	if err != nil {
		return err
	}
	s.skills.mu.Lock()
	loaded := s.skills.loaded
	s.skills.mu.Unlock()
	visible := skills.Visible(catalog, s.location.Current().Cwd, loaded)
	budget := 4096
	if configuration, err := s.settings.Current(); err == nil {
		if entry, ok := configuration.Get("skills.listing-budget"); ok {
			if value, ok := entry.Value.(int64); ok {
				budget = int(value)
			}
		}
	}
	var b strings.Builder
	for _, item := range skills.List(visible, budget) {
		state := "available"
		if loaded[item.Name] {
			state = "loaded"
		}
		fmt.Fprintf(&b, "%s [%s] %s %s\n", item.Name, state, item.Location, item.Description)
	}
	if b.Len() == 0 {
		b.WriteString("no skills\n")
	}
	a.present(ctx, "Workbench › Skills", b.String())
	return nil
}

func (a *app) promptCommand(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	configuration, err := s.settings.Current()
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, key := range []string{"prompt.enabled", "prompt.template", "prompt.delivery"} {
		entry, ok := configuration.Get(key)
		if !ok {
			fmt.Fprintf(&b, "%s: unavailable\n", key)
			continue
		}
		fmt.Fprintf(&b, "%s: %v\n", key, entry.Value)
	}
	rendered, err := renderBuiltinPrompt(s)
	if err != nil {
		return err
	}
	s.promptText = rendered
	if s.promptErr != "" {
		fmt.Fprintf(&b, "last error: %s\n", s.promptErr)
	}
	fmt.Fprintf(&b, "epoch: %s\n", promptEpoch(rendered))
	b.WriteString("rendered builtin/workbench:\n")
	b.WriteString(rendered)
	b.WriteString("\ndelivery auto still serves PiG until the approved A/B run; prompt.delivery=force replaces it\n")
	a.notify(ctx, b.String())
	return nil
}

func renderBuiltinPrompt(s *session) (string, error) {
	loader, err := prompt.NewFileSystemLoader(prompt.FileSystemOptions{Builtin: prompt.BuiltinFS()})
	if err != nil {
		return "", err
	}
	engine, err := prompt.New(prompt.Options{Loader: loader})
	if err != nil {
		return "", err
	}
	root := ""
	if s.location != nil {
		root = s.location.Current().Cwd
	}
	rendered, err := engine.Render(context.Background(), "builtin/workbench", prompt.Data{ProjectRoot: root})
	if err != nil {
		return "", err
	}
	return rendered.Text, nil
}

func (a *app) workbench(ctx sdk.Context, args string) error {
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
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
		return a.skillsCommand(ctx)
	case "prompt":
		return a.promptCommand(ctx)
	case "codemode":
		return a.codemodeCommand(ctx, rest)
	case "frames":
		return a.framesCommand(ctx)
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
		return fmt.Errorf("usage: /workbench show | tools | cd [path|--reset] | jobs [list|<id> status|output|kill|eof] | skills | prompt | codemode [call id] | frames | instructions [pin|unpin|ls|rules|reload] | ask [show|resume|discard] | trust [ls|add|rm] | projects [status|unlink-storage] | sessions [ls|send|trust|untrust|block|unblock|hide|show] | integrations [owner] | resources [scheme] | settings [show|catalog|reload|set|reset]")
	}
}
