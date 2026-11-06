package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

func workbenchCompletions(prefix string) ([]sdk.AutocompleteItem, error) {
	candidates := []string{"show", "tools", "cd", "jobs", "skills", "prompt", "instructions", "instructions pin", "instructions unpin", "instructions ls", "instructions rules", "instructions reload", "ask", "ask show", "ask resume", "ask discard", "trust", "trust ls", "trust add allow", "trust add deny", "trust rm", "projects", "projects status", "projects unlink-storage", "sessions", "sessions ls", "sessions send", "sessions trust", "sessions untrust", "sessions block", "sessions unblock", "sessions hide", "sessions show", "integrations", "resources", "frames", "settings", "settings show", "settings catalog", "settings reload", "settings set", "settings reset"}
	for _, scope := range []string{"user", "project", "runtime"} {
		candidates = append(candidates, "settings set "+scope, "settings reset "+scope)
	}
	for _, definition := range config.Catalog() {
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
	var result []sdk.AutocompleteItem
	for _, value := range candidates {
		if strings.HasPrefix(value, prefix) && !strings.Contains(strings.TrimPrefix(value, prefix), " ") {
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
	case "", "show":
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
		return notifyJSON(ctx, output)
	case "catalog":
		var definitions []config.Definition
		for _, definition := range config.Catalog() {
			if strings.HasPrefix(definition.Key, rest) {
				definitions = append(definitions, definition)
			}
		}
		return notifyJSON(ctx, map[string]any{"catalog": definitions})
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
		return notifyJSON(ctx, map[string]any{"settings": "reloaded", "configRevision": snapshot.Revision(), "warnings": snapshot.Warnings()})
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
		if err := notifyJSON(ctx, map[string]any{"settings": "save", "result": result}); err != nil {
			return err
		}
		return saveErr
	default:
		return errors.New("usage: /workbench settings [show [key]|catalog [prefix]|reload|set <scope> <key> <TOML value>|reset <scope> <key>]")
	}
}

func notifyJSON(ctx sdk.Context, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	notify(ctx, string(data))
	return nil
}

func (a *app) settingsChanged(ctx sdk.Context, s *session, revision uint64, keys []string) {
	event := api.Event{Version: 1, SessionID: s.id, Generation: s.generation, EventID: fmt.Sprintf("%s:settings:%d", s.generation, revision), Type: "settings.changed", Data: map[string]any{"configRevision": revision, "changed": keys}}
	if err := ctx.Events().Emit(api.EventChannel, event); err != nil {
		ctx.Notify("Configuration is applied, but its change notification failed; inspect settings to reconcile.", "warning")
	}
}
