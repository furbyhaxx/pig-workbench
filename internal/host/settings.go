package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

// workbenchCompletions completes subcommands statically and owners and schemes from the live registries.
func (a *app) workbenchCompletions(prefix string) ([]sdk.AutocompleteItem, error) {
	var live []string
	catalog := config.Catalog()
	if s, err := a.current(); err == nil {
		if snapshot, _ := s.settings.Inspect(); snapshot != nil {
			catalog = snapshot.Catalog()
		}
		for _, owner := range s.integrationOwners() {
			live = append(live, "integrations "+owner)
		}
		for _, scheme := range s.router.Schemes() {
			live = append(live, "resources "+scheme)
		}
		if catalog, err := s.skills.catalog(); err == nil {
			s.skills.mu.Lock()
			for _, skill := range catalog.Skills {
				if s.skills.loaded[skill.Name] {
					live = append(live, "skills unload "+skill.Name)
				} else {
					live = append(live, "skills load "+skill.Name)
				}
			}
			s.skills.mu.Unlock()
		}
		if s.projects != nil {
			entries, _ := s.projects.Peek(context.Background())
			for _, entry := range entries {
				live = append(live, "projects join "+entry.Project.Slug, "projects join "+entry.Project.Slug+" file", "projects join "+entry.Project.Slug+" marker")
			}
		}
	}
	return completeWorkbench(prefix, live, catalog)
}

func completeWorkbench(prefix string, live []string, catalog []config.Definition) ([]sdk.AutocompleteItem, error) {
	candidates := []string{"show", "tools", "cd", "jobs", "skills", "skills ls", "skills load", "skills unload", "prompt", "codemode", "instructions", "instructions pin", "instructions unpin", "instructions ls", "instructions rules", "instructions reload", "ask", "ask show", "ask resume", "ask discard", "trust", "trust ls", "trust add allow", "trust add deny", "trust rm", "projects", "projects status", "projects list", "projects create", "projects join", "projects join file", "projects join marker", "projects example", "projects unlink-storage", "sessions", "sessions ls", "sessions send", "sessions trust", "sessions untrust", "sessions block", "sessions unblock", "sessions hide", "sessions show", "integrations", "resources", "frames", "frames ls", "frames show", "frames collapse", "frames hide", "settings", "settings tools", "settings tools show", "settings tools set", "settings tools reset", "settings show", "settings catalog", "settings reload", "settings set", "settings reset"}
	for _, scope := range []string{"user", "project", "runtime"} {
		candidates = append(candidates, "settings set "+scope, "settings reset "+scope)
	}
	for _, definition := range catalog {
		parts, _ := parser.ParseKey(definition.Key)
		if len(parts) == 4 && parts[0] == "tools" {
			owner, key, field := commandIdentifier(parts[1]), commandIdentifier(parts[2]), commandIdentifier(parts[3])
			candidates = append(candidates, "settings tools "+owner, "settings tools "+owner+" "+key, "settings tools show "+owner+" "+key)
			for _, scope := range []string{"user", "project", "runtime"} {
				set := "settings tools set " + scope + " " + owner + " " + key + " " + field
				candidates = append(candidates, set, "settings tools reset "+scope+" "+owner+" "+key+" "+field)
				if definition.Kind == config.Boolean {
					candidates = append(candidates, set+" true", set+" false")
				}
				for _, choice := range definition.Choices {
					candidates = append(candidates, set+" "+strconv.Quote(choice))
				}
			}
		}
		candidates = append(candidates, "settings show "+definition.Key, "settings catalog "+definition.Key)
		for _, scope := range []string{"user", "project", "runtime"} {
			if scope == "project" && definition.Project == config.DenyProject {
				continue
			}
			base := "settings set " + scope + " " + definition.Key
			candidates = append(candidates, base, "settings reset "+scope+" "+definition.Key)
			if definition.Kind == config.Boolean {
				candidates = append(candidates, base+" true")
				if scope != "project" || definition.Project != config.RequireTrueProject {
					candidates = append(candidates, base+" false")
				}
			}
			for _, choice := range definition.Choices {
				candidates = append(candidates, base+" "+strconv.Quote(choice))
			}
		}
	}
	candidates = append(candidates, live...)
	var result []sdk.AutocompleteItem
	seen := map[string]bool{}
	for _, value := range candidates {
		if seen[value] || !strings.HasPrefix(value, prefix) {
			continue
		}
		_, tail, err := commandFields(strings.TrimPrefix(value, prefix), 1)
		if err == nil && tail == "" {
			seen[value] = true
			result = append(result, sdk.AutocompleteItem{Value: value, Label: value})
		}
	}
	return result, nil
}

func loadSettings(ctx sdk.Context) (*config.Manager, error) {
	agent, err := sdk.AgentDir()
	if err != nil {
		return nil, err
	}
	dirname, err := sdk.ConfigDirName()
	if err != nil {
		return nil, err
	}
	trusted, err := ctx.IsProjectTrusted()
	if err != nil {
		return nil, err
	}
	manager, err := config.New(config.Paths{
		User:    filepath.Join(agent, "Workbench.toml"),
		Project: filepath.Join(ctx.Cwd(), dirname, "Workbench.toml"),
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := manager.Load(requestContext{ctx}, trusted); err != nil {
		ctx.Notify(noticeText(err.Error(), 0), "error")
	}
	return manager, nil
}

func (a *app) settingsCommand(ctx sdk.Context, args string) error {
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	rest = strings.TrimSpace(rest)
	switch verb {
	case "tools":
		if rest == "" && panelsAvailable(ctx) {
			return a.toolDisplayPanel(ctx, work, s)
		}
		return a.displaySettingsCommand(ctx, work, s, rest)
	case "":
		if panelsAvailable(ctx) {
			return a.settingsPanel(ctx, work, s)
		}
		fallthrough
	case "show":
		snapshot, status := s.settings.Inspect()
		output := map[string]any{"settings": status}
		if snapshot != nil {
			output["warnings"] = snapshot.Warnings()
			if rest != "" {
				entry, ok := snapshot.Get(rest)
				if !ok {
					return fmt.Errorf("setting %q not found; use /workbench settings catalog", rest)
				}
				output["entry"] = entry
			} else {
				output["entries"] = snapshot.Entries()
			}
		}
		return a.notifyJSON(ctx, output)
	case "catalog":
		snapshot, _ := s.settings.Inspect()
		catalog := config.Catalog()
		if snapshot != nil {
			catalog = snapshot.Catalog()
		}
		var definitions []config.Definition
		for _, definition := range catalog {
			if strings.HasPrefix(definition.Key, rest) {
				definitions = append(definitions, definition)
			}
		}
		return a.notifyJSON(ctx, map[string]any{"catalog": definitions})
	case "reload":
		trusted, err := ctx.IsProjectTrusted()
		if err != nil {
			return err
		}
		if err := s.settings.Load(work, trusted); err != nil {
			return err
		}
		snapshot, err := s.settings.Current()
		if err != nil {
			return err
		}
		s.secrets.Reload()
		a.settingsChanged(ctx, s, snapshot.Revision(), nil)
		return a.notifyJSON(ctx, map[string]any{"settings": "reloaded", "configRevision": snapshot.Revision(), "warnings": snapshot.Warnings()})
	case "set", "reset":
		scope, tail, _ := strings.Cut(rest, " ")
		tail = strings.TrimSpace(tail)
		key, valueText, hasValue := strings.Cut(tail, " ")
		if scope == "" || key == "" || verb == "set" && !hasValue || verb == "reset" && hasValue {
			return errors.New("usage: /workbench settings set <user|project|runtime> <key> <TOML value> | reset <scope> <key>")
		}
		baseline, err := s.settings.Current()
		if err != nil {
			return err
		}
		edit := config.Edit{Key: key, Reset: verb == "reset"}
		if !edit.Reset {
			value, err := config.ParseValue(strings.TrimSpace(valueText))
			if err != nil {
				return err
			}
			edit.Value = value
		}
		result, saveErr := s.settings.Save(work, config.Scope(scope), baseline, []config.Edit{edit}, ctx.IsProjectTrusted)
		if result.Published {
			a.settingsChanged(ctx, s, result.Revision, result.Changed)
		}
		if err := a.notifyJSON(ctx, map[string]any{"settings": "save", "result": result}); err != nil {
			return err
		}
		return saveErr
	default:
		return errors.New("usage: /workbench settings [show [key]|catalog [prefix]|reload|set <scope> <key> <TOML value>|reset <scope> <key>]")
	}
}

// saveSettingEdits writes edits at scope and announces the change, as the set and reset commands do.
func (a *app) saveSettingEdits(ctx sdk.Context, work context.Context, s *session, scope config.Scope, edits []config.Edit) (config.SaveResult, error) {
	baseline, err := s.settings.Current()
	if err != nil {
		return config.SaveResult{}, err
	}
	result, saveErr := s.settings.Save(work, scope, baseline, edits, ctx.IsProjectTrusted)
	if result.Published {
		a.settingsChanged(ctx, s, result.Revision, result.Changed)
	}
	return result, saveErr
}

func (a *app) settingsPanel(ctx sdk.Context, work context.Context, s *session) error {
	return a.openSettingsPanel(ctx, work, s, "", "")
}

// toolDisplayPanel edits per-tool display preferences, or explains why there are none.
func (a *app) toolDisplayPanel(ctx sdk.Context, work context.Context, s *session) error {
	snapshot, _ := s.settings.Inspect()
	if snapshot == nil || len(s.displayGroups(snapshot, "", "")) == 0 {
		a.present(ctx, "Workbench › Tool display", noToolDisplayText)
		return nil
	}
	return a.openSettingsPanel(ctx, work, s, "tools.", "Tool display")
}

// noToolDisplayText is shown when no extension has registered a tool presentation.
const noToolDisplayText = "Tool display preferences\n\nNothing to configure yet.\n\nThese preferences appear when another extension registers a tool presentation with Workbench, for example a web-search extension. Workbench's own tools (read, edit, bash, ...) use PiG's standard rows and have no display preferences in this version.\n\nSee /workbench integrations for the extensions currently registered."

func (a *app) openSettingsPanel(ctx sdk.Context, work context.Context, s *session, prefix, heading string) error {
	snapshot, status := s.settings.Inspect()
	if snapshot == nil {
		return fmt.Errorf("no valid settings snapshot is available: %s", status.Error)
	}
	scopes := []config.Scope{config.User, config.Runtime}
	if trusted, err := ctx.IsProjectTrusted(); err == nil && trusted {
		scopes = []config.Scope{config.User, config.Project, config.Runtime}
	}
	panel := newSettingsPanel(snapshot, scopes, ctx.Height, func(scope config.Scope, edits []config.Edit) (config.SaveResult, *config.Snapshot, error) {
		result, err := a.saveSettingEdits(ctx, work, s, scope, edits)
		if err != nil {
			return result, nil, err
		}
		fresh, err := s.settings.Current()
		return result, fresh, err
	})
	panel.prefix, panel.heading = prefix, heading
	_, err := ctx.Custom(panel, nil)
	return err
}

func (a *app) notifyJSON(ctx sdk.Context, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	a.notify(ctx, string(data))
	return nil
}

func (a *app) settingsChanged(ctx sdk.Context, s *session, revision uint64, keys []string) {
	if snapshot, err := s.settings.Current(); err == nil {
		s.refreshPresentationDisplay(snapshot)
	}
	event := api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: fmt.Sprintf("%s:settings:%d", s.generation, revision), Type: "settings.changed", Data: map[string]any{"configRevision": revision, "changed": keys}}
	if err := ctx.Events().Emit(api.EventChannel, event); err != nil {
		ctx.Notify("Configuration is applied, but its change notification failed; inspect settings to reconcile.", "warning")
	}
	events, failure := a.displayChangeEvents(s, revision, keys)
	if failure != nil {
		ctx.Notify("Display settings saved, but display notification data is unavailable; query settings.get to reconcile.", "warning")
		return
	}
	for _, event := range events {
		if err := ctx.Events().Emit(api.EventChannel, event); err != nil {
			ctx.Notify("Display settings saved, but notification failed; query settings.get to reconcile.", "warning")
		}
	}
}
